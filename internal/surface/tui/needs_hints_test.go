package tui

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestHintOnlySectionHeading(t *testing.T) {
	view.SetLang("en")
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
