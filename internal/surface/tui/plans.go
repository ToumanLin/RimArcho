package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// Plans in the TUI (04 §1.3, 06 §9.1): the editor is always the current plan's. P (or the
// palette's "plans") opens a chooser: go to another plan, make a new one, rename or delete
// the current one. Every verb is the view function the window calls too.

// planChanged: the board shows another list now; cursors, searches and the selection
// belonged to the old one.
func (m *model) planChanged(text string) tea.Cmd {
	m.tab, m.full, m.insp = tabList, false, nil
	m.clearSel()
	m.reload()
	for k := range m.cols {
		m.cols[k].cursor, m.cols[k].off, m.cols[k].filter = 0, 0, ""
		m.refilterCol(colKind(k), "")
	}
	m.detailRef, m.prob.loaded = "", false
	m.say(sayOK, text)
	return m.requestDetail()
}

// usePlan switches to a plan (no game file is written).
func (m *model) usePlan(id int64) tea.Cmd {
	_, t, err := view.UsePlan(m.app, id)
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return nil
	}
	return m.planChanged(t)
}

// choosePlan is P: every plan (enter goes to it), then new, rename and delete.
func (m *model) choosePlan() tea.Cmd {
	rows, err := view.PlanRows(m.app)
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return nil
	}
	p := &panel{title: T("lists")}
	runs := map[string]func(m *model) tea.Cmd{}
	add := func(label string, run func(m *model) tea.Cmd) {
		p.choices = append(p.choices, label)
		runs[label] = run
	}
	var cur *view.PlanRow
	for i := range rows {
		r := rows[i]
		label := r.Name
		if r.Current {
			label += "  " + T("editing")
			cur = &rows[i]
			p.choice = i
		}
		if r.InGame {
			label += "  " + T("in the game")
		}
		if r.Conflict {
			label += "  " + r.ConflictText
		}
		id := r.ID
		add(label, func(m *model) tea.Cmd {
			if r.Current {
				m.tab, m.full, m.insp = tabList, false, nil
				return nil
			}
			return m.usePlan(id)
		})
	}
	add(T("new list…"), func(m *model) tea.Cmd { return m.newPlan() })
	if cur != nil {
		c := *cur
		add(Tf("rename %s…", c.Name), func(m *model) tea.Cmd { return m.renamePlan(c) })
		add(Tf("delete %s…", c.Name), func(m *model) tea.Cmd { return m.deletePlan(c) })
	}
	p.onPick = func(m *model, v string) tea.Cmd { return runs[v](m) }
	m.panel = p
	return nil
}

// newPlan picks where the list comes from, then asks for the name.
func (m *model) newPlan() tea.Cmd {
	froms, err := view.PlanFroms(m.app)
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return nil
	}
	fromImport := T("from an import: a file, a share code, a link…")
	p := &panel{title: T("new list") + "  " + T("start from")}
	by := map[string]string{}
	for _, f := range froms {
		p.choices = append(p.choices, f.Label)
		by[f.Label] = f.From
	}
	p.choices = append(p.choices, fromImport)
	p.onPick = func(m *model, v string) tea.Cmd {
		if v == fromImport {
			return m.importPrompt() // its preview offers "as a new list"
		}
		from := by[v]
		m.namePrompt(T("new list")+"  ", view.FreePlanName(m.app, ""), 0, func(m *model, name string) tea.Cmd {
			_, t, err := view.NewPlan(m.app, name, from)
			if err != nil {
				m.say(sayFail, m.humanize(err.Error()))
				return nil
			}
			return m.planChanged(t)
		})
		return nil
	}
	m.panel = p
	return nil
}

// importAsPlan is the import preview's third way (04 §9.1).
func (m *model) importAsPlan(imp *view.Import, p *view.ImportPreview) tea.Cmd {
	base := p.Name
	if base == "" {
		base = T(imp.Label)
	}
	m.namePrompt(T("as a new list")+"  ", view.FreePlanName(m.app, base), 0, func(m *model, name string) tea.Cmd {
		_, t, err := view.ImportAsPlan(m.app, imp, name)
		if err != nil {
			m.say(sayFail, m.humanize(err.Error()))
			return nil
		}
		return m.planChanged(t)
	})
	return nil
}

func (m *model) renamePlan(r view.PlanRow) tea.Cmd {
	m.namePrompt(Tf("rename %s", r.Name)+"  ", r.Name, r.ID, func(m *model, name string) tea.Cmd {
		t, err := view.RenamePlan(m.app, r.ID, name)
		if err != nil {
			m.say(sayFail, m.humanize(err.Error()))
			return nil
		}
		m.reload()
		m.say(sayOK, t)
		return nil
	})
	return nil
}

// deletePlan asks first: it cannot be undone (06 §6); the last plan cannot go.
func (m *model) deletePlan(r view.PlanRow) tea.Cmd {
	warn, err := view.RemovePlanWarn(m.app, r.ID)
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return nil
	}
	yes, no := Tf("delete %s", r.Name), T("cancel")
	m.panel = &panel{title: warn, choices: []string{no, yes}, onPick: func(m *model, v string) tea.Cmd {
		if v != yes {
			m.say(sayInfo, T("cancelled"))
			return nil
		}
		t, err := view.RemovePlan(m.app, r.ID)
		if err != nil {
			m.say(sayFail, m.humanize(err.Error()))
			return nil
		}
		return m.planChanged(t)
	}}
	return nil
}

// namePrompt reads a plan name; a taken or empty name says so and asks again.
// self is the plan being renamed (0 for a new one).
func (m *model) namePrompt(label, value string, self int64, done func(m *model, name string) tea.Cmd) {
	m.tab = tabList
	m.prompt = &prompt{label: label + T("name") + ": ", text: value, submit: func(m *model, text string) tea.Cmd {
		if why := view.CheckPlanName(m.app, text, self); why != "" {
			m.say(sayFail, why)
			m.namePrompt(label, text, self, done)
			return nil
		}
		return done(m, text)
	}}
}
