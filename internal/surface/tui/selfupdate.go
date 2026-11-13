package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// update RimArcho from the command palette or `:update` (17 §5.1, 16 §9.1): a panel with the
// version, what the last check found and the release notes; enter checks, or installs
// once a newer version is known. The install runs off the screen goroutine; the status
// line says its phase (it waits while another RimArcho is open); then the TUI quits and
// Run starts the new archo in the same terminal.

// selfStepMsg: the install reached a phase; selfDoneMsg: it ended.
type selfStepMsg struct{ text string }
type selfDoneMsg struct {
	res *app.SelfUpdateResult
	err error
}

// selfUpdate is the running install: steps arrive on ch, cancel stops it.
type selfUpdate struct {
	ch     chan tea.Msg
	cancel context.CancelFunc
	phase  string
}

func (m *model) selfUpdatePanel() tea.Cmd {
	if run := m.selfRun; run != nil {
		m.openPanel(T("update RimArcho"), run.phase+"\n\n"+T("enter: stop the update (nothing is changed before the files are replaced); esc: keep going")+"\n")
		m.panel.onEnter = func(m *model) tea.Cmd { m.panel = nil; run.cancel(); return nil }
		return nil
	}
	it := view.SelfUpdateItem(m.app)
	var b strings.Builder
	fmt.Fprintf(&b, "RimArcho %s", it.Value)
	if it.ValueNote != "" {
		fmt.Fprintf(&b, "  %s", it.ValueNote)
	}
	b.WriteString("\n")
	if it.Note != "" {
		fmt.Fprintf(&b, "%s\n", it.Note)
	}
	var install, check, page *view.SettingAction
	for i, a := range it.Actions {
		switch {
		case a.URL != "":
			page = &it.Actions[i]
		case a.Pick != "": // the TUI has no file dialog: :update --from below
		case a.Task == view.TaskSelfUpdate:
			install = &it.Actions[i]
		case a.Task == view.TaskSelfCheck:
			check = &it.Actions[i]
		}
	}
	if d := it.Details; d != nil {
		fmt.Fprintf(&b, "\n%s\n", d.Summary)
		if d.Text != "" {
			for _, line := range strings.Split(strings.ReplaceAll(d.Text, "\r\n", "\n"), "\n") {
				fmt.Fprintf(&b, "  %s\n", line)
			}
		}
		if d.Link != "" {
			fmt.Fprintf(&b, "  %s\n", d.Link)
		}
	}
	if page != nil {
		// The manual update (17 §5.1): the zip from the Lanzou share, then :update --from.
		fmt.Fprintf(&b, "\n%s\n  %s  %s\n  :update --from <%s>\n\n%s\n", T("download the zip in the browser, then install it:"),
			page.URL, page.Note, T("path to the zip"), T("o: open the download page"))
	}
	switch {
	case install != nil:
		fmt.Fprintf(&b, "\n%s\n\n%s\n", install.Confirm, T("enter: install; esc: cancel   :update --check checks again"))
	case check != nil:
		fmt.Fprintf(&b, "\n%s\n", T("enter: check for updates; esc: cancel"))
	}
	m.openPanel(T("update RimArcho"), b.String())
	switch {
	case install != nil:
		m.panel.onEnter = func(m *model) tea.Cmd { m.panel = nil; return m.startSelfUpdate() }
	case check != nil:
		m.panel.onEnter = func(m *model) tea.Cmd { m.panel = nil; return m.selfCheck() }
	}
	if page != nil {
		url := page.URL
		m.panel.keys = map[string]func(*model) tea.Cmd{"o": func(m *model) tea.Cmd {
			open := m.opts.OpenURL
			if open == nil {
				open = openURL
			}
			if err := open(url); err != nil {
				m.say(sayFail, err.Error())
			} else {
				m.say(sayOK, T("opened the download page"))
			}
			return nil
		}}
	}
	return nil
}

// selfCheck checks now on its own session and opens the panel again with what it found.
func (m *model) selfCheck() tea.Cmd {
	m.say(sayBusy, T("checking…"))
	return m.bg(func(a *app.App) func(m *model) tea.Cmd {
		ran, err := a.RunAutomaticNow(context.Background(), "self")
		if !ran && err == nil {
			_, _, err = a.CheckSelfUpdate(context.Background())
		}
		return func(m *model) tea.Cmd {
			if err != nil {
				m.say(sayFail, m.humanize(err.Error()))
				return nil
			}
			m.say(sayOK, view.SelfCheckResult(m.app))
			return m.selfUpdatePanel()
		}
	})
}

func (m *model) startSelfUpdate() tea.Cmd {
	return m.startSelfUpdateFrom("")
}

func (m *model) startSelfUpdateFrom(picked string) tea.Cmd {
	snap := m.snapshot()
	ctx, cancel := context.WithCancel(context.Background())
	run := &selfUpdate{ch: make(chan tea.Msg, 8), cancel: cancel, phase: T("downloading…")}
	m.selfRun = run
	go func() {
		a, err := snap.ReadSession()
		if err != nil {
			run.ch <- selfDoneMsg{err: err}
			return
		}
		defer a.Close()
		last := ""
		step := func(s app.SelfUpdateStep) {
			text := view.SelfUpdatePhase(s)
			if text == last {
				return
			}
			last = text
			select {
			case run.ch <- selfStepMsg{text}:
			default: // the screen is behind; the next step says more
			}
		}
		var res *app.SelfUpdateResult
		if picked != "" {
			res, err = a.InstallSelfUpdateFrom(ctx, picked, step)
		} else {
			res, err = a.InstallSelfUpdate(ctx, step)
		}
		run.ch <- selfDoneMsg{res: res, err: err}
	}()
	m.say(sayBusy, T("update RimArcho")+": "+run.phase)
	return run.next()
}

func (r *selfUpdate) next() tea.Cmd { return func() tea.Msg { return <-r.ch } }

// onSelfUpdate handles the install's messages.
func (m *model) onSelfUpdate(msg tea.Msg) tea.Cmd {
	run := m.selfRun
	if run == nil {
		return nil
	}
	switch msg := msg.(type) {
	case selfStepMsg:
		run.phase = msg.text
		m.say(sayBusy, T("update RimArcho")+": "+msg.text)
		return run.next()
	case selfDoneMsg:
		m.selfRun = nil
		run.cancel()
		if msg.err != nil {
			m.say(sayFail, m.humanize(msg.err.Error()))
			return nil
		}
		// Run starts the new archo in this terminal once the screen is closed.
		m.restartExe = app.SelfRestartProgram(msg.res.Program)
		m.quitting = true
		return tea.Quit
	}
	return nil
}
