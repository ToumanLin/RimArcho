package view_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// shellEnv: Alpha is enabled and needs Harmony, which is installed but not enabled: one
// "breaks the game" item.
func shellEnv(t *testing.T) (*testkit.Env, *view.Board, *view.Needs) {
	view.SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	a := testkit.Sample("tester.a", "Alpha")
	a.Deps = []string{"brrainz.harmony"}
	e.AddWorkshop("1001", a)
	e.WriteMod(filepath.Join(e.Game, "Mods", "B"), testkit.Sample("tester.b", "Beta"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.a")
	e.AddEnv(false)
	e.MustRun("scan")
	b, err := view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return e, b, view.BuildNeeds(e.App, b.St, ps)
}

func TestNavAndPath(t *testing.T) {
	e, _, n := shellEnv(t)
	plans, err := view.PlanRows(e.App)
	if err != nil || len(plans) != 1 {
		t.Fatalf("plans %+v %v", plans, err)
	}
	g := view.Nav(n.Count, plans, []view.PackRow{{Name: "Main"}})
	if len(g) != 2 || g[0].Title != "playing" || g[1].Title != "library" {
		t.Fatalf("two groups, 在玩 and 库 (06 §2.1): %+v", g)
	}
	var ids, keys []string
	for _, gr := range g {
		for _, it := range gr.Items {
			ids, keys = append(ids, it.ID), append(keys, it.Key)
		}
	}
	if strings.Join(ids, ",") != "home,needs,log,library,plan,packs,textures,env" {
		t.Fatalf("order %v", ids)
	}
	if keys[0] != "shift+1" || keys[7] != "shift+8" {
		t.Fatalf("keys %v", keys)
	}
	if g[0].Items[1].Badge != n.Count || n.Count == 0 {
		t.Fatalf("badge %d, needs %d", g[0].Items[1].Badge, n.Count)
	}
	// only 能不能跑 carries a number (06 §2.1)
	for _, gr := range g {
		for _, it := range gr.Items {
			if it.ID != view.PageNeeds && it.Badge != 0 {
				t.Fatalf("%s has a number: %+v", it.ID, it)
			}
		}
	}
	path := func(id string) string {
		var out []string
		for _, c := range view.NavPath(g, id) {
			out = append(out, c.Label+"="+c.ID)
		}
		return strings.Join(out, " / ")
	}
	for id, want := range map[string]string{
		"plan":        "library=library / lists=plan",
		plans[0].Page: "library=library / lists=plan / default=" + plans[0].Page,
		"plans":       "library=library / lists=plan / manage=plans",
		"needs":       "playing=home / can it run=needs",
		"log":         "playing=home / game log=log",
		"changes":     "library=library / mod library=library / changes=changes",
		"library":     "library=library / mod library=library / all=library",
		"pack:Main":   "library=library / packs=packs / Main=pack:Main",
		"settings":    "settings=settings",
		"nowhere":     "",
	} {
		if got := path(id); got != want {
			t.Errorf("NavPath(%s) = %q, want %q", id, got, want)
		}
	}
}

func TestBuildHome(t *testing.T) {
	e, b, n := shellEnv(t)
	if view.BreakCount(n) != 1 {
		t.Fatalf("break count %d: %+v", view.BreakCount(n), n.Items)
	}
	t0 := e.Now()
	packs := []view.PackRow{
		{Name: "Old", Facts: "f", Played: "played", PlayedAt: t0.Add(-48 * time.Hour)},
		{Name: "Zed", Played: "never played"},
		{Name: "Amber", Played: "never played"},
	}
	// The list was launched yesterday (a pack launch later does not count for it).
	e.App.Store.AddLaunchRecord(&store.LaunchRecord{EnvID: b.St.Ix.Env.ID, Started: t0.Add(-24 * time.Hour), Method: "steam", Command: "x", PlanID: e.App.Store.CurrentPlanID(b.St.Ix.Env.ID)})
	e.App.Store.AddLaunchRecord(&store.LaunchRecord{EnvID: b.St.Ix.Env.ID, Started: t0.Add(-time.Hour), Method: "pack:Old", Command: "x"})
	plans, _ := view.PlanRows(e.App)
	h := view.BuildHome(e.App, b, n, plans, packs)
	var order []string
	for _, r := range h.Rows {
		order = append(order, r.Name)
	}
	if strings.Join(order, ",") != "default,Old,Amber,Zed" {
		t.Fatalf("order %v", order)
	}
	plan := h.Rows[0]
	if plan.Kind != view.RowPlan || plan.Env != "main  1.6" || !strings.HasPrefix(plan.Played, "played ") {
		t.Fatalf("plan row %+v", plan)
	}
	// the meta line: how many mods, and how they change (06 §2.3)
	if !strings.HasSuffix(plan.Facts, " mods") && !strings.Contains(plan.Facts, " mods  ") || plan.Mode != "follows Workshop updates" {
		t.Fatalf("plan meta %q / %q", plan.Facts, plan.Mode)
	}
	// Never analysed: the state says so instead of a count that may be short.
	if n.Never && (plan.State != "not checked yet" || plan.Level != "") {
		t.Fatalf("plan state %+v", plan)
	}
	n.Never = false
	h = view.BuildHome(e.App, b, n, plans, packs)
	if r := h.Rows[0]; r.State != "1 problems break the game" || r.Level != view.LevelBad {
		t.Fatalf("plan state %+v", r)
	}
	if r := h.Rows[2]; r.State != "checked when created" || r.Level != view.HomeOK {
		t.Fatalf("pack state %+v", r)
	}
	if r := h.Rows[1]; r.Mode != "versions locked  own saves" || r.Facts != "f" {
		t.Fatalf("pack meta %+v", r)
	}
	packs[0].Mods = 42
	h = view.BuildHome(e.App, b, n, plans, packs)
	if r := h.Rows[1]; r.Facts != "42 mods" {
		t.Fatalf("pack facts %+v", r)
	}
	if h.Changes == nil || h.Changes.Text != "nothing changed" {
		t.Fatalf("the plan's changes: %+v", h.Changes)
	}
}

func TestFindAndAddGames(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	gs := view.FindGames(e.App)
	if len(gs) != 1 || gs[0].Name != "1.6" || gs[0].Launch != "steam" || gs[0].Workshop == "" || gs[0].Env != "" || gs[0].Version == "" {
		t.Fatalf("found %+v", gs)
	}
	names, err := view.AddGames(e.App, gs)
	if err != nil || strings.Join(names, ",") != "1.6" {
		t.Fatalf("added %v, %v", names, err)
	}
	if cur, err := e.App.Env(); err != nil || cur.Name != "1.6" {
		t.Fatalf("current %v %v", cur, err)
	}
	// Found again: it is in use, and adding it again does nothing.
	gs = view.FindGames(e.App)
	if len(gs) != 1 || gs[0].Env != "1.6" || gs[0].Name != "" {
		t.Fatalf("again %+v", gs)
	}
	if names, err := view.AddGames(e.App, gs); err != nil || len(names) != 0 {
		t.Fatalf("again: %v %v", names, err)
	}
	g, err := view.GameAt(e.App, `"`+e.Game+`"`)
	if err != nil || g.Env != "1.6" || g.Launch != "steam" {
		t.Fatalf("typed %+v %v", g, err)
	}
	if _, err := view.GameAt(e.App, e.Root); err == nil {
		t.Fatal("not a game folder was accepted")
	}
}

// The home page's large card is what is applied: a pack applied on the home page while the
// same plan is current, else the current plan; another environment's rows never are.
func TestPickFeatured(t *testing.T) {
	h := &view.Home{Rows: []view.HomeRow{
		{Kind: view.RowPlan, Name: "Main", Current: true, EnvName: "main"},
		{Kind: view.RowPack, Name: "Old", EnvName: "main"},
		{Kind: view.RowPack, Name: "Old", EnvName: "test", OtherEnv: true},
	}}
	view.PickFeatured(h, "", "main", 7)
	if h.Featured != 0 {
		t.Fatalf("no setting: %d", h.Featured)
	}
	view.PickFeatured(h, view.FeaturedValue("main", "Old", 7), "main", 7)
	if h.Featured != 1 {
		t.Fatalf("applied pack: %d", h.Featured)
	}
	// the current plan changed since: the pack is no longer what is applied
	view.PickFeatured(h, view.FeaturedValue("main", "Old", 7), "main", 8)
	if h.Featured != 0 {
		t.Fatalf("lapsed: %d", h.Featured)
	}
}
