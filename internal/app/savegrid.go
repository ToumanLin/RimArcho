package app

import (
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/savefile"
)

// gridDefTypes are the Def types a save stores by short hash in grids (12 §1.4): the maps'
// terrain and the world's biomes.
var gridDefTypes = []string{"TerrainDef", "BiomeDef"}

// defShortHash is the game's Def short hash before collisions are resolved
// (ShortHashGiver.GiveShortHash): GenText.StableStringHash of the defName, a 32-bit signed
// 23·31ⁿ polynomial over its UTF-16 units, modulo 65535, cast to 16 bits.
func defShortHash(defName string) uint16 {
	n := int32(23)
	for _, c := range utf16.Encode([]rune(defName)) {
		n = n*31 + int32(c)
	}
	return uint16(n % 65535)
}

// shortHashes gives the Defs of one type their short hashes as the game does: in defName
// order, each its hash or, when that is 0 or taken, the next free one. The game sorts with
// the culture's comparer and this by code unit, which differs only in which of two
// colliding names keeps the hash.
func shortHashes(names []string) map[uint16]string {
	sort.Strings(names)
	out := make(map[uint16]string, len(names))
	for _, name := range names {
		h := defShortHash(name)
		for i := 0; i < 1<<16 && (h == 0 || out[h] != ""); i++ {
			h++
		}
		out[h] = name
	}
	return out
}

// gridRefs names the Defs in the save's grids: Def type → defName → cells, with how many
// distinct hashes of each type no Def of the save's Mods has (a Def a Patch or code added,
// or a Mod that is not installed or not analysed). The Defs are those of insts that load
// with the save's Mods (active).
func (a *App) gridRefs(f *savefile.Facts, insts []string, active func(string) bool) (map[string]map[string]int, map[string]int, error) {
	refs, left := map[string]map[string]int{}, map[string]int{}
	if len(f.GridDefs) == 0 {
		return refs, left, nil
	}
	facts, err := a.Store.DefsOfTypes(insts, gridDefTypes)
	if err != nil {
		return nil, nil, err
	}
	names := map[string]map[string]bool{}
	for i := range facts {
		d := &facts[i]
		if d.Abstract || d.DefName == "" || !d.Loaded(active) {
			continue
		}
		if names[d.Type] == nil {
			names[d.Type] = map[string]bool{}
		}
		names[d.Type][d.DefName] = true
	}
	// Terrain the game generates at load is in the same hash space (12 §1.4).
	implied := map[string][2]string{} // generated defName → the reference it stands for
	if len(f.GridDefs["TerrainDef"]) > 0 {
		if implied, err = a.impliedTerrain(insts, active); err != nil {
			return nil, nil, err
		}
		if names["TerrainDef"] == nil {
			names["TerrainDef"] = map[string]bool{}
		}
		for n := range implied {
			if names["TerrainDef"][n] {
				delete(implied, n) // a Def file has it: that one is loaded
			}
			names["TerrainDef"][n] = true
		}
	}
	for typ, cells := range f.GridDefs {
		var list []string
		for n := range names[typ] {
			list = append(list, n)
		}
		byHash := shortHashes(list)
		for h, k := range cells {
			name := byHash[h]
			if name == "" {
				left[typ]++
				continue
			}
			to := [2]string{typ, name}
			if typ == "TerrainDef" {
				if g, ok := implied[name]; ok {
					to = g
				}
			}
			if refs[to[0]] == nil {
				refs[to[0]] = map[string]int{}
			}
			refs[to[0]][to[1]] += k
		}
	}
	return refs, left, nil
}

// impliedTerrain names the TerrainDefs the game generates at load, each with the reference
// that attributes it:
//   - rough, rough-hewn and smooth stone of every natural rock that is not a resource rock
//     (TerrainDefGenerator_Stone): "<rock>_Rough" …, attributed through the rock's ThingDef
//     as other generated names are (12 §2.2);
//   - a carpet of every TerrainTemplateDef in every structure colour
//     (TerrainDefGenerator_Carpet): the template's defName followed by the ColorDef's without
//     "Structure_", attributed to the template. A colour's type is not in the facts, so the
//     structure colours are taken to be the ColorDefs named "Structure_…", as the game's are.
func (a *App) impliedTerrain(insts []string, active func(string) bool) (map[string][2]string, error) {
	out := map[string][2]string{}
	things, err := a.Store.DefsLite(insts, "ThingDef", "Rock")
	if err != nil {
		return nil, err
	}
	byName := map[string]*analyze.DefFact{}
	for i := range things {
		if d := &things[i]; d.Name != "" && d.Loaded(active) {
			byName[d.Name] = d
		}
	}
	// rock is a ThingDef's own or nearest inherited building/<field>
	rock := func(d *analyze.DefFact, field string) bool {
		for depth := 0; d != nil && depth < 32; depth++ {
			if d.Content != nil {
				for _, f := range d.Content.Fields {
					if f.Name != "building" {
						continue
					}
					for _, c := range f.Children {
						if c.Name == field {
							return strings.EqualFold(c.Value, "true")
						}
					}
				}
			}
			d = byName[d.Parent]
		}
		return false
	}
	for i := range things {
		d := &things[i]
		if d.Abstract || d.DefName == "" || !d.Loaded(active) || !rock(d, "isNaturalRock") || rock(d, "isResourceRock") {
			continue
		}
		for _, suffix := range []string{"_Rough", "_RoughHewn", "_Smooth"} {
			out[d.DefName+suffix] = [2]string{"TerrainDef", d.DefName + suffix}
		}
	}
	more, err := a.Store.DefsOfTypes(insts, []string{"TerrainTemplateDef", "ColorDef"})
	if err != nil {
		return nil, err
	}
	var templates, colours []string
	for i := range more {
		d := &more[i]
		switch {
		case d.Abstract || d.DefName == "" || !d.Loaded(active):
		case d.Type == "TerrainTemplateDef":
			templates = append(templates, d.DefName)
		case strings.HasPrefix(d.DefName, "Structure_"):
			colours = append(colours, strings.ReplaceAll(d.DefName, "Structure_", ""))
		}
	}
	for _, t := range templates {
		for _, c := range colours {
			out[t+c] = [2]string{"TerrainTemplateDef", t}
		}
	}
	return out, nil
}
