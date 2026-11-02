package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func (m *model) queueSourceDecompile(envName string) tea.Cmd {
	ts := m.taskList()
	return m.queueSelf(view.TaskSourceDecompile, m.envName, []string{"source", "decompile", "--active"}, func(a *app.App, id string) (string, error) {
		if envName != "" {
			a.EnvName = envName
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ts.SetStop(id, func() error { cancel(); return nil })
		err := a.DecompileActive(ctx, func(done, total int, file string) {
			ts.Update(id, func(t *app.Task) {
				t.Done, t.Total, t.Phase = done, total, file
				if total > 0 {
					t.Frac = float64(done) / float64(total)
				}
			})
		})
		return view.T("the current list's mods are in the source library"), err
	})
}

// Background tasks (06 §2.16) in the terminal: the subscription actions (add a subscription, check for updates, update,
// remove subscription) queue into app.Tasks — the record and lanes the window uses, <data>/tasks.json —
// and run on their own database session, so the screen never waits for Git, a web page,
// Steam or a folder move. The status line says when a task is queued and how it ended; the
// palette's Background tasks shows the record as text (showTasks), and the window's page acts on it.

// taskEndedMsg: a background task ended.
type taskEndedMsg struct{ t app.Task }

// bgMsg carries what a background read made (a dry run, a page) back to the screen.
type bgMsg struct{ apply func(m *model) tea.Cmd }

type taskRunner struct {
	mu      sync.Mutex
	ts      *app.Tasks
	snap    *app.App // the options and environment a lane item runs with
	ended   chan app.Task
	pending int  // tasks queued here that have not ended (screen goroutine only)
	waiting bool // a waitTask command is out (screen goroutine only)
	// scanning: a scan runs; the files lane waits for it (06 §2.16).
	scanning atomic.Bool
	// runSub runs an archo command on a session (cmds.Execute; tests point Git URLs at a
	// local repository).
	runSub func(a *app.App, argv []string) (string, int)
}

// snapshot is m.app's options with the current environment pinned, for another goroutine.
func (m *model) snapshot() *app.App {
	snap := &app.App{Store: m.app.Store}
	snap.ReadOptions(m.app)
	if e, err := m.app.Env(); err == nil {
		snap.EnvName = e.Name
	}
	return snap
}

// bg runs read on its own session off the screen goroutine (06 §3.1 rule 9) and hands the
// result back as a message.
func (m *model) bg(read func(a *app.App) func(m *model) tea.Cmd) tea.Cmd {
	snap := m.snapshot()
	return func() tea.Msg {
		a, err := snap.ReadSession()
		if err != nil {
			return bgMsg{func(m *model) tea.Cmd { m.say(sayFail, m.humanize(err.Error())); return nil }}
		}
		defer a.Close()
		a.WorkspaceSnapshot()
		return bgMsg{read(a)}
	}
}

// taskList opens the record the first time a task is queued (the window may have written
// it since this TUI started) and refreshes the snapshot lane items run with.
func (m *model) taskList() *app.Tasks {
	r := &m.tasks
	snap := m.snapshot()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.snap = snap
	if r.ts != nil {
		return r.ts
	}
	if r.runSub == nil {
		r.runSub = cmds.Execute
	}
	path := ""
	if m.app.Home != "" {
		path = filepath.Join(m.app.Home, "tasks.json")
	}
	ts := app.OpenTasks(path)
	r.ended = make(chan app.Task, 64)
	ts.Do = r.item
	ts.Batch = r.batch
	ts.Gate = func(lane string) bool { return lane != app.LaneFiles || !r.scanning.Load() }
	ended := r.ended
	ts.Ended = func(t app.Task) {
		select {
		case ended <- t:
		default: // more than 64 unread: the screen catches up from the record
		}
	}
	r.ts = ts
	return ts
}

// session opens a database session for a lane item.
func (r *taskRunner) session() (*app.App, error) {
	r.mu.Lock()
	snap := r.snap
	r.mu.Unlock()
	a, err := snap.ReadSession()
	if err != nil {
		return nil, err
	}
	a.WorkspaceSnapshot()
	return a, nil
}

// item runs one item of a lane task: Mod library's action on one copy (check for updates, update).
func (r *taskRunner) item(kind, ref string) error {
	a, err := r.session()
	if err != nil {
		return err
	}
	defer a.Close()
	st, _ := a.Loadout()
	if _, err := view.LibDo(a, kind, []string{ref}); err != nil {
		return errors.New(view.Humanize(st, err.Error()))
	}
	return nil
}

// batch runs remove subscription as one task: every folder checked, moved, then one scan (the window's
// batchRemoveSubscriptions).
func (r *taskRunner) batch(kind string, refs []string, b app.BatchCalls) bool {
	if kind != view.OpSubRemove {
		return false
	}
	a, err := r.session()
	if err != nil {
		for i := range refs {
			b.Done(i, err)
		}
		return true
	}
	defer a.Close()
	if op, err := a.RemoveSubscriptionsWith(refs, app.RemoveSubscriptionOptions{Workers: 4}, b); op == nil && err != nil {
		for i := range refs {
			b.Done(i, err)
		}
	}
	return true
}

// submitLane queues a files-lane op (check for updates, update, remove subscription) on copies and says so.
func (m *model) submitLane(kind string, refs, names []string) tea.Cmd {
	if len(refs) == 0 {
		return nil
	}
	items := make([]app.TaskItem, len(refs))
	for i, ref := range refs {
		items[i] = app.TaskItem{Ref: ref, Name: names[i]}
	}
	t := m.taskList().Submit(kind, view.TaskLane(kind), view.TaskObj(names), items)
	m.tasks.pending++
	m.say(sayBusy, view.TaskQueuedText(*t))
	return m.waitTask()
}

// queueSelf queues work in the RimArcho lane (one of each kind at a time) on its own
// session, with the environment of the moment it was queued.
func (m *model) queueSelf(kind, obj string, args []string, run func(a *app.App, id string) (string, error)) tea.Cmd {
	ts := m.taskList()
	snap := m.snapshot()
	t := ts.EnqueueSelf(app.Task{Kind: kind, Obj: obj, Args: args, Env: snap.EnvName}, func(id string) (string, error) {
		a, err := snap.ReadSession()
		if err != nil {
			return "", err
		}
		defer a.Close()
		ts.Update(id, func(t *app.Task) { t.Phase = view.TaskPending(kind) })
		text, err := run(a, id)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return "", err
			}
			return "", errors.New(view.Humanize(nil, err.Error()))
		}
		return text, nil
	})
	m.tasks.pending++
	m.say(sayBusy, view.TaskQueuedText(t))
	return m.waitTask()
}

