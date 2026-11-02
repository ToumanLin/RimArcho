package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// G13 in the TUI (06 §2.4): Can it run shows the four sections in order with their counts,
// every item with its evidence chip, and the top bar's number equals the counted items,
// the running game's errors included.
func TestG13NeedsSectionsTUI(t *testing.T) {
	for _, set := range []string{"compat", "nerd"} {
		t.Run(set, func(t *testing.T) {
			SetIcons(set)
			defer SetIcons("compat")
			e, _ := fixture(t)
			m := direct(t, e, 160, 40)
			press(m, "@") // shift+2
			if m.tab != tabProblems {
				t.Fatalf("tab %d", m.tab)
			}
			s := ansi.Strip(m.View())
			chip := ic.evDeclared + " " + T("declared by the mod") + "  About.xml modDependencies"
			for _, want := range []string{T(view.SectionTitles[view.SectionBreak]) + "  1", "Alpha should load after Harmony", chip} {
				if !strings.Contains(s, want) {
					t.Fatalf("no %q:\n%s", want, s)
				}
			}
			for _, it := range m.prob.items {
				if it.evidence != nil && it.evidence.Label == "" {
					t.Errorf("chip without a label: %+v", it)
				}
			}
			if got, want := m.needCount(), countedNeeds(m); got != want {
				t.Fatalf("top bar %d, counted items %d", got, want)
			}
		})
	}
}

func TestNeedsPhaseHeadingsAreNotCursorItems(t *testing.T) {
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	press(m, "@")
	m.prob.items = m.toNeeds([]view.Need{
		{Group: view.GroupLastRun, Section: view.SectionRun, Text: "Alpha — 3 warnings", PhaseHeading: "while loading the game", Counted: true},
		{Group: view.GroupLastRun, Section: view.SectionRun, Text: "Alpha — 2 warnings", PhaseHeading: "while loading a save", Counted: true},
	})
	m.prob.cursor = 0
	s := ansi.Strip(strings.Join(m.needLines(30), "\n"))
	for _, heading := range []string{"while loading the game", "while loading a save"} {
		if !strings.Contains(s, heading) {
			t.Fatal(s)
		}
	}
	press(m, "down")
	if m.prob.cursor != 1 || len(m.prob.items) != 2 {
		t.Fatalf("cursor %d items %d", m.prob.cursor, len(m.prob.items))
	}
}

// The running game's errors come after Will cause errors, under errors from the last run marked "this run";
// ← → jump between the sections, and the top bar counts them too.
func TestNeedsSectionsLive(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	text := e.HarmonyLogFixture()
	// the run before this one: the game moved its log aside when it started
	prev := filepath.Join(e.UserData, "Player-prev.log")
	e.WriteFile(prev, text)
	os.Chtimes(prev, e.Now().Add(-20*time.Minute), e.Now().Add(-20*time.Minute))
	e.App.GameProbe = func(string) *app.GameRun {
		return &app.GameRun{PID: 42, Started: e.Now().Add(-10 * time.Minute)}
	}
	m := direct(t, e, 160, 60)
	m.Update(liveTick{})
	press(m, "@")
	s := ansi.Strip(m.View())
	brk, run := strings.Index(s, T(view.SectionTitles[view.SectionBreak])), strings.Index(s, T("errors this run")+"  ")
	title := s[run:]
	if i := strings.IndexByte(title, '\n'); i >= 0 {
		title = title[:i]
	}
	// the window's title while the game runs, not followed by "this run" again
	if brk < 0 || run < brk || strings.Count(title, T("this run")) != 1 {
		t.Fatalf("sections out of order:\n%s", s)
	}
	both := false
	for i := 1; i < len(m.prob.items); i++ {
		if m.prob.items[i].section < m.prob.items[i-1].section {
			t.Fatalf("item %d out of its section: %+v", i, m.prob.items)
		}
		both = both || m.prob.items[i].group == view.GroupLastRun && m.prob.items[i-1].group == view.GroupThisRun
	}
	// the last run's items after this run's say which run they are, and come from the run
	// before, not again from the log the game writes (VM run 2026-10-28)
	if ps, err := e.App.Problems(false, nil); err != nil || len(ps.Logs) != 1 || ps.Logs[0].Path != prev {
		t.Fatalf("problem log while running: %+v %v", ps.Logs, err)
	}
	label := false
	for _, line := range strings.Split(s[run:], "\n") {
		line = strings.TrimSpace(line)
		label = label || line == T("last run") || strings.HasPrefix(line, T("last run")+"  ")
	}
	if !both || !label {
		t.Fatalf("this run and the last run not told apart (both %v):\n%s", both, s)
	}
	if got, want := m.needCount(), countedNeeds(m); got != want || got <= m.prob.need {
		t.Fatalf("top bar %d, counted items %d (analysis alone %d)", got, want, m.prob.need)
	}
	// the window's lead sentence: what to handle before the start and the run's groups add
	// up to the top bar's number
	before, runs := 0, 0
	for _, it := range m.prob.items {
		switch {
		case it.counted && it.section == view.SectionRun:
			runs++
		case it.counted:
			before++
		}
	}
	if before+runs != m.needCount() || !strings.Contains(s, view.NeedsLead(m.st.Plan.Name, before, runs, false, false)) {
		t.Fatalf("the head's counts %d + %d are not the top bar's %d:\n%s", before, runs, m.needCount(), s)
	}
	press(m, "home")
	press(m, "right")
	if it := m.prob.items[m.prob.cursor]; it.section != view.SectionHint {
		t.Fatalf("first → should reach memory hint: %+v", it)
	}
	press(m, "right")
	if it := m.prob.items[m.prob.cursor]; it.section != view.SectionRun || it.group != view.GroupThisRun {
		t.Fatalf("→ went to %+v", it)
	}
	press(m, "left")
	press(m, "left")
	if m.prob.cursor != 0 {
		t.Fatalf("← went to %d", m.prob.cursor)
	}
	// the number keys go to the n-th section shown, whose heading carries the number
	press(m, "3")
	if it := m.prob.items[m.prob.cursor]; it.section != view.SectionRun {
		t.Fatalf("3 went to %+v", it)
	}
	if s := ansi.Strip(m.View()); !strings.Contains(s, "3 "+T("errors this run")) {
		t.Fatalf("no number on the section:\n%s", s)
	}
}

// 06 §3.5: Mod library's areas are numbered, 1 the filter, 2 the table, 3 the details.
func TestLibraryAreaKeys(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	press(m, "^")
	s := ansi.Strip(strings.Join(m.libraryLines(30), "\n"))
	for _, want := range []string{"1 ", "2 " + Tf("%d mods", m.libr.page.Total), "3 " + T("details")} {
		if !strings.Contains(s, want) {
			t.Fatalf("no %q:\n%s", want, s)
		}
	}
	press(m, "1")
	if !m.libr.bar.open {
		t.Fatal("1 did not open the filter")
	}
	press(m, "esc")
	press(m, "3")
	if !m.libr.detail && !m.libr.full {
		t.Fatal("3 did not focus the details")
	}
	press(m, "2")
	if m.libr.detail || m.libr.full {
		t.Fatal("2 did not go back to the table")
	}
}
