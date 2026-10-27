package gui

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 后台任务 (06 §2.16): Mod 库's file and Steam actions run in app.Tasks' lanes, one item
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
		if t.Lane == app.LaneSelf && !view.IsSettingTask(t.Kind) && t.Kind != view.TaskAccount && t.Kind != view.TaskSiteDownload && t.Kind != view.TaskCreditReplace {
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

// taskItem runs one item of a lane's task: Mod 库's action on one copy.
func (s *API) taskItem(kind, ref string) error {
	if kind == view.OpSubCheck || kind == view.OpSubUpdate || kind == view.OpSubRemove {
		s.mu.Lock()
		a, err := s.a.ReadSession()
		s.mu.Unlock()
		if err != nil {
			return err
		}
		defer a.Close()
		_, err = view.LibDo(a, kind, []string{ref})
		s.mu.Lock()
		s.board, s.needs, s.lib = nil, nil, nil
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, _ := s.a.Loadout() // for the reasons: a copy's reference becomes its name
	s.board, s.needs, s.lib = nil, nil, nil
	_, err := view.LibDo(s.a, kind, []string{ref})
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

// submitTask queues Mod 库's action kind on refs (the caller holds s.mu).
func (s *API) submitTask(kind string, refs []string) (string, error) {
	st, err := s.a.Loadout()
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
				if r := s.a.SubscriptionOf(m); r != nil && r.Kind == "page" {
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

// Tasks is the 后台任务 page (and the sidebar's line).
func (s *API) Tasks() *view.TaskPage {
	return view.BuildTasks(s.tasks.List(), time.Now())
}

// TasksSeen: the page was opened; the failures shown so far are not news any more.
func (s *API) TasksSeen() {
	s.tasks.MarkSeen()
}

// TaskAct runs a task action (view.TaskAct's ID) on tasks; ref picks one item (重试 one).
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
				nt := s.queueSettingTaskLocked(t.Kind, args, t.Env)
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

// endSelf ends it: done, or failed with the reason.
func (s *API) endSelf(id string, err error, text string) {
	if err != nil {
		s.tasks.Finish(id, app.TaskFailed, view.Humanize(nil, err.Error()))
		return
	}
	s.tasks.Finish(id, app.TaskDone, text)
}

func init() {
	view.Register(map[string]string{
		"%s, %s: in the %s queue":                                  "%s，%s：已排进%s队列",
		"Steam did not confirm it (%s)":                            "Steam 没有确认（%s）",
		"no task":                                                  "没有选中任务",
		"%s cannot be stopped part way":                            "%s 不能中途停止",
		"cancelled %s, %s: nothing was touched":                    "已取消 %s，%s：什么都没动",
		"%s, %s stops after the item it is on; what is done stays": "%s，%s 做完手上这一项就停，做完的留着",
		"stopping %d tasks":                                        "正在停止 %d 个任务",
		"a scan is running":                                        "正在扫描",
		"a check is running":                                       "正在检查",
		"do it again from its page":                                "到它的页面上再做一次",
		"it is the next one already":                               "已经是下一个要做的",
		"it is the last one already":                               "已经排在最后",
		"moved one place sooner":                                   "往前排了一位",
		"moved one place later":                                    "往后排了一位",
		"took %d off the record (no file is touched)":              "从记录里去掉 %d 项（文件不受影响）",
		"no task is done":                                          "没有完成的任务",
		"%s cannot do that now":                                    "%s 现在不能这样做",
		"copied %s":                                                "已复制 %s",
		"copy the name":                                            "复制名字",
		"default columns":                                          "恢复默认列",
		"drag to resize, double click: back to the default":        "拖动改大小，双击恢复",
		"every item":                                               "每一项",
		"leave out":                                                "去掉",
		"narrower: %s":                                             "变窄“%s”",
		"no task matches the filter, esc clears it":                "没有符合筛选的任务，esc 清掉筛选",
		"nothing has run in the background yet":                    "后台还没有做过任何事",
		"pick a task":                                              "选一个任务",
		"only these":                                               "只看这些",
		"pick a value":                                             "选值",
		"queue":                                                    "队列",
		"search tasks and mod names":                               "搜索任务、Mod 名",
		"see it in the mod library":                                "在 Mod 库里看",
		"sort and columns":                                         "排序和列",
		"sorted by %s":                                             "按%s排序",
		"sort by %s":                                               "按%s排序",
		"started":                                                  "开始",
		"tasks":                                                    "任务",
		"took":                                                     "用时",
		"try only this one again":                                  "只重试这一项",
		"what RimArcho does in the background; pick a task to see every item it touched and how it went": "RimArcho 在后台做的事。选一个任务，右边是它碰过的每一项和结果。",
		"wider: %s":                 "加宽“%s”",
		"now":                       "现在",
		"today":                     "今天",
		"earlier":                   "更早",
		"mod":                       "Mod",
		"result":                    "结果",
		"reason":                    "原因",
		"location":                  "位置",
		"state":                     "状态",
		"time":                      "时间",
		"put %d back on the record": "放回记录 %d 项",
	})
}

// fileWorkers is how many folders go to the recycle bin at once. The shell spends most of
// its time per folder (it walks it), not per byte, so several in parallel go much faster;
// memory is not the concern here.
const fileWorkers = 8

// steamChunk is how many items go to the Steam client per call (its worker takes 50).
const steamChunk = 50

// taskBatch runs a whole lane task at once when its kind allows (app.Tasks.Batch): 移到回收站
// and 取消隐藏 check every item, move the folders several at a time and rescan once;
// 退订 and 重新订阅 check every item and hand Steam 50 at a time. The rest (锁定, 移入锁定库…)
// need a scan after each item to know the new copy, and go one by one.
func (s *API) taskBatch(kind string, refs []string, b app.BatchCalls) bool {
	switch kind {
	case view.LibDelete, view.LibUnhide:
		s.batchFiles(kind, refs, b)
	case view.LibUnsub, view.LibResub:
		s.batchSteam(kind, refs, b)
	default:
		return false
	}
	return true
}

func (s *API) batchFiles(kind string, refs []string, b app.BatchCalls) {
	type job struct {
		i        int
		from, to string
	}
	var jobs []job
	s.mu.Lock()
	st, _ := s.a.Loadout()
	for i, ref := range refs {
		if b.Stopped() {
			break
		}
		j := job{i: i}
		var err error
		if kind == view.LibDelete {
			j.from, err = s.a.TrashPlan(ref)
		} else {
			j.from, j.to, err = s.a.UnhidePlan(ref)
		}
		if err != nil {
			b.Done(i, errors.New(view.Humanize(st, err.Error())))
			continue
		}
		jobs = append(jobs, j)
	}
	s.mu.Unlock()
	ch := make(chan job)
	var wg sync.WaitGroup
	var moved atomic.Int32
	for w := 0; w < fileWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runtime.LockOSThread() // the shell's file operation stays on one thread
			defer runtime.UnlockOSThread()
			for j := range ch {
				b.Start(j.i)
				var err error
				if kind == view.LibDelete {
					err = s.a.TrashPath(j.from)
				} else {
					err = os.Rename(j.from, j.to)
				}
				if err == nil {
					moved.Add(1)
				}
				b.Done(j.i, err)
			}
		}()
	}
	for _, j := range jobs {
		if b.Stopped() {
			break // the rest are not touched
		}
		ch <- j
	}
	close(ch)
	wg.Wait()
	if moved.Load() == 0 {
		return
	}
	// one scan for the whole batch, so the library sees what moved
	s.mu.Lock()
	s.a.Scan(false)
	s.board, s.needs, s.lib = nil, nil, nil
	s.mu.Unlock()
}

func (s *API) batchSteam(kind string, refs []string, b app.BatchCalls) {
	action := view.SteamAction(kind)
	var ok []int
	s.mu.Lock()
	st, _ := s.a.Loadout()
	for i, ref := range refs {
		if _, err := s.a.Workshop(action, []string{ref}, true); err != nil {
			b.Done(i, errors.New(view.Humanize(st, err.Error())))
			continue
		}
		ok = append(ok, i)
	}
	a, sessionErr := s.a.ReadSession()
	s.mu.Unlock()
	if sessionErr != nil {
		for _, i := range ok {
			b.Done(i, sessionErr)
		}
		return
	}
	defer a.Close()
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
