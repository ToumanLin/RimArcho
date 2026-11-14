package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// While the game runs (or another Game Config's game blocks launching) Start draws no launch
// button, so L does nothing but say why: no launch, and the editor keeps the list it has open
// instead of going back to the current one first.
func TestStartLaunchKeyWhileRunning(t *testing.T) {
	t.Parallel()
	for _, live := range []*view.Live{
		{Running: true, Since: "14:05"},
		{Blocked: Tf("a game is already running (%s)", "second")},
	} {
		e, _ := fixture(t)
		e.MustRun("plan", "new", "second")
		m := direct(t, e, 160, 40)
		envID := m.st.Ix.Env.ID
		cur := e.App.Store.CurrentPlanID(envID)
		key(m, runes("P"))
		for i, c := range m.panel.choices {
			if strings.HasPrefix(c, "second") {
				m.panel.choice = i
			}
		}
		key(m, tea.KeyMsg{Type: tea.KeyEnter})
		if m.launches == "" {
			t.Fatalf("second is not just open: launches %q", m.launches)
		}
		key(m, runes("!"))
		m.live = live
		for _, k := range []tea.KeyMsg{runes("L"), {Type: tea.KeyEnter}} {
			key(m, k)
			if len(e.Launched) != 0 || m.tab != tabHome {
				t.Fatalf("%v launched %v (tab %d)", live, e.Launched, m.tab)
			}
			if m.st.Plan.Name != "second" || m.launches == "" || e.App.Store.CurrentPlanID(envID) != cur {
				t.Fatalf("%v switched the editor: editing %s, launches %q", live, m.st.Plan.Name, m.launches)
			}
			if want := m.launchRefusal(); want == "" || !strings.Contains(m.status, want) {
				t.Fatalf("%v status %q, want %q", live, m.status, want)
			}
		}
		for _, p := range m.homePairs() {
			if p[0] == "L" {
				t.Fatalf("%v key row still offers L", live)
			}
		}
	}
}
