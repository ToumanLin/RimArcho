package view

import (
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// The same action groups are shown in the GUI dialog and TUI confirmation.
// Reuse the preview's sizes; never walk the filesystem to build this summary.
type UninstallConfirmation struct {
	Changes   []UninstallSummaryRow `json:"changes"`
	Kept      []UninstallSummaryRow `json:"kept"`
	Untouched string                `json:"untouched"`
}

type UninstallSummaryRow struct {
	Title        string   `json:"title"`
	Count        int      `json:"count"`
	Size         string   `json:"size"`
	Method       string   `json:"method"`
	Destinations []string `json:"destinations,omitempty"`
}

func uninstallConfirmation(p *app.UninstallPlan) *UninstallConfirmation {
	c := &UninstallConfirmation{Untouched: T("ordinary mods, Workshop subscriptions and the game's default saves stay; only recorded optimized DDS are removed from mod folders")}
	for _, s := range p.Steps {
		var actions []string
		groups := map[string][]*app.UninstallItem{}
		for _, it := range s.Items {
			if len(groups[it.Do]) == 0 {
				actions = append(actions, it.Do)
			}
			groups[it.Do] = append(groups[it.Do], it)
		}
		for _, action := range actions {
			row := UninstallSummaryRow{Title: UninstallTitle(s.Key), Count: len(groups[action])}
			var size int64
			seen := map[string]bool{}
			for _, it := range groups[action] {
				size += it.Bytes
				if it.To != "" && !seen[it.To] {
					row.Destinations = append(row.Destinations, it.To)
					seen[it.To] = true
				}
			}
			row.Size = render.Size(size)
			if p.EstimatedSizes {
				row.Size = "≈ " + row.Size
			}
			// These plan entries describe metadata operations, not payload byte counts.
			if action == app.DoUnlink || action == app.DoClean || action == app.DoRestore || action == "forget-accounts" {
				row.Size = T("size not measured")
			}
			method := unVerbs[action]
			switch action {
			case app.DoDelete:
				method = "delete directly"
			case app.DoLater:
				method = "delete directly after closing"
			case app.DoUnlink:
				method = "delete links only; targets stay"
			case app.DoClean:
				method = "clean the list; delete RimArcho's backup"
			case app.DoRestore:
				method = "restore the list backup"
			case app.DoMove:
				method = "keep; move out"
			}
			row.Method = T(method)
			if action == app.DoKeep || action == app.DoLeave || action == app.DoMove {
				c.Kept = append(c.Kept, row)
			} else {
				c.Changes = append(c.Changes, row)
			}
		}
	}
	return c
}

// Shared by the native shutdown guard and the interfaces.
func UninstallCloseReason() string { return T("uninstalling; you can close RimArcho when it finishes") }
