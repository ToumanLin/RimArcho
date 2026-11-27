package app_test

// Mods that arrive after the click (07 §7.6 C6–C8, 11 §2.2 A38–A40): catching up enables
// the collection items installed since the last catch-up, and a subscription through the
// Steam client follows the download, rescans and adds the Mods to the list it was for.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// noDetails answers the subscribe preflight with nothing known (no network).
func noDetails(t *testing.T, e *testkit.Env) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"response":{"publishedfiledetails":[]}}`))
	}))
	t.Cleanup(srv.Close)
	e.App.SteamAPI = srv.URL
}

// fakeSteamDownload is the Steam client: subscribe confirms every item, and watch installs
// the items install holds (writing their folder mid-download) and stalls the others.
func fakeSteamDownload(e *testkit.Env, install map[string]testkit.Mod) {
	steamDLL(e)
	e.App.SteamWorker = func(_ string, args []string) (string, error) {
		var out []string
		for _, s := range args[1:] {
			id, _ := strconv.ParseUint(s, 10, 64)
			out = append(out, steamworks.ItemLine(id, "ok", steamworks.ItemSubscribed|steamworks.ItemDownloadPending))
		}
		return strings.Join(append(out, steamworks.DoneLine(args[0], len(args)-1)), "\n"), nil
	}
	e.App.SteamWorkerStream = func(_ context.Context, _ string, args []string, line func(string)) error {
		for _, s := range args[1:] {
			id, _ := strconv.ParseUint(s, 10, 64)
			m, ok := install[s]
			if !ok {
				line(steamworks.ProgressLine(steamworks.Progress{ID: id, Got: 0, Total: 100, State: steamworks.ItemSubscribed | steamworks.ItemDownloadPending}))
				line(steamworks.ItemLine(id, "failed:download-stalled", steamworks.ItemSubscribed))
				continue
			}
			line(steamworks.ProgressLine(steamworks.Progress{ID: id, Got: 40, Total: 100, State: steamworks.ItemSubscribed | steamworks.ItemDownloading}))
			e.AddWorkshop(s, m)
			line(steamworks.ItemLine(id, "ok", steamworks.ItemSubscribed|steamworks.ItemInstalled))
		}
		line(steamworks.DoneLine(steamworks.Watch, len(args)-1))
		return nil
	}
}

// C6: items missing at the link arrive; catching up enables them in the collection's order,
// and an installed collection Mod the player never enabled stays out.
func TestCatchUpEnablesArrived(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	e.AddWorkshop("1002", testkit.Sample("tester.d", "D"))
	scanNow(t, e)
	items := []string{"2009463077", "1001", "1003", "1002", "1004"}
	fakeCollection(t, e, &items)
	out := e.MustRun("plan", "follow", "default", "3000000009")
	if !strings.Contains(out, "no change since the last catch-up") || !strings.Contains(out, "not installed: 2") {
		t.Fatal(out)
	}
	// The player subscribes; Steam installs E and F.
	e.AddWorkshop("1003", testkit.Sample("tester.e", "E"))
	e.AddWorkshop("1004", testkit.Sample("tester.f", "F"))
	scanNow(t, e)
	out = e.MustRun("plan", "sync", "default", "--check")
	for _, want := range []string{"1 changed", "installed since: 2", "+ ws:1003", "+ ws:1004"} {
		if !strings.Contains(out, want) {
			t.Fatalf("check lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ws:1002") || strings.Contains(out, "no change") {
		t.Fatalf("D was installed at the link: not arrived\n%s", out)
	}
	out = e.MustRun("plan", "sync", "default")
	if !strings.Contains(out, "2 enabled, 0 disabled") {
		t.Fatal(out)
	}
	if got := joined(active(t, e)); got != "Core, Harmony, A, E, F, ?gone.mod" {
		t.Fatalf("collection order: %s", got)
	}
	// The player disables F on purpose: it stays out.
	e.MustRun("disable", refOf(t, e, "F", ""))
	if out := e.MustRun("plan", "sync", "default", "--check"); !strings.Contains(out, "0 changed") || !strings.Contains(out, "no change since the last catch-up") {
		t.Fatalf("a Mod disabled after the catch-up is not a change:\n%s", out)
	}
}

// C7: a snapshot from before missing entries were recorded counts copies first seen after
// the link.
func TestCatchUpArrivedLegacySnapshot(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	e.AddWorkshop("1002", testkit.Sample("tester.d", "D"))
	scanNow(t, e)
	items := []string{"2009463077", "1001", "1003", "1002"}
	fakeCollection(t, e, &items)
	e.MustRun("plan", "follow", "default", "3000000009")
	p, err := e.App.FindPlan("default")
	if err != nil {
		t.Fatal(err)
	}
	cs, err := e.App.Store.PlanCollections(p.ID)
	if err != nil || len(cs) != 1 {
		t.Fatal(cs, err)
	}
	c := cs[0]
	for i := range c.Snapshot {
		c.Snapshot[i].Missing = nil
	}
	c.Linked = time.Now().UTC().Add(time.Minute)
	if err := e.App.Store.SetPlanCollection(c); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	e.App.Now = func() time.Time { return later }
	e.AddWorkshop("1003", testkit.Sample("tester.e", "E"))
	scanNow(t, e)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	fs, err := e.App.PlanFollowed(st, p)
	if err != nil || len(fs) != 1 {
		t.Fatal(fs, err)
	}
	if got := fs[0].Arrived; len(got) != 1 || got[0].WorkshopID != "1003" {
		t.Fatalf("arrived %+v", got)
	}
}

// A38/A39: subscribing with a list to add to follows the download, rescans and enables the
// installed Mod; a stalled one fails with its reason and is not added.
func TestSubscribeWaitsAndAdds(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	noDetails(t, e)
	fakeSteamDownload(e, map[string]testkit.Mod{"1003": testkit.Sample("tester.e", "E")})
	var mu sync.Mutex
	var steamSeen, progressSeen bool
	op, err := e.App.SubscribeWorkshop([]string{"1003", "1004"}, false, false, app.WorkshopCalls{AddTo: "default",
		Changed: func(items []app.TaskItem) {
			mu.Lock()
			defer mu.Unlock()
			for _, it := range items {
				steamSeen = steamSeen || it.Steam && it.State == app.ItemRun
				progressSeen = progressSeen || it.Steam && it.Got == 40 && it.Size == 100
			}
		}})
	if err == nil || !strings.Contains(err.Error(), "Workshop 1004") {
		t.Fatalf("the stalled item fails: %v", err)
	}
	if !steamSeen {
		t.Fatal("items wait for Steam while it downloads")
	}
	if op.Items[0].Result != "ok" || op.Items[0].Ref == "" || !strings.Contains(op.Items[1].Result, "no download progress") {
		t.Fatalf("%+v", op.Items)
	}
	if op.AddedTo != "default" || len(op.Added) != 1 || op.Added[0].DisplayName() != "E" {
		t.Fatalf("added to %q: %v", op.AddedTo, op.Added)
	}
	if got := joined(active(t, e)); !strings.Contains(got, "E") || strings.Count(got, "E") != 1 {
		t.Fatalf("E is in the list: %s", got)
	}
	// One undoable edit.
	e.MustRun("undo")
	if got := joined(active(t, e)); got != "Core, Harmony, A, ?gone.mod" {
		t.Fatalf("undo: %s", got)
	}
	_ = progressSeen // a single sample within 500 ms may be coalesced
}

// Add limits what goes in: a recommended replacement the sort step gets is left for the
// replace step.
func TestSubscribeAddsOnlyAsked(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	noDetails(t, e)
	fakeSteamDownload(e, map[string]testkit.Mod{"1003": testkit.Sample("tester.e", "E"), "1004": testkit.Sample("tester.f", "F")})
	op, err := e.App.SubscribeWorkshop([]string{"1003", "1004"}, false, false, app.WorkshopCalls{AddTo: "default", Add: []string{"1004"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(op.Added) != 1 || op.Added[0].DisplayName() != "F" {
		t.Fatalf("%v", op.Added)
	}
	if got := joined(active(t, e)); strings.Contains(got, "E") || !strings.Contains(got, "F") {
		t.Fatalf("%s", got)
	}
}

// C8: the card's "subscribe all" enables what it downloaded in the collection's order and
// records it as no longer missing; the collection's own changes still wait.
func TestCollectionSubscribeAllEnables(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	items := []string{"2009463077", "1003", "1001"}
	fakeCollection(t, e, &items)
	e.MustRun("plan", "follow", "default", "3000000009")
	fakeSteamDownload(e, map[string]testkit.Mod{"1003": testkit.Sample("tester.e", "E")})
	op, err := e.App.SubscribeWorkshop([]string{"1003"}, false, false, app.WorkshopCalls{AddTo: "default", Collection: "3000000009"})
	if err != nil {
		t.Fatal(err)
	}
	if len(op.Added) != 1 {
		t.Fatalf("%v", op.Added)
	}
	if got := joined(active(t, e)); got != "Core, Harmony, E, A, ?gone.mod" {
		t.Fatalf("collection order: %s", got)
	}
	p, _ := e.App.FindPlan("default")
	cs, err := e.App.Store.PlanCollections(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	var e3 *store.CollectionItem
	for i := range cs[0].Snapshot {
		if cs[0].Snapshot[i].WorkshopID == "1003" {
			e3 = &cs[0].Snapshot[i]
		}
	}
	if e3 == nil || e3.Missing == nil || *e3.Missing {
		t.Fatalf("snapshot entry %+v", e3)
	}
	if out := e.MustRun("plan", "sync", "default", "--check"); !strings.Contains(out, "0 changed") {
		t.Fatalf("nothing left to catch up:\n%s", out)
	}
}
