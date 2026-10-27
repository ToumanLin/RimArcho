package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// 后台任务 (06 §2.16): the record of app.Tasks as the page shows it. The rows go by group
// (running, waiting, today, yesterday, earlier); a row carries its summary, its actions
// (with their keys) and its items, so the page draws without asking again.

// Task kinds of RimArcho's own jobs (the lane "self"); the files and Steam lanes' kinds
// are Mod 库's ops (LibDelete, LibUnsub, …).
const (
	TaskCreditReplace    = "steam-credit-replace"
	TaskScan             = "scan"
	TaskAccount          = "account-operation"
	TaskSiteDownload     = "site-download"
	TaskAnalyze          = "analyze"
	TaskPack             = "pack"
	TaskTextures         = OpTextures
	TaskTexClean         = OpTexClean
	TaskDownload         = "download"
	TaskSteamCMD         = "install-steamcmd"
	TaskTodds            = "install-todds"
	TaskWezTerm          = "install-wezterm"
	TaskDepRemove        = "remove-dependency"
	TaskData             = "data-update"
	TaskCache            = "cache-clear"
	TaskBackup           = "db-backup"
	TaskVacuum           = "db-vacuum"
	TaskGC               = "gc"
	TaskSubscribe        = "subscribe"
	TaskSourceCheck      = "source-check"
	TaskWorkshopUpdate   = "workshop-update"
	TaskSubscriptionFrom = "subscription-from"
	PageTasks            = "tasks"
	taskKeyStop          = "x"
	taskKeyRetry         = "r"
	taskKeyGoto          = "o"
	taskKeyFolder        = "ctrl+o"
	taskKeyForget        = "Del"
)

// IsSettingTask identifies maintenance tasks shared by settings and the guide.
func IsSettingTask(kind string) bool {
	switch kind {
	case TaskSourceCheck, TaskWorkshopUpdate, TaskSubscribe, TaskSubscriptionFrom, TaskSteamCMD, TaskTodds, TaskWezTerm, TaskDepRemove, TaskData, TaskCache, TaskBackup, TaskVacuum, TaskGC:
		return true
	}
	return false
}

// TaskLane returns the lane a Mod 库 op runs in ("" when it is not a background task:
// plan edits run at once).
func TaskLane(kind string) string {
	switch kind {
	case LibUnsub, LibResub:
		return app.LaneSteam
	case OpSubRemove, OpSubRemoveFast, OpSubCheck, OpSubUpdate, LibDelete, LibMove, LibUnhide, LibPin, LibPinUpdate, LibUnpin:
		return app.LaneFiles
	}
	return ""
}

// TaskPage is the page's data.
type TaskPage struct {
	Rows    []TaskRow `json:"rows"`
	Running int       `json:"running"`
	Queued  int       `json:"queued"`
	Unseen  int       `json:"unseen"` // failures not looked at: the sidebar's red count
	Pct     int       `json:"pct"`    // the running tasks' progress together
	Foot    string    `json:"foot"`   // the sidebar's line ("" none)
}

// TaskRow is one task.
type TaskRow struct {
	ID        string        `json:"id"`
	Kind      string        `json:"kind"`
	Lane      string        `json:"lane"`
	LaneText  string        `json:"laneText"`
	LaneNote  string        `json:"laneNote"`
	Group     string        `json:"group"` // run, queue, today, yesterday, earlier
	State     string        `json:"state"`
	StateText string        `json:"stateText"`
	Title     string        `json:"title"`
	Obj       string        `json:"obj"`
	Time      string        `json:"time"` // when it started ("" while waiting)
	Dur       string        `json:"dur"`
	Line      string        `json:"line"`  // the list's second line: what it does now, or how it ended
	Say       string        `json:"say"`   // the summary's sentence
	Count     string        `json:"count"` // "10 / 14"
	Pct       int           `json:"pct"`
	Bar       bool          `json:"bar"` // a progress bar in the summary
	Steps     []TaskStep    `json:"steps,omitempty"`
	Acts      []TaskAct     `json:"acts"`
	Items     []TaskItemRow `json:"items,omitempty"`
	Counts    string        `json:"counts,omitempty"` // "完成 12  失败 2"
	NoItems   string        `json:"noItems,omitempty"`
	Target    string        `json:"target,omitempty"` // the page o opens
	Seen      bool          `json:"seen"`
	Search    string        `json:"search"` // what the search box matches (lower case)
}