// waitTask delivers the next ended task to the screen while tasks queued here still run.
func (m *model) waitTask() tea.Cmd {
	r := &m.tasks
	if r.ended == nil || r.waiting || r.pending <= 0 {
		return nil
	}
	r.waiting = true
	ch := r.ended
	return func() tea.Msg { return taskEndedMsg{<-ch} }
}

// taskEnded: the status line says how the task ended; the pages read the new state.
func (m *model) taskEnded(t app.Task) tea.Cmd {
	r := &m.tasks
	r.waiting = false
	r.pending--
	m.reload()
	m.libCache = nil
	switch m.tab {
	case tabChanges:
		m.loadChanges()
	case tabLibrary:
		m.loadLibrary()
	}
	kind := sayOK
	switch t.State {
	case app.TaskFailed, app.TaskPartial:
		kind = sayFail
	case app.TaskStopped:
		kind = sayInfo
	}
	if m.prompt == nil {
		m.say(kind, view.TaskStatus(view.BuildTasks([]app.Task{t}, time.Now()).Rows[0]))
	}
	return m.waitTask()
}

// closeTasks marks what still runs or waits as stopped (the TUI closed) and saves the record.
func (m *model) closeTasks() {
	m.tasks.mu.Lock()
	ts := m.tasks.ts
	m.tasks.mu.Unlock()
	if ts != nil {
		ts.Close()
	}
}

// showTasks is Background tasks as text (06 §9.1): the record the window and this TUI share, by
// group, a task a line with how it ended. Stopping and retrying stay in the window.
func (m *model) showTasks() {
	p := view.BuildTasks(m.taskList().List(), time.Now())
	titles := map[string]string{"run": "running", "queue": "queued", "today": "today", "yesterday": "yesterday", "earlier": "earlier"}
	var b strings.Builder
	group := ""
	for _, r := range p.Rows {
		if r.Group != group {
			if group != "" {
				b.WriteString("\n")
			}
			group = r.Group
			b.WriteString(T(titles[group]) + "\n")
		}
		line := "  " + r.StateText + "  " + view.TaskStatus(r)
		if r.Time != "" {
			line += "  " + r.Time
		}
		b.WriteString(line + "\n")
	}
	if len(p.Rows) == 0 {
		b.WriteString(T("nothing has run in the background yet") + "\n")
	}
	m.openPanel(T("background tasks"), b.String())
}
