package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/logs"
)

// GroupThisRun holds the errors of the running game (06 §2.1); interfaces show it first.
const GroupThisRun = 6

// Live is the running game as the interfaces show it.
type Live struct {
	Running bool   `json:"running"`
	Since   string `json:"since,omitempty"` // HH:MM
	Errors  int    `json:"errors"`
	Items   []Need `json:"items"`
	// Pack is the pack the game plays ("" for the environment's list); Guess: taken from
	// the launch records because the game's command line could not be read.
	Pack  string `json:"pack,omitempty"`
	Guess bool   `json:"guess,omitempty"`
	// Blocked is set while another environment's game runs: launching is refused until it
	// exits (08 §1), so the interfaces disable launch and say why.
	Blocked string `json:"blocked,omitempty"`
	// NewLog is the "analyse this run's log" hint (08 §2 "游戏退出后"): set while no game runs
	// and Player.log is newer than the last launch and the last log the player looked at.
	NewLog string `json:"newLog,omitempty"`
}

// newLog fills l's hint from app.NewLog.
func newLog(a *app.App, st *app.LoadoutState, l *Live) *Live {
	if _, ok := a.NewLog(st); ok {
		l.NewLog = T("analyse this run's log")
	}
	return l
}

// LiveTracker follows the game's log while it runs: start when it starts, stop when it exits
// (the last-run group then takes over from the full analysis).
type LiveTracker struct {
	lv   *app.LiveLog
	pid  int
	what app.RunWhat
}

// Update checks the game and reads what its log gained.
func (t *LiveTracker) Update(a *app.App, st *app.LoadoutState) (*Live, error) {
	// One look at the process table for every environment, the current one first (08 §4).
	g, err := a.RunningGame()
	if err != nil {
		return nil, err
	}
	var run *app.GameRun
	if g != nil && g.Env.ID == st.Ix.Env.ID {
		run = g.Run
	}
	if run == nil {
		if t.lv != nil {
			t.lv.Close(a)
			t.lv, t.pid = nil, 0
		}
		if g != nil {
			return newLog(a, st, &Live{Blocked: Tf("a game is already running (%s)", g.Env.Name)}), nil
		}
		return newLog(a, st, &Live{}), nil
	}
	if t.lv == nil || t.pid != run.PID {
		// a pack run writes its own log on 1.7 (08 §5.3): follow the one this run writes
		t.what = a.WhatRuns(st.Ix.Env, run)
		path := a.RunLog(st, t.what)
		if path == "" {
			return &Live{Running: true, Since: run.Started.In(a.Loc).Format("15:04"), Pack: t.what.Pack, Guess: t.what.Guess}, nil
		}
		t.lv, t.pid = a.FollowLog(path, a.CompanionLog(st)), run.PID
	}
	if _, err := t.lv.Poll(a); err != nil {
		return nil, err
	}
	l := &Live{Running: true, Since: run.Started.In(a.Loc).Format("15:04"), Pack: t.what.Pack, Guess: t.what.Guess}
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
		out = append(out, Need{Group: GroupThisRun, Section: SectionRun, Text: text, Counted: true, Mods: ids, Status: StatusLog,
			Evidence: evidence(EvLog, Tf("Player.log · this run, %d times", c.Count)),
			Actions:  []Action{{Kind: ActList, Key: "enter", Label: T("see the log and evidence"), Title: T("this run"), Lines: lines}}})
	}
	return out
}

// LiveText is the top bar's "game running since 14:05 · 3 errors", naming the pack when a
// pack is played ("" when the game is not running).
func LiveText(l *Live) string {
	if l == nil || !l.Running {
		return ""
	}
	s := Tf("game running since %s", l.Since)
	if l.Pack != "" {
		s = Tf("pack %s running since %s", l.Pack, l.Since)
	}
	if l.Errors > 0 {
		s += " · " + Tf("%d errors", l.Errors)
	}
	return s
}
