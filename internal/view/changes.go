package view

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/modchanges"
)

// Changes (16 §2.9): the page is built by modchanges (shared with the CLI and MCP); the
// interfaces word it in their language.

// ChangeText is the interfaces' wording of the page.
func ChangeText() modchanges.Text {
	// Shared format keys keep CLI punctuation; only the interface removes the separator.
	return modchanges.Text{T: func(s string) string { return T(strings.ReplaceAll(s, string('\u00b7'), "")) }, Date: func(t time.Time) string {
		if Lang() == "zh" {
			return fmt.Sprintf(localeExtra("zh", "@date.short"), int(t.Month()), t.Day())
		}
		return t.Format("Jan 2")
	}}
}

// changeText dates in the app's time zone.
func changeText(a *app.App) modchanges.Text {
	tx := ChangeText()
	d := tx.Date
	tx.Date = func(t time.Time) string { return d(t.In(a.Loc)) }
	return tx
}

// BuildChanges builds the page.
func BuildChanges(a *app.App, st *app.LoadoutState, o modchanges.Opts) (*modchanges.Page, error) {
	return modchanges.Build(a, st, o, changeText(a))
}

// ChangeFiles is the raw file list behind a row.
func ChangeFiles(a *app.App, before, after int64) []string {
	return modchanges.Files(a, changeText(a), before, after)
}

// ChangeNotesURL is a Workshop item's change notes.
func ChangeNotesURL(id string) string {
	return "https://steamcommunity.com/sharedfiles/filedetails/changelog/" + id
}

// ChangeHome is the home page's line about the current plan (16 §2.3).
func ChangeHome(a *app.App, st *app.LoadoutState) *modchanges.Home {
	return modchanges.BuildHome(a, st, changeText(a))
}

// LineageIDs is every instance of the Lineage of the instance id (the instance alone when it
// has none).
func LineageIDs(ix *app.Index, id string) []string {
	m := ix.ByID[id]
	if m == nil {
		return nil
	}
	if m.LineageID == "" || len(ix.ByLineage[m.LineageID]) == 0 {
		return []string{id}
	}
	var out []string
	for _, x := range ix.ByLineage[m.LineageID] {
		out = append(out, x.ID)
	}
	if !slices.Contains(out, id) {
		out = append(out, id)
	}
	return out
}

// KindOrder puts what a player most wants to know first.
func KindOrder(k string) int {
	for i, x := range []string{identity.EvUpdated, identity.EvPackageIDChanged, identity.EvRenamed, identity.EvDiscovered,
		identity.EvDisappeared, identity.EvReappeared, identity.EvDependencies, identity.EvLoadRules, identity.EvVersionSupport,
		identity.EvAuthorChanged, identity.EvMoved, identity.EvCopied} {
		if k == x {
			return i
		}
	}
	return 99
}

// ChangeSummary: "12 mods updated · 1 changed its package id" (the status line after a scan).
func ChangeSummary(counts map[string]int) string {
	var kinds []string
	for k := range counts {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool { return KindOrder(kinds[i]) < KindOrder(kinds[j]) })
	var parts []string
	for _, k := range kinds {
		n := counts[k]
		switch k {
		case identity.EvUpdated:
			parts = append(parts, Tf("%d mods updated", n))
		case identity.EvDiscovered:
			parts = append(parts, Tf("%d new mods", n))
		case identity.EvDisappeared:
			parts = append(parts, Tf("%d mods gone", n))
		case identity.EvPackageIDChanged:
			parts = append(parts, Tf("%d changed their package id", n))
		default:
			parts = append(parts, fmt.Sprintf("%d %s", n, EventText(k)))
		}
	}
	return strings.Join(parts, "  ")
}
