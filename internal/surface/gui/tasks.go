package gui

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/perftrace"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Background tasks (06 §2.16): Mod library's file and Steam actions run in app.Tasks' lanes, one item
// at a time, each item through the same single-item action as before (view.LibDo); the
// window's own jobs (scan, analysis, packs, textures, downloads) report into the same
// record. "tasks" events say the record changed (at most every 500 ms); a task of a lane
// that ended also carries its result for the status line.

// taskTick is how often "tasks" events go out while tasks change.
const taskTick = 500 * time.Millisecond

type taskEvents struct {
	mu      sync.Mutex
	pending bool
}

// startTasks opens the record in the data folder and wires its lanes to the window.
func (s *API) startTasks() {
	path := ""
	if s.a.Home != "" {
		path = filepath.Join(s.a.Home, "tasks.json")
	}
	ts := app.OpenTasks(path)
	ts.Do = s.taskItem
	ts.Batch = s.taskBatch
	ts.Gate = func(lane string) bool {
		if lane != app.LaneFiles {
			return true
		}
		// a scan reads the folders the files lane moves: one waits for the other (06 §2.16)
		s.mu.Lock()
		defer s.mu.Unlock()
		return !s.busy["scan"]
	}
	ts.Changed = s.tasksChanged
	ts.Ended = func(t app.Task) {
		s.subRevision.Add(1)
		if t.Lane == app.LaneSelf && !view.IsSettingTask(t.Kind) && t.Kind != view.TaskAccount && t.Kind != view.TaskSiteDownload && t.Kind != view.TaskCreditReplace && t.Kind != view.TaskTexClean {
			return // its own event says how it ended
		}
		r := view.BuildTasks([]app.Task{t}, time.Now()).Rows[0]
		ok := t.State == app.TaskDone
		ref := ""
		if ok && (t.Kind == view.TaskSubscribe || t.Kind == view.TaskSubscriptionFrom) {
			ref = t.Target
		}
		s.send("tasks", map[string]any{"ended": map[string]any{"ok": ok, "id": t.ID, "kind": t.Kind, "ref": ref, "text": view.TaskStatus(r)}})
	}
	s.tasks = ts
}

// tasksChanged sends one "tasks" event per tick however many changes came in it.
func (s *API) tasksChanged() {
	s.taskEv.mu.Lock()
	if s.taskEv.pending {
		s.taskEv.mu.Unlock()
		return
	}
	s.taskEv.pending = true
	s.taskEv.mu.Unlock()
	time.AfterFunc(taskTick, func() {
		s.taskEv.mu.Lock()
		s.taskEv.pending = false
		s.taskEv.mu.Unlock()
		s.send("tasks", map[string]any{})
	})
}

// taskItem runs one item of a lane's task: Mod library's action on one copy.
func (s *API) taskItem(kind, ref string) error {
	if view.TaskLane(kind) == app.LaneFiles {
		s.filesMu.RLock()
		defer s.filesMu.RUnlock()
	}
	a, err := s.detachedReader()
	if err != nil {
		return err
	}
	defer a.Close()
	a.WorkspaceSnapshot()
	st, _ := a.Loadout()
	_, err = view.LibDo(a, kind, []string{ref})
	s.invalidateInventory()
	// Steam out of reach: the Workshop page instead, as the action did before (06 §2.9)
	var pages *view.OpenPages
	if errors.As(err, &pages) {
		for _, u := range pages.URLs {
			s.open(u)
		}
		return errors.New(pages.Msg)
	}
	if err != nil {
		return errors.New(view.Humanize(st, err.Error()))
	}
	return nil
}

func (s *API) invalidateInventory() {
	s.mu.Lock()
	s.board, s.needs, s.lib = nil, nil, nil
	s.mu.Unlock()
	s.subRevision.Add(1)
}

