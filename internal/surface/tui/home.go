package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Start (16 §2.3, §9.1) in the terminal, kept simple: the current list (the one launched),
// its game and version, how it stands, the last run, the most likely trouble, and ▶ launch game.
// The data is the window's view.Home for the current list; the rows of the other lists,
// packs and games, the changes line and the picture are the window's only. The editor's L
// comes here first, and L or enter here launches (16 §2.5).

type homeState struct {
	data    *view.Home
	err     error
	loading bool
	seq     int
}

// openHome shows Start and reads its data on a session of its own.
func (m *model) openHome() tea.Cmd {
	m.tab, m.full, m.insp, m.panel = tabHome, false, nil, nil
	h := &m.home
	h.seq++
	h.loading = true
	seq := m.home.seq
	return m.bg(func(a *app.App) func(m *model) tea.Cmd {
		// the page is about the current list, whichever list the editor has open (07 §1.3)
		a.OpenPlan, a.PlanName = 0, ""
		data, err := homeOf(a)
		return func(m *model) tea.Cmd {
			if m.home.seq == seq {
				m.home.data, m.home.err, m.home.loading = data, err, false
				if m.tab == tabHome {
					if row := m.homeRow(); row != nil && row.Memory != nil && (!row.Memory.Assessment.Estimated || row.Memory.Assessment.Stale) {
						return m.startMemoryEstimateAt(row.Plan, row.EnvName)
					}
				}
			}
			return nil
		}
	})
}

// homeOf builds the page from what is stored: no analysis and no log parse is started.
func homeOf(a *app.App) (*view.Home, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, nil
	}
	ps, err := a.Problems(false, nil)
	if err != nil {
		return nil, err
	}
	plans, err := view.PlanRows(a)
	if err != nil {
		return nil, err
	}
	return view.BuildHomeFromState(a, st, view.BuildNeeds(a, st, ps), plans, nil), nil
}

// goLaunch is the editor's L (and the palette's launch game): the list in the editor becomes
// the current one, then Start opens with ▶ launch game under the cursor (16 §2.5).
func (m *model) goLaunch() tea.Cmd {
	if m.launches != "" && m.st != nil && m.st.Plan != nil {
		if _, _, err := view.UsePlan(m.app, m.st.Plan.ID); err != nil {
			m.say(sayFail, m.humanize(err.Error()))
			return nil
		}
		m.reload()
	}
	return m.openHome()
}

// homeRow is the current list's row.
func (m *model) homeRow() *view.HomeRow {
	if m.home.data == nil {
		return nil
	}
	for i := range m.home.data.Rows {
		r := &m.home.data.Rows[i]
		if r.Kind == view.RowPlan && r.Current && !r.OtherEnv {
			return r
		}
	}
	return nil
}

func (m *model) homeKey(s string) tea.Cmd {
	switch s {
	case "esc":
		m.tab = tabList
		return m.requestDetail()
	case "L", "enter":
		if m.launches != "" {
			// the editor shows another list: launching plays the current one, so the
			// editor goes back to it first (what plays is what it shows)
			if cur := m.launchesID; cur != 0 {
				if _, err := view.OpenPlan(m.app, cur); err != nil {
					m.say(sayFail, m.humanize(err.Error()))
					return nil
				}
				m.planChanged("")
				m.tab = tabHome
			}
		}
		cmd := m.listAction(view.ListLaunch)
		if m.panel != nil {
			return cmd
		}
		return tea.Batch(cmd, m.openHome())
	case "w":
		if m.launches != "" || m.unsaved == 0 {
			return nil
		}
		cmd := m.listAction(view.ListSave)
		return tea.Batch(cmd, m.openHome())
	case "P":
		return m.choosePlan()
	}
	return nil
}

// homeLines draws the page in n lines.
func (m *model) homeLines(n int) []string {
	h := &m.home
	var out []string
	add := func(s string) { out = append(out, s) }
	switch {
	case h.err != nil:
		add("  " + sErrS.Render(m.humanize(h.err.Error())))
		return out
	case h.data == nil:
		add("  " + sDimS.Render(T("loading…")))
		return out
	}
	r := m.homeRow()
	if r == nil {
		add("  " + sDimS.Render(T("no game")))
		return out
	}
	add("")
	add("  " + sBold.Render(r.Name))
	add("")
	label := func(s string) string { return sDimS.Render(fit(T(s), 8)) + "  " }
	ver := sText.Render(r.EnvName) // the full version: the chip's short one says less
	if r.GameVersion != "" {
		ver += "  " + sDimS.Render(r.GameVersion)
	}
	add("  " + label("game") + ver)
	state := sText.Render(r.State)
	if r.Level == view.LevelBad {
		state = sErrS.Render(r.State)
	}
	add("  " + label("list") + sText.Render(r.Facts) + "  " + state)
	if d := h.data; d.LastRun != "" {
		errs := T("no errors")
		if d.LastRunErrors > 0 {
			errs = Tf("%d errors", d.LastRunErrors)
		}
		add("  " + label("last game") + sText.Render(d.LastRun) + "  " + sText.Render(errs))
	}
	if l := h.data.Launch; l != nil && l.Text != "" {
		add("  " + sWarnS.Render(Tf("watch for: %s", l.Text)) + "  " + sDimS.Render("shift+2"))
	}
	add("")
	launchLabel, launchStyle := T("launch the game"), sAccent
	if r.Memory != nil {
		switch r.Memory.Assessment.RAMSeverity {
		case "severe":
			launchLabel, launchStyle = T("launch with RAM risk"), sErrS
		case "warning":
			launchLabel, launchStyle = T("launch with limited RAM"), sWarnS
		}
	}
	add("  " + launchStyle.Render(ic.play+" "+launchLabel) + "  " + sText.Render("L"))
	if r.Unsaved > 0 && m.launches == "" {
		add("  " + sDimS.Render(Tf("save %d changes", r.Unsaved)) + "  " + sText.Render("w"))
	}
	return out
}

func (m *model) homePairs() [][2]string {
	pairs := [][2]string{{"L", T("launch")}}
	if r := m.homeRow(); r != nil && r.Unsaved > 0 && m.launches == "" {
		pairs = append(pairs, [2]string{"w", T("save")})
	}
	return append(pairs, [2]string{"P", T("lists")}, [2]string{"@", T("can it run")}, [2]string{"esc", T("back")}, [2]string{"?", T("all keys")})
}