// TaskStatus is the completion sentence shared with the GUI status line.
func TaskStatus(r TaskRow) string {
	if r.Obj == "" {
		return joinFacts(r.Title, r.Line)
	}
	return r.Title + "  " + Tf("%s: %s", r.Obj, r.Line)
}

// TaskStep is a step of a pack build (06 §2.6).
type TaskStep struct {
	Label string `json:"label"`
	State string `json:"state"` // ok, run, wait, fail
}

// TaskAct is an action on a task: the button, the menu entry and the key are one.
type TaskAct struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Key     string `json:"key"`
	Primary bool   `json:"primary,omitempty"`
	Danger  bool   `json:"danger,omitempty"`
}

// TaskItemRow is one item of a task.
type TaskItemRow struct {
	Ref          string `json:"ref"`
	Name         string `json:"name"`
	State        string `json:"state"`
	StateText    string `json:"stateText"`
	Rank         int    `json:"rank"` // the result column's order: failures first
	Why          string `json:"why"`
	Size         string `json:"size"`
	Bytes        int64  `json:"bytes"`
	Got          int64  `json:"got"`
	DownloadSize int64  `json:"downloadSize"` // bytes; size is the existing formatted file-size label
	Permanent    bool   `json:"permanent,omitempty"`
	WorkshopURL  string `json:"workshopURL,omitempty"`
}

// TaskTitle names a kind.
func TaskTitle(kind string) string {
	switch kind {
	case OpSubRemoveFast:
		return T("quick delete (cannot be undone)")
	case TaskCreditReplace:
		return T("replace SteamCMD mods with Steam subscriptions")
	case TaskSourceCheck:
		return T("check for updates")
	case TaskWorkshopUpdate:
		return T("update subscription")
	case TaskAccount:
		return T("account operation")
	case TaskSiteDownload:
		return T("LoversLab download / install")
	case TaskSubscribe:
		return T("subscribe")
	case TaskSubscriptionFrom:
		return T("update subscription")
	case TaskScan:
		return T("scan the mods")
	case TaskAnalyze:
		return T("check whether it runs")
	case TaskPack:
		return T("create a pack")
	case TaskDownload:
		return T("download the game")
	case TaskSteamCMD:
		return T("download SteamCMD")
	case TaskTodds:
		return T("download todds…")
	case TaskWezTerm:
		return T("download WezTerm")
	case TaskDepRemove:
		return T("remove external tool")
	case TaskData:
		return T("update the community data")
	case TaskCache:
		return T("clear cache")
	case TaskBackup:
		return T("back up the database")
	case TaskVacuum:
		return T("compact the database")
	case TaskGC:
		return T("remove old tombstones")
	}
	if l := opLabel(kind); l != "" {
		return l
	}
	return kind
}

func TaskLaneText(l string) (string, string) {
	switch l {
	case app.LaneFiles:
		return T("files"), T("one at a time, in order")
	case app.LaneSteam:
		return "Steam", T("handed to the running Steam client one by one")
	}
	return "RimArcho", T("one of each kind at a time; different kinds can run together")
}

// TaskStateText is a task state in words.
func TaskStateText(s string) string {
	return map[string]string{
		app.TaskQueued: T("queued"), app.TaskRunning: T("running"), app.TaskDone: T("done"),
		app.TaskFailed: T("failed"), app.TaskPartial: T("partly done"), app.TaskStopped: T("stopped"),
	}[s]
}

func itemStateText(s string) string {
	return map[string]string{
		app.ItemWait: T("waiting"), app.ItemRun: T("working on it"), app.ItemOK: T("done"),
		app.ItemFail: T("failed"), app.ItemSkip: T("not touched"),
	}[s]
}

var itemRank = map[string]int{app.ItemFail: 0, app.ItemRun: 1, app.ItemWait: 2, app.ItemSkip: 3, app.ItemOK: 4}

// TaskObj is what a lane task works on, for its row: the mod's name, or "N mods".
func TaskObj(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return Tf("%d mods", len(names))
}

func clock(ms int64, now time.Time) string {
	if ms == 0 {
		return ""
	}
	t := time.UnixMilli(ms).Local()
	n := now.Local()
	y, m, d := n.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, n.Location())
	switch {
	case !t.Before(day):
		return t.Format("15:04")
	case !t.Before(day.AddDate(0, 0, -1)):
		return T("yesterday") + " " + t.Format("15:04")
	}
	return ShortDate(t)
}

