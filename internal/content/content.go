// Package content resolves a mod's effective content (03 §1): the load folders the game
// uses for the environment's version and the current Loadout, and the XML assets in them.
// Semantics follow ModContentPack.InitLoadFolders and DirectXmlLoader.XmlAssetsInModFolder
// (the M0 xpathcompat spike).
package content

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/textenc"
)

// Active reports whether a packageId (lower case, no _steam suffix) is in the Loadout.
type Active func(pkg string) bool

// Resolved is a mod's effective content layout.
type Resolved struct {
	Folders []string // absolute, highest priority first
	// Conds is the outcome of every conditional LoadFolders entry, e.g.
	// "IfModActive=a.b:1;IfModNotActive=c.d:0". It is part of the cache key (03 §1.2).
	Conds string
	// Tag is the LoadFolders version tag used ("" without LoadFolders.xml).
	Tag string
	// Requires maps a loaded conditional folder to the packageIds its IfModActive /
	// IfModActiveAll condition names: content there targets those mods.
	Requires map[string][]string
}

// Key is the effective-content cache key: snapshot hash + game version + condition results.
func (r *Resolved) Key(snapHash []byte, mm string) string {
	return fmt.Sprintf("%x|%s|%s", snapHash, mm, r.Conds)
}

type lfEntry struct {
	folder                 string
	anyOf, allOf, notAnyOf []string
	condText               string
}

// Resolve computes the load folders of the mod at root. loadFoldersXML is the verbatim
// LoadFolders.xml from the declaration ("" if none); mm is the game's major.minor.
func Resolve(root, loadFoldersXML, mm string, active Active) Resolved {
	var r Resolved
	if lf := parseLoadFolders(loadFoldersXML); len(lf) > 0 {
		tag := pickTag(lf, mm)
		if pick, ok := lf[tag]; ok {
			r.Tag = tag
			var conds []string
			for i := len(pick) - 1; i >= 0; i-- {
				e := pick[i]
				ok := shouldLoad(e, active)
				if e.condText != "" {
					b := "0"
					if ok {
						b = "1"
					}
					conds = append(conds, e.condText+":"+b)
				}
				if ok {
					f := filepath.Join(root, e.folder)
					r.Folders = append(r.Folders, f)
					if req := append(append([]string{}, e.anyOf...), e.allOf...); len(req) > 0 {
						if r.Requires == nil {
							r.Requires = map[string][]string{}
						}
						r.Requires[f] = req
					}
				}
			}
			sort.Strings(conds)
			r.Conds = strings.Join(conds, ";")
			return r
		}
	}
	vs := versionDirs(root)
	best := ""
	for _, v := range vs { // ascending
		if versionLE(trimV(v), mm) {
			best = v
		}
	}
	if best == "" && len(vs) > 0 {
		best = vs[0] // none at or below: the lowest
	}
	for _, v := range vs {
		if trimV(v) == mm {
			best = v
		}
	}
	if best != "" {
		r.Folders = append(r.Folders, filepath.Join(root, best))
	}
	if st, err := os.Stat(filepath.Join(root, "Common")); err == nil && st.IsDir() {
		r.Folders = append(r.Folders, filepath.Join(root, "Common"))
	}
	r.Folders = append(r.Folders, root)
	return r
}

// pickTag: the exact version, else the highest tag not above it, else "default", else the
// lowest (03 §1.1).
func pickTag(lf map[string][]lfEntry, mm string) string {
	if _, ok := lf[mm]; ok {
		return mm
	}
	var tags []string
	for k := range lf {
		if isVersion(k) {
			tags = append(tags, k)
		}
	}
	sort.Slice(tags, func(i, j int) bool { return versionLess(tags[i], tags[j]) })
	best := ""
	for _, t := range tags {
		if versionLE(t, mm) {
			best = t
		}
	}
	switch {
	case best != "":
		return best
	case lf["default"] != nil:
		return "default"
	case len(tags) > 0:
		return tags[0]
	}
	return ""
}

func shouldLoad(e lfEntry, active Active) bool {
	anyActive := func(ids []string) bool {
		for _, id := range ids {
			if active(id) {
				return true
			}
		}
		return false
	}
	if len(e.anyOf) > 0 && !anyActive(e.anyOf) {
		return false
	}
	for _, id := range e.allOf {
		if !active(id) {
			return false
		}
	}
	return len(e.notAnyOf) == 0 || !anyActive(e.notAnyOf)
}

