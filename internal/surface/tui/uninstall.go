package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 卸载 RimArcho from the command palette (06 §2.14, §9.1): the plan as a panel, enter asks
// for the word "uninstall" (with flags for other answers), then the report; any key quits.

// uninstallText is a plan or report as panel text.
func uninstallText(pg *view.UninstallPage) string {
	var b strings.Builder
	if pg.Blocked != "" {
		fmt.Fprintf(&b, "✗ %s\n\n", Tf("can't uninstall now: %s", pg.Blocked))
	}
	if pg.Result != "" {
		fmt.Fprintf(&b, "%s\n\n", pg.Result)
	}
	for _, s := range pg.Steps {
		line := s.Title + " — " + s.Text
		if s.Size != "" {
			line += "  " + s.Size
		}
		b.WriteString(line + "\n")
		for _, it := range s.Items {
			mark := " "
			switch {
			case it.Err != "":
				mark = "✗"
			case it.Done:
				mark = "✓"
			}
			l := fmt.Sprintf("  %s %s  %s", mark, it.Verb, it.Path)
			if it.To != "" {
				l += " → " + it.To
			}
			if it.Name != "" {
				l += "  " + it.Name
			}
			if it.Size != "" {
				l += "  " + it.Size
			}
			if it.Note != "" {
				l += "  " + it.Note
			}
			if it.Err != "" {
				l += "  " + it.Err
			}
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}

func (m *model) uninstallPanel() tea.Cmd {
	p, err := m.app.PlanUninstall(app.UninstallOptions{})
	if err != nil {
		m.say(sayFail, err.Error())
		return nil
	}
	pg := view.UninstallView(p, false)
	text := uninstallText(pg)
	if pg.Blocked == "" {
		text += "\n" + T("enter: type uninstall to run it with these answers; esc: cancel") + "\n" + T("other answers: add flags, e.g. uninstall --keep-locked --remove-packs") + "\n"
	}
	m.openPanel(T("uninstall RimArcho"), text)
	if pg.Blocked == "" {
		m.panel.onEnter = func(m *model) tea.Cmd { m.uninstallPrompt(); return nil }
	}
	return nil
}

// uninstallPrompt is the typed confirmation: "uninstall" and its flags run it, anything else
// cancels.
func (m *model) uninstallPrompt() {
	m.prompt = &prompt{label: T("type uninstall to run it (anything else cancels): "), submit: func(m *model, text string) tea.Cmd {
		argv := strings.Fields(text)
		if len(argv) > 0 && argv[0] == "archo" {
			argv = argv[1:]
		}
		if len(argv) == 0 || argv[0] != "uninstall" {
			m.say(sayInfo, T("cancelled"))
			return nil
		}
		o, err := cmds.ParseUninstall(argv)
		if err != nil {
			m.say(sayFail, err.Error())
			return nil
		}
		p, err := m.app.Uninstall(o, nil)
		if err != nil {
			m.say(sayFail, m.humanize(err.Error()))
			return nil
		}
		report := uninstallText(view.UninstallView(p, true))
		if !m.app.Uninstalled {
			m.openPanel(T("uninstall RimArcho"), report)
			return nil
		}
		m.openPanel(T("uninstall RimArcho"), report+"\n"+T("enter quits; RimArcho deletes its data and the program once it has closed")+"\n")
		m.panel.onEnter = func(m *model) tea.Cmd { m.quitting = true; return tea.Quit }
		return nil
	}}
}
