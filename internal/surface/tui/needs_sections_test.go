package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// G13 in the TUI (06 §2.4): 能不能跑 shows the four sections in order with their counts,
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
			chip := ic.evDeclared + " " + T("declared by the mod") + " · About.xml modDependencies"
			for _, want := range []string{T(view.SectionTitles[view.SectionBreak]) + " · 1", "Alpha should load after Harmony", chip} {
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

// The running game's errors come after 会让游戏出错, under 上次运行的报错 marked "this run";
// ← → jump between the sections, and the top bar counts them too.
func TestNeedsSectionsLive(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.App.GameProbe = func(string) *app.GameRun {
		return &app.GameRun{PID: 42, Started: e.Now().Add(-10 * time.Minute)}
	}
	m := direct(t, e, 160, 60)
	m.Update(liveTick{})
	press(m, "@")
	s := ansi.Strip(m.View())
	brk, run := strings.Index(s, T(view.SectionTitles[view.SectionBreak])), strings.Index(s, T(view.SectionTitles[view.SectionRun])+" · ")
	if brk < 0 || run < brk || !strings.Contains(s[run:], T("this run")) {
		t.Fatalf("sections out of order:\n%s", s)
	}
	for i := 1; i < len(m.prob.items); i++ {
		if m.prob.items[i].section < m.prob.items[i-1].section {
			t.Fatalf("item %d out of its section: %+v", i, m.prob.items)
		}
	}
	if got, want := m.needCount(), countedNeeds(m); got != want || got <= m.prob.need {
		t.Fatalf("top bar %d, counted items %d (analysis alone %d)", got, want, m.prob.need)
	}
	if !strings.Contains(s, Tf("%d to handle", m.needCount())) {
		t.Fatalf("the head's count is not the top bar's %d:\n%s", m.needCount(), s)
	}
	press(m, "home")
	press(m, "right")
	if it := m.prob.items[m.prob.cursor]; it.section != view.SectionRun || it.group != view.GroupThisRun {
		t.Fatalf("→ went to %+v", it)
	}
	press(m, "left")
	if m.prob.cursor != 0 {
		t.Fatalf("← went to %d", m.prob.cursor)
	}
	// the number keys go to the n-th section shown, whose heading carries the number
	press(m, "2")
	if it := m.prob.items[m.prob.cursor]; it.section != view.SectionRun {
		t.Fatalf("2 went to %+v", it)
	}
	if s := ansi.Strip(m.View()); !strings.Contains(s, "2 "+T(view.SectionTitles[view.SectionRun])) {
		t.Fatalf("no number on the section:\n%s", s)
	}
}

// 06 §3.5: Mod 库存's areas are numbered, 1 the filter, 2 the table, 3 the details.
func TestLibraryAreaKeys(t *testing.T) {
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	press(m, "$")
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
