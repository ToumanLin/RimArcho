package view_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	_, _, n := shellEnv(t)
	g := view.Nav(n.Count, []view.PackRow{{Name: "Main"}})
	if len(g) != 1 || g[0].Title != "playing" {
		t.Fatalf("groups with nothing in them are left out: %+v", g)
	}
	var ids, keys []string
	for _, it := range g[0].Items {
		ids, keys = append(ids, it.ID), append(keys, it.Key)
	}
	if strings.Join(ids, ",") != "home,needs,plan,packs,env,library,log" {
		t.Fatalf("order %v", ids)
	}
	if keys[0] != "shift+1" || keys[6] != "shift+7" {
		t.Fatalf("keys %v", keys)
	}
	if g[0].Items[1].Badge != n.Count || n.Count == 0 {
		t.Fatalf("badge %d, needs %d", g[0].Items[1].Badge, n.Count)
	}
	path := func(id string) string {
		var out []string
		for _, c := range view.NavPath(g, id) {
			out = append(out, c.Label+"="+c.ID)
		}
		return strings.Join(out, " / ")
	}
	for id, want := range map[string]string{
		"plan":      "playing=home / plans=plan / current list=plan",
		"needs":     "playing=home / can it run=needs",
		"changes":   "playing=home / mod library=library / changes=changes",
		"pack:Main": "playing=home / packs=packs / Main=pack:Main",
		"settings":  "settings=settings",
		"nowhere":   "",
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
		{Name: "Light", Light: true, Played: "never played"},
	}
	// The list was launched yesterday (a pack launch later does not count for it).
	e.App.Store.AddLaunch(b.St.Ix.Env.ID, t0.Add(-24*time.Hour), "steam", "x")
	e.App.Store.AddLaunch(b.St.Ix.Env.ID, t0.Add(-time.Hour), "pack:Old", "x")
	h := view.BuildHome(e.App, b, n, packs)
	var order []string
	for _, r := range h.Rows {
		order = append(order, r.Name)
	}
	if strings.Join(order, ",") != "current list,Old,Light,Zed" {
		t.Fatalf("order %v", order)
	}
	plan := h.Rows[0]
	if plan.Kind != view.RowPlan || plan.Env != "main · 1.6" || !strings.HasPrefix(plan.Played, "played ") {
		t.Fatalf("plan row %+v", plan)
	}
	// Never analysed: the state says so instead of a count that may be short.
	if n.Never && (plan.State != "not checked yet" || plan.Level != "") {
		t.Fatalf("plan state %+v", plan)
	}
	n.Never = false
	h = view.BuildHome(e.App, b, n, packs)
	if r := h.Rows[0]; r.State != "1 problems break the game" || r.Level != view.LevelBad {
		t.Fatalf("plan state %+v", r)
	}
	if r := h.Rows[2]; !r.Light || r.State != "list only" {
		t.Fatalf("light pack %+v", r)
	}
	if h.ScanWhen == "" {
		t.Fatalf("no scan time: %+v", h)
	}
}

func TestFindAndAddGames(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	gs := view.FindGames(e.App)
	if len(gs) != 1 || gs[0].Name != "steam" || gs[0].Launch != "steam" || gs[0].Workshop == "" || gs[0].Env != "" || gs[0].Version == "" {
		t.Fatalf("found %+v", gs)
	}
	names, err := view.AddGames(e.App, gs)
	if err != nil || strings.Join(names, ",") != "steam" {
		t.Fatalf("added %v, %v", names, err)
	}
	if cur, err := e.App.Env(); err != nil || cur.Name != "steam" {
		t.Fatalf("current %v %v", cur, err)
	}
	// Found again: it is in use, and adding it again does nothing.
	gs = view.FindGames(e.App)
	if len(gs) != 1 || gs[0].Env != "steam" || gs[0].Name != "" {
		t.Fatalf("again %+v", gs)
	}
	if names, err := view.AddGames(e.App, gs); err != nil || len(names) != 0 {
		t.Fatalf("again: %v %v", names, err)
	}
	g, err := view.GameAt(e.App, `"`+e.Game+`"`)
	if err != nil || g.Env != "steam" || g.Launch != "steam" {
		t.Fatalf("typed %+v %v", g, err)
	}
	if _, err := view.GameAt(e.App, e.Root); err == nil {
		t.Fatal("not a game folder was accepted")
	}
}
