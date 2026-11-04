package tui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/cmds"
)

func keyCmd(m *model, t tea.KeyType) tea.Cmd {
	_, cmd := m.update(tea.KeyMsg{Type: t})
	return cmd
}

// runLine opens the command line, types line, presses enter and, when a background run
// started, delivers its result.
func runLine(t *testing.T, m *model, line string) {
	t.Helper()
	typeText(m, ":")
	if !m.cmdOn {
		t.Fatal(": did not open the command line")
	}
	typeText(m, line)
	cmd := keyCmd(m, tea.KeyEnter)
	if cmd == nil {
		return
	}
	msg := cmd()
	if d, ok := msg.(cmdDoneMsg); ok {
		m.update(d)
	}
}

func lineText(m *model) string { return string(m.cmdl.text) }

// 06 §9.4: quotes keep spaces in one argument, backslashes stay (Windows paths), # is text.
func TestCmdLineWords(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		in   string
		want []string
	}{
		{`archo pack launch "My pack"`, []string{"pack", "launch", "My pack"}},
		{`plan new 'Two words'  --from x`, []string{"plan", "new", "Two words", "--from", "x"}},
		{`folder set src:a D:\Mods\x`, []string{"folder", "set", "src:a", `D:\Mods\x`}},
		{`mod #a3f9`, []string{"mod", "#a3f9"}},
		{`note a3f9 ""`, []string{"note", "a3f9", ""}},
		{`--name="a b"c`, []string{"--name=a bc"}},
		{`find "unclosed words`, []string{"find", "unclosed words"}},
		{"   ", nil},
	} {
		if got := cmdArgv([]rune(c.in)); !slices.Equal(got, c.want) {
			t.Errorf("%q: %q, want %q", c.in, got, c.want)
		}
	}
	if env, rest, _ := cmdEnv([]string{"list", "--env", "other", "--active"}); env != "other" || strings.Join(rest, " ") != "list --active" {
		t.Errorf("--env: %q %q", env, rest)
	}
	if _, _, bad := cmdEnv([]string{"list", "--env"}); bad == "" {
		t.Error("--env without a name")
	}
}

// 06 §9.4: tab completes command names, flags, mods, list names and setting values; several
// candidates open the list, tab cycles, enter takes one without running, esc puts the word back.
func TestCmdLineComplete(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 120, 30)
	typeText(m, ":")
	typeText(m, "pl")
	keyCmd(m, tea.KeyTab)
	l := m.cmdl
	if len(l.cands) < 2 || !strings.HasPrefix(lineText(m), "plan ") {
		t.Fatalf("pl⇥: %q, %d candidates", lineText(m), len(l.cands))
	}
	for _, c := range l.cands {
		if !strings.HasPrefix(c.insert, "plan ") && !strings.Contains(c.insert, "pl") {
			t.Fatalf("pl⇥ offers %q", c.insert)
		}
	}
	first := lineText(m)
	keyCmd(m, tea.KeyTab)
	if lineText(m) == first {
		t.Fatalf("tab did not move to the next candidate: %q", lineText(m))
	}
	view := m.View()
	if !strings.Contains(view, l.cands[l.sel].label) {
		t.Fatalf("the candidate list is not drawn:\n%s", view)
	}
	keyCmd(m, tea.KeyEsc)
	if lineText(m) != "pl" || l.cands != nil || !m.cmdOn {
		t.Fatalf("esc in the list: %q, open %v", lineText(m), m.cmdOn)
	}
	// enter takes a candidate and does not run it
	keyCmd(m, tea.KeyCtrlU)
	typeText(m, "plan ne")
	keyCmd(m, tea.KeyTab)
	if lineText(m) != "plan new " || l.cands != nil {
		t.Fatalf("one candidate goes in with a space: %q", lineText(m))
	}
	// the usage of the recognised command is on the line above
	if v := m.View(); !strings.Contains(v, "archo plan new <name>") {
		t.Fatalf("no usage line:\n%s", v)
	}
	// setting values
	keyCmd(m, tea.KeyCtrlU)
	typeText(m, "config set ui.icons ")
	keyCmd(m, tea.KeyTab)
	if got := []string{l.cands[0].insert, l.cands[1].insert}; !slices.Equal(got, []string{"compat", "nerd"}) {
		t.Fatalf("ui.icons values: %q", got)
	}
	keyCmd(m, tea.KeyEnter)
	if lineText(m) != "config set ui.icons compat " || !m.cmdOn {
		t.Fatalf("enter in the list takes the candidate: %q, open %v", lineText(m), m.cmdOn)
	}
	// flags
	keyCmd(m, tea.KeyCtrlU)
	typeText(m, "sort --dr")
	keyCmd(m, tea.KeyTab)
	if lineText(m) != "sort --dry-run " {
		t.Fatalf("flag: %q", lineText(m))
	}
	// a mod: an empty word starts with the mod under the cursor; a name finds its reference
	keyCmd(m, tea.KeyCtrlU)
	typeText(m, "mod ")
	keyCmd(m, tea.KeyTab)
	cur := m.current()
	if want := "mod " + strings.TrimPrefix(cur.ref, "#"); !strings.HasPrefix(lineText(m), want) {
		t.Fatalf("mod⇥: %q, want %q", lineText(m), want)
	}
	keyCmd(m, tea.KeyEsc)
	keyCmd(m, tea.KeyCtrlU)
	typeText(m, "why harm")
	keyCmd(m, tea.KeyTab)
	if !strings.HasPrefix(lineText(m), "why ") || strings.Contains(lineText(m), "harm") || l.cands != nil {
		t.Fatalf("why harm⇥: %q", lineText(m))
	}
	// nothing to complete
	keyCmd(m, tea.KeyCtrlU)
	typeText(m, "zzzz")
	keyCmd(m, tea.KeyTab)
	if l.info == "" || lineText(m) != "zzzz" {
		t.Fatalf("no candidates: %q %q", lineText(m), l.info)
	}
	// esc closes and keeps the unsent line for next time
	keyCmd(m, tea.KeyEsc)
	if m.cmdOn {
		t.Fatal("esc did not close")
	}
	typeText(m, ":")
	if lineText(m) != "zzzz" {
		t.Fatalf("the unsent line is gone: %q", lineText(m))
	}
}

