package view

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestNameCore(t *testing.T) {
	for in, want := range map[string]string{
		"Ratkin Vehicles+ (Continued)":    "ratkinvehicles",
		"[RK] Ratkin Vehicles 1.6":        "ratkinvehicles",
		"Ratkin Vehicles+ UnOfficial":     "ratkinvehicles",
		"Vanilla Expanded - Updated v2.1": "vanillaexpanded",
		// a Chinese name with a full-width tag
		"\u9f20\u65cf\u8f7d\u5177\u30101.7\u3011": "\u9f20\u65cf\u8f7d\u5177",
	} {
		if got := nameCore(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// A translation of an outdated mod, its continuation under another packageId enabled, and
// another translation of the same mod (which itself requires the original): Can it run
// offers the continuation, not the other translation, and the choice makes a replaces
// rule that clears the item. Ignore covers every mod that needs it.
func TestNeedsStandInAndIgnore(t *testing.T) {
	t.Parallel()
	SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "fxz.ratkinvehicle.continued", Name: "Ratkin Vehicles (Continued)"})
	e.AddWorkshop("1002", testkit.Mod{PackageID: "zh.ratkinvehicle.cn", Name: "Ratkin Vehicles CN", Deps: []string{"old.ratkinvehicle"}})
	e.AddWorkshop("1003", testkit.Mod{PackageID: "ru.ratkinvehicle.ru", Name: "Ratkin Vehicles RU", Deps: []string{"old.ratkinvehicle"}})
	e.AddWorkshop("1004", testkit.Mod{PackageID: "tester.other", Name: "Unrelated"})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "fxz.ratkinvehicle.continued", "zh.ratkinvehicle.cn", "ru.ratkinvehicle.ru", "tester.other")
	e.AddEnv(false)
	e.MustRun("scan")
	item := func() *Need {
		t.Helper()
		st, err := e.App.Loadout()
		if err != nil {
			t.Fatal(err)
		}
		ps, err := e.App.Problems(false, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range BuildNeeds(e.App, st, ps).Items {
			if it.Target == "old.ratkinvehicle" {
				return &it
			}
		}
		return nil
	}
	it := item()
	if it == nil || len(it.RequiredBy) != 2 {
		t.Fatalf("item %+v", it)
	}
	var stand, ignore *Action
	for i := range it.Actions {
		switch it.Actions[i].Kind {
		case ActStandIn:
			stand = &it.Actions[i]
		case ActIgnore:
			ignore = &it.Actions[i]
		}
	}
	if stand == nil || len(stand.Options) != 1 || !strings.Contains(stand.Options[0].Label, "Ratkin Vehicles (Continued)") || stand.Options[0].Refs[1] != "pkg:old.ratkinvehicle" {
		t.Fatalf("stand-in %+v", stand)
	}
	if ignore == nil || len(ignore.Refs) != 2 {
		t.Fatalf("ignore %+v", ignore)
	}
	// Ignoring hides it for both translations.
	if _, err := Do(e.App, *ignore, 0); err != nil {
		t.Fatal(err)
	}
	if it := item(); it != nil {
		t.Fatalf("after ignore %+v", it)
	}
	e.MustRun("problems", "unignore", ignore.Refs[0])
	e.MustRun("problems", "unignore", ignore.Refs[1])
	if item() == nil {
		t.Fatal("unignore did not bring it back")
	}
	// The stand-in choice adds a rule and the dependency is met.
	done, err := Do(e.App, *stand, 0)
	if err != nil || !strings.Contains(done, "now stands in for") {
		t.Fatal(done, err)
	}
	if it := item(); it != nil {
		t.Fatalf("after rule %+v", it)
	}
	if out := e.MustRun("rule", "list"); !strings.Contains(out, "replaces") || !strings.Contains(out, "pkg:old.ratkinvehicle") {
		t.Fatal(out)
	}
	rs, err := BuildRules(e.App)
	if err != nil || len(rs.Rows) != 1 || rs.Rows[0].Text != "Ratkin Vehicles (Continued) stands in for old.ratkinvehicle" {
		t.Fatalf("rules %+v %v", rs, err)
	}
}
