//go:build windows || cgo

package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func actionLockFixture(t *testing.T) *testkit.Env {
	t.Helper()
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("scan")
	if _, err := e.App.Loadout(); err != nil {
		t.Fatal(err)
	}
	return e
}

// Hold the slow operation until the read returns. The timer releases it on a
// regression, so the test can measure the blocked call and still join the writer.
func measureConcurrentRead(t *testing.T, run func() error, entered <-chan struct{}, release func(), read func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- run() }()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("operation ended before reaching the gate: %v", err)
	case <-time.After(5 * time.Second):
		release()
		<-done
		t.Fatal("operation did not reach the gate")
	}
	timer := time.AfterFunc(600*time.Millisecond, release)
	start := time.Now()
	err := read()
	elapsed := time.Since(start)
	timer.Stop()
	release()
	if runErr := <-done; runErr != nil {
		t.Errorf("operation: %v", runErr)
	}
	t.Logf("concurrent API response: %.3f ms", float64(elapsed)/float64(time.Millisecond))
	if err != nil || elapsed >= 300*time.Millisecond {
		t.Errorf("concurrent API did not return within 300 ms: %v, %v", elapsed, err)
	}
}

func actionGate() (hold func(), entered <-chan struct{}, release func()) {
	atGate, released := make(chan struct{}), make(chan struct{})
	var first, finish sync.Once
	return func() {
		first.Do(func() { close(atGate) })
		<-released
	}, atGate, func() { finish.Do(func() { close(released) }) }
}

func TestLaunchActionsLeaveLiveAvailable(t *testing.T) {
	for _, action := range []string{"Run", "RunAnyway", "RunMoving", "Preview"} {
		t.Run(action, func(t *testing.T) {
			e := actionLockFixture(t)
			hold, entered, release := actionGate()
			e.App.GameProbe = func(string) *app.GameRun { hold(); return nil }
			api := NewAPI(e.App, nil)
			t.Cleanup(api.Close)
			run := func() error {
				if action == "Preview" {
					p, err := api.Preview(view.ListLaunch)
					if err != nil || p == nil || !p.Apply {
						return fmt.Errorf("launch preview %+v: %v", p, err)
					}
					return nil
				}
				f := map[string]func(string) (*view.Outcome, error){
					"Run": api.Run, "RunAnyway": api.RunAnyway, "RunMoving": api.RunMoving,
				}[action]
				o, err := f(view.ListLaunch)
				if err != nil || o == nil || len(o.Refused)+len(o.Confirm)+len(o.Moves) != 0 {
					return fmt.Errorf("launch outcome %+v: %v", o, err)
				}
				return nil
			}
			measureConcurrentRead(t, run, entered, release, func() error {
				_, err := api.Live()
				return err
			})
		})
	}
}

func TestColdLogsDuringLaunchLock(t *testing.T) {
	e := actionLockFixture(t)
	e.WriteFile(filepath.Join(e.UserData, "Player.log"), "synthetic game log")
	if err := e.App.Store.SetMeta("test:cold-loadout", "1"); err != nil {
		t.Fatal(err)
	}
	r, err := e.ChildApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	hold, entered, release := actionGate()
	r.GameProbe = func(string) *app.GameRun { hold(); return nil }
	api := NewAPI(e.App, nil)
	t.Cleanup(api.Close)
	measureConcurrentRead(t, func() error {
		_, err := r.Launch(true)
		return err
	}, entered, release, func() error {
		if got := api.Logs(); len(got) == 0 {
			return fmt.Errorf("missing synthetic game log")
		}
		return nil
	})
}

