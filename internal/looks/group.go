package looks

import (
	"sort"
	"strconv"
	"strings"
)

var facings = [...]string{"south", "east", "north", "west"}

// cutSuffixes cuts the mask suffix and then the facing suffix off a key (21 §2.2). The
// game names a facing's mask "<path>_<facing>m" (Graphic_Multi) and a single texture's
// mask "<path>_m" (Graphic_Single); "<path>_<facing>_m" is read the same way.
func cutSuffixes(key string) (rest, facing string, mask bool) {
	cut := func(s, suffix string) (string, bool) {
		r, ok := strings.CutSuffix(s, suffix)
		if !ok || r == "" || strings.HasSuffix(r, "/") {
			return s, false
		}
		return r, true
	}
	rest, mask = cut(key, "_m")
	for _, f := range facings {
		if r, ok := cut(rest, "_"+f); ok {
			return r, f, mask
		}
	}
	if !mask {
		for _, f := range facings {
			if r, ok := cut(key, "_"+f+"m"); ok {
				return r, f, true
			}
		}
	}
	return rest, "", mask
}

// classify places a key: the look it belongs to and its slot there.
func classify(key string, fa *defFacts) (kind, id, slot string) {
	if i := strings.LastIndexByte(key, '/'); i > 0 {
		if _, ok := fa.folders[key[:i]]; ok {
			name := key[i+1:]
			if base, ok := strings.CutSuffix(name, "_m"); ok && base != "" {
				name = base + maskSuffix
			}
			return KindFolder, key[:i], name
		}
	}
	rest, facing, mask := cutSuffixes(key)
	if facing == "" {
		facing = "single"
	}
	kind, id, slot = KindPlain, rest, facing
	if fa.worn[rest] {
		kind = KindWorn
	} else if i := strings.LastIndexByte(rest, '_'); i > strings.LastIndexByte(rest, '/')+1 && i < len(rest)-1 && fa.worn[rest[:i]] {
		kind, id, slot = KindWorn, rest[:i], rest[i+1:]+"_"+facing
	}
	if mask {
		slot += maskSuffix
	}
	return kind, id, slot
}

var kindRank = map[string]int{KindPlain: 0, KindWorn: 1, KindFolder: 2}

// group builds the looks from the scanned textures and the Def facts.
func group(idx *Index, scans []*modScan, fa *defFacts) {
	for _, s := range scans {
		for _, key := range s.keys {
			kind, id, slot := classify(key, fa)
			l := idx.ByID[id]
			if l == nil {
				l = &Look{ID: id, Kind: kind, Category: category(id, kind)}
				idx.ByID[id] = l
				idx.Looks = append(idx.Looks, l)
			} else if kindRank[kind] > kindRank[l.Kind] {
				l.Kind, l.Category = kind, category(id, kind)
			}
			p := l.provider(s.mod.Pos)
			if p == nil {
				p = &Provider{Pos: s.mod.Pos, Slots: map[string]*File{}}
				l.Providers = append(l.Providers, p)
			}
			if p.Slots[slot] == nil {
				p.Slots[slot] = s.tex[key]
			}
		}
	}
	sort.Slice(idx.Looks, func(i, j int) bool { return idx.Looks[i].ID < idx.Looks[j].ID })
	lookOf := attachDefs(idx, fa)
	for _, l := range idx.Looks {
		markIdentical(l)
		l.describe(fa)
	}
	for c := range fa.categories {
		idx.Categories = append(idx.Categories, c)
	}
	sort.Strings(idx.Categories)
	styleIndex(idx, fa, lookOf)
}

// category is the first three folders of a look's path.
func category(id, kind string) string {
	parts := strings.Split(id, "/")
	if kind != KindFolder {
		parts = parts[:len(parts)-1]
	}
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, "/")
}

