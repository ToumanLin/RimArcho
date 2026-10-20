package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPalScore(t *testing.T) {
	it := func(name, alias, desc string) palItem { return palItem{name: name, alias: alias, desc: desc} }
	for _, c := range []struct {
		q    string
		item palItem
		want int
	}{
		{"so", it("自动排序", "sort", ""), 4},                    // the English alias, prefix
		{"排序", it("自动排序", "sort", ""), 3},                    // the Chinese name, substring
		{"srt", it("自动排序", "sort", ""), 2},                   // subsequence
		{"brrainz", it("Harmony", "", "brrainz.harmony"), 1}, // description only
		{"xyz", it("Harmony", "", "brrainz.harmony"), 0},
	} {
		if got := palScore(c.q, c.item); got != c.want {
			t.Errorf("palScore(%q, %q) = %d, want %d", c.q, c.item.name, got, c.want)
		}
	}
}

func typeIn(m *model, s string) {
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
}

// 06 §8: ctrl+k; commands by Chinese name or English alias; mods jump; esc closes.
func TestPalette(t *testing.T) {
	e, _ := fixture(t)
	m := direct(t, e, 140, 40)
	ctrlK := tea.KeyMsg{Type: tea.KeyCtrlK}
	m.update(ctrlK)
	if m.pal == nil || len(m.pal.results) == 0 || m.pal.results[0].kind != palCommand {
		t.Fatal("ctrl+k: no palette with commands first")
	}
	typeIn(m, "sort")
	if r := m.pal.results[0]; r.alias != "sort" {
		t.Fatalf("sort: first result %q", r.name)
	}
	lines := strings.Join(m.paletteLines(30), "\n")
	if !strings.Contains(lines, "sort") || !strings.Contains(lines, "❯") {
		t.Fatalf("palette screen:\n%s", lines)
	}
	m.update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.pal != nil {
		t.Fatal("esc did not close")
	}
	// Chinese names in zh, the English alias still matches
	SetLang("zh")
	defer SetLang("en")
	m.update(ctrlK)
	typeIn(m, "排序")
	if r := m.pal.results[0]; r.name != "自动排序" {
		t.Fatalf("排序: first result %q", r.name)
	}
	m.update(tea.KeyMsg{Type: tea.KeyCtrlU})
	typeIn(m, "save")
	if r := m.pal.results[0]; r.name != "保存到游戏" {
		t.Fatalf("save: first result %q", r.name)
	}
	m.update(tea.KeyMsg{Type: tea.KeyEsc})
	SetLang("en")
	// a mod in another column: enter jumps there, even through that column's search
	m.cols[cInactive].filter = "zzz"
	m.refilterCol(cInactive, "")
	m.update(ctrlK)
	typeIn(m, "beta")
	if r := m.pal.results[0]; r.kind != palMod || r.name != "Beta" {
		t.Fatalf("beta: first result %q", r.name)
	}
	m.update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.pal != nil || m.focus != cInactive || m.current() == nil || m.current().name != "Beta" || m.cols[cInactive].filter != "" {
		t.Fatalf("jump: focus %d, current %v, filter %q", m.focus, m.current(), m.cols[cInactive].filter)
	}
	// a command runs: needs attention
	m.update(ctrlK)
	typeIn(m, "needs")
	m.update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.tab != tabProblems {
		t.Fatal("needs attention did not open")
	}
}