func TestSaveBackupLeavesSettingsAvailable(t *testing.T) {
	e := actionLockFixture(t)
	now := e.Now()
	e.WriteSave("Colony.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}, ModTime: now.Add(-time.Hour)})
	list, err := e.App.ListSaves()
	if err != nil {
		t.Fatal(err)
	}
	hold, entered, release := actionGate()
	api := NewAPI(e.App, nil)
	t.Cleanup(api.Close)
	api.saves.mu.Lock()
	api.saves.list = list
	api.saves.mu.Unlock()
	p, err := api.SaveBackup([]string{"Colony"}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	api.mu.Lock()
	var backupClock sync.Once
	e.App.Now = func() time.Time {
		first := false
		backupClock.Do(func() { first = true })
		if first {
			hold()
		}
		return now
	}
	api.mu.Unlock()
	measureConcurrentRead(t, func() error {
		r, err := api.SaveBackup([]string{"Colony"}, true, p.Preview.Token)
		if err != nil || r == nil || r.Failed {
			return fmt.Errorf("backup %+v: %v", r, err)
		}
		return nil
	}, entered, release, func() error {
		if len(api.Settings()) == 0 {
			return fmt.Errorf("missing settings")
		}
		return nil
	})
	files, err := filepath.Glob(filepath.Join(e.Home, "backups", "saves", "*", "Colony.rws"))
	if err != nil || len(files) != 1 {
		t.Fatalf("backup files %v: %v", files, err)
	}
	if info, err := os.Stat(files[0]); err != nil || info.Size() == 0 {
		t.Fatalf("backup file %v: %v", info, err)
	}
}

func TestSaveTrashLeavesStampAvailable(t *testing.T) {
	e := actionLockFixture(t)
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	e.WriteSave("Colony.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}, ModTime: e.Now().Add(-time.Hour)})
	list, err := e.App.ListSaves()
	if err != nil {
		t.Fatal(err)
	}
	hold, entered, release := actionGate()
	e.App.TrashFunc = func(path string) error {
		hold()
		return e.Recycle(path)
	}
	api := NewAPI(e.App, nil)
	t.Cleanup(api.Close)
	api.saves.mu.Lock()
	api.saves.list = list
	api.saves.mu.Unlock()
	p, err := api.SaveTrash([]string{"Colony"}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	measureConcurrentRead(t, func() error {
		r, err := api.SaveTrash([]string{"Colony"}, true, p.Preview.Token)
		if err != nil || r == nil || r.Failed {
			return fmt.Errorf("recycle %+v: %v", r, err)
		}
		return nil
	}, entered, release, func() error {
		if api.Stamp() == "" {
			return fmt.Errorf("missing database stamp")
		}
		return nil
	})
}

func TestGameActionKeepsAutomaticSaveCopy(t *testing.T) {
	e := actionLockFixture(t)
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	e.WriteSave("Colony.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}, ModTime: e.Now().Add(-time.Hour)})
	api := NewAPI(e.App, nil)
	o, err := api.RunAnyway(view.ListLaunch)
	api.Close()
	e.App.WaitLaunchCopies()
	if err != nil || o == nil || len(o.Refused) != 0 || len(e.Launched) != 1 {
		t.Fatalf("launch outcome %+v: %v, stub calls %v", o, err, e.Launched)
	}
	markers, err := filepath.Glob(filepath.Join(e.Home, "backups", "saves", "*", "rimarcho-auto.json"))
	if err != nil || len(markers) != 1 {
		t.Fatalf("automatic copy marker %v: %v", markers, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(markers[0]), "Colony.rws")); err != nil {
		t.Fatal(err)
	}
}

func TestEditorWaitsForBriefWorkspaceLock(t *testing.T) {
	e, api, _ := fixture(t)
	e.MustRun("env", "set", "main", "--launch", "direct")
	r, err := e.ChildApp()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	holdLaunch, entered, release := actionGate()
	r.GameProbe = func(string) *app.GameRun { holdLaunch(); return nil }
	launchDone := make(chan error, 1)
	go func() {
		_, err := r.Launch(true)
		launchDone <- err
	}()
	select {
	case <-entered:
	case err := <-launchDone:
		t.Fatalf("lock holder ended before reaching the gate: %v", err)
	case <-time.After(5 * time.Second):
		release()
		<-launchDone
		t.Fatal("lock holder did not reach the gate")
	}
	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := api.Toggle(int(view.Inactive), nil, []string{"pkg:tester.b"})
		done <- err
	}()
	time.Sleep(75 * time.Millisecond)
	hold := time.Since(start)
	release()
	if err := <-launchDone; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("edit did not finish after release")
	}
	t.Logf("hold %.3f ms, edit %.3f ms", hold.Seconds()*1000, time.Since(start).Seconds()*1000)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if st.List.Index(st.Reg.Resolve("tester.b").Instance) < 0 {
		t.Fatal("Beta was not enabled")
	}
}

func TestEditorWaitDoesNotBlockSettings(t *testing.T) {
	e, api, _ := fixture(t)
	r, err := e.ChildApp()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	hold, entered, release := actionGate()
	r.GameProbe = func(string) *app.GameRun { hold(); return nil }
	e.MustRun("env", "set", "main", "--launch", "direct")
	var editDone chan error
	measureConcurrentRead(t, func() error {
		_, err := r.Launch(true)
		return err
	}, entered, release, func() error {
		editDone = make(chan error, 1)
		go func() {
			_, err := api.Toggle(int(view.Inactive), nil, []string{"pkg:tester.b"})
			editDone <- err
		}()
		select {
		case err := <-editDone:
			editDone = nil
			return fmt.Errorf("edit finished before the lock was released: %v", err)
		case <-time.After(60 * time.Millisecond):
		}
		start := time.Now()
		if len(api.Settings()) == 0 {
			return fmt.Errorf("missing settings")
		}
		t.Logf("Settings while edit waits: %.3f ms", time.Since(start).Seconds()*1000)
		return nil
	})
	if editDone != nil {
		select {
		case err := <-editDone:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("edit did not resume")
		}
	}
}

func TestEditorRefusesDuringWindowLaunch(t *testing.T) {
	e := actionLockFixture(t)
	hold, entered, release := actionGate()
	e.App.GameProbe = func(string) *app.GameRun { hold(); return nil }
	api := NewAPI(e.App, nil)
	t.Cleanup(api.Close)
	measureConcurrentRead(t, func() error {
		o, err := api.RunAnyway(view.ListLaunch)
		if err != nil || o == nil || len(o.Refused) != 0 {
			return fmt.Errorf("launch outcome %+v: %v", o, err)
		}
		return nil
	}, entered, release, func() error {
		start := time.Now()
		_, err := api.Toggle(int(view.Active), nil, []string{"pkg:ludeon.rimworld"})
		t.Logf("edit reply during launch: %.3f ms", time.Since(start).Seconds()*1000)
		if err == nil || err.Error() != view.T("RimArcho is preparing a launch. Try editing the list again after the launch finishes.") {
			return fmt.Errorf("expected launch-specific refusal, got %v", err)
		}
		_, err = api.Live()
		return err
	})
}
