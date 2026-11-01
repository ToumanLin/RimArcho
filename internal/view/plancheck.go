package view

import (
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// PlanCheck is a plan's state for its home-page row (06 §2.3), from stored results only:
// it never runs an analysis or parses a log (03 §3 "cache per list").
type PlanCheck struct {
	PlanID int64 `json:"planId"`
	// Count is BreakCount of the plan's Needs: the actionable items that break the game.
	Count int `json:"count"`
	// Pending: the plan's enabled set has no stored analysis yet ("not checked yet"); Count
	// then covers the declaration layer only.
	Pending bool `json:"pending,omitempty"`
	// Stale: the stored analysis is older than the plan's list (the Count is the old run's).
	Stale bool `json:"stale,omitempty"`
	// LastRunErrors are the counted errors of the plan's last run (a current log only).
	LastRunErrors int `json:"lastRunErrors,omitempty"`
}

// PlanCheckOf is plan p's PlanCheck.
func PlanCheckOf(a *app.App, p *store.Plan) (PlanCheck, error) {
	ps, err := a.PlanProblems(p)
	if err != nil {
		return PlanCheck{PlanID: p.ID}, err
	}
	n := BuildNeeds(a, ps.State, ps)
	return PlanCheck{PlanID: p.ID, Count: BreakCount(n), Pending: n.Never, Stale: n.Stale, LastRunErrors: LastRunErrors(n)}, nil
}

// PlanChecks is the PlanCheck of every plan of the current environment, in plan order.
func PlanChecks(a *app.App) ([]PlanCheck, error) {
	plans, err := a.Plans()
	if err != nil {
		return nil, err
	}
	out := make([]PlanCheck, 0, len(plans))
	for _, p := range plans {
		c, err := PlanCheckOf(a, p.Plan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}