// submitTask resolves item names on a detached session before queuing.
func (s *API) submitTask(kind string, refs []string) (string, error) {
	a, err := s.detachedReader()
	if err != nil {
		return "", err
	}
	defer a.Close()
	a.WorkspaceSnapshot()
	st, err := a.Loadout()
	if err != nil {
		return "", err
	}
	var items []app.TaskItem
	var names []string
	skipped := false
	for _, ref := range refs {
		it := app.TaskItem{Ref: ref, Name: ref}
		if m, err := st.Ix.ResolveOne(ref); err == nil {
			if kind == view.OpSubUpdate {
				if r := a.SubscriptionOf(m); r != nil && r.Kind == "page" {
					skipped = true
					continue
				}
			}
			it.Name, it.Bytes = m.DisplayName(), m.TotalSize
		}
		items = append(items, it)
		names = append(names, it.Name)
	}
	lane := view.TaskLane(kind)
	note := ""
	if skipped {
		note = view.T("page items skipped; choose downloaded files to update them")
	}
	if len(items) == 0 {
		return note, nil
	}
	t := s.tasks.Submit(kind, lane, view.TaskObj(names), items)
	ln, _ := view.TaskLaneText(lane)
	return strings.TrimSpace(view.Tf("%s, %s: in the %s queue", view.TaskTitle(kind), t.Obj, ln) + "  " + note), nil
}

// Tasks is the Background tasks page (and the sidebar's line).
func (s *API) Tasks() *view.TaskPage {
	defer perftrace.Span("Tasks")()
	return view.BuildTasks(s.tasks.List(), time.Now())
}

// TasksSeen: the page was opened; the failures shown so far are not news any more.
func (s *API) TasksSeen() {
	s.tasks.MarkSeen()
}

