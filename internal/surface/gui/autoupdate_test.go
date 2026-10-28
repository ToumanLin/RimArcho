package gui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestAutomaticSlowNetworkDoesNotBlockSettingsAndCloseCancels(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	for _, kind := range []string{"mods", "tools"} {
		if _, err := e.App.RunAutomatic(context.Background(), kind, func(context.Context) (int, string, error) { return 0, "check complete", nil }); err != nil {
			t.Fatal(err)
		}
	}
	entered := make(chan struct{}, 3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { entered <- struct{}{}; <-r.Context().Done() }))
	defer srv.Close()
	for _, ds := range []string{"rules", "steamdb", "replacements"} {
		e.MustRun("config", "set", "data."+ds+".url", srv.URL+"/"+ds)
	}
	api := newAPI(e.App)
	api.autoWG.Add(1)
	done := make(chan struct{})
	go func() { defer api.autoWG.Done(); defer close(done); api.automaticCycle(api.branchCtx) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		api.Close()
		t.Fatal("check did not start")
	}
	settings := make(chan int, 1)
	go func() { settings <- len(api.Settings()) }()
	select {
	case n := <-settings:
		if n == 0 {
			t.Error("settings unavailable")
		}
	case <-time.After(time.Second):
		api.Close()
		t.Fatal("network held page lock")
	}
	api.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("close left a network worker")
	}
}
