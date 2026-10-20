package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/logs"
)

// GroupThisRun holds the errors of the running game (10 §3.4); interfaces show it first.
const GroupThisRun = 6

// Live is the running game as the interfaces show it.
type Live struct {
	Running bool   `json:"running"`
	Since   string `json:"since,omitempty"` // HH:MM
	Errors  int    `json:"errors"`
	Items   []Need `json:"items"`
}

// LiveTracker follows the game's log while it runs: start when it starts, stop when it exits
// (the last-run group then takes over from the full analysis).
type LiveTracker struct {
	lv  *app.LiveLog
	pid int
}

// Update checks the game and reads what its log gained.
func (t *LiveTracker) Update(a *app.App, st *app.LoadoutState) (*Live, error) {
	run, err := a.GameRunning()
	if err != nil {
		return nil, err
	}
	if run == nil {
		if t.lv != nil {
			t.lv.Close(a)
			t.lv, t.pid = nil, 0
		}
		return &Live{}, nil
	}
	if t.lv == nil || t.pid != run.PID {
		path, _ := a.LogPaths(st)
		if path == "" {
			return &Live{Running: true, Since: run.Started.In(a.Loc).Format("15:04")}, nil
		}
		t.lv, t.pid = a.FollowLog(path, a.CompanionLog(st)), run.PID
	}
	if _, err := t.lv.Poll(a); err != nil {
		return nil, err
	}
	l := &Live{Running: true, Since: run.Started.In(a.Loc).Format("15:04")}
	l.Items = LiveNeeds(st, t.lv.Clusters())
	l.Errors = len(l.Items)
	return l, nil
}

// LiveNeeds turns the running game's clusters into 需要处理 items, newest first.
func LiveNeeds(st *app.LoadoutState, cs []*app.LiveCluster) []Need {
	var out []Need
	for i := len(cs) - 1; i >= 0; i-- {
		c := cs[i]
		if logs.Informational(c.Class) || c.Report == nil {
			continue
		}
		msg := c.Report.First.Message
		if len(msg) > 90 {
			msg = msg[:87] + "…"
		}
		var ids []string
		text := Tf("this run, %d× %s", c.Count, msg)
		lines := []string{c.Report.First.Message}
		for _, at := range c.Report.Attr {
			ids = append(ids, at.Instance)
			lines = append(lines, "· "+TokName(st, finding.Inst(at.Instance))+" — "+T(at.Role)+": "+strings.Join(at.Evidence, "; "))
		}
		if len(c.Report.Attr) > 0 {
			text = Tf("this run, %s: %d× %s", TokName(st, finding.Inst(c.Report.Attr[0].Instance)), c.Count, msg)
		}
		out = append(out, Need{Group: GroupThisRun, Text: text, Counted: true, Mods: ids, Status: StatusLog,
			Actions: []Action{{Kind: ActList, Key: "enter", Label: T("see the log and evidence"), Title: T("this run"), Lines: lines}}})
	}
	return out
}