// TaskAct runs a task action (view.TaskAct's ID) on tasks; ref picks one item (retry one).
func (s *API) TaskAct(act string, ids []string, ref string) (string, error) {
	if len(ids) == 0 && act != "clear" && act != "undo" {
		return "", errors.New(view.T("no task"))
	}
	byID := map[string]app.Task{}
	for _, t := range s.tasks.List() {
		byID[t.ID] = t
	}
	switch act {
	case "stop":
		n := 0
		for _, id := range ids {
			t := byID[id]
			if err := s.tasks.Stop(id); err != nil {
				if errors.Is(err, app.ErrCannotStop) {
					return "", errors.New(view.Tf("%s cannot be stopped part way", view.TaskTitle(t.Kind)))
				}
				return "", err
			}
			n++
		}
		if len(ids) == 1 {
			t := byID[ids[0]]
			if t.State == app.TaskRunning && t.WorkshopSubscription() {
				return view.T("cancelling the download; completed items are kept"), nil
			}
			if t.State == app.TaskQueued {
				return view.Tf("cancelled %s, %s: nothing was touched", view.TaskTitle(t.Kind), t.Obj), nil
			}
			return view.Tf("%s, %s stops after the item it is on; what is done stays", view.TaskTitle(t.Kind), t.Obj), nil
		}
		return view.Tf("stopping %d tasks", n), nil
	case "retry", "resume":
		t := byID[ids[0]]
		if t.Lane == app.LaneSelf {
			if view.IsSettingTask(t.Kind) {
				if !t.Ended() {
					return "", errors.New(view.Tf("%s cannot do that now", view.TaskTitle(t.Kind)))
				}
				args := t.Args
				if t.Kind == view.TaskSubscribe && len(args) > 2 && args[1] == "workshop" && len(t.Items) > 0 {
					state := app.ItemFail
					if act == "resume" {
						state = app.ItemSkip
					}
					args = append([]string(nil), args[:2]...)
					for _, it := range t.Items {
						if it.State == state && !it.Permanent && (ref == "" || ref == it.Ref) {
							args = append(args, strings.TrimPrefix(it.Ref, "ws:"))
						}
					}
					if len(args) == 2 {
						return "", errors.New(view.T("nothing to retry"))
					}
					for _, flag := range t.Args[2:] {
						if strings.HasPrefix(flag, "--") {
							args = append(args, flag)
						}
					}
				}
				s.mu.Lock()
				var resume []app.Task
				if act == "resume" && t.WorkshopSubscription() {
					resume = append(resume, t)
				}
				nt := s.queueSettingTaskLocked(t.Kind, args, t.Env, resume...)
				s.mu.Unlock()
				return settingQueued(nt), nil
			}
			switch t.Kind {
			case view.TaskCreditReplace:
				s.mu.Lock()
				id, err := s.startCreditLocked()
				s.mu.Unlock()
				return id, err
			case view.TaskScan:
				if s.Scan() {
					return view.T("scanning…"), nil
				}
				return "", errors.New(view.T("a scan is running"))
			case view.TaskAnalyze:
				if s.Analyze() {
					return view.T("checking the mods' files…"), nil
				}
				return "", errors.New(view.T("a check is running"))
			}
			return "", errors.New(view.T("do it again from its page"))
		}
		state := app.ItemFail
		if act == "resume" {
			state = app.ItemSkip
		}
		var names []string
		for _, it := range t.Items {
			if it.State == state && (ref == "" || it.Ref == ref) {
				names = append(names, it.Name)
			}
		}
		nt, err := s.tasks.Again(t.ID, state, ref, view.TaskObj(names))
		if err != nil {
			return "", err
		}
		ln, _ := view.TaskLaneText(nt.Lane)
		return view.Tf("%s, %s: in the %s queue", view.TaskTitle(nt.Kind), nt.Obj, ln), nil
	case "up", "down":
		d := -1
		if act == "down" {
			d = 1
		}
		if err := s.tasks.Move(ids[0], d); err != nil {
			if act == "up" {
				return "", errors.New(view.T("it is the next one already"))
			}
			return "", errors.New(view.T("it is the last one already"))
		}
		if act == "up" {
			return view.T("moved one place sooner"), nil
		}
		return view.T("moved one place later"), nil
	case "forget":
		n := s.tasks.Forget(ids)
		return view.Tf("took %d off the record (no file is touched)", n), nil
	case "clear":
		n := s.tasks.ClearDone()
		if n == 0 {
			return view.T("no task is done"), nil
		}
		return view.Tf("took %d off the record (no file is touched)", n), nil
	case "undo":
		n := s.tasks.Unforget()
		return view.Tf("put %d back on the record", n), nil
	case "folder":
		t := byID[ids[0]]
		r := ref
		if r == "" && len(t.Items) > 0 {
			r = t.Items[0].Ref
		}
		s.mu.Lock()
		st, err := s.a.Loadout()
		s.mu.Unlock()
		if err != nil {
			return "", err
		}
		m, err := st.Ix.ResolveOne(r)
		if err != nil {
			return "", errors.New(view.T("RimArcho has not seen this copy"))
		}
		return "", s.OpenFolder(m.ID)
	}
	return "", errors.New("unknown action " + act)
}

// beginSelf records one of the window's own jobs.
func (s *API) beginSelf(kind, obj string, stop func() error) string {
	return s.tasks.Begin(kind, obj, stop)
}

// failedText is a job's failure for the status line. It can arrive while the player does something
// else, so it names the job as a task row does (06 §11 A12); the task record keeps the bare reason.
func failedText(kind, obj, reason string) string {
	return view.TaskStatus(view.TaskRow{Title: view.TaskTitle(kind), Obj: obj, Line: reason})
}

// endSelf ends it: done, or failed with the reason.
func (s *API) endSelf(id string, err error, text string) {
	if err != nil {
		s.tasks.Finish(id, app.TaskFailed, view.Humanize(nil, err.Error()))
		return
	}
	s.tasks.Finish(id, app.TaskDone, text)
}

const steamChunk = 50

// taskBatch runs a whole lane task at once when its kind allows (app.Tasks.Batch): move to the recycle bin
// and unhide use the CLI app actions, including link lifecycle and affected game scans;
// unsubscribe and resubscribe check every item and hand Steam 50 at a time. The rest (lock, move into the lock library…)
// need a scan after each item to know the new copy, and go one by one.
func (s *API) taskBatch(kind string, refs []string, b app.BatchCalls) bool {
	if kind == view.OpSubRemove || kind == view.OpSubRemoveFast {
		s.filesMu.RLock()
		defer s.filesMu.RUnlock()
	}
	switch kind {
	case view.LibDelete, view.LibUnhide:
		s.batchFiles(kind, refs, b)
	case view.LibUnsub, view.LibResub:
		s.batchSteam(kind, refs, b)
	case view.OpSubRemove, view.OpSubRemoveFast:
		s.batchRemoveSubscriptions(kind, refs, b)
	default:
		return false
	}
	return true
}

