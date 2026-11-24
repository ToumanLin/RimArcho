package gui_test

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/surface/gui"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The Launch page draws before other lists' problems are collected (16 §2.3): Home leaves
// their states out until they are summarized, HomeAll fills them, and the next Home has
// them without collecting again.
func TestHomeLeavesUnsummarizedChecksToHomeAll(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.alpha", Name: "Alpha"})
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.alpha")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	for _, name := range []string{"Second", "Third"} {
		if _, _, err := view.NewPlan(e.App, name, "current"); err != nil {
			t.Fatal(err)
		}
	}
	api := gui.NewAPI(e.App, nil)
	defer api.Close()
	others := func(h *view.Home) map[string]string {
		out := map[string]string{}
		for _, r := range h.Rows {
			if r.Kind == view.RowPlan && !r.Current {
				out[r.Name] = r.State
			}
		}
		return out
	}
	h := api.Home()
	if h.Error != "" || !h.ChecksPending {
		t.Fatalf("first Home: error %q, pending %v; want the other lists left out", h.Error, h.ChecksPending)
	}
	for name, state := range others(h) {
		if state != "" {
			t.Fatalf("first Home: %s already has state %q", name, state)
		}
	}
	all := api.HomeAll()
	if all.ChecksPending || len(others(all)) != 2 {
		t.Fatalf("HomeAll: pending %v, rows %v", all.ChecksPending, others(all))
	}
	for name, state := range others(all) {
		if state == "" {
			t.Fatalf("HomeAll: %s has no state", name)
		}
	}
	again := api.Home()
	if again.ChecksPending {
		t.Fatal("Home after HomeAll still pending: the summaries were not kept")
	}
	for name, state := range others(all) {
		if got := others(again)[name]; got != state {
			t.Fatalf("Home after HomeAll: %s = %q, want %q", name, got, state)
		}
	}
}

// A plan's summary is made once per kept problem set, and again after the set changes.
func TestPlanSummaryOncePerProblemSet(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.alpha", Name: "Alpha"})
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.alpha")
	e.AddEnv(false)
	e.MustRun("scan")
	if _, _, err := view.NewPlan(e.App, "Second", "current"); err != nil {
		t.Fatal(err)
	}
	a := e.App
	p, err := a.FindPlan("default")
	if err != nil || p == nil {
		t.Fatalf("plan: %v", err)
	}
	made := 0
	f := func(*app.ProblemSet) any { made++; return made }
	for i := 0; i < 3; i++ {
		if _, err := a.PlanSummary(p, f); err != nil {
			t.Fatal(err)
		}
	}
	if made != 1 {
		t.Fatalf("summary made %d times for one problem set", made)
	}
	if v, known, err := a.PlanSummaryKnown(p); err != nil || !known || v != 1 {
		t.Fatalf("known summary %v %v %v", v, known, err)
	}
	e.AddWorkshop("1002", testkit.Mod{PackageID: "tester.beta", Name: "Beta"})
	e.MustRun("scan")
	if _, known, _ := a.PlanSummaryKnown(p); known {
		t.Fatal("a summary outlived its problem set")
	}
	if v, _ := a.PlanSummary(p, f); v != 2 {
		t.Fatalf("after a change: %v, want a new summary", v)
	}
}
