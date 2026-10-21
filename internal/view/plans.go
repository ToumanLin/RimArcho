package view

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Plans in the interfaces (04 §1.3, 06 §2.1, §2.3, §2.5): the rows the sidebar, the home
// page and the TUI's plan chooser show, and the managing verbs both interfaces call —
// new, switch, rename, delete. Switching writes no game file; the game holds what the
// write record says (InGame).

// PagePlanNew is the sidebar's "+ 新方案"; a plan's page is PlanPage(id).
const PagePlanNew = "plan:new"

// PlanPage is the sidebar page ID of a plan: "plan:<id>". IDs, not names, so a page
// survives a rename.
func PlanPage(id int64) string { return "plan:" + strconv.FormatInt(id, 10) }

// PlanOfPage is the plan ID a page ID names (0: not a plan's page).
func PlanOfPage(page string) int64 {
	if !strings.HasPrefix(page, "plan:") {
		return 0
	}
	n, _ := strconv.ParseInt(page[5:], 10, 64)
	return n
}

// PlanRow is one plan of the environment.
type PlanRow struct {
	ID       int64  `json:"id"`
	Page     string `json:"page"`
	Name     string `json:"name"`
	Current  bool   `json:"current,omitempty"`
	Entries  int    `json:"entries"`
	Versions int    `json:"versions"`
	// Played is "played 10 月 20 日" or "never played".
	Played string `json:"played"`
	// Conflict: an external version was not loaded (04 §4.3); ConflictText says so.
	Conflict     bool   `json:"conflict,omitempty"`
	ConflictText string `json:"conflictText,omitempty"`
	// InGame: this plan is what the game's ModsConfig.xml holds now (the write record).
	InGame   bool      `json:"inGame,omitempty"`
	PlayedAt time.Time `json:"-"`
}

// PlanRows lists the current environment's plans in creation order.
func PlanRows(a *app.App) ([]PlanRow, error) {
	ps, err := a.Plans()
	if err != nil {
		return nil, err
	}
	g := InGame(a)
	var out []PlanRow
	for _, p := range ps {
		r := PlanRow{ID: p.ID, Page: PlanPage(p.ID), Name: p.Name, Current: p.Current, Entries: p.Entries, Versions: p.Versions,
			Played: T("never played"), PlayedAt: p.LastPlayed, InGame: g != nil && g.PlanID == p.ID}
		if !p.LastPlayed.IsZero() {
			r.Played = Tf("played %s", ShortDate(p.LastPlayed))
		}
		if ws, err := a.Store.Workspace(p.ID); err == nil && ws != nil && ws.Conflict != 0 {
			r.Conflict, r.ConflictText = true, Tf("an external version was not loaded (v%d)", ws.Conflict)
		}
		out = append(out, r)
	}
	return out, nil
}

// InGameState is what the game's ModsConfig.xml holds, by the write record of the
// environment's user data folder (04 §1.3, §4.3).
type InGameState struct {
	PlanID  int64  `json:"planId"`
	Plan    string `json:"plan"`
	Version int    `json:"version"`
	// Text is the status line's "游戏里现在是：<方案> v<n>".
	Text string `json:"text"`
	// Other: the plan belongs to another environment that shares the folder.
	Other bool `json:"other,omitempty"`
}

// InGame reads the write record; nil when RimArcho has not written the file (or the
// writer is gone).
func InGame(a *app.App) *InGameState {
	e, err := a.Env()
	if err != nil || e.UserDataRoot == "" {
		return nil
	}
	ms, err := a.Store.ModsConfigStateAt(e.UserDataRoot)
	if err != nil || ms.WriterPlan == 0 {
		return nil
	}
	p, err := a.Store.PlanByID(ms.WriterPlan)
	if err != nil || p == nil {
		return nil
	}
	g := &InGameState{PlanID: p.ID, Plan: p.Name, Version: ms.WriterVersion, Other: p.EnvID != e.ID}
	name := p.Name
	if g.Other {
		if pe, err := a.Store.EnvByID(p.EnvID); err == nil && pe != nil {
			name = pe.Name + " · " + p.Name
		}
	}
	if g.Version > 0 {
		g.Text = Tf("in the game now: %s v%d", name, g.Version)
	} else {
		g.Text = Tf("in the game now: %s", name)
	}
	return g
}

// PlanProblems is the number of "会让游戏出错" items of a plan that is not the current
// one, and whether its analysis is known. Until the per-plan analysis cache exists
// (log-launch, 03 §3) no other plan has been checked; that work replaces this hook.
var PlanProblems = func(a *app.App, planID int64) (count int, checked bool) { return 0, false }

// ---- managing (04 §1.3) ----

// PlanFrom is where a new plan's list comes from: "" empty, "current", "v<n>".
type PlanFrom struct {
	From  string `json:"from"`
	Label string `json:"label"`
}

// PlanFroms are the new-plan dialog's starting lists: empty, a copy of the current plan,
// then its versions, newest first (at most 30).
func PlanFroms(a *app.App) ([]PlanFrom, error) {
	p, err := a.CurrentPlan()
	if err != nil {
		return nil, err
	}
	out := []PlanFrom{{From: "", Label: T("an empty list")}, {From: "current", Label: Tf("a copy of %s", p.Name)}}
	vs, err := a.Store.Versions(p.ID)
	if err != nil {
		return out, err
	}
	for i, v := range vs {
		if i == 30 {
			break
		}
		l := Tf("a copy of %s v%d", p.Name, v.N) + " · " + ShortDate(v.Created) + " · " + T(sourceWord(v.Source))
		if n := versionNote(v); n != "" {
			l += " · " + n
		}
		out = append(out, PlanFrom{From: fmt.Sprintf("v%d", v.N), Label: l})
	}
	return out, nil
}

