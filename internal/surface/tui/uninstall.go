package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// uninstall RimArcho from the command palette (16 §2.14, §9.1): the plan as a panel, enter asks
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
	if c := pg.Confirmation; c != nil {
		for _, group := range []struct {
			title string
			rows  []view.UninstallSummaryRow
		}{{"will remove or change", c.Changes}, {"will keep", c.Kept}} {
			b.WriteString(T(group.title) + "\n")
			for _, row := range group.rows {
				fmt.Fprintf(&b, "  %s — %d  %s  %s\n", row.Title, row.Count, row.Size, row.Method)
				for _, to := range row.Destinations {
					fmt.Fprintf(&b, "    → %s\n", to)
				}
			}
		}
		b.WriteString(c.Untouched + "\n\n")
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

// Confirm the displayed answers; changed flags first show their matching summary.
func (m *model) uninstallPrompt() { m.uninstallPromptFor(app.UninstallOptions{}) }

func (m *model) uninstallPromptFor(shown app.UninstallOptions) {
	m.prompt = &prompt{label: T("type uninstall to run it (anything else cancels): "), submit: func(m *model, text string) tea.Cmd {
		argv := cmdArgv([]rune(text))
		if len(argv) == 0 || argv[0] != "uninstall" {
			m.say(sayInfo, T("cancelled"))
			return nil
		}
		o, err := cmds.ParseUninstall(argv)
		if len(argv) == 1 {
			o = shown
		}
		if err != nil {
			m.say(sayFail, err.Error())
			return nil
		}
		if o != shown {
			p, err := m.app.PlanUninstall(o)
			if err != nil {
				m.say(sayFail, err.Error())
				return nil
			}
			pg := view.UninstallView(p, false)
			m.openPanel(T("uninstall RimArcho"), uninstallText(pg)+"\n"+T("answers changed; review the updated summary before typing uninstall"))
			if pg.Blocked == "" {
				m.panel.onEnter = func(m *model) tea.Cmd { m.uninstallPromptFor(o); return nil }
			}
			return nil
		}
		return m.startUninstall(o)
	}}
}

// The worker owns its session; only messages mutate the screen's model.
type uninstallRun struct {
	ch   chan tea.Msg
	done chan struct{}
}
type uninstallStepMsg struct{ progress app.UninstallProgress }
type uninstallDoneMsg struct {
	page        *view.UninstallPage
	uninstalled bool
	err         error
}

func (r *uninstallRun) next() tea.Cmd { return func() tea.Msg { return <-r.ch } }

func (m *model) startUninstall(o app.UninstallOptions) tea.Cmd {
	return m.startUninstallFor(o, "")
}

func (m *model) startUninstallFor(o app.UninstallOptions, envName string) tea.Cmd {
	if m.uninstallRun != nil {
		return nil
	}
	a, err := m.app.ReadSession()
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return nil
	}
	if envName != "" {
		a.EnvName = envName
	}
	run := &uninstallRun{ch: make(chan tea.Msg, 8), done: make(chan struct{})}
	m.uninstallRun = run
	m.panel, m.pal, m.menu = nil, nil, nil
	m.openPanel(T("uninstall RimArcho"), T("uninstalling…"))
	m.say(sayBusy, T("uninstalling…"))
	go func() {
		defer close(run.done)
		p, err := a.Uninstall(o, func(pr app.UninstallProgress) {
			// Reserve one slot for completion even if the terminal disappears.
			// There is one producer; only the consumer can reduce len(ch).
			if len(run.ch) < cap(run.ch)-1 {
				run.ch <- uninstallStepMsg{pr}
			}
		})
		msg := uninstallDoneMsg{uninstalled: a.Uninstalled, err: err}
		if p != nil {
			msg.page = view.UninstallView(p, true)
			if err != nil {
				msg.page.Failed = max(1, msg.page.Failed)
				msg.page.Result = err.Error()
			}
		}
		a.Close()
		run.ch <- msg
	}()
	return run.next()
}

func (m *model) onUninstall(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case uninstallStepMsg:
		p := msg.progress
		m.openPanel(T("uninstall RimArcho"), fmt.Sprintf("%s %s  %d / %d", T("uninstalling…"), view.UninstallTitle(p.Step), p.I, p.N))
		return m.uninstallRun.next()
	case uninstallDoneMsg:
		m.uninstallRun = nil
		m.app.Uninstalled = msg.uninstalled
		report := ""
		if msg.page != nil {
			report = uninstallText(msg.page)
		}
		if msg.err != nil {
			m.say(sayFail, m.humanize(msg.err.Error()))
			report += "\n" + m.humanize(msg.err.Error())
		} else if msg.page != nil {
			m.say(sayInfo, msg.page.Result)
		}
		if msg.uninstalled {
			report += "\n" + T("enter quits; RimArcho deletes its data and the program once it has closed") + "\n"
		}
		m.openPanel(T("uninstall RimArcho"), report)
		if msg.uninstalled {
			m.panel.onEnter = func(m *model) tea.Cmd { m.quitting = true; return tea.Quit }
		}
	}
	return nil
}
