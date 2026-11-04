package tui

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestHintOnlySectionHeading(t *testing.T) {
	t.Parallel()
	m := &model{}
	m.prob.items = []need{{section: view.SectionBreak, status: statusHint}}
	if got := m.sectionHead(view.SectionBreak, 100); !strings.Contains(got, "suggestions") || strings.Contains(got, "breaks the game") {
		t.Fatalf("hint-only section sounds like an error: %q", got)
	}
	m.prob.items = append(m.prob.items, need{section: view.SectionBreak, status: statusWarn, counted: true})
	if got := m.sectionHead(view.SectionBreak, 100); !strings.Contains(got, "breaks the game") || !strings.HasSuffix(got, "  1") {
		t.Fatalf("mixed section lost warning count: %q", got)
	}
}

// worth knowing keeps its name though it holds only hints; what merged into another item and what
// another page handles are not items of their own; the other page is a faint last line
// (06 §2.4).
func TestNeedsMergedAndElsewhere(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 100, 40)
	press(m, "@")
	m.prob.items = m.toNeeds([]view.Need{
		{ID: "sort", Section: view.SectionBreak, Text: "Fix the order", Counted: true, Also: []string{"also meets 2 weaker order preferences"}},
		{ID: "weak", Section: view.SectionBreak, Text: "two weaker order preferences", Into: "sort"},
		{ID: "hid", Section: view.SectionElsewhere, Text: "Hidden One is hidden", Counted: true},
		{ID: "hint", Section: view.SectionHint, Text: "often needs Lib", Status: view.StatusHint},
	})
	m.prob.elsewhere = []view.Elsewhere{{Page: "library", Count: 1, Text: "1 hidden or leftover mod: Mod library"}}
	if len(m.prob.items) != 2 {
		t.Fatalf("items %+v", m.prob.items)
	}
	if got := m.sectionHead(view.SectionHint, 100); !strings.Contains(got, "worth knowing") {
		t.Errorf("hint heading: %q", got)
	}
	s := strings.Join(m.needLines(30), "\n")
	for _, want := range []string{"+ also meets 2 weaker order preferences", "→ 1 hidden or leftover mod"} {
		if !strings.Contains(s, want) {
			t.Errorf("no %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "Hidden One") || strings.Contains(s, "two weaker") {
		t.Errorf("a merged or elsewhere item drawn:\n%s", s)
	}
}
