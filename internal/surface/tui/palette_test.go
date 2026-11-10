package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestPalScore(t *testing.T) {
	t.Parallel()
	it := func(name, alias, desc string) palItem { return palItem{name: name, alias: alias, desc: desc} }
	for _, c := range []struct {
		q    string
		item palItem
		want int
	}{
		{"so", it("自动排序", "sort", ""), 6},                    // the English alias, prefix
		{"排序", it("自动排序", "sort", ""), 6},                    // the Chinese name's head noun, suffix
		{"动排", it("自动排序", "sort", ""), 5},                    // substring
		{"srt", it("自动排序", "sort", ""), 2},                   // subsequence
		{"sotr", it("自动排序", "sort", ""), 3},                  // a typo
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

// 16 §3.5: ctrl+k; commands by Chinese name or English alias; mods jump; esc closes.
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
		t.Fatalf("sort: first result %q", r.name)
	}
	m.update(tea.KeyMsg{Type: tea.KeyCtrlU})
	typeIn(m, "save")
	if r := m.pal.results[0]; r.name != "保存" {
		t.Fatalf("save: first result %q", r.name)
	}
	m.update(tea.KeyMsg{Type: tea.KeyEsc})
	SetLang("en")
	// a mod in another column: enter jumps there, even through that column's search
	m.bar.search = "zzz"
	m.barChanged()
	m.update(ctrlK)
	typeIn(m, "beta")
	if r := m.pal.results[0]; r.kind != palMod || r.name != "Beta" {
		t.Fatalf("beta: first result %q", r.name)
	}
	m.update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.pal != nil || m.focus != cInactive || m.current() == nil || m.current().name != "Beta" || m.bar.search != "" {
		t.Fatalf("jump: focus %d, current %v, filter %q", m.focus, m.current(), m.bar.search)
	}
	// a command runs: needs attention
	m.update(ctrlK)
	typeIn(m, "can it run")
	m.update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.tab != tabProblems {
		t.Fatal("needs attention did not open")
	}
}

// 16 §3.5: the panel's own entries over a thousand Mods, many holding the typed letters in
// order: a typo still finds its command first, in English and Chinese.
func TestPaletteLibrary(t *testing.T) {
	e, _ := fixture(t)
	m := direct(t, e, 140, 40)
	for i, name := range testkit.ModNames(1000) {
		m.cols[cInactive].items = append(m.cols[cInactive].items, item{id: fmt.Sprintf("syn%d", i), name: name, pkg: "syn.mod"})
	}
	check := func(lang string) {
		for _, c := range []struct{ q, alias string }{
			{"sotr", "sort"},
			{"subscirbe", "add a subscription"},
			{"analyze", "check the files again"},
		} {
			m.update(tea.KeyMsg{Type: tea.KeyCtrlK})
			typeIn(m, c.q)
			if r := m.pal.results; len(r) == 0 || r[0].kind != palCommand || r[0].alias != c.alias {
				t.Errorf("%s %q: %d results, first %+v", lang, c.q, len(r), r)
			}
			m.update(tea.KeyMsg{Type: tea.KeyEsc})
		}
	}
	check("en")
	SetLang("zh")
	defer SetLang("en")
	check("zh")
}

// 16 §3.5: a command run from the full detail opens its page with the keys of that page; the
// full detail closes when the command leaves the lists.
func TestPaletteFromFullDetail(t *testing.T) {
	e, _ := fixture(t)
	m := direct(t, e, 140, 40)
	press(m, "enter")
	if !m.full {
		t.Fatal("enter: no full detail")
	}
	m.update(tea.KeyMsg{Type: tea.KeyCtrlK})
	typeIn(m, "mod library")
	if r := m.pal.results; len(r) == 0 || r[0].alias != "mod library" {
		t.Fatalf("mod library: results %+v", r)
	}
	press(m, "enter")
	if m.tab != tabLibrary || m.full {
		t.Fatalf("after the command: tab %d, full %v", m.tab, m.full)
	}
	// the library takes its keys: tab goes to the next page under the same sidebar entry
	pressKey(m, tea.KeyTab)
	if m.tab == tabLibrary {
		t.Fatal("tab in the library did nothing")
	}
}
