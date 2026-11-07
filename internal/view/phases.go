package view

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/logs"
)

var phaseOrder = []string{"loading", "save", "new", "play", "menu", "unknown"}

func phaseRank(name string) int {
	for i, p := range phaseOrder {
		if p == name {
			return i
		}
	}
	return len(phaseOrder)
}

func phaseHeading(name string) string {
	return T(map[string]string{"loading": "while loading the game", "save": "while loading a save", "new": "while starting a new colony", "play": "while playing", "menu": "at the main menu", "unknown": "phase not recorded"}[name])
}

func phaseLabel(p *logs.Phase, loc *time.Location) string {
	name := logs.PhaseName(p)
	label := T(map[string]string{"loading": "loading the game", "save": "loading a save", "new": "starting a new colony", "play": "playing", "menu": "main menu", "unknown": "phase not recorded"}[name])
	if p != nil {
		if name == "save" && p.Save != "" {
			label = Tf("loading %s", p.Save)
		}
		if !p.At.IsZero() {
			label += "  " + p.At.In(loc).Format("15:04")
		}
	}
	return label
}

// The last run's errors are three blocks on Can it run (16 §2.4): starting the game
// (loading and the main menu), loading the save, and playing it (a new colony too).
// Errors whose phase was not recorded are a fourth.
var runBlockOrder = []string{"start", "load", "play", "unknown"}

func runBlock(phase string) string {
	switch phase {
	case "loading", "menu":
		return "start"
	case "save":
		return "load"
	case "new", "play":
		return "play"
	}
	return "unknown"
}

func runBlockRank(block string) int {
	for i, b := range runBlockOrder {
		if b == block {
			return i
		}
	}
	return len(runBlockOrder)
}

// runBlockHeading names a block, with the save the run loaded or played when Archo
// Monitor recorded it (19 §4).
func runBlockHeading(block string, r *app.LogRun) string {
	switch block {
	case "start":
		return T("errors when the game last started")
	case "load":
		names := r.LoadedSaves()
		switch len(names) {
		case 0:
			return T("errors when the last save was loaded")
		case 1:
			return Tf("errors when %s was last loaded", shownSave(names[0]))
		}
		return Tf("errors when %s and %d more saves were last loaded", shownSave(names[len(names)-1]), len(names)-1)
	case "play":
		if n := r.PlayedSave(); n != "" {
			return Tf("errors when %s was last played", shownSave(n))
		}
		return T("errors when the game was last played")
	}
	return T("errors whose phase was not recorded")
}

// shownSave is a save's name as the Saves page shows it: no .rws, no RimSaves folder.
func shownSave(name string) string {
	if !strings.HasSuffix(strings.ToLower(name), ".rws") {
		name += ".rws"
	}
	_, shown := app.SplitSaveName(name)
	return shown
}

func samePhase(a, b *logs.Phase) bool {
	return logs.PhaseName(a) == logs.PhaseName(b) && (a == nil && b == nil || a != nil && b != nil && a.Index == b.Index)
}

// lastRunNeeds preserves the summary wording, with only each block's observed counts
// (runBlock): one item per Mod and block, headed by the block (Phase is the block).
func lastRunNeeds(name string, ps []*app.Problem, when string, attributed bool) []Need {
	by := map[string][]*app.Problem{}
	marked := false
	var run *app.LogRun
	for _, p := range ps {
		var counts []logs.PhaseCount
		if p.Log != nil {
			counts = p.Log.Phases[p.Key]
			run = p.Log
		}
		if len(counts) == 0 {
			by["unknown"] = append(by["unknown"], p)
			continue
		}
		marked = true
		totals := map[string]int{}
		for _, c := range counts {
			totals[runBlock(logs.PhaseName(c.Phase))] += c.Count
		}
		for phase, n := range totals {
			cp, f := *p, *p.Finding
			f.Claim = logCountRe.ReplaceAllString(f.Claim, fmt.Sprintf(" x%d: ", n))
			cp.Finding = &f
			by[phase] = append(by[phase], &cp)
		}
	}
	if !marked {
		return []Need{lastRunNeed(name, ps, when, attributed)}
	}
	var out []Need
	for phase, items := range by {
		n := lastRunNeed(name, items, when, attributed)
		n.Phase, n.PhaseHeading, n.rank = phase, runBlockHeading(phase, run), runBlockRank(phase)
		out = append(out, n)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].rank < out[j].rank })
	return out
}

func splitLivePhases(cs []*app.LiveCluster) []*app.LiveCluster {
	var out []*app.LiveCluster
	for _, c := range cs {
		if len(c.Phases) == 0 {
			out = append(out, c)
			continue
		}
		totals := map[string]int{}
		for _, pc := range c.Phases {
			totals[logs.PhaseName(pc.Phase)] += pc.Count
		}
		for _, phase := range phaseOrder {
			if n := totals[phase]; n > 0 {
				cp := *c
				cp.Count = n
				cp.Phases = []logs.PhaseCount{{Phase: &logs.Phase{Name: phase}, Count: n}}
				out = append(out, &cp)
			}
		}
	}
	return out
}
