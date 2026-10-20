package tui

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// The v3 interaction model (06 §13): T17–T20.

// v3Fixture: the fixture plus P001 … P125 enabled after it, and Late (loads after P050),
// not enabled. P001 has a long description.
func v3Fixture(t *testing.T) (*testkit.Env, *model) {
	e, _ := fixture(t)
	pkgs := []string{"ludeon.rimworld", "tester.a", "brrainz.harmony"}
	for i := 1; i <= 125; i++ {
		pkg := fmt.Sprintf("tester.p%03d", i)
		e.WriteMod(filepath.Join(e.Game, "Mods", fmt.Sprintf("P%03d", i)), testkit.Mod{PackageID: pkg, Name: fmt.Sprintf("P%03d", i), Author: "Tester"})
		pkgs = append(pkgs, pkg)
	}
	long := testkit.Mod{PackageID: "tester.p001", Name: "P001", Author: "Tester"}
	e.WriteFile(filepath.Join(e.Game, "Mods", "P001", "About", "About.xml"), strings.Replace(long.AboutXML(), "</ModMetaData>",
		"<description>"+strings.Repeat("A long description line that wraps a little.\n", 40)+"</description></ModMetaData>", 1))
	dir := filepath.Join(e.Game, "Mods", "Late")
	late := testkit.Sample("tester.late", "Late")
	e.WriteMod(dir, late)
	e.WriteFile(filepath.Join(dir, "About", "About.xml"), strings.Replace(late.AboutXML(), "</ModMetaData>",
		"<loadAfter><li>tester.p050</li></loadAfter></ModMetaData>", 1))
	e.WriteModsConfig("1.6.4633 rev1261", pkgs...)
	e.MustRun("scan")
	return e, direct(t, e, 160, 40)
}

// workspace is what no navigation may change: the list, staging, tags and the undo stack.
func workspace(t *testing.T, m *model) string {
	t.Helper()
	st, err := m.app.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	staged, _, _ := m.app.Staged()
	var s []string
	for _, x := range staged {
		s = append(s, x.Instance)
	}
	tags, _ := m.app.Store.TagCounts(st.Ix.Env.ID)
	return fmt.Sprintf("%s | %v | %v | %d", st.List.Encode(), s, tags, m.app.Store.StepCount(st.Ix.Env.ID, "undo"))
}

func pressAll(m *model, ks ...string) {
	for _, k := range ks {
		press(m, k)
	}
}

func pressKey(m *model, t tea.KeyType) { m.update(tea.KeyMsg{Type: t}) }

// v3State is one of the states the TUI can be in, set up from the list area.
type v3State struct {
	name  string
	setup func(m *model)
}

var v3States = []v3State{
	{"Disabled", func(m *model) { press(m, "1") }},
	{"Enabled", func(m *model) { press(m, "2") }},
	{"Staging", func(m *model) { press(m, "2"); press(m, "t"); press(m, "3") }},
	{"selection", func(m *model) { press(m, "2"); pressAll(m, "down", "shift+down", "shift+down") }},
	{"detail", func(m *model) { press(m, "4") }},
	{"full screen", func(m *model) { press(m, "2"); press(m, "down"); press(m, "enter") }},
	{"technical pages", func(m *model) { press(m, "2"); press(m, "enter"); press(m, "I") }},
	{"search", func(m *model) { press(m, "1"); press(m, "/"); typeIn(m, "p0") }},
	{"command palette", func(m *model) { m.update(tea.KeyMsg{Type: tea.KeyCtrlK}) }},
	{"tags", func(m *model) { press(m, "2"); pressAll(m, "shift+down", "#") }},
	{"move to", func(m *model) { press(m, "2"); pressAll(m, "down", "m") }},
	{"confirm", func(m *model) { press(m, "S") }},
	{"help", func(m *model) { press(m, "?") }},
	{"needs attention", func(m *model) { press(m, "!") }},
	{"recent changes", func(m *model) { press(m, "c") }},
}

