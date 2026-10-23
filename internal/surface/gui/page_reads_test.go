package gui

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func stampResponds(t *testing.T, api *API) {
	t.Helper()
	done := make(chan string, 1)
	go func() { done <- api.Stamp() }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("slow page holds the shared API lock")
	}
}

func TestPreviewReadsDoNotHoldSharedLock(t *testing.T) {
	for _, page := range []string{"build", "uninstall"} {
		t.Run(page, func(t *testing.T) {
			e, api, _ := fixture(t)
			api.Board()
			started, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			block := func() { once.Do(func() { close(started) }); <-release }
			if page == "build" {
				now := e.Now()
				e.App.Now = func() time.Time { block(); return now }
			} else {
				e.App.GameProbe = func(string) *app.GameRun { block(); return nil }
			}
			done := make(chan error, 1)
			go func() {
				var err error
				if page == "build" {
					_, err = api.BuildPage(0)
				} else {
					_, err = api.UninstallPlan(view.UninstallChoice{})
				}
				done <- err
			}()
			defer func() {
				close(release)
				select {
				case err := <-done:
					if err != nil {
						t.Error(err)
					}
				case <-time.After(5 * time.Second):
					t.Error("preview did not finish after release")
				}
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("preview did not start")
			}
			stampResponds(t, api)
		})
	}
}

func TestHomeAndNeedsReadersLeaveNavigationResponsive(t *testing.T) {
	for _, page := range []string{"home", "needs"} {
		t.Run(page, func(t *testing.T) {
			e, api, _ := fixture(t)
			e.App.LazyLog, e.App.LogCacheOnly = true, true
			e.App.AnalyzeRunner = func(string, func(string)) error { return errors.New("fixture") }
			if _, err := api.LogPage(""); err != nil {
				t.Fatal(err)
			}
			reader := api.logRead.a.Store.DB
			conn, err := reader.Conn(context.Background()) // hold only the log reader's sole connection
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			before := reader.Stats().WaitCount
			done := make(chan struct{})
			go func() {
				defer close(done)
				if page == "home" {
					api.Home()
				} else {
					api.Needs()
				}
			}()
			deadline := time.Now().Add(3 * time.Second)
			for reader.Stats().WaitCount == before && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if reader.Stats().WaitCount == before {
				t.Fatal("page did not use its independent reader")
			}
			stampResponds(t, api)
			if groups := api.Settings(); len(groups) == 0 {
				t.Fatal("settings blocked or returned no groups")
			}
			conn.Close()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("page did not finish")
			}
			if !e.App.LazyLog || !e.App.LogCacheOnly {
				t.Fatal("page enabled eager parsing on main App")
			}
		})
	}
}

func TestBackgroundLookupUsesOnlyPublishedEnvironment(t *testing.T) {
	_, api, _ := fixture(t)
	st := api.Board().St
	api.bgsEnv = st.Ix.Env.ID
	api.bgs = []app.Background{{Key: "fixture", Src: "fixture.png"}}
	if b := api.bgFind("fixture"); b == nil {
		t.Fatal("published background missing")
	}
	// A stale board must not rebuild another environment's image allow-list during an
	// image request. No folder discovery should occur in bgFind.
	api.bgsEnv = "another environment"
	if b := api.bgFind("fixture"); b != nil {
		t.Fatal("background from previous environment was served")
	}
	if api.bgsEnv != "another environment" {
		t.Fatal("image lookup triggered background discovery")
	}
}
