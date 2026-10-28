package app_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestWorkshopSourceCheckKeepsKnownUpdateOnFailure(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Sample("tester.workshop", "Workshop"))
	e.SetTimeUpdated("1001", e.Now().Unix())
	e.AddEnv(false)
	e.MustRun("scan")
	failed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failed {
			http.Error(w, "offline", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintf(w, `{"response":{"publishedfiledetails":[{"publishedfileid":"1001","result":1,"time_updated":%d}]}}`, e.Now().Add(time.Hour).Unix())
	}))
	defer server.Close()
	e.App.SteamAPI = server.URL
	if err := e.App.CheckSubscriptionSource(app.LibWorkshop); err != nil {
		t.Fatal(err)
	}
	before := e.App.SubscriptionSourceCheck(app.LibWorkshop)
	if len(before.Latest) != 1 || before.Result != "check complete" {
		t.Fatalf("check: %+v", before)
	}
	failed = true
	e.Advance(time.Hour)
	if err := e.App.CheckSubscriptionSource(app.LibWorkshop); err == nil {
		t.Fatal("failure hidden")
	}
	after := e.App.SubscriptionSourceCheck(app.LibWorkshop)
	for id, latest := range before.Latest {
		if after.Latest[id] != latest {
			t.Fatal("failed check discarded known update")
		}
	}
	if !after.At.After(before.At) || len(after.Failures) != 1 || !strings.Contains(after.Failures[0].Reason, "503") {
		t.Fatalf("failure status: %+v", after)
	}
	// The dry run cannot start Steam; execution sends only the requested item.
	calls := 0
	e.WriteFile(filepath.Join(e.Game, "steam_api64.dll"), "fixture DLL")
	e.App.SteamWorker = func(_ string, argv []string) (string, error) {
		calls++
		if strings.Join(argv, " ") != "download 1001" {
			t.Fatalf("download arguments: %v", argv)
		}
		return "ITEM 1001 ok 8\nDONE download 1\n", nil
	}
	if err := e.App.UpdateWorkshopClient("ws:1001", true); err != nil || calls != 0 {
		t.Fatalf("preview: %v calls %d", err, calls)
	}
	if err := e.App.UpdateWorkshopClient("ws:1001", false); err != nil || calls != 1 {
		t.Fatalf("download: %v calls %d", err, calls)
	}
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42} }
	if err := e.App.UpdateWorkshopClient("ws:1001", false); err == nil || calls != 1 {
		t.Fatal("game-running guard did not refuse before contacting Steam")
	}
}

// A big library is checked 100 items per request. A failed request and an item Steam
// was too busy for are asked once more; items gone from the Workshop are listed but do
// not fail the check (the host had 5182 mods and two-minute checks that always failed).
func TestWorkshopSourceCheckBatches(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	const n = 230
	for i := 0; i < n; i++ {
		id := fmt.Sprint(5000 + i)
		e.AddWorkshop(id, testkit.Sample("tester.ws"+id, "Workshop "+id))
		e.SetTimeUpdated(id, e.Now().Unix())
	}
	e.AddEnv(false)
	e.MustRun("scan")
	var mu sync.Mutex
	requests, failedOnce, busyOnce := 0, false, false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		mu.Lock()
		defer mu.Unlock()
		requests++
		count, _ := strconv.Atoi(r.Form.Get("itemcount"))
		if count < 1 || count > 100 {
			t.Errorf("itemcount %d", count)
		}
		ids := make([]string, count)
		for i := range ids {
			ids[i] = r.Form.Get(fmt.Sprintf("publishedfileids[%d]", i))
		}
		if slices.Contains(ids, "5150") && !failedOnce {
			failedOnce = true
			http.Error(w, "busy", http.StatusBadGateway)
			return
		}
		var out []string
		for _, id := range ids {
			switch {
			case id == "5007":
				out = append(out, fmt.Sprintf(`{"publishedfileid":%q,"result":9}`, id))
			case id == "5008" && !busyOnce:
				busyOnce = true
				out = append(out, fmt.Sprintf(`{"publishedfileid":%q,"result":16}`, id))
			case id == "5009": // Steam leaves it out
			default:
				out = append(out, fmt.Sprintf(`{"publishedfileid":%q,"result":1,"time_updated":%d}`, id, e.Now().Add(time.Hour).Unix()))
			}
		}
		fmt.Fprintf(w, `{"response":{"publishedfiledetails":[%s]}}`, strings.Join(out, ","))
	}))
	defer server.Close()
	e.App.SteamAPI = server.URL
	err := e.App.CheckSubscriptionSource(app.LibWorkshop)
	c := e.App.SubscriptionSourceCheck(app.LibWorkshop)
	// 3 batches, the failed one again, then the busy item again.
	if requests != 5 || len(c.Latest) != n-2 {
		t.Fatalf("requests %d latest %d", requests, len(c.Latest))
	}
	if err == nil || len(c.Failures) != 2 || app.CheckFailures(c.Failures) != 1 || c.Result != "some checks failed" {
		t.Fatalf("err %v check %+v", err, c.Failures)
	}
	for _, f := range c.Failures {
		if f.ID == "5007" && !f.Gone() || f.ID == "5009" && f.Reason != "Steam returned no details" {
			t.Fatalf("failure %+v", f)
		}
	}
	// Only gone items left: the check is complete.
	e.Remove(filepath.Join(e.Workshop, "5009"))
	e.MustRun("scan")
	if err := e.App.CheckSubscriptionSource(app.LibWorkshop); err != nil {
		t.Fatal(err)
	}
	if c := e.App.SubscriptionSourceCheck(app.LibWorkshop); c.Result != "check complete" || len(c.Failures) != 1 || !c.Failures[0].Gone() {
		t.Fatalf("gone only: %+v", c)
	}
}
