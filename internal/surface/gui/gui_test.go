package gui

import (
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

var textRe = regexp.MustCompile(`\bTf?\("((?:[^"\\]|\\.)*)"`)

// Every string the page translates has a Chinese entry (06 §2.3; the page has no table of
// its own: it gets view.Table).
func TestPageTextTranslated(t *testing.T) {
	view.SetLang("zh")
	t.Cleanup(func() { view.SetLang("en") })
	table := view.Table()
	var missing []string
	fs.WalkDir(assets, "assets/js", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".js") {
			return err
		}
		b, _ := fs.ReadFile(assets, p)
		for _, m := range textRe.FindAllStringSubmatch(string(b), -1) {
			s := strings.ReplaceAll(m[1], `\"`, `"`)
			if _, ok := table[s]; !ok {
				missing = append(missing, s)
			}
		}
		return nil
	})
	sort.Strings(missing)
	missing = compactStrings(missing)
	if len(missing) > 0 {
		t.Errorf("%d page strings have no Chinese text:\n%s", len(missing), strings.Join(missing, "\n"))
	}
}

func compactStrings(s []string) []string {
	var out []string
	for i, x := range s {
		if i == 0 || x != s[i-1] {
			out = append(out, x)
		}
	}
	return out
}
