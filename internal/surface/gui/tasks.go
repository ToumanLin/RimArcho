package gui

import (
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 后台任务 (06 §2.16): Mod 库's file and Steam actions run in app.Tasks' lanes, one item
// at a time, each item through the same single-item action as before (view.LibDo); the
// window's own jobs (scan, analysis, packs, textures, downloads) report into the same
// record. "tasks" events say the record changed (at most every 250 ms); a task of a lane
// that ended also carries its result for the status line.

// taskTick is how often "tasks" events go out while tasks change.
const taskTick = 250 * time.Millisecond

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
		if t.Lane == app.LaneSelf {
			return // its own event says how it ended
		}
		r := view.BuildTasks([]app.Task{t}, time.Now()).Rows[0]
		ok := t.State == app.TaskDone
		s.send("tasks", map[string]any{"ended": map[string]any{"ok": ok, "kind": t.Kind, "text": r.Title + " · " + r.Obj + "：" + r.Line}})
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
	for _, ref := range refs {
		it := app.TaskItem{Ref: ref, Name: ref}
		if m, err := st.Ix.ResolveOne(ref); err == nil {
			it.Name, it.Bytes = m.DisplayName(), m.TotalSize
		}
		items = append(items, it)
		names = append(names, it.Name)
	}
	lane := view.TaskLane(kind)
	t := s.tasks.Submit(kind, lane, view.TaskObj(names), items)
	ln, _ := view.TaskLaneText(lane)
	return view.Tf("%s · %s: in the %s queue", view.TaskTitle(kind), t.Obj, ln), nil
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
			if t.State == app.TaskQueued {
				return view.Tf("cancelled %s · %s: nothing was touched", view.TaskTitle(t.Kind), t.Obj), nil
			}
			return view.Tf("%s · %s stops after the item it is on; what is done stays", view.TaskTitle(t.Kind), t.Obj), nil
		}
		return view.Tf("stopping %d tasks", n), nil
	case "retry", "resume":
		t := byID[ids[0]]
		if t.Lane == app.LaneSelf {
			switch t.Kind {
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
		return view.Tf("%s · %s: in the %s queue", view.TaskTitle(nt.Kind), nt.Obj, ln), nil
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
		"%s · %s: in the %s queue":                                  "%s · %s：已排进%s队列",
		"no task":                                                   "没有选中任务",
		"%s cannot be stopped part way":                             "%s 不能中途停止",
		"cancelled %s · %s: nothing was touched":                    "已取消 %s · %s：什么都没动",
		"%s · %s stops after the item it is on; what is done stays": "%s · %s 做完手上这一项就停，做完的留着",
		"stopping %d tasks":                                         "正在停止 %d 个任务",
		"a scan is running":                                         "正在扫描",
		"a check is running":                                        "正在检查",
		"do it again from its page":                                 "到它的页面上再做一次",
		"it is the next one already":                                "已经是下一个要做的",
		"it is the last one already":                                "已经排在最后",
		"moved one place sooner":                                    "往前排了一位",
		"moved one place later":                                     "往后排了一位",
		"took %d off the record (no file is touched)":               "从记录里去掉 %d 项（文件不受影响）",
		"no task is done":                                           "没有完成的任务",
		"%s cannot do that now":                                     "%s 现在不能这样做",
		"copied %s":                                                 "已复制 %s",
		"copy the name":                                             "复制名字",
		"default columns":                                           "恢复默认列",
		"drag to resize · double click: back to the default":        "拖动改大小 · 双击恢复",
		"every item":                                                "每一项",
		"leave out":                                                 "去掉",
		"narrower: %s":                                              "变窄“%s”",
		"no task matches the filter · esc clears it":                "没有符合筛选的任务 · esc 清掉筛选",
		"nothing has run in the background yet":                     "后台还没有做过任何事",
		"pick a task":                                               "选一个任务",
		"only these":                                                "只看这些",
		"pick a value":                                              "选值",
		"queue":                                                     "队列",
		"search tasks and mod names":                                "搜索任务、Mod 名",
		"see it in the mod library":                                 "在 Mod 库里看",
		"sort and columns":                                          "排序和列",
		"sorted by %s":                                              "按%s排序",
		"sort by %s":                                                "按%s排序",
		"started":                                                   "开始",
		"tasks":                                                     "任务",
		"took":                                                      "用时",
		"try only this one again":                                   "只重试这一项",
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
