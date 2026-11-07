package gui

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestRemovalIOKeepsPagesAndEditorAvailable(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	e.WriteMod(filepath.Join(e.SteamCMD, "7001"), testkit.Sample("tester.remove", "Remove me"))
	e.MustRun("folder", "add", "steamcmd", e.SteamCMD)
	e.MustRun("scan")
	b := api.Board()
	id := b.Cols[view.Active][1].ID
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	e.App.TrashFunc = func(p string) error { close(started); <-release; return os.RemoveAll(p) }
	if _, err := api.LibraryDo(view.OpSubRemove, []string{"pkg:tester.remove"}); err != nil {
		t.Fatal(err)
	}
	awaitInventory(t, started)
	// A separate writer can commit while the recycler is blocked: no SQL write
	// transaction encloses the filesystem operation.
	r, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Store.SetMeta("test:removing", "yes"); err != nil {
		t.Fatal(err)
	}
	workspace := make(chan *EnvironmentResult, 1)
	api.emit = func(name string, data any) {
		if name == "environment" {
			workspace <- data.(*EnvironmentResult)
		}
	}
	if !api.Workspace(1) {
		t.Fatal("workspace not accepted")
	}
	select {
	case result := <-workspace:
		if result.Error != "" || result.Board == nil {
			t.Fatalf("workspace: %+v", result)
		}
	case <-time.After(10 * time.Second): // the blocker is held until released; this only bounds a hang
		t.Fatal("Workspace waits for recycler")
	}
	for name, read := range map[string]func() error{
		"Home": func() error {
			if h := api.Home(); h.Error != "" {
				return errors.New(h.Error)
			}
			return nil
		},
		"Shell": func() error {
			if h := api.Shell(); h.Error != "" {
				return errors.New(h.Error)
			}
			return nil
		},
		"PlanManage": func() error { _, err := api.PlanManage(0); return err },
		"Library":    func() error { _, err := api.Library("", nil, ""); return err },
	} {
		done := make(chan error, 1)
		go func() { done <- read() }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		case <-time.After(10 * time.Second): // the blocker is held until released; this only bounds a hang
			t.Fatalf("%s waits for recycler", name)
		}
	}
	toggleResponds(t, api, id)
	unblock()
	if task := endedTask(t, api, view.OpSubRemove); task.State != app.TaskDone {
		t.Fatalf("task: %+v", task)
	}
}

func TestColdPagesReadDuringWALWriter(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	api.Board() // persist initial workspace, then open a different writer
	w, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	tx, err := w.Store.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO meta(key,value) VALUES('test:writer','held')`); err != nil {
		t.Fatal(err)
	}
	done := make(chan string, 1)
	go func() { h, s := api.Home(), api.Shell(); done <- h.Error + s.Error }()
	select {
	case err := <-done:
		if err != "" {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second): // the blocker is held until released; this only bounds a hang
		t.Fatal("cold readers wait for WAL writer")
	}
}

func TestReadFailuresAreExplicit(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	if _, err := e.App.Store.DB.Exec(`DROP TABLE environment`); err != nil {
		t.Fatal(err)
	}
	if h := api.Home(); h.Error == "" {
		t.Fatal("Home disguised failed read as no game")
	}
	if s := api.Shell(); s.Error == "" {
		t.Fatal("Shell disguised failed read as no lists")
	}
}

func TestAccountReservationDoesNotHoldMutex(t *testing.T) {
	t.Parallel()
	_, api, _ := fixture(t)
	release, err := api.beginAccount()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if !api.accountMu.TryLock() {
		t.Fatal("account mutex held for operation lifetime")
	}
	api.accountMu.Unlock()
	if _, err := api.beginAccount(); err == nil {
		t.Fatal("concurrent account operation accepted")
	}
}

func TestGUIFileBatchUsesAppLifecycle(t *testing.T) {
	t.Parallel()
	for _, unhide := range []bool{false, true} {
		name, kind := "trash", view.LibDelete
		if unhide {
			name, kind = "unhide", view.LibUnhide
		}
		t.Run(name, func(t *testing.T) {
			e, api, _ := fixture(t)
			api.emit = nil
			target := filepath.Join(e.Game, "Mods", "B")
			if unhide {
				if err := os.Rename(filepath.Join(target, "About", "About.xml"), filepath.Join(target, "About", "About.xml.disabled")); err != nil {
					t.Fatal(err)
				}
				e.MustRun("scan")
			}
			ix, err := e.App.Index()
			if err != nil {
				t.Fatal(err)
			}
			mod, err := ix.ResolveOne("pkg:tester.b")
			if err != nil {
				t.Fatal(err)
			}
			game := e.OtherGame("Incoming", "1.6.4633 rev1261")
			e.MustRun("env", "add", "incoming", "--game", game, "--saves", "own")
			link := filepath.Join(game, "Mods", "_Archo_env_"+mod.ID)
			e.Junction(link, target)
			e.App.TrashFunc = os.RemoveAll
			done := 0
			api.batchFiles(kind, []string{ix.Ref(mod.ID)}, app.BatchCalls{
				Stopped: func() bool { return false }, Start: func(int) {},
				Done: func(_ int, err error) {
					if err != nil {
						t.Fatal(err)
					}
					done++
				},
			})
			if done != 1 || source.IsLink(link) != unhide {
				t.Fatalf("GUI bypassed lifecycle: done=%d link=%v", done, source.IsLink(link))
			}
			m, err := e.App.Store.Mod(mod.ID)
			if err != nil || unhide && m.Unloadable != "" || !unhide && m.Status != identity.Tombstone {
				t.Fatalf("GUI index not reconciled: %+v %v", m, err)
			}
			env, _ := e.App.Store.EnvByName("incoming")
			if scan, _, err := e.App.LastScan(env.ID); err != nil || scan == nil {
				t.Fatalf("incoming environment was not scanned: %v", err)
			}
		})
	}
}
