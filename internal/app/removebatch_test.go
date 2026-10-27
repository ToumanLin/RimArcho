package app_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestRemovalCommitWaitsForConcurrentWriter(t *testing.T) {
	e := setup(t, true)
	e.WriteMod(filepath.Join(e.SteamCMD, "7709"), testkit.Sample("remove.writer", "Writer"))
	scanNow(t, e)
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	m, err := ix.ResolveOne("pkg:remove.writer")
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	tx, err := e.App.Store.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO meta(key,value) VALUES('test:removal-writer','held')`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- r.Store.RecordRemoved(m.ID, m.RealPath, e.Now()) }()
	select {
	case err := <-done:
		t.Fatalf("removal did not wait for writer: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("removal did not resume after writer")
	}
	m, err = r.Store.Mod(m.ID)
	if err != nil || m.Status != identity.Tombstone {
		t.Fatalf("removal: %+v %v", m, err)
	}
}

func TestSubscriptionRemovalCommitsBeforeCancellation(t *testing.T) {
	e := setup(t, true)
	for _, id := range []string{"7701", "7702"} {
		e.WriteMod(filepath.Join(e.SteamCMD, id), testkit.Sample("remove."+id, "Remove "+id))
	}
	scanNow(t, e)
	ss, err := e.App.Subscriptions()
	if err != nil || len(ss) != 2 {
		t.Fatalf("subscriptions: %v %v", ss, err)
	}
	stopped, done := false, 0
	e.App.TrashFunc = os.RemoveAll
	_, err = e.App.RemoveSubscriptionsWith([]string{ss[0].Ref, ss[1].Ref}, app.RemoveSubscriptionOptions{}, app.BatchCalls{
		Stopped: func() bool { return stopped },
		Done: func(i int, err error) {
			if err != nil {
				t.Fatal(err)
			}
			r, err := e.App.ReadSession()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			m, err := r.Store.Mod(ss[i].Mod.ID)
			if err != nil || m.Status != identity.Tombstone {
				t.Fatalf("Done before commit: %+v %v", m, err)
			}
			done++
			stopped = true
		},
	})
	if err != nil || done != 1 {
		t.Fatalf("removal: %d %v", done, err)
	}
	if _, err := os.Stat(ss[1].Folder); err != nil {
		t.Fatalf("cancelled item touched: %v", err)
	}
}

func TestParallelRemovalKeepsValidItemsAndFastSkipsRecycler(t *testing.T) {
	e := setup(t, true)
	path := filepath.Join(e.SteamCMD, "7703")
	e.WriteMod(path, testkit.Sample("remove.fast", "Fast"))
	scanNow(t, e)
	e.App.TrashFunc = func(string) error { t.Error("fast removal called recycle bin"); return nil }
	op, err := e.App.RemoveSubscriptionsWith([]string{"pkg:remove.fast", "unknown:bad"}, app.RemoveSubscriptionOptions{Permanent: true, Workers: 4}, app.BatchCalls{})
	if err == nil || op == nil || op.Items[0].Result != "permanently deleted" {
		t.Fatalf("mixed batch: %+v %v", op, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("fast item remains: %v", err)
	}
}

func TestParallelRemovalWithIncomingLinks(t *testing.T) {
	e := setup(t, true)
	for _, id := range []string{"7711", "7712"} {
		e.WriteMod(filepath.Join(e.SteamCMD, id), testkit.Sample("linked."+id, "Linked "+id))
	}
	scanNow(t, e)
	ss, err := e.App.Subscriptions()
	if err != nil || len(ss) != 2 {
		t.Fatalf("subscriptions: %v %v", ss, err)
	}
	var refs []string
	for _, s := range ss {
		if err := source.CreateLink(filepath.Join(e.Game, "Mods", "_Archo_env_"+s.Mod.ID), s.Folder); err != nil {
			t.Fatal(err)
		}
		refs = append(refs, s.Ref)
	}
	e.App.TrashFunc = os.RemoveAll
	op, err := e.App.RemoveSubscriptionsWith(refs, app.RemoveSubscriptionOptions{Workers: 4}, app.BatchCalls{})
	if err != nil || op == nil {
		t.Fatalf("linked batch: %+v %v", op, err)
	}
	for _, s := range ss {
		if source.IsLink(filepath.Join(e.Game, "Mods", "_Archo_env_"+s.Mod.ID)) {
			t.Fatal("incoming link remains")
		}
	}
}

func TestTrashAndUnhideReconcileIncomingEnvironments(t *testing.T) {
	for _, unhide := range []bool{false, true} {
		name := "trash"
		if unhide {
			name = "unhide"
		}
		t.Run(name, func(t *testing.T) {
			e, target, id, links := incomingFixture(t)
			if unhide {
				hide(e, target)
			}
			envs, err := e.App.Store.Envs()
			if err != nil {
				t.Fatal(err)
			}
			for _, env := range envs {
				e.App.EnvName = env.Name
				if _, err := e.App.ScanWith(app.ScanOptions{GameOnly: true}); err != nil {
					t.Fatal(err)
				}
			}
			e.App.EnvName = "main"
			e.App.TrashFunc = os.RemoveAll
			if unhide {
				_, err = e.App.Unhide("#"+id, false)
			} else {
				_, err = e.App.Trash("#"+id, false)
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, link := range links {
				if source.IsLink(link) != unhide {
					t.Fatalf("link state after %s: %s", name, link)
				}
			}
			for _, env := range envs {
				mods, err := e.App.Store.Mods(env.ID)
				if err != nil {
					t.Fatal(err)
				}
				for _, m := range mods {
					if m.RealPath == target && (unhide && m.Unloadable != "" || !unhide && m.Status == identity.Live) {
						t.Fatalf("stale %s entry in %s: %+v", name, env.Name, m)
					}
				}
			}
		})
	}
}