func dayGroup(ms int64, now time.Time) string {
	t := time.UnixMilli(ms).Local()
	n := now.Local()
	y, m, d := n.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, n.Location())
	switch {
	case !t.Before(day):
		return "today"
	case !t.Before(day.AddDate(0, 0, -1)):
		return "yesterday"
	}
	return "earlier"
}

// DurText: "12 秒", "3 分 34 秒".
func DurText(d time.Duration) string {
	s := int(d.Round(time.Second) / time.Second)
	if s < 60 {
		return Tf("%d s", s)
	}
	return Tf("%d min %d s", s/60, s%60)
}

// packSteps are the build's steps as its page names them (06 §2.6); packStep maps
// PackProgress.Step to them (the copy leaves out what is not used and locks the versions
// in one go).
var packSteps = []string{"check whether it runs", "copy, leaving out what is not used", "textures", "put it in place"}
var packStep = map[int]int{0: 0, 1: 0, 2: 1, 3: 2, 4: 3, 5: 4}

// BuildTasks makes the page from the record.
func BuildTasks(list []app.Task, now time.Time) *TaskPage {
	p := &TaskPage{}
	var done, total float64
	for i := range list {
		t := &list[i]
		r := taskRow(t, now)
		switch t.State {
		case app.TaskRunning:
			p.Running++
			done += float64(r.Pct)
			total += 100
		case app.TaskQueued:
			p.Queued++
			total += 100
		case app.TaskFailed, app.TaskPartial:
			if !t.Seen {
				p.Unseen++
			}
		}
		p.Rows = append(p.Rows, r)
	}
	if total > 0 {
		p.Pct = int(done * 100 / total)
	}
	if n := p.Running + p.Queued; n > 0 {
		p.Foot = Tf("background: %d", n)
	} else if p.Unseen > 0 {
		p.Foot = T("background tasks")
	}
	// the groups in order; waiting tasks in the order they will run
	order := map[string]int{"run": 0, "queue": 1, "today": 2, "yesterday": 3, "earlier": 4}
	rows := p.Rows
	byQueued := map[string]int64{}
	for _, t := range list {
		byQueued[t.ID] = t.Queued
	}
	sortStable(rows, func(a, b TaskRow) bool {
		if order[a.Group] != order[b.Group] {
			return order[a.Group] < order[b.Group]
		}
		if a.Group == "queue" {
			return byQueued[a.ID] < byQueued[b.ID]
		}
		return byQueued[a.ID] > byQueued[b.ID]
	})
	return p
}

func sortStable(rows []TaskRow, less func(a, b TaskRow) bool) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && less(rows[j], rows[j-1]); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