// attachDefs gives each look the Defs that name it and returns, per Def path, the look id
// it leads to. A path that is no look id itself (an icon named "…_south", say) leads to
// the look its texture was grouped into.
func attachDefs(idx *Index, fa *defFacts) map[string]string {
	lookOf := map[string]string{}
	for p, refs := range fa.refs {
		id := p
		if idx.ByID[id] == nil {
			if _, id, _ = classify(p, fa); idx.ByID[id] == nil {
				continue
			}
		}
		lookOf[p] = id
		l := idx.ByID[id]
		l.Defs = append(l.Defs, refs...)
	}
	for _, l := range idx.Looks {
		sort.Slice(l.Defs, func(i, j int) bool {
			a, b := l.Defs[i], l.Defs[j]
			if a.ModPos != b.ModPos {
				return a.ModPos < b.ModPos
			}
			return refKey(a) < refKey(b)
		})
		out := l.Defs[:0]
		for i, d := range l.Defs {
			if i == 0 || d != l.Defs[i-1] {
				out = append(out, d)
			}
		}
		l.Defs = out
	}
	return lookOf
}

func refKey(d DefRef) string {
	return strings.Join([]string{d.Type, d.Name, d.Field, d.GraphicClass, d.Label, strconv.FormatBool(d.Abstract)}, "|")
}

// markIdentical records, per slot, the earliest earlier provider with the same content.
func markIdentical(l *Look) {
	for i, p := range l.Providers {
		for slot, f := range p.Slots {
			if f.Hash == "" {
				continue
			}
			for _, q := range l.Providers[:i] {
				if g := q.Slots[slot]; g != nil && identical(f, g) {
					f.SameAs = q.Pos
					break
				}
			}
		}
	}
}

// describe sets a look's styles and tags.
func (l *Look) describe(fa *defFacts) {
	seen := map[string]bool{}
	for _, d := range l.Defs {
		if d.Type != "ThingStyleDef" || d.Abstract || seen[d.Name] {
			continue
		}
		seen[d.Name] = true
		st := LookStyle{StyleDef: d.Name}
		cats, things := map[string]bool{}, map[string]bool{}
		for _, u := range fa.styleUses[d.Name] {
			if !cats[u.category] {
				cats[u.category] = true
				st.Categories = append(st.Categories, u.category)
			}
			if u.thing != "" && !things[u.thing] {
				things[u.thing] = true
				st.ThingDefs = append(st.ThingDefs, u.thing)
			}
		}
		sort.Strings(st.Categories)
		sort.Strings(st.ThingDefs)
		l.Styles = append(l.Styles, st)
	}
	if len(l.Styles) > 0 {
		l.Tags = append(l.Tags, "style")
	}
	if l.Kind == KindFolder {
		l.Tags = append(l.Tags, fa.folders[l.ID])
	}
}

// styleIndex fills Index.Styles: per thing Def, its styles with the look each one draws. A
// style whose textures no Mod of the plan ships loose (official content) keeps its path.
func styleIndex(idx *Index, fa *defFacts, lookOf map[string]string) {
	for style, uses := range fa.styleUses {
		paths := fa.stylePaths[style]
		if len(paths) == 0 {
			paths = []string{""}
		}
		for _, u := range uses {
			if u.thing == "" {
				continue
			}
			for _, p := range paths {
				id := p
				if l, ok := lookOf[p]; ok {
					id = l
				}
				idx.Styles[u.thing] = append(idx.Styles[u.thing], StyleRef{StyleDef: style, Category: u.category, LookID: id})
			}
		}
	}
	for thing, refs := range idx.Styles {
		sort.Slice(refs, func(i, j int) bool {
			a, b := refs[i], refs[j]
			switch {
			case a.Category != b.Category:
				return a.Category < b.Category
			case a.StyleDef != b.StyleDef:
				return a.StyleDef < b.StyleDef
			}
			return a.LookID < b.LookID
		})
		out := refs[:0]
		for i, r := range refs {
			if i == 0 || r != refs[i-1] {
				out = append(out, r)
			}
		}
		idx.Styles[thing] = out
	}
}
