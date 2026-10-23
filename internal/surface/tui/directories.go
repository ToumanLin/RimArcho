package tui

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/view"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) promptDirectories() bool {
	dirs, err := m.app.ManagedDirectories()
	if err != nil {
		m.say(sayFail, err.Error())
		return false
	}
	p := &panel{title: T("some directories need their new location")}
	ids := map[string]string{}
	for _, d := range dirs {
		if view.DirectoryNeedsLocation(d) {
			label := view.DirectoryLabel(d) + " · " + d.Path
			p.choices = append(p.choices, label)
			ids[label] = d.ID
		}
	}
	if len(p.choices) == 0 {
		return false
	}
	p.choices = append(p.choices, T("skip for now"))
	p.onPick = func(m *model, choice string) tea.Cmd {
		if id := ids[choice]; id != "" {
			return m.locateDirectory(id, choice)
		}
		return nil
	}
	m.panel = p
	return true
}

func (m *model) locateDirectory(id, label string) tea.Cmd {
	m.prompt = &prompt{label: label + " · " + T("new directory path") + ": ", submit: func(m *model, text string) tea.Cmd {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		if err := m.app.SetDirectory(id, text); err != nil {
			m.say(sayFail, err.Error())
			return m.locateDirectory(id, label)
		}
		m.reload()
		m.problemCounts()
		m.say(sayOK, T("done"))
		m.promptDirectories()
		return nil
	}}
	return nil
}
