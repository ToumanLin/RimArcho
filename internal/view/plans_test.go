package view_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func planNames(t *testing.T, rows []view.PlanRow) string {
	t.Helper()
	var out []string
	for _, r := range rows {
		n := r.Name
		if r.Current {
			n += "*"
		}
		out = append(out, n)
	}
	return strings.Join(out, ",")
}

type appT = app.App

func itoa(n int) string { return strconv.Itoa(n) }

func mustPlans(t *testing.T, a *app.App) []view.PlanRow {
	t.Helper()
	rows, err := view.PlanRows(a)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// 04 §1.3 through the view verbs both interfaces call: new (empty, a copy, a version, an
// import) switches to the new plan; rename keeps the history; the last plan cannot go.
func TestPlanVerbs(t *testing.T) {
	e, _, _ := shellEnv(t)
	a := e.App
	rows := mustPlans(t, a)
	if planNames(t, rows) != "default*" || rows[0].Page != view.PlanPage(rows[0].ID) || view.PlanOfPage(rows[0].Page) != rows[0].ID {
		t.Fatalf("rows %+v", rows)
	}
	def := rows[0].ID
	if _, err := view.RemovePlanWarn(a, def); err == nil {
		t.Fatal("the only plan may not be deleted")
	}

	froms, err := view.PlanFroms(a)
	if err != nil || len(froms) < 3 || froms[0].From != "" || froms[1].From != "current" || froms[2].From != "v1" {
		t.Fatalf("froms %+v %v", froms, err)
	}

	p, text, err := view.NewPlan(a, "Copy", "current")
	if err != nil || !strings.Contains(text, "new plan Copy: 2 mods") {
		t.Fatalf("new: %v %q", err, text)
	}
	if cur, _ := a.CurrentPlan(); cur.ID != p.ID {
		t.Fatalf("the new plan is not current: %+v", cur)
	}
	if _, _, err := view.NewPlan(a, "copy", ""); err == nil || err.Error() != "Copy already exists" {
		t.Fatalf("a taken name (any case): %v", err)
	}
	if got := view.CheckPlanName(a, " COPY ", 0); got != "Copy already exists" {
		t.Fatalf("check %q", got)
	}
	if got := view.CheckPlanName(a, "copy", p.ID); got != "" {
		t.Fatalf("renaming to itself: %q", got)
	}
	if got := view.FreePlanName(a, "Copy"); got != "Copy (2)" {
		t.Fatalf("free name %q", got)
	}
	if _, _, err := view.NewPlan(a, "Empty", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := view.NewPlan(a, "Old", "v1"); err != nil {
		t.Fatal(err)
	}
	rows = mustPlans(t, a)
	if planNames(t, rows) != "default,Copy,Empty,Old*" || rows[1].Entries != 2 || rows[2].Entries != 0 {
		t.Fatalf("rows %+v", rows)
	}

	// Switching writes no game file.
	before := e.ReadModsConfig()
	if _, text, err := view.UsePlan(a, rows[2].ID); err != nil || !strings.HasPrefix(text, "now editing Empty") {
		t.Fatalf("use: %v %q", err, text)
	}
	if e.ReadModsConfig() != before {
		t.Fatal("switching wrote ModsConfig.xml")
	}
	if st, _ := a.Loadout(); st.Plan.Name != "Empty" || len(st.List) != 0 {
		t.Fatalf("editor shows %s with %d", st.Plan.Name, len(st.List))
	}

	if text, err := view.RenamePlan(a, rows[1].ID, "Main"); err != nil || text != "renamed Copy to Main" {
		t.Fatalf("rename: %v %q", err, text)
	}
	if _, err := view.RenamePlan(a, rows[1].ID, "old"); err == nil {
		t.Fatal("renamed onto another plan's name")
	}

	// Deleting the current plan makes another one current.
	if w, err := view.RemovePlanWarn(a, rows[2].ID); err != nil || !strings.Contains(w, "cannot be undone") {
		t.Fatalf("warn %q %v", w, err)
	}
	if text, err := view.RemovePlan(a, rows[2].ID); err != nil || !strings.HasPrefix(text, "deleted Empty; now editing ") {
		t.Fatalf("remove: %v %q", err, text)
	}
	if _, _, err := view.UsePlan(a, rows[2].ID); err == nil || err.Error() != "that plan is gone" {
		t.Fatalf("a deleted plan: %v", err)
	}
	rows = mustPlans(t, a)
	if len(rows) != 3 {
		t.Fatalf("rows %+v", rows)
	}

	// An import read and previewed becomes a plan of its own.
	imp, err := view.ReadImport(a, view.ImportText, "tester.b\nbrrainz.harmony\nsomeone.missing")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := view.PreviewImport(a, imp); err != nil {
		t.Fatal(err)
	}
	p, text, err = view.ImportAsPlan(a, imp, "Pasted")
	if err != nil || !strings.HasPrefix(text, "new plan Pasted: 3 mods") {
		t.Fatalf("import: %v %q", err, text)
	}
	st, _ := a.Loadout()
	if st.Plan.ID != p.ID || len(st.List) != 3 {
		t.Fatalf("imported plan %s %d", st.Plan.Name, len(st.List))
	}
	if v, _ := a.Store.Version(p.ID, 1); v == nil || v.Source != "import" || v.Note != "pasted list" {
		t.Fatalf("v1 %+v", v)
	}
}

// The status line's "游戏里现在是" follows the write record; a plan's conflict mark shows
// on its row, the sidebar and the home page.
func TestInGameAndMarks(t *testing.T) {
	e, _, _ := shellEnv(t)
	a := e.App
	if g := view.InGame(a); g != nil {
		t.Fatalf("nothing written yet: %+v", g)
	}
	e.MustRun("save")
	g := view.InGame(a)
	cur, _ := a.CurrentPlan()
	if g == nil || g.PlanID != cur.ID || g.Version < 1 || g.Text != "in the game now: default v"+itoa(g.Version) {
		t.Fatalf("in game %+v", g)
	}
	// Another plan becomes current: the game still holds default.
	p, _, err := view.NewPlan(a, "Second", "")
	if err != nil {
		t.Fatal(err)
	}
	if g2 := view.InGame(a); g2 == nil || g2.PlanID != cur.ID {
		t.Fatalf("switching changed the record: %+v", g2)
	}
	rows := mustPlans(t, a)
	if !rows[0].InGame || rows[1].InGame {
		t.Fatalf("in-game flags %+v", rows)
	}

	ws, _ := a.Store.Workspace(p.ID)
	ws.Conflict = 1
	if err := a.Store.SetWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	rows = mustPlans(t, a)
	if !rows[1].Conflict || rows[1].ConflictText == "" || rows[0].Conflict {
		t.Fatalf("marks %+v", rows)
	}
	nav := view.Nav(0, rows, nil)
	kids := nav[0].Items[2].Kids
	if len(kids) != 3 || kids[1].Mark == "" || !kids[1].Current || kids[2].ID != view.PagePlanNew || !kids[2].Add {
		t.Fatalf("plan kids %+v", kids)
	}
	// each plan's sub-entry carries its number of mods, as magpie's sub-pages do
	if kids[0].Count != rows[0].Entries || kids[2].Count != 0 {
		t.Fatalf("plan kid counts %+v", kids)
	}
}

// G12 in part: one home row per plan, by the last launch; never played ones by name. The
// current plan counts its needs; the others use PlanProblems ("not checked yet" until
// the per-plan analysis exists).
func TestHomePerPlan(t *testing.T) {
	e, _, n := shellEnv(t)
	a := e.App
	def, _ := a.CurrentPlan()
	for _, name := range []string{"Zeta", "Beta", "Alpha"} {
		if _, _, err := view.NewPlan(a, name, "current"); err != nil {
			t.Fatal(err)
		}
	}
	beta, _ := a.FindPlan("Beta")
	t0 := e.Now()
	a.Store.AddLaunchRecord(&store.LaunchRecord{EnvID: def.EnvID, PlanID: def.ID, Started: t0.Add(-48 * time.Hour), Method: "steam"})
	a.Store.AddLaunchRecord(&store.LaunchRecord{EnvID: def.EnvID, PlanID: beta.ID, Started: t0.Add(-time.Hour), Method: "steam"})
	b, err := view.BuildBoard(a) // Alpha, the last one made, is current
	if err != nil {
		t.Fatal(err)
	}
	plans := mustPlans(t, a)
	packs := []view.PackRow{{Name: "Pack", Played: "played", PlayedAt: t0.Add(-24 * time.Hour)}}
	n.Never = false
	h := view.BuildHome(a, b, n, plans, packs)
	var order []string
	for _, r := range h.Rows {
		order = append(order, r.Name+"="+r.State)
	}
	want := "Beta=not checked yet,Pack=checked when built,default=not checked yet,Alpha=1 problems break the game,Zeta=not checked yet"
	if strings.Join(order, ",") != want {
		t.Fatalf("order\n got %v\nwant %s", order, want)
	}
	if r := h.Rows[3]; !r.Current || r.Page != view.PlanPage(r.Plan) {
		t.Fatalf("current row %+v", r)
	}

	// The per-plan count plugs in through the hook.
	defer func(f func(*appT, int64) (int, bool)) { view.PlanProblems = f }(view.PlanProblems)
	view.PlanProblems = func(_ *appT, id int64) (int, bool) {
		if id == beta.ID {
			return 2, true
		}
		return 0, true
	}
	h = view.BuildHome(a, b, n, plans, packs)
	if h.Rows[0].State != "2 problems break the game" || h.Rows[2].State != "ready to play" {
		t.Fatalf("hooked %+v", h.Rows)
	}
}