// T17: in every state, 1000 random arrow keys (PgUp / PgDn and shift / ctrl arrows too)
// never change the workspace: nothing is moved, enabled, staged or tagged.
func TestT17ArrowsOnlyNavigate(t *testing.T) {
	e, _ := v3Fixture(t)
	arrows := []tea.KeyType{tea.KeyUp, tea.KeyDown, tea.KeyLeft, tea.KeyRight, tea.KeyPgUp, tea.KeyPgDown,
		tea.KeyShiftUp, tea.KeyShiftDown, tea.KeyShiftLeft, tea.KeyShiftRight, tea.KeyCtrlUp, tea.KeyCtrlDown}
	r := rand.New(rand.NewSource(17))
	for _, s := range v3States {
		t.Run(s.name, func(t *testing.T) {
			m := direct(t, e, 160, 40)
			s.setup(m)
			before := workspace(t, m)
			for i := 0; i < 1000; i++ {
				pressKey(m, arrows[r.Intn(len(arrows))])
				if i%97 == 0 {
					m.View()
				}
			}
			m.update(tea.MouseMsg{X: 50, Y: 10, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
			m.update(tea.MouseMsg{X: 50, Y: 10, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
			if after := workspace(t, m); after != before {
				t.Fatalf("arrows changed the workspace:\nbefore %s\nafter  %s", before, after)
			}
		})
	}
}

// T18: the tasks of 06 §13.9, each in no more keys than the v3 column, with the right result.
func TestT18TaskSteps(t *testing.T) {
	e, m := v3Fixture(t)
	n := 0
	key := func(ks ...string) {
		for _, k := range ks {
			press(m, k)
			n++
		}
	}
	key2 := func(k tea.KeyType) { pressKey(m, k); n++ }
	task := func(name string, want int) {
		t.Helper()
		if n > want {
			t.Fatalf("%s: %d keys, want ≤ %d", name, n, want)
		}
		n = 0
	}
	pos := func(name string) int { return m.st.Position(m.st.Ix.ByID[idOf(t, m, name)].ID) }

	// 1. Enable a mod by name at the right place, focus in Enabled: / name tab space.
	press(m, "2")
	n = 0
	key("/")
	typeIn(m, "late")
	key2(tea.KeyTab)
	key("space")
	task("enable Late", 3)
	if p := pos("Late"); p != pos("P050")+1 {
		t.Fatalf("Late at %d, P050 at %d (status %q)", p, pos("P050"), m.status)
	}
	if !strings.Contains(m.status, fmt.Sprintf("placed at position %d · m changes it", pos("Late"))) {
		t.Fatalf("status %q", m.status)
	}
	// 2. Move a mod after another, 120 apart: m name enter.
	press(m, "esc")
	press(m, "2")
	press(m, "g")
	for m.current().name != "P003" {
		press(m, "down")
	}
	n = 0
	key("m")
	typeIn(m, "p124")
	key("enter")
	task("move P003 after P124", 2)
	if pos("P003") != pos("P124")+1 {
		t.Fatalf("P003 at %d, P124 at %d", pos("P003"), pos("P124"))
	}
	// 3. Disable three mods that are not next to each other: moves, ctrl+space ×3, space.
	press(m, "g")
	n = 0
	for _, name := range []string{"P010", "P020", "P030"} {
		for m.current().name != name {
			press(m, "down")
		}
		key("ctrl+space")
	}
	key("space")
	task("disable three", 4)
	if got := activeNames(t, e); strings.Contains(got, "P010") || strings.Contains(got, "P020") || strings.Contains(got, "P030") || !strings.Contains(got, "P011") {
		t.Fatalf("after space: %s", got)
	}
	// 4. Read the current mod's description to the end: PgDn PgDn.
	press(m, "g")
	for m.current().name != "P001" {
		press(m, "down")
	}
	runCmd(m, m.loadData(m.current().id, false, false))
	m.View()
	n = 0
	key2(tea.KeyPgDown)
	m.View()
	key2(tea.KeyPgDown)
	out := ansi.Strip(m.View())
	task("read the description", 2)
	if !strings.Contains(out, "technical information · i") || m.detailFocus || m.current().name != "P001" {
		t.Fatalf("not at the end, or the focus moved (detail %v, %s):\n%s", m.detailFocus, m.current().name, out)
	}
	// 5. A pinned tag for five mods in a row: shift+↓ ×4, # → enter.
	press(m, "esc")
	for m.current().name != "P040" {
		press(m, "down")
	}
	n = 0
	key("shift+down", "shift+down", "shift+down", "shift+down", "#", "right", "enter")
	task("tag five", 7)
	tagged := e.MustRun("list", "--filter", "tag:"+m.tagsDefault(1))
	for _, name := range []string{"P040", "P041", "P042", "P043", "P044"} {
		if !strings.Contains(tagged, name) {
			t.Fatalf("%s not tagged %q:\n%s", name, m.tagsDefault(1), tagged)
		}
	}
	// 6. Half of the list into staging: shift+↓ N times, t.
	press(m, "esc")
	press(m, "g")
	n = 0
	half := len(m.cols[cActive].items) / 2
	for i := 0; i < half-1; i++ {
		key("shift+down")
	}
	key("t")
	task("stage half", half)
	if got := len(m.cols[cStaging].items); got != half {
		t.Fatalf("staged %d, want %d (%q)", got, half, m.status)
	}
}

// idOf finds an instance by its name.
func idOf(t *testing.T, m *model, name string) string {
	t.Helper()
	for _, mod := range m.st.Ix.Mods {
		if mod.Name == name {
			return mod.ID
		}
	}
	t.Fatalf("no mod %q", name)
	return ""
}

// tagsDefault is the i-th pinned tag (06 §12.3).
func (m *model) tagsDefault(i int) string {
	m.tagKey()
	defer func() { m.tags = nil }()
	return m.tags.cands[i].tag
}

// T19: every state, popups and the full screen included, shows the focused area and the
// place in it on the position line, and the open popup's name.
func TestT19PositionLine(t *testing.T) {
	e, m := v3Fixture(t)
	// The states share one environment (Staging stages a mod), so the counts are patterns.
	want := map[string]string{
		"Disabled":        `Disabled · 1 / \d+$`,
		"Enabled":         `Enabled · 1 / \d+$`,
		"Staging":         `Staging · 1 / \d+$`,
		"selection":       `Enabled · 4 / \d+ · selected 3$`,
		"detail":          `details · \S+`,
		"full screen":     `Enabled › \S+ \(2 / \d+\) › summary`,
		"technical pages": `Enabled › \S+ \(1 / \d+\) › technical pages`,
		"search":          `Disabled · \d+ / \d+ · filtered › search$`,
		"command palette": `Enabled · 1 / \d+ › command palette$`,
		"tags":            `Enabled · 2 / \d+ · selected 2 › tags$`,
		"move to":         `Enabled · 2 / \d+ › move to…$`,
		"confirm":         `Enabled · 1 / \d+ › sort — preview$`,
		"help":            `Enabled · 1 / \d+ › all keys$`,
		"needs attention": `› needs attention · 1 / \d+`,
		"recent changes":  `› recent changes`,
	}
	for _, s := range v3States {
		t.Run(s.name, func(t *testing.T) {
			m := direct(t, e, 160, 40)
			s.setup(m)
			top := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(ansi.Strip(strings.Split(m.View(), "\n")[0])), "L launch ►"))
			if !regexp.MustCompile(want[s.name]).MatchString(top) {
				t.Fatalf("position line %q, want %s", top, want[s.name])
			}
		})
	}
	// The position line in Chinese, and the full screen's sub-section.
	SetLang("zh")
	defer SetLang("en")
	m = direct(t, e, 160, 40)
	pressAll(m, "down", "shift+down")
	if top := ansi.Strip(strings.Split(m.View(), "\n")[0]); !regexp.MustCompile(`已启用 · 3 / \d+ · 已选 2`).MatchString(top) {
		t.Fatalf("zh: %q", top)
	}
}

// T20: from any state, esc at most three times gets back to a list area with nothing
// selected and nothing open.
func TestT20EscBack(t *testing.T) {
	e, _ := v3Fixture(t)
	deep := []v3State{
		{"selection + search", func(m *model) { pressAll(m, "shift+down", "/"); typeIn(m, "p") }},
		{"detail + selection + tags", func(m *model) { pressAll(m, "shift+down", "4", "#") }},
		{"detail + selection + move to", func(m *model) { pressAll(m, "shift+down", "4", "m") }},
		{"full + selection + palette", func(m *model) {
			pressAll(m, "shift+down", "enter")
			m.update(tea.KeyMsg{Type: tea.KeyCtrlK})
		}},
		{"full + technical pages two deep + selection", func(m *model) {
			pressAll(m, "shift+down", "enter", "I", "down", "down", "enter", "down", "enter")
		}},
		{"detail + selection + confirm", func(m *model) { pressAll(m, "shift+down", "4", "S") }},
		{"armed", func(m *model) {
			m.arm("d", "press d again", func(*model) tea.Cmd { return nil })
		}},
	}
	for _, s := range append(append([]v3State{}, v3States...), deep...) {
		t.Run(s.name, func(t *testing.T) {
			m := direct(t, e, 160, 40)
			s.setup(m)
			for i := 0; ; i++ {
				if m.tab == tabList && !m.full && !m.detailFocus && m.insp == nil && m.popupName() == "" && m.selCount() == 0 {
					break
				}
				if i == 3 {
					t.Fatalf("not back after 3 esc: %q", ansi.Strip(strings.Split(m.View(), "\n")[0]))
				}
				press(m, "esc")
			}
		})
	}
}

// The position input (06 §13.7): a number, a name with tab for before, enter alone is the
// suggested place; from Disabled it enables there, one undo step.
func TestPositionInput(t *testing.T) {
	e, m := v3Fixture(t)
	pos := func(name string) int { return m.st.Position(idOf(t, m, name)) }
	press(m, "2")
	press(m, "g")
	press(m, "down") // Alpha
	press(m, "m")
	typeIn(m, "5")
	if m.pos == nil || m.pos.opts[0].label != "position 5" {
		t.Fatalf("options %+v", m.pos)
	}
	press(m, "enter")
	if pos("Alpha") != 5 || !strings.Contains(m.status, "Alpha is now number 5") {
		t.Fatalf("Alpha at %d (%q)", pos("Alpha"), m.status)
	}
	press(m, "m")
	typeIn(m, "p100")
	pressKey(m, tea.KeyTab) // before
	if !strings.HasPrefix(m.pos.opts[0].label, "before") {
		t.Fatalf("tab: %+v", m.pos.opts[0])
	}
	press(m, "enter")
	if pos("Alpha") != pos("P100")-1 {
		t.Fatalf("Alpha %d, P100 %d", pos("Alpha"), pos("P100"))
	}
	// From Disabled: m, enter alone = the suggested place (after P050).
	steps := e.App.Store.StepCount(m.st.Ix.Env.ID, "undo")
	press(m, "1")
	for m.current().name != "Late" {
		press(m, "down")
	}
	press(m, "m")
	if !strings.Contains(ansi.Strip(m.View()), "(suggested)") {
		t.Fatalf("no suggestion:\n%s", ansi.Strip(m.View()))
	}
	press(m, "enter")
	if pos("Late") != pos("P050")+1 || m.focus != cInactive {
		t.Fatalf("Late at %d, P050 %d, focus %d", pos("Late"), pos("P050"), m.focus)
	}
	if n := e.App.Store.StepCount(m.st.Ix.Env.ID, "undo"); n != steps+1 {
		t.Fatalf("undo steps %d → %d", steps, n)
	}
	// space then m: m moves the mod space just placed, from where the cursor is.
	press(m, "u")
	for m.current().name != "Late" {
		press(m, "up")
	}
	press(m, "space")
	press(m, "m")
	if m.pos == nil || m.pos.from != cActive || m.pos.what != "Late" {
		t.Fatalf("m after space: %+v", m.pos)
	}
	press(m, "esc")
	// J K move by one, alt+J by ten.
	press(m, "2")
	press(m, "g")
	for m.current().name != "Harmony" {
		press(m, "down")
	}
	at := pos("Harmony")
	press(m, "J")
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J"), Alt: true})
	if pos("Harmony") != at+11 || m.current().name != "Harmony" {
		t.Fatalf("Harmony at %d (from %d), cursor %s", pos("Harmony"), at, m.current().name)
	}
	press(m, "K")
	if pos("Harmony") != at+10 {
		t.Fatalf("K: %d", pos("Harmony"))
	}
}

// 06 §13.8: while searching, the status line counts the other lists' matches; tab takes
// the search to the next list with matches.
func TestSearchOtherLists(t *testing.T) {
	_, m := v3Fixture(t)
	press(m, "2")
	press(m, "/")
	typeIn(m, "beta")
	if l := ansi.Strip(m.statusLine()); !strings.Contains(l, "other lists: Disabled 1 · Staging 0") {
		t.Fatalf("status %q", l)
	}
	pressKey(m, tea.KeyTab)
	if m.focus != cInactive || m.editing || m.current().name != "Beta" || m.cols[cActive].filter != "" || m.cols[cInactive].filter != "beta" {
		t.Fatalf("tab: focus %d, editing %v, %s", m.focus, m.editing, m.current().name)
	}
}