func (s *API) batchFiles(kind string, refs []string, b app.BatchCalls) {
	a, err := s.detachedReader()
	if err != nil {
		for i := range refs {
			b.Done(i, err)
		}
		return
	}
	defer a.Close()
	a.WorkspaceSnapshot()
	st, _ := a.Loadout()
	for i, ref := range refs {
		if b.Stopped() {
			break
		}
		b.Start(i)
		s.filesMu.RLock()
		var err error
		if kind == view.LibDelete {
			_, err = a.Trash(ref, false)
		} else {
			_, err = a.Unhide(ref, false)
		}
		s.filesMu.RUnlock()
		if err != nil {
			err = errors.New(view.Humanize(st, err.Error()))
		}
		b.Done(i, err)
	}
	s.invalidateInventory()
}

func (s *API) batchSteam(kind string, refs []string, b app.BatchCalls) {
	a, sessionErr := s.detachedReader()
	if sessionErr != nil {
		for i := range refs {
			b.Done(i, sessionErr)
		}
		return
	}
	defer a.Close()
	action := view.SteamAction(kind)
	var ok []int
	st, _ := a.Loadout()
	for i, ref := range refs {
		if b.Stopped() {
			return
		}
		if _, err := a.Workshop(action, []string{ref}, true); err != nil {
			b.Done(i, errors.New(view.Humanize(st, err.Error())))
			continue
		}
		ok = append(ok, i)
	}
	opened := false
	for c := 0; c < len(ok); c += steamChunk {
		if b.Stopped() {
			return
		}
		part := ok[c:min(c+steamChunk, len(ok))]
		var chunk []string
		for _, i := range part {
			b.Start(i)
			chunk = append(chunk, refs[i])
		}
		progress := app.BatchCalls{Stopped: b.Stopped,
			Progress: func(k int, got, total int64) {
				if b.Progress != nil {
					b.Progress(part[k], got, total)
				}
			},
			Done: func(k int, err error) { b.Done(part[k], err) },
		}
		op, err := a.Workshop(action, chunk, false, progress)
		s.mu.Lock()
		s.board, s.needs, s.lib = nil, nil, nil
		s.mu.Unlock()
		if b.Stopped() {
			return
		}
		if pages := view.SteamPages(kind, err); pages != nil {
			// Steam out of reach: the Workshop pages instead, as before (06 §2.9); once
			if !opened {
				for _, u := range pages.URLs {
					s.open(u)
				}
				opened = true
			}
			for _, i := range part {
				b.Done(i, errors.New(pages.Msg))
			}
			continue
		}
		if op == nil {
			for _, i := range part {
				b.Done(i, errors.New(view.Humanize(st, err.Error())))
			}
			continue
		}
		for k, i := range part {
			if k < len(op.Items) && op.Items[k].Result != "ok" {
				b.Done(i, errors.New(view.Tf("Steam did not confirm it (%s)", op.Items[k].Result)))
			} else {
				b.Done(i, nil)
			}
		}
	}
}

func (s *API) batchRemoveSubscriptions(kind string, refs []string, b app.BatchCalls) {
	a, err := s.detachedReader()
	if err != nil {
		for i := range refs {
			b.Done(i, err)
		}
		return
	}
	defer a.Close()
	op, err := a.RemoveSubscriptionsWith(refs, app.RemoveSubscriptionOptions{Permanent: kind == view.OpSubRemoveFast, Workers: 4}, b)
	if op == nil && err != nil {
		for i := range refs {
			b.Done(i, err)
		}
	}
	s.invalidateInventory()
}