func parseLoadFolders(s string) map[string][]lfEntry {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	dec := xml.NewDecoder(bytes.NewReader(textenc.UTF8([]byte(s))))
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	dec.Strict = false
	out := map[string][]lfEntry{}
	depth := 0
	var key string
	var cur *lfEntry
	var text strings.Builder
	split := func(v string) []string {
		var r []string
		for _, p := range strings.Split(v, ",") {
			if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
				r = append(r, strings.TrimSuffix(p, "_steam"))
			}
		}
		return r
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch depth {
			case 2:
				key = strings.TrimPrefix(strings.ToLower(t.Name.Local), "v")
				if _, ok := out[key]; !ok {
					out[key] = []lfEntry{}
				}
			case 3:
				cur = &lfEntry{}
				text.Reset()
				var conds []string
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "IfModActive", "IfDLCActive":
						cur.anyOf = append(cur.anyOf, split(a.Value)...)
					case "IfModActiveAll":
						cur.allOf = append(cur.allOf, split(a.Value)...)
					case "IfModNotActive", "IfDLCNotActive":
						cur.notAnyOf = append(cur.notAnyOf, split(a.Value)...)
					default:
						continue
					}
					conds = append(conds, a.Name.Local+"="+strings.ToLower(strings.TrimSpace(a.Value)))
				}
				cur.condText = strings.Join(conds, "&")
			}
		case xml.CharData:
			if depth == 3 {
				text.Write(t)
			}
		case xml.EndElement:
			if depth == 3 && cur != nil {
				cur.folder = strings.TrimSpace(text.String())
				if cur.folder == "/" || cur.folder == `\` {
					cur.folder = ""
				}
				out[key] = append(out[key], *cur)
				cur = nil
			}
			depth--
		}
	}
	return out
}

var verRe = regexp.MustCompile(`^[vV]?\d+\.\d+$`)

func isVersion(s string) bool { return verRe.MatchString(s) }

// IsVersionFolder reports whether a folder name is a game version ("1.6", "v1.5").
func IsVersionFolder(s string) bool { return isVersion(s) }

func trimV(s string) string { return strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V") }

func versionParts(s string) (int, int) {
	var a, b int
	fmt.Sscanf(trimV(s), "%d.%d", &a, &b)
	return a, b
}

func versionLE(a, b string) bool {
	a1, a2 := versionParts(a)
	b1, b2 := versionParts(b)
	return a1 < b1 || a1 == b1 && a2 <= b2
}

func versionLess(a, b string) bool { return versionLE(a, b) && !versionLE(b, a) }

// versionDirs lists version-named subdirectories of root in ascending version order.
func versionDirs(root string) []string {
	ents, _ := os.ReadDir(root)
	var vs []string
	for _, e := range ents {
		if e.IsDir() && isVersion(e.Name()) {
			vs = append(vs, e.Name())
		}
	}
	sort.Slice(vs, func(i, j int) bool { return versionLess(vs[i], vs[j]) })
	return vs
}

// Asset is one XML file of the effective content.
type Asset struct {
	Path string // absolute
	Rel  string // relative to the mod root, forward slashes
}

// Assets lists *.xml under <folder>/<sub> for each load folder, highest priority first,
// de-duplicated by path relative to the load folder. Order approximates .NET
// GetFiles(AllDirectories): files of a directory in name order, then subdirectories
// breadth-first.
func Assets(root string, folders []string, sub string) []Asset {
	return Files(root, folders, sub, ".xml")
}

// CodeFiles lists a mod's DLLs: <folder>/Assemblies, plus <folder>/Lunar/Components, where
// Lunar framework mods (Map Preview, Geological Landforms …) keep the code their loader
// DLL loads at run time (log corpus: their stack frames had no owner).
func CodeFiles(root string, folders []string) []Asset {
	return append(Files(root, folders, "Assemblies", ".dll"), Files(root, folders, "Lunar/Components", ".dll")...)
}

// Files is Assets for another extension (".dll" for Assemblies).
func Files(root string, folders []string, sub, ext string) []Asset {
	seen := map[string]bool{}
	var out []Asset
	for _, f := range folders {
		queue := []string{filepath.Join(f, sub)}
		for len(queue) > 0 {
			dir := queue[0]
			queue = queue[1:]
			ents, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			sort.Slice(ents, func(i, j int) bool { return strings.ToLower(ents[i].Name()) < strings.ToLower(ents[j].Name()) })
			for _, e := range ents {
				p := filepath.Join(dir, e.Name())
				if e.IsDir() {
					queue = append(queue, p)
					continue
				}
				n := e.Name()
				if !strings.EqualFold(filepath.Ext(n), ext) || strings.HasPrefix(n, ".") {
					continue
				}
				key := strings.ToLower(p[len(f):])
				if !seen[key] {
					seen[key] = true
					out = append(out, Asset{Path: p, Rel: relSlash(root, p)})
				}
			}
		}
	}
	return out
}

func relSlash(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(r)
}

// OtherXML lists XML files under any <sub> directory of the mod that are not part of the
// effective content (for `wrong-version-route`).
func OtherXML(root string, folders []string, sub string) []Asset {
	eff := map[string]bool{}
	for _, a := range Assets(root, folders, sub) {
		eff[strings.ToLower(a.Path)] = true
	}
	var out []Asset
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".xml") {
			return nil
		}
		rel := relSlash(root, p)
		inSub := false
		for _, seg := range strings.Split(rel, "/") {
			if strings.EqualFold(seg, sub) {
				inSub = true
			}
		}
		if inSub && !eff[strings.ToLower(p)] {
			out = append(out, Asset{Path: p, Rel: rel})
		}
		return nil
	})
	return out
}