func taskRow(t *app.Task, now time.Time) TaskRow {
	r := TaskRow{ID: t.ID, Kind: t.Kind, Lane: t.Lane, State: t.State, StateText: TaskStateText(t.State),
		Title: TaskTitle(t.Kind), Obj: t.Obj, Seen: t.Seen}
	r.LaneText, r.LaneNote = TaskLaneText(t.Lane)
	r.Time = clock(t.Start, now)
	switch t.State {
	case app.TaskRunning:
		r.Group = "run"
		r.Dur = DurText(now.Sub(time.UnixMilli(t.Start)))
	case app.TaskQueued:
		r.Group = "queue"
	default:
		r.Group = dayGroup(t.Queued, now)
		if t.Start > 0 && t.End >= t.Start {
			r.Dur = DurText(time.Duration(t.End-t.Start) * time.Millisecond)
		}
	}
	if t.Total > 0 {
		r.Count = fmt.Sprintf("%d / %d", t.Done, t.Total)
		r.Pct = t.Done * 100 / t.Total
	} else if t.Frac > 0 {
		r.Pct = int(t.Frac * 100)
	}
	if (t.Lane == app.LaneSteam || t.Kind == TaskSubscribe) && t.Frac > 0 {
		r.Pct = int(t.Frac * 100)
	}
	if t.State == app.TaskDone {
		r.Pct = 100
	}
	if t.Kind == TaskSubscribe && t.State == app.TaskRunning {
		r.Pct = min(r.Pct, 99)
	}
	c := t.Counts()
	result := Text(t.Result)
	if t.Lane != app.LaneSelf && len(t.Items) > 0 && result == "" {
		switch {
		case t.State == app.TaskDone && len(t.Items) == 1:
			result = T("done")
		case t.State == app.TaskDone:
			result = Tf("all %d done", c[app.ItemOK])
		case t.State == app.TaskStopped && c[app.ItemOK] == 0:
			result = T("nothing was touched")
		case t.State == app.TaskStopped:
			result = Tf("%d done, %d left untouched", c[app.ItemOK], c[app.ItemSkip])
		case c[app.ItemOK] == 0 && len(t.Items) == 1:
			result = t.Items[0].Why
		default:
			result = Tf("%d done, %d failed", c[app.ItemOK], c[app.ItemFail])
		}
	}
	if t.Closed {
		result = joinFacts(T("stopped when the window closed"), result)
	}
	phase := t.Phase
	if phase != "" && t.Lane == app.LaneFiles {
		phase = Tf("on %s", phase)
	} else if phase != "" && t.Lane == app.LaneSteam {
		if strings.HasPrefix(phase, "Steam is downloading ") {
			phase = Tf("Steam is downloading %d of %d", min(t.Done+1, t.Total), t.Total)
		} else {
			phase = Tf("waiting for Steam: %s", phase)
		}
	}
	switch t.State {
	case app.TaskRunning:
		r.Line = phase
		r.Say = joinFacts(T("running"), r.Count, phase)
		r.Bar = true
	case app.TaskQueued:
		r.Line = Tf("waiting in the %s queue for the one before", r.LaneText)
		r.Say = joinFacts(T("queued"), r.Line)
	default:
		r.Line = result
		r.Say = joinFacts(r.StateText, result)
		r.Bar = t.State == app.TaskStopped || t.State == app.TaskPartial || (t.State == app.TaskFailed && t.Total > 1)
	}
	if t.Kind == TaskPack {
		for i, l := range packSteps {
			st := app.ItemWait
			at := packStep[t.Step]
			switch {
			case t.State == app.TaskDone || i < at:
				st = app.ItemOK
			case i == at && t.State == app.TaskRunning:
				st = app.ItemRun
			case i == at:
				st = app.ItemFail
			}
			r.Steps = append(r.Steps, TaskStep{Label: T(l), State: st})
		}
	}
	for _, it := range t.Items {
		row := TaskItemRow{Ref: it.Ref, Name: it.Name, State: it.State, StateText: itemStateText(it.State), Rank: itemRank[it.State], Why: Humanize(nil, it.Why), Bytes: it.Bytes, Got: it.Got, DownloadSize: it.Size}
		row.Permanent = it.Permanent
		if t.Kind == TaskSubscribe && strings.HasPrefix(it.Ref, "ws:") {
			row.WorkshopURL = "https://steamcommunity.com/sharedfiles/filedetails/?id=" + strings.TrimPrefix(it.Ref, "ws:")
			row.Why = ""
			if it.State == app.ItemFail {
				row.Why = subscriptionReason(it, false)
			}
			if it.State == app.ItemSkip {
				row.StateText = T("cancelled")
			}
		}
		if it.State == app.ItemRun && t.Lane == app.LaneSteam && t.Kind == LibResub {
			row.StateText = T("waiting for Steam")
			if it.Size > 0 && it.Got > 0 {
				row.StateText = Tf("downloading %d%%", min(int(float64(it.Got)*100/float64(it.Size)), 100))
			}
		}
		if it.Bytes > 0 {
			row.Size = SizeText(it.Bytes)
		}
		r.Items = append(r.Items, row)
	}
	if len(t.Items) > 0 {
		var parts []string
		for _, s := range []string{app.ItemOK, app.ItemFail, app.ItemRun, app.ItemWait, app.ItemSkip} {
			if c[s] > 0 {
				parts = append(parts, itemStateText(s)+" "+fmt.Sprint(c[s]))
			}
		}
		r.Counts = strings.Join(parts, "  ")
	}
	if t.Kind == TaskSubscribe && len(t.Items) == 0 && t.WorkshopSubscription() {
		var names []string
		for _, arg := range t.Args[2:] {
			if !strings.HasPrefix(arg, "--") {
				names = append(names, "Workshop "+arg)
			}
		}
		r.Title, r.Obj = Tf("subscribe %s", TaskObj(names)), ""
	}
	if t.Kind == TaskSubscribe && len(t.Items) > 0 {
		subscriptionTaskText(t, &r, c)
	}
	r.Target = taskTarget(t)
	if len(t.Items) == 0 {
		r.NoItems = T("this task has no result per item")
	}
	r.Acts = taskActs(t, c)
	var hay []string
	hay = append(hay, r.Title, r.Obj, result, r.LaneText)
	for _, it := range t.Items {
		hay = append(hay, it.Name)
	}
	r.Search = strings.ToLower(strings.Join(hay, " "))
	return r
}

