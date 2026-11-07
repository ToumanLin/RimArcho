package app_test

import (
	"runtime"
	"testing"
	"weak"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func sharedFixture(t *testing.T) *testkit.Env {
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Sample("tester.a", "Alpha"))
	e.AddWorkshop("1002", testkit.Sample("tester.b", "Beta"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.a")
	e.AddEnv(false)
	e.MustRun("scan")
	return e
}

// Sessions of one process share a registry built from identical candidates (17 §7.3).
func TestSharedRegistryAcrossSessions(t *testing.T) {
	e := sharedFixture(t)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	st2, err := r.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if st.Reg != st2.Reg {
		t.Fatal("two sessions on the same data built separate registries")
	}
	// Appending to one session's candidates never reaches another's.
	given := st.Reg.Given()
	extra := append(given, loadout.Candidate{ID: "x", PackageID: "x"})
	if len(st2.Reg.Given()) != len(given) || &extra[0] == &given[0] {
		t.Fatal("shared candidates were appended in place")
	}
	// Different candidates get their own registry.
	e.AddWorkshop("1003", testkit.Sample("tester.c", "Gamma"))
	e.MustRun("scan")
	st3, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if st3.Reg == st.Reg || len(st3.Reg.Given()) != len(given)+1 {
		t.Fatal("a scan that added a Mod reused the old registry")
	}
}

func TestSharedGameLinksAcrossSessions(t *testing.T) {
	e := sharedFixture(t)
	if _, err := e.App.InspectGameLinks(); err != nil {
		t.Fatal(err)
	}
	r, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := r.InspectGameLinks(); err != nil {
		t.Fatal(err)
	}
	if !app.SameLinks(e.App, r) {
		t.Fatal("two sessions at one index generation built separate link instances")
	}
	app.ForgetLinksOf(e.App)
	r2, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	if _, err := r2.InspectGameLinks(); err != nil {
		t.Fatal(err)
	}
	if app.SameLinks(e.App, r2) {
		t.Fatal("a forgotten database still served shared link instances")
	}
}

// An entry lives only while a session uses it.
func TestSharedRegistryIsWeak(t *testing.T) {
	cands := []loadout.Candidate{{ID: "i:weak-test", PackageID: "weak.test", Kind: "workshop", Folder: "1"}}
	r := app.SharedRegistryFor(cands)
	if app.SharedRegistryFor(append([]loadout.Candidate(nil), cands...)) != r {
		t.Fatal("identical candidates built a second registry")
	}
	w := weak.Make(r)
	r = nil
	_ = r
	runtime.GC()
	runtime.GC()
	if w.Value() != nil {
		t.Fatal("the shared cache kept a registry no session uses")
	}
}
