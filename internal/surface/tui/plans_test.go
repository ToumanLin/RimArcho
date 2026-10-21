package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func choicesOf(t *testing.T, m *model) []string {
	t.Helper()
	if m.panel == nil || m.panel.choices == nil {
		t.Fatalf("no chooser open (status %q)", m.status)
	}
	return m.panel.choices
}

// pick moves the open chooser to the choice starting with prefix and takes it.
func pick(t *testing.T, m *model, prefix string) {
	t.Helper()
	cs := choicesOf(t, m)
	for i, c := range cs {
		if strings.HasPrefix(c, prefix) {
			for m.panel.choice < i {
				press(m, "down")
			}
			for m.panel.choice > i {
				press(m, "up")
			}
			press(m, "enter")
			return
		}
	}
	t.Fatalf("no choice %q in %q", prefix, cs)
}

func typeText(m *model, s string) {
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
}

// 06 §9.1, T21/T22: P manages the plans with the view verbs; the palette goes to another
// plan; the editor and the top bar show the current one.
func TestPlansInTUI(t *testing.T) {
	view.SetLang("en")
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	if !strings.Contains(m.topBar(), "plan default") {
		t.Fatalf("top bar %q", m.topBar())
	}
	enabled := len(m.cols[cActive].items)

	// P → new plan… → an empty list → the suggested name.
	press(m, "P")
	if cs := choicesOf(t, m); !strings.HasPrefix(cs[0], "default") || !strings.Contains(cs[0], "editing") {
		t.Fatalf("chooser %q", cs)
	}
	pick(t, m, "new plan…")
	pick(t, m, "an empty list")
	if m.prompt == nil || m.prompt.text != "new plan" {
		t.Fatalf("name prompt %+v", m.prompt)
	}
	press(m, "enter")
	if m.st.Plan.Name != "new plan" || len(m.cols[cActive].items) != 0 || !strings.Contains(m.topBar(), "plan new plan") {
		t.Fatalf("after new: plan %s, %d enabled, status %q", m.st.Plan.Name, len(m.cols[cActive].items), m.status)
	}

	// A taken name asks again.
	press(m, "P")
	pick(t, m, "rename new plan")
	for range "new plan" {
		m.update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	typeText(m, "DEFAULT")
	press(m, "enter")
	if m.prompt == nil || !strings.Contains(m.status, "already exists") {
		t.Fatalf("taken name: prompt %v, status %q", m.prompt, m.status)
	}
	m.update(tea.KeyMsg{Type: tea.KeyEsc})

	// T22: the palette goes to the other plan; its list, staging and history come back.
	m.update(tea.KeyMsg{Type: tea.KeyCtrlK})
	typeText(m, "default")
	if len(m.pal.results) == 0 || m.pal.results[0].kind != palPlan {
		t.Fatalf("palette %+v", m.pal.results)
	}
	press(m, "enter")
	if m.st.Plan.Name != "default" || len(m.cols[cActive].items) != enabled {
		t.Fatalf("after switch: %s with %d", m.st.Plan.Name, len(m.cols[cActive].items))
	}

	// Delete asks first; cancel keeps it, the second choice deletes.
	press(m, "P")
	pick(t, m, "delete default")
	if !strings.Contains(m.panel.title, "cannot be undone") {
		t.Fatalf("warning %q", m.panel.title)
	}
	pick(t, m, "cancel")
	press(m, "P")
	pick(t, m, "delete default")
	pick(t, m, "delete default")
	if m.st.Plan.Name != "new plan" || !strings.HasPrefix(m.status, "deleted default") {
		t.Fatalf("after delete: %s, %q", m.st.Plan.Name, m.status)
	}
	// The last plan cannot go.
	press(m, "P")
	pick(t, m, "delete new plan")
	if m.panel != nil || !strings.Contains(m.status, "only plan") {
		t.Fatalf("last plan: panel %v, status %q", m.panel, m.status)
	}
}