func joinFacts(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "  ")
}

func taskTarget(t *app.Task) string {
	switch t.Kind {
	case TaskSubscribe, TaskSubscriptionFrom, TaskSourceCheck, TaskWorkshopUpdate, TaskSiteDownload, OpSubCheck, OpSubUpdate, OpSubRemove, OpSubRemoveFast:
		return PageChanges
	case TaskAccount:
		return PageSettings
	case TaskSteamCMD, TaskTodds, TaskWezTerm, TaskDepRemove, TaskData, TaskCache, TaskBackup, TaskVacuum, TaskGC:
		return PageSettings
	case TaskScan:
		return PageChanges
	case TaskAnalyze:
		return PageNeeds
	case TaskPack:
		if t.State == app.TaskDone && t.Target != "" {
			return "pack:" + t.Target
		}
		return PagePacks
	case TaskTextures, TaskTexClean:
		return PageTextures
	case TaskDownload:
		return PageEnv
	}
	return PageLibrary
}

// TaskTargetName is the page o opens, for the action's label.
func TaskTargetName(target string) string {
	switch {
	case target == PageChanges:
		return T("subscriptions and updates")
	case target == PageNeeds:
		return T("can it run")
	case target == PagePacks || strings.HasPrefix(target, "pack:"):
		return T("packs")
	case target == PageTextures:
		return T("texture optimisation")
	case target == PageEnv:
		return T("game")
	case target == PageSettings:
		return T("settings")
	}
	return T("mod library")
}

// TaskCanStop says which self tasks stop part way (a pack build, a texture run).
func TaskCanStop(kind string) bool {
	return kind == TaskCreditReplace || kind == TaskPack || kind == TaskTextures || kind == TaskTexClean
}

func taskActs(t *app.Task, c map[string]int) []TaskAct {
	var a []TaskAct
	retry := 0
	for _, it := range t.Items {
		if it.State == app.ItemFail && !it.Permanent {
			retry++
		}
	}
	switch t.State {
	case app.TaskRunning:
		if t.Lane != app.LaneSelf || TaskCanStop(t.Kind) || t.WorkshopSubscription() {
			a = append(a, TaskAct{ID: "stop", Label: T("stop the rest"), Key: taskKeyStop})
			if t.Kind == TaskSubscribe {
				a[len(a)-1].Label = T("cancel")
			}
		}
	case app.TaskQueued:
		if t.Kind != TaskAccount && t.Kind != TaskSiteDownload {
			a = append(a, TaskAct{ID: "stop", Label: T("cancel the wait"), Key: taskKeyStop},
				TaskAct{ID: "up", Label: T("sooner"), Key: "K"}, TaskAct{ID: "down", Label: T("later"), Key: "J"})
		}
	case app.TaskFailed, app.TaskPartial:
		switch {
		case retry > 0 && t.Kind != TaskCreditReplace:
			a = append(a, TaskAct{ID: "retry", Label: Tf("retry %d items", retry), Key: taskKeyRetry, Primary: true})
		case len(t.Items) == 0 && t.Lane == app.LaneSelf && (t.Kind == TaskScan || t.Kind == TaskAnalyze || t.Kind == TaskCreditReplace || IsSettingTask(t.Kind)):
			a = append(a, TaskAct{ID: "retry", Label: T("try again"), Key: taskKeyRetry, Primary: true})
		}
	case app.TaskStopped:
		if t.Kind == TaskCreditReplace {
			a = append(a, TaskAct{ID: "resume", Label: T("continue replacement"), Key: taskKeyRetry, Primary: true})
		}
		if IsSettingTask(t.Kind) && len(t.Items) == 0 {
			a = append(a, TaskAct{ID: "retry", Label: T("try again"), Key: taskKeyRetry, Primary: true})
		}
		if c[app.ItemSkip] > 0 {
			a = append(a, TaskAct{ID: "resume", Label: Tf("do the %d left", c[app.ItemSkip]), Key: taskKeyRetry, Primary: true})
			if t.Kind == TaskSubscribe {
				a[len(a)-1].Label = Tf("continue %d items", c[app.ItemSkip])
			}
		}
	}
	a = append(a, TaskAct{ID: "goto", Label: Tf("go to %s", TaskTargetName(taskTarget(t))), Key: taskKeyGoto})
	if t.Kind == TaskSubscribe && t.Ended() && len(a) == 1 {
		a[0].Primary = true
		a[0].Label = T("go to subscriptions and updates")
	}
	if t.Lane == app.LaneFiles && len(t.Items) == 1 {
		a = append(a, TaskAct{ID: "folder", Label: T("open the folder"), Key: taskKeyFolder})
	}
	if t.Ended() {
		a = append(a, TaskAct{ID: "forget", Label: T("take off the record"), Key: taskKeyForget, Danger: true})
	}
	return a
}

