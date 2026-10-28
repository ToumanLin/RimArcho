package tui

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/view"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) openVersionWarnings(a view.Action) tea.Cmd {
	data, err := view.ReadVersionWarnings(m.app, a.GameVersion, a.VersionMuted)
	if err != nil {
		m.say(sayFail, err.Error())
		return nil
	}
	p := &panel{title: data.Title}
	var actions []func(*model) tea.Cmd
	add := func(label string, run func(*model) tea.Cmd) {
		p.choices = append(p.choices, label)
		actions = append(actions, run)
	}
	if len(data.Groups) > 0 {
		add(data.All.Label, m.actionKey(data.All, data.Title).run)
	}
	for _, group := range data.Groups {
		add(group.Title+"  "+group.Action.Label, m.actionKey(group.Action, data.Title).run)
		for _, row := range group.Rows {
			add(strings.TrimSpace(row.Name+"  "+row.Supported), func(m *model) tea.Cmd {
				m.panel = &panel{title: row.Name, choices: []string{view.T("show in list"), row.Action.Label},
					onPick: func(m *model, choice string) tea.Cmd {
						if choice == view.T("show in list") {
							return m.jumpTo(row.ID)
						}
						return m.actionKey(row.Action, row.Name).run(m)
					}}
				return nil
			})
		}
	}
	if len(actions) == 0 {
		m.openPanel(data.Title, view.T("no version warnings"))
		return nil
	}
	p.onPick = func(m *model, _ string) tea.Cmd { return actions[p.choice](m) }
	m.panel = p
	return nil
}
