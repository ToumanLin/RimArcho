package app

import (
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/savefile"
)

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
	for i, name := range names {
		if i > 0 && names[i-1] == name {
			continue
		}
		h := defShortHash(name)
		for i := 0; i < 1<<16 && (h == 0 || out[h] != ""); i++ {
			h++
		}
		out[h] = name
	}
	return out
}

// gridResult is what the save's grids hold (12 §1.4).
type gridResult struct {
	refs map[string]map[string]int // Def type → defName → cells: typed references
	left map[string]int            // grid Def type → distinct hashes no Def could be given
	// may are the copies defining something a grid of each type can hold: with hashes left
	// of that type, one of them may be theirs.
	may map[string]map[string]bool
}

// gridThings are the ThingDefs of the save's Mods, read once for what the grids need.
type gridThings struct {
	all    []analyze.DefFact
	byName map[string]*analyze.DefFact // Name attribute → Def, for ParentName chains
}

// inherits says whether d or one of its parents satisfies has; the nearest answer wins.
func (t *gridThings) inherits(d *analyze.DefFact, has func(*analyze.DefFact) (bool, bool)) bool {
	for depth := 0; d != nil && depth < 32; depth++ {
		if v, ok := has(d); ok {
			return v
		}
		d = t.byName[d.Parent]
	}
	return false
}

// building reads a ThingDef's own building/<field>.
func building(field string) func(*analyze.DefFact) (bool, bool) {
	return func(d *analyze.DefFact) (bool, bool) {
		if d.Content != nil {
			for _, f := range d.Content.Fields {
				if f.Name != "building" {
					continue
				}
				for _, c := range f.Children {
					if c.Name == field {
						return strings.EqualFold(c.Value, "true"), true
					}
				}
			}
		}
		return false, false
	}
}

// chunk says a ThingDef derives from the game's chunk bases. saveCompressible itself is not
// in the Def facts; the game sets it on natural rock and on these two.
func chunk(d *analyze.DefFact) (bool, bool) {
	return true, d.Name == "ChunkBase" || d.Name == "ChunkRockBase"
}

// gridRefs names the Defs in the save's grids, among those of insts that load with the
// save's Mods (active). A hash is left when no Def has it: a Def a Patch or code added, or a
// Mod that is not installed or not analysed.
func (a *App) gridRefs(f *savefile.Facts, insts []string, active func(string) bool) (*gridResult, error) {
	g := &gridResult{refs: map[string]map[string]int{}, left: map[string]int{}, may: map[string]map[string]bool{}}
	if len(f.GridDefs) == 0 {
		return g, nil
	}
	may := func(typ, inst string) {
		if g.may[typ] == nil {
			g.may[typ] = map[string]bool{}
		}
		g.may[typ][inst] = true
	}
	add := func(typ, name string, cells int) {
		if g.refs[typ] == nil {
			g.refs[typ] = map[string]int{}
		}
		g.refs[typ][name] += cells
	}
	facts, err := a.Store.DefsOfTypes(insts, []string{"TerrainDef", "BiomeDef", "TerrainTemplateDef", "ColorDef"})
	if err != nil {
		return nil, err
	}
	names := map[string][]string{}
	for i := range facts {
		d := &facts[i]
		if d.Abstract || d.DefName == "" || !d.Loaded(active) {
			continue
		}
		names[d.Type] = append(names[d.Type], d.DefName)
		switch d.Type {
		case "TerrainDef", "BiomeDef":
			may(d.Type, d.Instance)
		case "TerrainTemplateDef":
			may("TerrainDef", d.Instance)
		}
	}
	var t gridThings
	if len(f.GridDefs["TerrainDef"])+len(f.GridDefs["ThingDef"]) > 0 {
		if t.all, err = a.Store.DefsLite(insts, "ThingDef", "Rock"); err != nil {
			return nil, err
		}
		t.byName = map[string]*analyze.DefFact{}
		for i := range t.all {
			if d := &t.all[i]; d.Name != "" && d.Loaded(active) {
				t.byName[d.Name] = d
			}
		}
	}
	// Biomes: the Def files' own.
	if cells := f.GridDefs["BiomeDef"]; len(cells) > 0 {
		byHash := shortHashes(names["BiomeDef"])
		for h, k := range cells {
			if n := byHash[h]; n != "" {
				add("BiomeDef", n, k)
			} else {
				g.left["BiomeDef"]++
			}
		}
	}
	// Terrain: the Def files' own and what the game generates at load, in one hash space.
	if cells := f.GridDefs["TerrainDef"]; len(cells) > 0 {
		list := names["TerrainDef"]
		own := map[string]bool{}
		for _, n := range list {
			own[n] = true
		}
		implied := map[string][2]string{} // generated defName → the reference it stands for
		for n, to := range impliedTerrain(&t, active, names["TerrainTemplateDef"], names["ColorDef"], may) {
			if !own[n] { // a Def file has it: that one is loaded
				implied[n] = to
				list = append(list, n)
			}
		}
		byHash := shortHashes(list)
		for h, k := range cells {
			n := byHash[h]
			switch to, gen := implied[n]; {
			case n == "":
				g.left["TerrainDef"]++
			case gen:
				add(to[0], to[1], k)
			default:
				add("TerrainDef", n, k)
			}
		}
	}
	// Compressed things: rock, ore and chunks, by ThingDef hash. The game hashes every
	// ThingDef, those it generates included (blueprints, frames, corpses, meat…), which are
	// not all known here: a hash the Def files' own ThingDefs do not explain is tried as a
	// compressible Def pushed up to three places on by ones not seen.
	if cells := f.GridDefs["ThingDef"]; len(cells) > 0 {
		var list []string
		comp := map[string]bool{}
		base := map[uint16][]string{}
		for i := range t.all {
			d := &t.all[i]
			if d.Abstract || d.DefName == "" || !d.Loaded(active) {
				continue
			}
			list = append(list, d.DefName)
			if t.inherits(d, building("isNaturalRock")) || t.inherits(d, chunk) {
				may("ThingDef", d.Instance)
				if !comp[d.DefName] {
					comp[d.DefName] = true
					base[defShortHash(d.DefName)] = append(base[defShortHash(d.DefName)], d.DefName)
				}
			}
		}
		byHash := shortHashes(list)
		for h, k := range cells {
			n := byHash[h]
			if !comp[n] {
				n = ""
				for back := uint16(0); back <= 3 && n == ""; back++ {
					if c := base[h-back]; len(c) == 1 {
						n = c[0]
					}
				}
			}
			if n == "" {
				g.left["ThingDef"]++
				continue
			}
			add("ThingDef", n, k)
		}
	}
	return g, nil
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
func impliedTerrain(t *gridThings, active func(string) bool, templates, colours []string, may func(typ, inst string)) map[string][2]string {
	out := map[string][2]string{}
	for i := range t.all {
		d := &t.all[i]
		if d.Abstract || d.DefName == "" || !d.Loaded(active) || !t.inherits(d, building("isNaturalRock")) || t.inherits(d, building("isResourceRock")) {
			continue
		}
		may("TerrainDef", d.Instance)
		for _, suffix := range []string{"_Rough", "_RoughHewn", "_Smooth"} {
			out[d.DefName+suffix] = [2]string{"TerrainDef", d.DefName + suffix}
		}
	}
	for _, tp := range templates {
		for _, c := range colours {
			if strings.HasPrefix(c, "Structure_") {
				out[tp+strings.ReplaceAll(c, "Structure_", "")] = [2]string{"TerrainTemplateDef", tp}
			}
		}
	}
	return out
}
