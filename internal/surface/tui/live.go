package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// The running game (06 §2.1): every two seconds view.LiveTracker checks the game and reads
// what its log gained. While it runs the status line says so (statusLine), and its errors
// lead their section of 能不能跑.

const livePoll = 2 * time.Second

type liveTick struct{}

func liveNext() tea.Cmd { return tea.Tick(livePoll, func(time.Time) tea.Msg { return liveTick{} }) }

// pollLive updates the running game; 需要处理 is rebuilt when its errors changed.
func (m *model) pollLive() {
	m.refreshMemoryCapacity()
	if m.st == nil || m.loadErr != nil {
		return
	}
	l, err := m.liveTr.Update(m.app, m.st)
	if err != nil {
		return // the next poll tries again
	}
	if liveKey(l) != liveKey(m.live) {
		m.live = l
		m.composeNeeds()
		return
	}
	m.live = l
}

func liveKey(l *view.Live) string {
	if l == nil {
		return ""
	}
	k := ""
	for _, it := range l.Items {
		k += it.Text + "\n"
	}
	return k
}
