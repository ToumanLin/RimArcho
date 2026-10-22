package view

import (
	"fmt"
	"strings"
	"testing"
)

func barItem(name, kind string, tags []string, vers ...string) Item {
	it := Item{ID: strings.ToLower(name), Name: name, Kind: kind, Tags: tags, Versions: vers}
	it.names = strings.ToLower(name)
	it.text = it.names
	return it
}

func barBoard() *Board {
	b := &Board{}
	b.Cols[Inactive] = []Item{
		barItem("Alpha", "workshop", []string{"美化"}, "1.5", "1.6"),
		barItem("Bravo", "local", []string{"框架"}, "1.6"),
		barItem("Charlie", "workshop", nil, "1.4"),
	}
	b.Cols[Active] = []Item{
		barItem("Delta", "workshop", []string{"美化", "框架"}, "1.6"),
		barItem("Echo", "official", nil, "1.6"),
	}
	return b
}

// The bar filters 未启用 and 已启用 together; values of one group are "or", groups "and",
// a leading "-" leaves out (06 §2.5).
func TestFilterBoard(t *testing.T) {
	b := barBoard()
	cases := []struct {
		search  string
		filters []string
		want    string
	}{
		{"", nil, "[[0 1 2] [0 1]]"},
		{"", []string{"source:workshop"}, "[[0 2] [0]]"},
		{"", []string{"source:workshop", "source:local"}, "[[0 1 2] [0]]"}, // or
		{"", []string{"source:workshop", "tag:美化"}, "[[0] [0]]"},           // and
		{"", []string{"-tag:美化"}, "[[1 2] [1]]"},
		{"", []string{"version:1.6"}, "[[0 1] [0 1]]"},
		{"a", []string{"source:workshop"}, "[[0 2] [0]]"}, // alpha, charlie, delta
		{"version:1.4", nil, "[[2] []]"},
	}
	for _, c := range cases {
		got := FilterBoard(b, c.search, c.filters, MatchFacts{}, false, "1.6")
		if fmt.Sprint(got.Rows) != c.want {
			t.Errorf("%q %v: got %v, want %s", c.search, c.filters, got.Rows, c.want)
		}
	}
}

// A group's counts ignore its own choices; the game's version leads its group, marked.
func TestFilterBoardGroups(t *testing.T) {
	got := FilterBoard(barBoard(), "", []string{"source:workshop"}, MatchFacts{}, true, "1.6")
	by := map[string]LibGroup{}
	for _, g := range got.Groups {
		by[g.Key] = g
	}
	src := ""
	for _, v := range by["source"].Values {
		src += fmt.Sprintf("%s=%d/%d ", v.Filter, v.Count, v.On)
	}
	if src != "source:workshop=3/1 source:local=1/0 source:official=1/0 " {
		t.Errorf("source: %s", src)
	}
	ver := by["version"].Values
	if len(ver) == 0 || ver[0].Filter != "version:1.6" || !ver[0].Cur || ver[0].Count != 2 {
		t.Errorf("version: %+v", ver)
	}
	if len(got.Conds) != 1 || got.Conds[0].Filter != "source:workshop" {
		t.Errorf("conds: %+v", got.Conds)
	}
}

// J / K while the bar filters: the rows trade places with the shown rows (06 §3.5).
func TestStepAt(t *testing.T) {
	// 10 enabled mods; the bar shows 2, 5, 8, 9
	vis := []int{2, 5, 8, 9}
	cases := []struct {
		moving []int
		d      int
		want   int
	}{
		{[]int{5}, 1, 8},     // after 8: in the list without 5, 8 is 7th → place 8
		{[]int{5}, -1, 2},    // before 2
		{[]int{5}, 10, 9},    // alt+J: as far as the last shown row
		{[]int{2}, -1, 0},    // the first shown row: nowhere to go
		{[]int{9}, 1, 0},     // the last
		{[]int{2, 5}, 1, 7},  // the block after 8: without 2 and 5, 8 is 6th → 7
		{[]int{8, 9}, -1, 5}, // the block before 5
	}
	for _, c := range cases {
		if got := StepAt(vis, c.moving, c.d); got != c.want {
			t.Errorf("StepAt(%v, %v, %d) = %d, want %d", vis, c.moving, c.d, got, c.want)
		}
	}
	// every row shown: MoveBy's place
	all := []int{1, 2, 3, 4, 5, 6}
	for p := 2; p <= 5; p++ {
		if got := StepAt(all, []int{p}, 1); got != p+1 {
			t.Errorf("all shown, %d down: %d", p, got)
		}
		if got := StepAt(all, []int{p}, -1); got != p-1 {
			t.Errorf("all shown, %d up: %d", p, got)
		}
	}
}