// versionNote is a version's note worth showing next to its source: where an import
// came from, or which plan a copy came from (external and saved versions say no more).
func versionNote(v *store.Version) string {
	if v.Source != "import" || v.Note == "" || v.Note == "empty" {
		return ""
	}
	if r, ok := strings.CutPrefix(v.Note, "copied from "); ok {
		return Tf("copied from %s", r)
	}
	return v.Note
}

func sourceWord(s string) string {
	switch s {
	case "import":
		return "imported"
	case "external":
		return "changed outside RimArcho"
	case "launch":
		return "launched"
	case "save", "user":
		return "written to the game"
	}
	return s
}

// FreePlanName is base, or "base (2)", "base (3)"… when the environment has it.
func FreePlanName(a *app.App, base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = T("new plan")
	}
	ps, _ := a.Plans()
	taken := map[string]bool{}
	for _, p := range ps {
		taken[strings.ToLower(p.Name)] = true
	}
	n := base
	for i := 2; taken[strings.ToLower(n)]; i++ {
		n = fmt.Sprintf("%s (%d)", base, i)
	}
	return n
}

// NewPlan creates a plan (from: "" empty, "current", "v<n>") and switches to it: the
// player makes a plan to edit it. The game's file is not touched.
func NewPlan(a *app.App, name, from string) (*store.Plan, string, error) {
	r, err := a.NewPlan(name, from)
	if err != nil {
		return nil, "", planErr(err)
	}
	return switchToNew(a, r)
}

// ImportAsPlan makes a previewed import a new plan (04 §9.1) and switches to it.
func ImportAsPlan(a *app.App, imp *Import, name string) (*store.Plan, string, error) {
	if imp == nil {
		return nil, "", errors.New(T("nothing to import"))
	}
	r, err := a.NewPlanFromImport(name, imp.List, imp.Label)
	if err != nil {
		return nil, "", planErr(err)
	}
	return switchToNew(a, r)
}

func switchToNew(a *app.App, r *app.NewPlanResult) (*store.Plan, string, error) {
	if _, err := a.UsePlan(r.Plan.Name); err != nil {
		return r.Plan, "", err
	}
	t := Tf("new plan %s: %d mods; the game's list is unchanged until you write or launch", r.Plan.Name, len(r.List))
	if len(r.Unresolved) > 0 {
		t += " · " + Tf("%d not installed", len(r.Unresolved))
	}
	return r.Plan, t, nil
}

// UsePlan makes a plan current; it writes no game file (04 §1.3).
func UsePlan(a *app.App, id int64) (*store.Plan, string, error) {
	p, err := planByID(a, id)
	if err != nil {
		return nil, "", err
	}
	if _, err := a.UsePlan(p.Name); err != nil {
		return nil, "", planErr(err)
	}
	return p, Tf("now editing %s; the game's list is unchanged until you write or launch", p.Name), nil
}

// RenamePlan renames a plan; its history stays.
func RenamePlan(a *app.App, id int64, name string) (string, error) {
	p, err := planByID(a, id)
	if err != nil {
		return "", err
	}
	q, old, err := a.RenamePlan(p.Name, name)
	if err != nil {
		return "", planErr(err)
	}
	return Tf("renamed %s to %s", old, q.Name), nil
}

// RemovePlanWarn is the delete confirmation's text; "" with an error when the plan
// cannot be removed (the environment's last plan).
func RemovePlanWarn(a *app.App, id int64) (string, error) {
	p, err := planByID(a, id)
	if err != nil {
		return "", err
	}
	ps, err := a.Plans()
	if err != nil {
		return "", err
	}
	if len(ps) <= 1 {
		return "", errors.New(Tf("%s is the environment's only plan; it cannot be deleted", p.Name))
	}
	return Tf("delete %s with its history, versions and staging? This cannot be undone. Mods and the game's list stay.", p.Name), nil
}

// RemovePlan deletes a plan (after RemovePlanWarn was confirmed).
func RemovePlan(a *app.App, id int64) (string, error) {
	if _, err := RemovePlanWarn(a, id); err != nil {
		return "", err
	}
	p, _ := planByID(a, id)
	gone, cur, err := a.RemovePlan(p.Name)
	if err != nil {
		return "", planErr(err)
	}
	if cur != nil && gone.ID == id && cur.ID != id {
		return Tf("deleted %s; now editing %s", gone.Name, cur.Name), nil
	}
	return Tf("deleted %s", gone.Name), nil
}

// CheckPlanName says what is wrong with a name the player is typing ("" fine); self is
// the plan being renamed (0 for a new one).
func CheckPlanName(a *app.App, name string, self int64) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return T("a plan needs a name")
	}
	ps, _ := a.Plans()
	for _, p := range ps {
		if p.ID != self && strings.EqualFold(p.Name, n) {
			return Tf("%s already exists", p.Name)
		}
	}
	return ""
}

func planByID(a *app.App, id int64) (*store.Plan, error) {
	ps, err := a.Plans()
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		if p.ID == id {
			return p.Plan, nil
		}
	}
	return nil, errors.New(T("that plan is gone"))
}

// planErr words app's plan errors for the interfaces (no CLI hints).
func planErr(err error) error {
	s := err.Error()
	switch {
	case strings.Contains(s, "already exists"):
		n := strings.TrimPrefix(strings.SplitN(s, " already", 2)[0], "plan:")
		return errors.New(Tf("%s already exists", n))
	case s == "a plan needs a name":
		return errors.New(T(s))
	case strings.Contains(s, "only plan"):
		return errors.New(T("the environment's only plan cannot be deleted"))
	}
	return err
}
