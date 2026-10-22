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
	press(m, "home")
	press(m, "right")
	if it := m.prob.items[m.prob.cursor]; it.section != view.SectionRun || it.group != view.GroupThisRun {
		t.Fatalf("→ went to %+v", it)
	}
	press(m, "left")
	if m.prob.cursor != 0 {
		t.Fatalf("← went to %d", m.prob.cursor)
	}
}
