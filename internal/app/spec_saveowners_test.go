package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/speccheck"
)

// 12 §4.6 "Defs" lists the generated-Def prefixes and suffixes exactly as impliedPrefixes and
// impliedSuffixes (saveowners.go) do.
func TestSpecImpliedDefs(t *testing.T) {
	t.Parallel()
	sec, err := speccheck.Section("12", "4.6")
	if err != nil {
		t.Fatal(err)
	}
	var where speccheck.Line
	var prefixes, suffixes []string
	for _, l := range sec {
		if !strings.HasPrefix(l.Text, "- **Defs**") {
			continue
		}
		where = l
		_, rest, ok := strings.Cut(l.Text, " prefixes ")
		if !ok {
			t.Fatalf("%s: 12 §4.6: no prefixes", l.Pos())
		}
		pre, suf, _ := strings.Cut(rest, " suffixes ")
		for _, s := range speccheck.Spans(pre) {
			if strings.HasSuffix(s, "_") {
				prefixes = append(prefixes, s)
			}
		}
		for _, s := range speccheck.Spans(suf) {
			if strings.HasPrefix(s, "_") {
				suffixes = append(suffixes, s)
			}
		}
	}
	if len(prefixes) == 0 || len(suffixes) == 0 {
		t.Fatal("12 §4.6: no **Defs** item with prefixes and suffixes")
	}
	for _, c := range []struct {
		name      string
		doc, code []string
	}{{"impliedPrefixes", prefixes, impliedPrefixes}, {"impliedSuffixes", suffixes, impliedSuffixes}} {
		for _, v := range c.code {
			if !slices.Contains(c.doc, v) {
				t.Errorf("%s: 12 §4.6: %s (app.%s) is missing", where.Pos(), v, c.name)
			}
		}
		for _, v := range c.doc {
			if !slices.Contains(c.code, v) {
				t.Errorf("%s: 12 §4.6 lists %s, which app.%s does not have", where.Pos(), v, c.name)
			}
		}
	}
}
