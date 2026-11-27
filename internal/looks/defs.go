package looks

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// The Def fields the index reads (21 §2.3).
const (
	fLabel = iota
	fTexPath
	fGraphicClass
	fApparelWorn
	fWorn // wornGraphicPath directly under the Def (ThingStyleDef)
	fUIIcon
	fIcon
	nFields
)

// pathFields are the fields that name a look, with the name DefRef.Field gets.
var pathFields = [...]struct {
	f    int
	name string
}{{fTexPath, "graphicData/texPath"}, {fApparelWorn, "apparel/wornGraphicPath"}, {fWorn, "wornGraphicPath"},
	{fUIIcon, "uiIconPath"}, {fIcon, "iconPath"}}

var fieldOf = map[string]int{"label": fLabel, "uiIconPath": fUIIcon, "iconPath": fIcon, "wornGraphicPath": fWorn,
	"graphicData/texPath": fTexPath, "graphicData/graphicClass": fGraphicClass, "apparel/wornGraphicPath": fApparelWorn}

const maxParents = 32

type styleRow struct{ thing, style string }

// rawDef is one Def element as written, before inheritance.
type rawDef struct {
	typ, defName, name, parent string
	abstract                   bool
	modIdx, pos                int
	f                          [nFields]string
	rows                       []styleRow // StyleCategoryDef/thingDefStyles

	res       *[nFields]string
	resolving bool
}

func (d *rawDef) refName() string {
	if d.defName != "" {
		return d.defName
	}
	return d.name
}

// readDefs parses Defs/**/*.xml of the load folders. A file at the same path in a
// lower-priority folder is not loaded, as in the game. Unreadable files become warnings.
func (s *modScan) readDefs(ctx context.Context, folders []string) error {
	seen := map[string]bool{}
	for _, folder := range folders {
		dir := childFold(folder, "Defs")
		if dir == "" {
			continue
		}
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
			if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".xml") {
				return nil
			}
			rel, _ := filepath.Rel(dir, p)
			if k := strings.ToLower(rel); seen[k] {
				return nil
			} else {
				seen[k] = true
			}
			data, err := os.ReadFile(p)
			if err == nil {
				var defs []*rawDef
				if defs, err = parseDefs(data); err == nil {
					s.defs = append(s.defs, defs...)
					return nil
				}
			}
			s.warnings = append(s.warnings, fmt.Sprintf("%s: %s: %v", s.mod.Name, p, err))
			return nil
		})
		if err != nil {
			return err
		}
	}
	for _, d := range s.defs {
		d.pos = s.mod.Pos
	}
	return nil
}

// decodeText turns UTF-16 with a byte order mark into UTF-8 and drops a UTF-8 mark.
func decodeText(b []byte) []byte {
	var order binary.ByteOrder
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return b[3:]
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		order = binary.LittleEndian
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		order = binary.BigEndian
	default:
		return b
	}
	u := make([]uint16, 0, len(b)/2)
	for i := 2; i+1 < len(b); i += 2 {
		u = append(u, order.Uint16(b[i:]))
	}
	return []byte(string(utf16.Decode(u)))
}

// parseDefs reads the Defs of one file in a single pass, keeping only the fields of
// fieldOf and the rows of thingDefStyles. Any XML error rejects the whole file, as the
// game's own reader does.
func parseDefs(data []byte) ([]*rawDef, error) {
	dec := xml.NewDecoder(bytes.NewReader(decodeText(data)))
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	var (
		out   []*rawDef
		stack []string
		text  []byte
		cur   *rawDef
		row   styleRow
	)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			text = text[:0]
			if len(stack) == 2 {
				cur = newRawDef(t)
			}
		case xml.CharData:
			if n := len(stack); n >= 3 && n <= 5 {
				text = append(text, t...)
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("unexpected </%s>", t.Name.Local)
			}
			if cur != nil && len(stack) > 2 && len(stack) <= 5 {
				cur.end(stack[2:], strings.TrimSpace(string(text)), &row)
			}
			if len(stack) == 2 && cur != nil {
				if cur.defName != "" || cur.name != "" {
					out = append(out, cur)
				}
				cur = nil
			}
			stack = stack[:len(stack)-1]
			text = text[:0]
		}
	}
}

func newRawDef(t xml.StartElement) *rawDef {
	d := &rawDef{typ: t.Name.Local}
	for _, a := range t.Attr {
		switch a.Name.Local {
		case "Name":
			d.name = strings.TrimSpace(a.Value)
		case "ParentName":
			d.parent = strings.TrimSpace(a.Value)
		case "Abstract":
			d.abstract = strings.EqualFold(strings.TrimSpace(a.Value), "true")
		}
	}
	return d
}

// end records the element that just closed; path is its place under the Def.
func (d *rawDef) end(path []string, text string, row *styleRow) {
	switch len(path) {
	case 1:
		if path[0] == "defName" {
			d.defName = text
		} else if f, ok := fieldOf[path[0]]; ok {
			d.f[f] = text
		}
	case 2:
		if path[0] == "thingDefStyles" {
			if d.typ == "StyleCategoryDef" && (row.thing != "" || row.style != "") {
				d.rows = append(d.rows, *row)
			}
			*row = styleRow{}
		} else if f, ok := fieldOf[path[0]+"/"+path[1]]; ok {
			d.f[f] = text
		}
	case 3:
		if path[0] != "thingDefStyles" {
			return
		}
		switch path[2] {
		case "thingDef":
			row.thing = text
		case "styleDef":
			row.style = text
		}
	}
}

