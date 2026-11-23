package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// E groups the Game Configs by installation (16 §9.1): a faint heading with the game and
// its folder, then that installation's Game Configs, then + Add Game Config.
func TestEnvPickerGroupsByInstallation(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	other := e.OtherGame("other-game", "1.5.4409 rev1118")
	e.MustRun("env", "add", "legacy", "--game", other, "--saves", "own")
	e.MustRun("env", "add", "copy", "--like", "main")
	e.MustRun("env", "use", "main")
	m := direct(t, e, 160, 40)
	key(m, runes("E"))
	p := m.panel
	if p == nil || p.title != T("switch Game Config") {
		t.Fatalf("no picker: %+v", p)
	}
	game := map[string]string{}
	es, _ := e.App.Store.Envs()
	for _, x := range es {
		game[x.Name] = x.GameID
	}
	if len(p.choiceHeads) != 2 || p.choiceHeads[0] == "" {
		t.Fatalf("headings %v", p.choiceHeads)
	}
	for i := 1; i < len(p.choices)-1; i++ {
		head := p.choiceHeads[i] != ""
		if changed := game[p.choices[i]] != game[p.choices[i-1]]; changed != head {
			t.Fatalf("choice %d %q: heading %v, installation changed %v (%q)", i, p.choices[i], head, changed, p.choices)
		}
	}
	if !p.choiceCurrent[p.choice] || p.choices[p.choice] != "main" {
		t.Fatalf("cursor on %q", p.choices[p.choice])
	}
	s := screen(m)
	if !strings.Contains(s, "RimWorld 1.5") || !strings.Contains(s, "RimWorld 1.6") {
		t.Fatalf("no installation headings:\n%s", s)
	}
	// a heading is not a choice: down from the last of one installation lands on the next's first
	for p.choice = 0; p.choiceHeads[p.choice+1] == ""; p.choice++ {
	}
	press(m, "down")
	if p.choiceHeads[p.choice] == "" {
		t.Fatalf("down landed on %d %q", p.choice, p.choices[p.choice])
	}
}

// The shared-game-folder notice stays until the player acknowledges it with x: opening and
// closing the picker, or the window's Game Config page drawing it, is not reading it.
func TestEnvPickerAliasNoticeUntilAcknowledged(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	other := e.OtherGame("old-alias", "1.6.4633 rev1261")
	e.MustRun("env", "add", "legacy", "--game", other, "--saves", "own")
	legacy, _ := e.App.Store.EnvByName("legacy")
	if _, err := e.App.Store.DB.Exec("UPDATE game SET game_root=? WHERE id=?", e.Game, legacy.GameID); err != nil {
		t.Fatal(err)
	}
	m := direct(t, e, 160, 40)
	notice := Tf("these two Game Configs use the same game folder: %s", "legacy, main")
	for range 2 {
		key(m, runes("E"))
		if s := ansi.Strip(screen(m)); !strings.Contains(s, notice) || !strings.Contains(s, T("x: got it, don't show this again")) {
			t.Fatalf("no notice:\n%s", s)
		}
		press(m, "esc")
		if got := e.App.Store.Meta("notice.shared-game-folder"); got != "" {
			t.Fatalf("showing the notice marked it %q", got)
		}
	}
	key(m, runes("E"))
	key(m, runes("x"))
	if m.panel == nil || len(m.panel.lines) != 0 || len(e.App.AliasNotice()) != 0 {
		t.Fatalf("x: panel %v, notice %q", m.panel, e.App.AliasNotice())
	}
	press(m, "esc")
	key(m, runes("E"))
	if strings.Contains(ansi.Strip(screen(m)), notice) {
		t.Fatal("acknowledged notice came back")
	}
}

// No Game Config loads (no game added yet): keys that need a list or a Game Config do nothing
// and say why; quit, help, : and E still work, and help shows over the no-game screen.
func TestNoGameConfigKeys(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	scans := 0
	m := New(Options{App: e.App, Scan: func(string) (string, error) { scans++; return "", errors.New("no game") }}).(*model)
	m.Init()
	m.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if !errors.Is(m.loadErr, app.ErrNoEnv) {
		t.Fatalf("loadErr %v", m.loadErr)
	}
	for _, k := range []tea.KeyMsg{runes("R"), runes("P"), runes("L"), runes("!"), runes("@"), runes("%"), runes("&"), runes("w"), runes("T"), runes("S"), runes("e"), runes("c"),
		{Type: tea.KeyTab}, {Type: tea.KeyF10}, {Type: tea.KeyCtrlK}, {Type: tea.KeyEnter}, {Type: tea.KeySpace, Runes: []rune(" ")}} {
		key(m, k)
		if m.panel != nil || m.pal != nil || m.tab != tabList || scans != 0 || m.status != T("No game added yet.") {
			t.Fatalf("%s: panel %v palette %v tab %d scans %d status %q", k, m.panel != nil, m.pal != nil, m.tab, scans, m.status)
		}
	}
	key(m, runes("E"))
	if m.status != T("no Game Configs yet (archo env add)") {
		t.Fatalf("E: %q", m.status)
	}
	key(m, runes("?"))
	if m.panel == nil || strings.Contains(screen(m), T("No game added yet.")) {
		t.Fatalf("help is not shown over the no-game screen:\n%s", screen(m))
	}
	press(m, "esc")
	key(m, runes(":"))
	if !m.cmdOn {
		t.Fatal(": did not open the command line")
	}
	m.update(tea.KeyMsg{Type: tea.KeyEsc})
	key(m, runes("q"))
	if !m.quitting {
		t.Fatal("q did not quit")
	}
}
