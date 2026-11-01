package view_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestArrivalGroupNamesTheChange(t *testing.T) {
	view.SetLang("en")
	for _, tc := range []struct {
		name                     string
		local, workshop, removed int
		want                     string
	}{
		{"one local", 1, 0, 0, "1 new mod: Local 0"},
		{"several local", 3, 0, 0, "3 new mods"},
		{"one subscription", 0, 1, 0, "1 new subscription: Workshop 0"},
		{"several subscriptions", 0, 2, 0, "2 new subscriptions"},
		{"one removed", 0, 0, 1, "1 mod removed: Removed 0"},
		{"several removed", 0, 0, 2, "2 mods removed"},
		{"mixed", 1, 2, 1, "2 new subscriptions  1 new mod: Local 0  1 mod removed: Removed 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := testkit.New(t)
			e.AddEnv(false)
			for i := 0; i < tc.removed; i++ {
				e.WriteMod(filepath.Join(e.Game, "Mods", fmt.Sprint("Removed", i)), testkit.Sample(fmt.Sprint("test.removed", i), fmt.Sprint("Removed ", i)))
			}
			e.MustRun("scan")
			e.Advance(time.Hour)
			for i := 0; i < tc.local; i++ {
				e.WriteMod(filepath.Join(e.Game, "Mods", fmt.Sprint("Local", i)), testkit.Sample(fmt.Sprint("test.local", i), fmt.Sprint("Local ", i)))
			}
			for i := 0; i < tc.workshop; i++ {
				e.AddWorkshop(fmt.Sprint(1000+i), testkit.Sample(fmt.Sprint("test.workshop", i), fmt.Sprint("Workshop ", i)))
			}
			for i := 0; i < tc.removed; i++ {
				e.Remove(filepath.Join(e.Game, "Mods", fmt.Sprint("Removed", i)))
			}
			e.MustRun("scan")
			st, err := e.App.Loadout()
			if err != nil {
				t.Fatal(err)
			}
			p, err := view.BuildChanges(e.App, st, modchanges.Opts{Library: true})
			if err != nil {
				t.Fatal(err)
			}
			if p.Arrivals == nil || p.Arrivals.Text != tc.want {
				t.Fatalf("group: %+v, want %q", p.Arrivals, tc.want)
			}
			for _, r := range p.Arrivals.Rows {
				if r.ID == "" || r.Ref == "" {
					t.Fatalf("no copy identity: %+v", r)
				}
				if strings.HasPrefix(r.Mod, "Removed") {
					if r.Present || r.Text != "removed from the Mods folder" {
						t.Fatalf("removal: %+v", r)
					}
				} else {
					if !r.Present {
						t.Fatalf("installed copy has no library link: %+v", r)
					}
					if strings.HasPrefix(r.Mod, "Local") && r.Text != "found in the Mods folder" {
						t.Fatal(r.Text)
					}
					if strings.HasPrefix(r.Mod, "Workshop") && r.Text != "new subscription from Workshop" {
						t.Fatal(r.Text)
					}
				}
			}
		})
	}
}

func TestRemovedWorkshopSectionUsesInstalledCopiesAndSavedChecks(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Sample("test.removed", "Same name"))
	e.AddWorkshop("1002", testkit.Sample("test.network", "Network failure"))
	e.WriteMod(filepath.Join(e.SteamCMD, "1001"), testkit.Sample("test.removed", "Same name"))
	e.AddEnv(true)
	e.MustRun("scan")

	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	clientID := ""
	for _, m := range st.Ix.Mods {
		if m.WorkshopID() == "1001" && string(m.Kind) == "workshop" {
			clientID = m.ID
		}
	}
	if clientID == "" {
		t.Fatal("client copy missing")
	}
	e.MustRun("enable", st.Ix.Ref(clientID))
	st, err = e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	client := st.Ix.ByID[clientID]
	save := func(key string, c app.SourceCheck) {
		t.Helper()
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		if err = e.App.Store.SetMeta(key, string(b)); err != nil {
			t.Fatal(err)
		}
	}
	// A stale latest value cannot turn a deleted/private item into a pending update.
	save("subscriptions:check:"+st.Ix.Env.Name+":workshop", app.SourceCheck{At: e.Now(), Result: "check complete", Latest: map[string]int64{client.ID: e.Now().Add(time.Hour).Unix()}, Failures: []app.SourceFailure{
		{ID: "1001", Name: "Same name", Code: 9}, {ID: "1002", Name: "Network failure", Reason: "connection timed out"}, {ID: "9999", Name: "Already removed locally", Code: 9},
	}})
	save("subscriptions:check:shared:steamcmd", app.SourceCheck{At: e.Now(), Result: "check complete", Failures: []app.SourceFailure{{ID: "1001", Name: "Same name", Code: 9}}})
	// Any network access on draw would hit a closed endpoint and fail; all rows use saved checks.
	e.App.SteamAPI = "http://127.0.0.1:1"
	p, err := view.BuildSubscriptions(e.App, st, modchanges.Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Gone) != 2 || len(p.Pending) != 0 || p.Summary.Failures != 1 || !strings.Contains(p.Summary.Line, "1 check failed") {
		t.Fatalf("page: %+v", p)
	}
	if p.Gone[0].ID == p.Gone[1].ID || p.Gone[0].Ref == p.Gone[1].Ref {
		t.Fatalf("copies collapsed: %+v", p.Gone)
	}
	inPlan := 0
	for _, r := range p.Gone {
		if r.InPlan {
			inPlan++
		}
		if r.Name != "Same name" {
			t.Fatal(r)
		}
	}
	if inPlan != 1 {
		t.Fatalf("scope membership: %+v", p.Gone)
	}
	if p.Sources[0].Result != "1 check failed: connection timed out" || len(p.Sources[0].Failures) != 1 || !strings.Contains(p.Sources[0].Failures[0].Items[0], "Network failure") {
		t.Fatal(p.Sources[0])
	}
	if p.Sources[1].Result != "everything is up to date" || len(p.Sources[1].Failures) != 0 {
		t.Fatal(p.Sources[1])
	}
	// A successful later check restores update eligibility and removes the gone section.
	save("subscriptions:check:"+st.Ix.Env.Name+":workshop", app.SourceCheck{At: e.Now(), Result: "check complete"})
	save("subscriptions:check:shared:steamcmd", app.SourceCheck{At: e.Now(), Result: "check complete"})
	p, err = view.BuildSubscriptions(e.App, st, modchanges.Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Gone) != 0 || p.Summary.Failures != 0 || strings.Contains(p.Summary.Line, "failed") {
		t.Fatalf("stale failure/removed rows: %+v", p)
	}
}