func init() {
	Register(map[string]string{
		"continue replacement":    "继续替换",
		"clear cache":             "清除缓存",
		"back up the database":    "备份数据库",
		"compact the database":    "压缩数据库",
		"scan the mods":           "扫描 Mod",
		"check whether it runs":   "检查能不能跑",
		"download the game":       "下载游戏",
		"files":                   "文件",
		"one at a time, in order": "一次一项，按顺序做",
		"handed to the running Steam client one by one":                "一项一项交给正在运行的 Steam 客户端",
		"one of each kind at a time; different kinds can run together": "同一种一次一个，不同种可以同时",
		"queued":                             "排队中",
		"running":                            "进行中",
		"done":                               "完成",
		"failed":                             "失败",
		"partly done":                        "部分完成",
		"stopped":                            "已停止",
		"waiting for Steam":                  "等 Steam 开始下载",
		"downloading %d%%":                   "下载中 %d%%",
		"Steam is downloading %d of %d":      "Steam 正在下载 %d / %d",
		"working on it":                      "正在做",
		"not touched":                        "没动",
		"yesterday":                          "昨天",
		"%d s":                               "%d 秒",
		"%d min %d s":                        "%d 分 %d 秒",
		"textures":                           "贴图",
		"copy, leaving out what is not used": "复制并去掉用不到的",
		"put it in place":                    "放好",
		"background: %d":                     "后台 %d 项",
		"background tasks":                   "后台任务",
		"all %d done":                        "%d 项都完成了",
		"nothing was touched":                "什么都没动",
		"%d done, %d left untouched":         "完成 %d 项，%d 项没动",
		"%d done, %d failed":                 "%d 个完成，%d 个失败",
		"stopped when the window closed":     "窗口关闭时停止",
		"on %s":                              "正在处理 %s",
		"waiting for Steam: %s":              "等 Steam 处理 %s",
		"waiting in the %s queue for the one before": "排在%s队列，等前一项做完",
		"this task has no result per item":           "这个任务没有逐项的结果",
		"stop the rest":                              "停止后续",
		"cancel the wait":                            "取消排队",
		"sooner":                                     "往前排",
		"later":                                      "往后排",
		"try the %d failed again":                    "重试失败的 %d 项",
		"do the %d left":                             "继续剩下的 %d 项",
		"go to %s":                                   "去%s",
		"take off the record":                        "从记录里去掉",
	})
}

func init() {
	Register(map[string]string{
		"the task's items":                                                "这个任务的每一项",
		"back to the tasks":                                               "回到任务",
		"stop the rest / cancel the wait":                                 "停止后续 / 取消排队",
		"try the failed again / do the ones left":                         "重试失败的 / 继续剩下的",
		"a waiting task later / sooner":                                   "排队的任务往后 / 往前",
		"go to the page it changed":                                       "去它改动的页面",
		"take off the record, put it back":                                "从记录里去掉，放回",
		"take the done ones off the record":                               "清除已完成",
		"the items' headers: sort and columns, the sorted column's width": "每一项的表头：排序和列，排序列的宽度",
		"select tasks":                                                    "选择任务",
		"background tasks; again goes back":                               "后台任务；再按一次回去",
	})
}
