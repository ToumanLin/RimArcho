package app

import (
	"bytes"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// A missing type that nothing installed defines (03 §6.2, "Missing types"): the XML naming it
// is the likely source (a typo or a wrong namespace), so the enabled mods' Defs and Patches
// are searched for it (refQuery.class), and the nearest type names of the analysed and game
// assemblies are offered.

// typesNearLimit bounds the type names read per root namespace.
const typesNearLimit = 20000

// nearTypes lists up to three existing types close to t: the same simple name in another
// namespace, or a small edit distance (StatParts_Bin → StatPart_Bin), each with where it is
// defined ("#ref File.dll", or "game").
func (cx *attrCtx) nearTypes(t string) []string {
	t = strings.ReplaceAll(t, "+", ".")
	root, _, ok := strings.Cut(t, ".")
	if !ok {
		return nil
	}
	type cand struct {
		name string
		dist int
		at   string
	}
	var cs []cand
	consider := func(name, at string) {
		dotted := strings.ReplaceAll(name, "+", ".")
		if strings.EqualFold(dotted, t) {
			return
		}
		d := editDistance(strings.ToLower(dotted), strings.ToLower(t))
		same := strings.EqualFold(dotted[strings.LastIndexByte(dotted, '.')+1:], t[strings.LastIndexByte(t, '.')+1:])
		if same || (d <= 3 && d*5 <= len(t)) {
			if same && d > 1 {
				d = 1 // another namespace, same class: as close as a one-letter typo
			}
			cs = append(cs, cand{dotted, d, at})
		}
	}
	if owners, err := cx.a.Store.TypesUnder(root, typesNearLimit); err == nil {
		for name, insts := range owners {
			at := ""
			for _, id := range insts {
				if m := cx.st.Ix.ByID[id]; m != nil && (at == "" || cx.st.Position(id) > 0) {
					at = cx.st.Ix.Ref(id)
				}
			}
			if at != "" { // installed in this environment
				consider(name, at)
			}
		}
	}
	if names, err := cx.a.Store.GameTypesUnder(root, typesNearLimit); err == nil {
		for _, name := range names {
			consider(name, "game")
		}
	}
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].dist != cs[j].dist {
			return cs[i].dist < cs[j].dist
		}
		return cs[i].name < cs[j].name
	})
	var out []string
	for i, c := range cs {
		if i == 3 {
			break
		}
		at := c.at
		if at != "game" {
			if owners, err := cx.a.Store.TypeOwners(c.name); err == nil {
				for _, o := range owners {
					if cx.st.Ix.Ref(o[0]) == at && o[1] != "" {
						at += " " + path.Base(filepath.ToSlash(o[1]))
						break
					}
				}
			}
		}
		out = append(out, fmt.Sprintf("%s (%s)", c.name, at))
	}
	return out
}

// editDistance is the Levenshtein distance of two strings, by byte.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// classAttrs calls hit for each Class="…" attribute value in an XML file, with its offset.
func classAttrs(b []byte, hit func(v []byte, off int)) {
	for off := 0; ; {
		i := indexFrom(b, classAttrTag, off)
		if i < 0 {
			return
		}
		v := i + len(classAttrTag)
		off = v
		if i == 0 || !xmlSpace(b[i-1]) {
			continue
		}
		e := indexFrom(b, []byte{'"'}, v)
		if e < 0 {
			return
		}
		if e > v && e-v <= maxRefName {
			hit(b[v:e], v)
		}
		off = e + 1
	}
}

func indexFrom(b, sep []byte, off int) int {
	if off > len(b) {
		return -1
	}
	i := bytes.Index(b[off:], sep)
	if i < 0 {
		return -1
	}
	return off + i
}

// shadowing finds why an enabled mod's type can still be missing: another enabled mod, earlier
// in load order, ships a DLL with the same file name that lacks the type. The game keeps the
// first copy of an assembly name (03 §4.3 asm.duplicate-name), so that mod is the source.
// It returns the shadowing instance and the evidence for both mods ("" when none).
func (cx *attrCtx) shadowing(t, provider string) (string, string, string) {
	owners, err := cx.a.Store.TypeOwners(t)
	if err != nil {
		return "", "", ""
	}
	defines := map[string]bool{}
	file := ""
	for _, o := range owners {
		defines[o[0]] = true
		if o[0] == provider && file == "" {
			file = path.Base(filepath.ToSlash(o[1]))
		}
	}
	pp := cx.st.Position(provider)
	if file == "" || pp == 0 {
		return "", "", ""
	}
	if cx.dllMemo == nil {
		cx.dllMemo = map[string][]*store.AssemblyRow{}
		for _, e := range cx.st.List {
			if m := cx.st.Ix.ByID[e.Instance]; m != nil && !cx.official(m.ID) {
				asms, _ := cx.a.Store.Assemblies(m.ID)
				for _, as := range asms {
					k := strings.ToLower(path.Base(filepath.ToSlash(as.File)))
					cx.dllMemo[k] = append(cx.dllMemo[k], as)
				}
			}
		}
	}
	for _, as := range cx.dllMemo[strings.ToLower(file)] {
		id := as.Instance
		q := cx.st.Position(id)
		if id == provider || defines[id] || q == 0 || q >= pp {
			continue
		}
		ix := cx.st.Ix
		return id,
			fmt.Sprintf("ships %s (%s) without %s and loads before %s (list position %d < %d): the game keeps the first %s, so %s's copy is never loaded", as.File, render.Plural(as.Types, "type"), t, ix.Ref(provider), q, pp, file, ix.Ref(provider)),
			fmt.Sprintf("provides type %s in its %s, but %s's copy of %s loads first and lacks it", t, file, ix.Ref(id), file)
	}
	return "", "", ""
}
