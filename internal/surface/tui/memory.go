package tui

import (
	"context"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/view"
	tea "github.com/charmbracelet/bubbletea"
)

type memoryMeasuredMsg struct{ err error }

func (m *model) startMemoryEstimate(force bool) tea.Cmd {
	if m.prob.memoryReading || m.st == nil {
		return nil
	}
	if c := m.app.CachedMemoryAssessment(m.st); !force && c.Estimated && !c.Stale {
		return nil
	}
	// Capture options on the UI thread; open the worker's DB only when the command
	// actually runs. A discarded command must not leak an open SQLite connection.
	return m.startMemoryEstimateAt(m.st.Plan.ID, m.st.Ix.Env.Name)
}

func (m *model) startMemoryEstimateAt(plan int64, env string) tea.Cmd {
	if m.prob.memoryReading {
		return nil
	}
	r := &app.App{}
	r.ReadOptions(m.app)
	r.WorkspaceSnapshot()
	r.EnvName = env
	path := m.app.Store.Path
	m.prob.memoryReading = true
	return func() tea.Msg {
		db, err := store.OpenSession(path)
		if err != nil {
			return memoryMeasuredMsg{err: err}
		}
		r.Store = db
		defer r.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_, err = r.PlanLoadCost(ctx, plan, nil)
		return memoryMeasuredMsg{err: err}
	}
}

func (m *model) memoryMeasured(msg memoryMeasuredMsg) {
	m.prob.memoryReading = false
	if msg.err != nil {
		m.say(sayFail, msg.err.Error())
		return
	}
	m.loadProblems()
}

func (m *model) refreshMemoryCapacity() {
	if m.st == nil || time.Since(m.prob.memoryAt) < 30*time.Second {
		return
	}
	m.prob.memoryAt = time.Now()
	if m.tab == tabHome {
		if r := m.homeRow(); r != nil {
			r.Memory = m.currentLaunchMemory()
		}
	}
	fresh := view.MemoryNeed(m.app.CachedMemoryAssessment(m.st), m.st.Plan.ID)
	for i, it := range m.prob.base {
		if it.Kind != "memory.capacity" {
			continue
		}
		if it.Counted {
			m.prob.need--
		}
		if fresh.Counted {
			m.prob.need++
		}
		m.prob.base[i] = fresh
		m.composeNeeds()
		return
	}
}

func (m *model) currentLaunchMemory() *view.MemoryLaunch {
	r, err := m.app.ReadSession()
	if err != nil {
		return nil
	}
	defer r.Close()
	r.WorkspaceSnapshot()
	r.OpenPlan, r.PlanName = 0, ""
	st, err := r.Loadout()
	if err != nil {
		return nil
	}
	return view.BuildMemoryLaunch(r, st)
}

func (m *model) confirmMemoryLaunch(c *view.MemoryLaunch) tea.Cmd {
	no, yes := T("cancel"), T("launch anyway")
	m.panel = &panel{title: T("RAM may be insufficient"), lines: []string{
		T("loading RAM needed") + ": " + c.Needed,
		T("RAM available now") + ": " + c.Available,
		T("close other apps or reduce the list before launching."),
	}, choices: []string{no, yes}, onPick: func(m *model, value string) tea.Cmd {
		if value != yes {
			return nil
		}
		fresh := m.currentLaunchMemory()
		if fresh == nil || fresh.Env != c.Env || fresh.Plan != c.Plan || fresh.Token != c.Token {
			m.say(sayFail, T("the list changed; review the updated estimate before launching"))
			return nil
		}
		return m.doListAction(view.ListLaunch)
	}}
	return nil
}
