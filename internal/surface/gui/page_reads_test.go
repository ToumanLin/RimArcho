//go:build windows || cgo

package gui

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
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
	t.Parallel()
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
					_, err = buildPage(t, api, 0)
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
	t.Parallel()
	for _, page := range []string{"home", "needs"} {
		t.Run(page, func(t *testing.T) {
			e, api, _ := fixture(t)
			e.App.LazyLog, e.App.LogCacheOnly = true, true
			e.App.AnalyzeRunner = func(*app.App, func(string)) error { return errors.New("fixture") }
			w := &api.launchRead
			if page == "needs" {
				w = &api.needsRead
			}
			api.mu.Lock()
			r, err := api.inventorySession(w)
			api.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			reader := r.Store.DB
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
			deadline := time.Now().Add(20 * time.Second)
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

func TestNeedsDoesNotWaitForLaunchRead(t *testing.T) {
	e, api, _ := fixture(t)
	e.App.AnalyzeRunner = func(*app.App, func(string)) error { return errors.New("fixture") }
	if _, err := api.HomeMemory(); err != nil {
		t.Fatal(err)
	}
	reader := api.launchRead.a.Store.DB
	conn, err := reader.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	launch := make(chan *view.Home, 1)
	needs := make(chan *view.Needs, 1)
	defer func() {
		conn.Close()
		select {
		case <-launch:
		case <-time.After(5 * time.Second):
			t.Error("Launch did not finish after releasing its connection")
		}
	}()
	before := reader.Stats().WaitCount
	go func() { launch <- api.Home() }()
	deadline := time.Now().Add(5 * time.Second)
	for reader.Stats().WaitCount == before {
		if time.Now().After(deadline) {
			t.Fatal("Launch did not reach its database read")
		}
		time.Sleep(time.Millisecond)
	}

	start := time.Now()
	go func() { needs <- api.Needs() }()
	select {
	case n := <-needs:
		t.Logf("Can it run returned after %s with Launch still blocked", time.Since(start))
		if n.Error != "" || n.Plan == 0 {
			t.Fatalf("Can it run did not return list findings: %+v", n)
		}
	case <-time.After(time.Second):
		stacks := make([]byte, 64<<10)
		n := runtime.Stack(stacks, true)
		t.Errorf("Can it run still waiting after %s:\n%s", time.Since(start), stacks[:n])
		conn.Close()
		select {
		case <-needs:
			t.Logf("Can it run returned after %s once Launch was released", time.Since(start))
		case <-time.After(5 * time.Second):
			t.Error("Can it run did not finish after releasing Launch")
		}
	}
}

func TestNeedsReturnsWhileAnalysisHoldsWriteLock(t *testing.T) {
	e, api, _ := fixture(t)
	e.App.LazyLog, e.App.LogCacheOnly = true, true
	e.RecordLaunch(e.Now().Add(-time.Minute))
	e.WriteFile(filepath.Join(e.UserData, "Player.log"), "RimWorld 1.6.4633 rev1\n\nCould not load Texture2D at 'Things/Alpha'\n\n")
	writer, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	started, release := make(chan error, 1), make(chan struct{})
	defer func() {
		close(release)
		api.needsJobs.Wait()
	}()
	analyze := e.App.AnalyzeRunner
	e.App.AnalyzeRunner = func(a *app.App, progress func(string)) error {
		tx, err := writer.Store.DB.Begin()
		started <- err
		if err != nil {
			return err
		}
		<-release
		if err := tx.Commit(); err != nil {
			return err
		}
		return analyze(a, progress)
	}
	if !api.Analyze() {
		t.Fatal("did not start the check")
	}
	select {
	case err := <-started:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("check did not acquire the write lock")
	}
	start := time.Now()
	done := make(chan *view.Needs, 1)
	go func() { done <- api.Needs() }()
	select {
	case n := <-done:
		t.Logf("Can it run returned after %s with the analysis write lock held", time.Since(start))
		if n.Error != "" || n.Plan == 0 || !n.Updating || !n.LastRunPending {
			t.Fatalf("wanted stored findings and a pending log, got plan %d, updating %v, pending %v, error %q", n.Plan, n.Updating, n.LastRunPending, n.Error)
		}
	case <-time.After(time.Second):
		t.Error("Can it run waited behind the analysis write lock")
	}
}

func TestBackgroundLookupUsesOnlyPublishedEnvironment(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	st := api.Board().St
	api.bgsGame = bgGame(st.Ix.Env)
	api.bgs = []app.Background{{Key: "fixture", Src: "fixture.png"}}
	if b := api.bgFind("fixture"); b == nil {
		t.Fatal("published background missing")
	}
	// Another Game Config of the same game shows the same list: it is kept per game
	// installation, so switching configs does not read the backgrounds again.
	e.MustRun("env", "add", "sibling", "--like", "main")
	if b := api.bgFind("fixture"); b == nil {
		t.Fatal("a Game Config of the same game lost the published backgrounds")
	}
	e.MustRun("env", "use", "main")
	// A stale board must not rebuild another environment's image allow-list during an
	// image request. No folder discovery should occur in bgFind.
	api.bgsGame = "another game"
	if b := api.bgFind("fixture"); b != nil {
		t.Fatal("background from previous Game Config was served")
	}
	if api.bgsGame != "another game" {
		t.Fatal("image lookup triggered background discovery")
	}
}