// 06 §9.4: a usage error does not run and keeps the line with the error; a line runs in the
// background, one at a time; one line of output goes to the status line, more to a panel;
// quotes make one argument.
func TestCmdLineRun(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 120, 30)
	typeText(m, ":")
	typeText(m, "plan new")
	if cmd := keyCmd(m, tea.KeyEnter); cmd != nil || !m.cmdOn || !strings.Contains(m.cmdl.err, "wrong number of arguments") || lineText(m) != "plan new" {
		t.Fatalf("usage error: open %v, err %q, line %q", m.cmdOn, m.cmdl.err, lineText(m))
	}
	if v := m.View(); !strings.Contains(v, "wrong number of arguments") {
		t.Fatalf("the error is not shown:\n%s", v)
	}
	typeText(m, ` "Two words"`)
	if m.cmdl.err != "" {
		t.Fatal("an edit keeps the old error")
	}
	cmd := keyCmd(m, tea.KeyEnter)
	if cmd == nil || m.cmdOn || m.cmdBusy == "" {
		t.Fatalf("enter: cmd %v, open %v, busy %q", cmd != nil, m.cmdOn, m.cmdBusy)
	}
	if !strings.Contains(m.View(), "running archo plan new") {
		t.Fatalf("no running note:\n%s", m.View())
	}
	// a second line waits for the first
	typeText(m, ":")
	typeText(m, "version")
	if c := keyCmd(m, tea.KeyEnter); c != nil || m.cmdl.err == "" {
		t.Fatal("a second command ran while the first was running")
	}
	keyCmd(m, tea.KeyCtrlU)
	keyCmd(m, tea.KeyEsc)
	m.update(cmd().(cmdDoneMsg))
	if m.cmdBusy != "" {
		t.Fatal("still busy")
	}
	out, _ := cmds.Execute(e.App, []string{"plan", "list"})
	if !strings.Contains(out, "Two words") {
		t.Fatalf("the quoted name was not one argument:\n%s", out)
	}
	// one line: the status line; several: a panel titled with the command
	runLine(t, m, "config set ui.mouse on")
	if m.panel != nil || !strings.Contains(m.status, "ui.mouse") || m.sayKind != sayOK {
		t.Fatalf("one line: panel %v, status %q", m.panel != nil, m.status)
	}
	runLine(t, m, "archo list --active")
	if m.panel == nil || m.panel.title != "archo list --active" {
		t.Fatalf("several lines: panel %+v", m.panel)
	}
	keyCmd(m, tea.KeyEsc)
	// a failure says so
	runLine(t, m, "mod nosuchmod")
	if m.sayKind != sayFail {
		t.Fatalf("a failed command: %q kind %d", m.status, m.sayKind)
	}
	// commands that only run as a whole command line say so and keep the line
	typeText(m, ":")
	keyCmd(m, tea.KeyCtrlU)
	typeText(m, "mcp")
	if c := keyCmd(m, tea.KeyEnter); c != nil {
		msg := c()
		m.update(msg.(cmdDoneMsg))
	}
	if !m.cmdOn || !strings.Contains(m.cmdl.err, "whole command line") {
		t.Fatalf("mcp: open %v err %q", m.cmdOn, m.cmdl.err)
	}
	keyCmd(m, tea.KeyCtrlU)
	typeText(m, "log --follow")
	if c := keyCmd(m, tea.KeyEnter); c != nil || m.cmdl.err == "" {
		t.Fatal("log --follow ran")
	}
}

// 06 §9.4: ↑↓ go through the commands that ran, newest first, back to the typed line; the
// history survives a restart and keeps one copy of each line.
func TestCmdLineHistory(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 120, 30)
	runLine(t, m, "version")
	runLine(t, m, "status")
	runLine(t, m, "version")
	typeText(m, ":")
	typeText(m, "draft")
	keyCmd(m, tea.KeyUp)
	if lineText(m) != "version" {
		t.Fatalf("↑: %q", lineText(m))
	}
	keyCmd(m, tea.KeyUp)
	if lineText(m) != "status" {
		t.Fatalf("↑↑: %q", lineText(m))
	}
	keyCmd(m, tea.KeyUp) // the oldest: stays
	keyCmd(m, tea.KeyDown)
	keyCmd(m, tea.KeyDown)
	if lineText(m) != "draft" {
		t.Fatalf("↓ back to the typed line: %q", lineText(m))
	}
	m2 := direct(t, e, 120, 30)
	typeText(m2, ":")
	keyCmd(m2, tea.KeyUp)
	if lineText(m2) != "version" || len(m2.cmdl.hist) != 2 {
		t.Fatalf("after a restart: %q, %q", lineText(m2), m2.cmdl.hist)
	}
}
