package view

import (
	"fmt"
	"sort"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/logs"
)

// GroupThisRun holds the errors of the running game (16 §2.1); interfaces show it first.
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
	// exits (09 §1), so the interfaces disable launch and say why.
	Blocked string `json:"blocked,omitempty"`
	// NewLog is the "analyse this run's log" hint (09 §2 "after the game exits"): set while no game runs
	// and Player.log is newer than the last launch and the last log the player looked at.
	// The TUI shows it in its status line; the GUI reads that log at once (API.ReadNewLog).
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
	// One look at the process table for every environment, the current one first (09 §4).
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
		// a pack run writes its own log on 1.7 (10 §3): follow the one this run writes
		t.what = a.WhatRuns(st.Ix.Env, run)
		path := a.RunLog(st, t.what)
		if path == "" {
			return &Live{Running: true, Since: run.Started.In(a.Loc).Format("15:04"), Pack: t.what.Pack, Guess: t.what.Guess}, nil
		}
		t.lv, t.pid = a.FollowRun(path, a.RunDataRoot(st, t.what)), run.PID
	}
	if _, err := t.lv.Poll(a); err != nil {
		return nil, err
	}
	l := &Live{Running: true, Since: run.Started.In(a.Loc).Format("15:04"), Pack: t.what.Pack, Guess: t.what.Guess}
	l.Items = LiveNeeds(st, t.lv.Clusters())
	l.Errors = len(l.Items)
	return l, nil
}

// LiveNeeds turns the running game's clusters into Can it run items, newest first.
func LiveNeeds(st *app.LoadoutState, cs []*app.LiveCluster) []Need {
	cs = splitLivePhases(cs)
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
		// The section heading already says "this run", and the text gives the count: the
		// item names only the mod (16 §5).
		var ids []string
		text := fmt.Sprintf("%d× %s", c.Count, msg)
		// A mod only named by the message is not blamed (06 §3).
		owners := c.Report.Owners()
		for _, at := range owners {
			ids = append(ids, at.Instance)
		}
		if len(owners) > 0 {
			text = Tf("%s: %d× %s", TokName(st, finding.Inst(owners[0].Instance)), c.Count, msg)
		}
		ev := evidence(EvLog, "Player.log")
		var texts []string
		for _, at := range c.Report.Attr {
			texts = append(texts, at.Evidence...)
		}
		if files := companionChips(texts); len(files) > 0 {
			ev = evidence(EvLog, files[0])
		} else if c.OnlyCompanion && c.Report.Companion != "" {
			ev = evidence(EvLog, c.Report.Companion)
		}
		n := Need{Group: GroupThisRun, Section: SectionRun, Text: text, Counted: true, Mods: ids, Status: StatusLog,
			Evidence: ev,
			// like the last run's items: the game log at this cluster (the TUI: `archo log`).
			// The live clusters are numbered per batch, not like the page: match the line.
			Actions: []Action{{Kind: ActGameLog, Key: "enter", Label: T("open game log"), Line: lineKey(c.Report.First.Message), Argv: []string{"log"}}}}
		if len(c.Phases) > 0 {
			n.Phase = logs.PhaseName(c.Phases[0].Phase)
			n.PhaseHeading = phaseHeading(n.Phase)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return out
	}
	names := newNeedNames(nil, st, nil)
	for i := range out {
		names.fill(&out[i])
	}
	sort.SliceStable(out, func(i, j int) bool { return phaseRank(out[i].Phase) < phaseRank(out[j].Phase) })
	return out
}

// LiveText is the running game's line, "game running since 14:05", naming the pack when a
// pack is played ("" when the game is not running). The GUI's sidebar foot and the TUI's
// status line show it with this run's errors as an amber number beside it (16 §2.1, §9.1).
func LiveText(l *Live) string {
	if l == nil || !l.Running {
		return ""
	}
	if l.Pack != "" {
		return Tf("pack %s running since %s", l.Pack, l.Since)
	}
	return Tf("game running since %s", l.Since)
}
