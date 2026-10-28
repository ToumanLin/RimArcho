package view

import (
	"fmt"
	"sort"
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

func samePhase(a, b *logs.Phase) bool {
	return logs.PhaseName(a) == logs.PhaseName(b) && (a == nil && b == nil || a != nil && b != nil && a.Index == b.Index)
}

// lastRunNeeds preserves the summary wording, with only each phase's observed counts.
func lastRunNeeds(name string, ps []*app.Problem, when string, attributed bool) []Need {
	by := map[string][]*app.Problem{}
	marked := false
	for _, p := range ps {
		var counts []logs.PhaseCount
		if p.Log != nil {
			counts = p.Log.Phases[p.Key]
		}
		if len(counts) == 0 {
			by[""] = append(by[""], p)
			continue
		}
		marked = true
		totals := map[string]int{}
		for _, c := range counts {
			totals[logs.PhaseName(c.Phase)] += c.Count
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
		n.Phase, n.PhaseHeading, n.rank = phase, phaseHeading(phase), phaseRank(phase)
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
