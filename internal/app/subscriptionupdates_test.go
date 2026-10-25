package app_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestWorkshopSourceCheckKeepsKnownUpdateOnFailure(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Sample("tester.workshop", "Workshop"))
	e.SetTimeUpdated("1001", e.Now().Unix())
	e.AddEnv(false)
	e.MustRun("scan")
	failed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failed {
			http.Error(w, "offline", 503)
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