// defSet resolves ParentName across the plan.
type defSet struct {
	byName map[string][]*rawDef // Name attribute -> Defs in load order
}

// parentOf finds the Def that d inherits from: one of its own Mod when there is one, else
// the one that loads last (a later Mod's base replaces an earlier one with the same Name).
func (s *defSet) parentOf(d *rawDef) *rawDef {
	if d.parent == "" {
		return nil
	}
	list := s.byName[d.parent]
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].modIdx == d.modIdx && list[i] != d {
			return list[i]
		}
	}
	for i := len(list) - 1; i >= 0; i-- {
		if list[i] != d {
			return list[i]
		}
	}
	return nil
}

// resolve returns d's fields with the empty ones filled from its parents. A cycle or a
// chain longer than maxParents ends the inheritance there.
func (s *defSet) resolve(d *rawDef, depth int) *[nFields]string {
	if d.res != nil {
		return d.res
	}
	r := d.f
	if p := s.parentOf(d); p != nil && !p.resolving && depth < maxParents {
		d.resolving = true
		pr := s.resolve(p, depth+1)
		d.resolving = false
		for i := range r {
			if r[i] == "" {
				r[i] = pr[i]
			}
		}
	} else if p != nil {
		return &r // cut short: not remembered, a shorter path may resolve it fully
	}
	d.res = &r
	return d.res
}

// normPath is the form texture paths are compared in: lower case, forward slashes.
func normPath(p string) string {
	return strings.ToLower(strings.Trim(strings.ReplaceAll(strings.TrimSpace(p), `\`, "/"), "/"))
}

// folderTag names the kind of folder graphic a graphic class is ("" for a class that
// draws one texture or one set of facings). Graphic_Collection's subclasses in the game
// and in Mods are recognised by name.
func folderTag(class string) string {
	if i := strings.LastIndexByte(class, '.'); i >= 0 {
		class = class[i+1:]
	}
	switch {
	case strings.Contains(class, "Appearances"):
		return "appearance"
	case strings.Contains(class, "StackCount"):
		return "stackcount"
	case strings.Contains(class, "Cluster"):
		return "cluster"
	case strings.Contains(class, "Random") && !strings.Contains(class, "RandomRotated"):
		return "random"
	case strings.Contains(class, "Collection"):
		return "collection"
	}
	switch class {
	case "Graphic_Flicker", "Graphic_Genepack", "Graphic_MealVariants", "Graphic_Indexed":
		return "collection"
	}
	return ""
}

type styleUse struct{ category, thing string }

// defFacts is what the Defs of the plan say about texture paths.
type defFacts struct {
	refs       map[string][]DefRef   // path -> Defs that name it
	folders    map[string]string     // folder path -> tag
	worn       map[string]bool       // wornGraphicPath values
	styleUses  map[string][]styleUse // style defName -> categories and thing Defs
	stylePaths map[string][]string   // style defName -> its paths
	categories map[string]bool
}

func resolveDefs(scans []*modScan) *defFacts {
	set := &defSet{byName: map[string][]*rawDef{}}
	for i, s := range scans {
		for _, d := range s.defs {
			d.modIdx = i
			if d.name != "" {
				set.byName[d.name] = append(set.byName[d.name], d)
			}
		}
	}
	fa := &defFacts{refs: map[string][]DefRef{}, folders: map[string]string{}, worn: map[string]bool{},
		styleUses: map[string][]styleUse{}, stylePaths: map[string][]string{}, categories: map[string]bool{}}
	for _, s := range scans {
		for _, d := range s.defs {
			fa.add(d, set.resolve(d, 0))
		}
	}
	return fa
}

func (fa *defFacts) add(d *rawDef, r *[nFields]string) {
	if d.typ == "StyleCategoryDef" && d.defName != "" {
		fa.categories[d.defName] = true
		for _, row := range d.rows {
			if row.style != "" {
				fa.styleUses[row.style] = append(fa.styleUses[row.style], styleUse{d.defName, row.thing})
			}
		}
	}
	style := d.typ == "ThingStyleDef"
	for _, pf := range pathFields {
		p := normPath(r[pf.f])
		if p == "" || (pf.f == fWorn && !style) {
			continue
		}
		ref := DefRef{Type: d.typ, Name: d.refName(), Label: r[fLabel], Field: pf.name, ModPos: d.pos, Abstract: d.abstract}
		switch pf.f {
		case fTexPath:
			ref.GraphicClass = r[fGraphicClass]
			if tag := folderTag(ref.GraphicClass); tag != "" && fa.folders[p] == "" {
				fa.folders[p] = tag
			}
		case fApparelWorn, fWorn:
			fa.worn[p] = true
		}
		fa.refs[p] = append(fa.refs[p], ref)
		if style && d.defName != "" {
			fa.stylePaths[d.defName] = append(fa.stylePaths[d.defName], p)
		}
	}
}
