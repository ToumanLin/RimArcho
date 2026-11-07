package view

import (
	"fmt"
	"testing"
)

func TestTypedBarConditions(t *testing.T) {
	t.Parallel()
	b := barBoard()
	for _, query := range []string{"source:local source:workshop", "tags:美化 source:workshop", "-source:local version:1.6", "source:"} {
		words, filters := splitBoardQuery(query)
		typed := FilterBoard(b, query, nil, MatchFacts{}, true, "1.6")
		clicked := FilterBoard(b, words, filters, MatchFacts{}, true, "1.6")
		if fmt.Sprint(typed.Rows) != fmt.Sprint(clicked.Rows) {
			t.Errorf("%s: %v != %v", query, typed.Rows, clicked.Rows)
		}
	}
	b.Cols[0][0].Sources = []string{"other", "dev"}
	b.Cols[0][0].SourceNames = map[string]string{"dev": "Development mods"}
	r := FilterBoard(b, "source:dev", nil, MatchFacts{}, true, "1.6")
	if fmt.Sprint(r.Rows) != "[[0] []]" {
		t.Fatalf("directory alias: %v", r.Rows)
	}
	for _, g := range r.Groups {
		if g.Key == "source" {
			for _, v := range g.Values {
				if v.Filter == "source:dev" && v.Label != "Development mods" {
					t.Fatalf("alias label: %+v", v)
				}
			}
		}
	}
}
