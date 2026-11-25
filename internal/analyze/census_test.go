package analyze

// TestXPathCensus (18 §2 "XPath shapes") classifies every <xpath> in the Patches folders
// under CENSUS_ROOT by whether the defName/@Name fast path (05 §5.2) covers it, and buckets
// the rest by shape; it writes counts and the commonest shapes (quoted values and numbers
// blanked) to CENSUS_OUT. Real Mods only in the VM (17 §4): build it with
// `go test -c ./internal/analyze`, copy the binary into the guest and run it there with
// `-test.run=TestXPathCensus` and CENSUS_ROOT set to a Workshop content folder.

import (
	"encoding/json"
	"encoding/xml"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/xdoc"
)

var (
	quoted   = regexp.MustCompile(`"[^"]*"|'[^']*'`)
	number   = regexp.MustCompile(`\b\d+\b`)
	censusRe = regexp.MustCompile(`^/?Defs/([^/\[]+)(\[(.*?)\])?(.*)$`)
)

func shapeOf(x string) string {
	x = strings.TrimSpace(x)
	x = quoted.ReplaceAllString(x, `"…"`)
	x = number.ReplaceAllString(x, "N")
	return x
}

func bucket(x string) string {
	t := strings.TrimSpace(x)
	switch {
	case strings.Contains(t, "|"):
		return "union |"
	case strings.HasPrefix(t, "//") || strings.Contains(t, "Defs//"):
		return "descendant //"
	case strings.HasPrefix(t, "*/") || strings.HasPrefix(t, "/*/"):
		return "*/ root"
	case !strings.HasPrefix(strings.TrimPrefix(t, "/"), "Defs"):
		return "no Defs prefix"
	}
	m := censusRe.FindStringSubmatch(t)
	if m == nil {
		return "other"
	}
	pred := m[3]
	switch {
	case m[2] == "":
		return "type, no predicate"
	case strings.Contains(pred, "defName") && (strings.Contains(pred, "contains(") || strings.Contains(pred, "starts-with(")):
		return "defName function"
	case strings.Contains(pred, "defName") || strings.Contains(pred, "@Name"):
		if strings.Contains(m[4], "..") || strings.Contains(m[4], "ancestor") || strings.Contains(m[4], "following") || strings.Contains(m[4], "preceding") {
			return "defName + axis in rest"
		}
		return "defName mixed predicate"
	case strings.Contains(pred, "@ParentName"):
		return "@ParentName"
	default:
		return "content predicate"
	}
}

func TestXPathCensus(t *testing.T) {
	root := os.Getenv("CENSUS_ROOT")
	if root == "" {
		t.Skip("CENSUS_ROOT")
	}
	names := xdoc.NewNames()
	doc := xdoc.New(names)
	r := doc.NewElement("Defs")
	doc.AppendChild(doc.Root(), r)
	s := newSim(doc, r, nil, "1.6", nil)
	counts := map[string]int{}
	shapes := map[string]int{}
	mods := map[string]map[string]bool{}
	files := 0
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".xml") {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		parts := strings.Split(rel, string(os.PathSeparator))
		inPatches := false
		for _, q := range parts {
			if strings.EqualFold(q, "Patches") {
				inPatches = true
			}
		}
		if !inPatches {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		files++
		dec := xml.NewDecoder(f)
		dec.Strict = false
		in := false
		var sb strings.Builder
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			switch v := tok.(type) {
			case xml.StartElement:
				if v.Name.Local == "xpath" {
					in = true
					sb.Reset()
				}
			case xml.CharData:
				if in {
					sb.Write(v)
				}
			case xml.EndElement:
				if v.Name.Local == "xpath" && in {
					in = false
					x := strings.TrimSpace(sb.String())
					if x == "" {
						continue
					}
					var k string
					if _, err := s.compile(x); err != nil {
						k = "compile error"
					} else if p := s.plan(x); p != nil {
						if p.alts != nil {
							k = "FAST or"
						} else {
							k = "FAST single"
						}
					} else {
						k = bucket(x)
						shapes[k+"  ::  "+shapeOf(x)]++
					}
					counts[k]++
					if mods[k] == nil {
						mods[k] = map[string]bool{}
					}
					mods[k][parts[0]] = true
				}
			}
		}
		return nil
	})
	type kv struct {
		K string
		N int
	}
	var top []kv
	for k, n := range shapes {
		top = append(top, kv{k, n})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].N > top[j].N })
	if len(top) > 60 {
		top = top[:60]
	}
	modCount := map[string]int{}
	for k, m := range mods {
		modCount[k] = len(m)
	}
	out, _ := json.MarshalIndent(map[string]any{"files": files, "counts": counts, "mods": modCount, "top_shapes": top}, "", " ")
	os.WriteFile(os.Getenv("CENSUS_OUT"), out, 0o644)
	t.Logf("%s", out[:min(len(out), 3000)])
}
