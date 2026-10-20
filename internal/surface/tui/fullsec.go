package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// The full-screen detail's sub-sections (06 §6.3): Def, patches, Harmony, rules and the
// full history, loaded when the full screen opens for a mod. 1–5 jump to them.

type fullSec = view.Section

type fullSecs struct {
	id   string
	secs []fullSec
}

type fullSecsMsg struct{ s *fullSecs }

// secCap: lines shown per sub-section; the technical pages (tab) have all of them.
const secCap = 60

// loadFullSecs reads the sub-sections off the key path.
func (m *model) loadFullSecs() tea.Cmd {
	it := m.current()
	if it == nil || it.id == "" || m.st == nil || (m.fsecs != nil && m.fsecs.id == it.id) {
		return nil
	}
	mod := m.st.Ix.ByID[it.id]
	if mod == nil {
		return nil
	}
	a, ix, id := m.app, m.st.Ix, it.id
	m.fsecs = &fullSecs{id: id} // loading
	return func() tea.Msg { return fullSecsMsg{&fullSecs{id: id, secs: view.Sections(a, ix, mod)}} }
}

// secLines lays out the sub-sections at width w; each header line carries its number.
func (m *model) secLines(w int) []dline {
	it := m.current()
	if m.fsecs == nil || it == nil || m.fsecs.id != it.id {
		return nil
	}
	var out []dline
	if m.fsecs.secs == nil {
		return []dline{{text: sDimS.Render(fit(T("loading…"), w))}}
	}
	for i, s := range m.fsecs.secs {
		title := fmt.Sprintf("%d  %s", i+1, s.Title)
		if s.Count >= 0 {
			title += " " + markDot + " " + fmt.Sprint(s.Count)
		}
		out = append(out, dline{text: sBold.Render(fit(title, w)), sec: i + 1})
		for j, l := range s.Lines {
			if j == secCap {
				out = append(out, dline{text: "  " + sDimS.Render(fit(Tf("… %d more · tab shows all", len(s.Lines)-secCap), w-2))})
				break
			}
			ind := min(len(l)-len(strings.TrimLeft(l, " ")), w/2)
			for k, x := range wrap(m.humanize(l), max(w-ind-2, 8)) {
				pad := ind
				if k > 0 {
					pad = ind + 2 // continuation lines hang under the first
				}
				out = append(out, dline{text: strings.Repeat(" ", pad) + sText.Render(fit(x, w-pad))})
			}
		}
		out = append(out, dline{})
	}
	return out
}
