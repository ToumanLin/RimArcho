package gui

import (
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// Block the first reader clock call; subsequent calls (including editor writes)
// remain usable. Library calls Now only after it has read the current list.
func pauseInventory(t *testing.T, api *API) (<-chan struct{}, func()) {
	t.Helper()
	started, release := make(chan struct{}), make(chan struct{})
	var first atomic.Bool
	now := api.a.Now()
	api.a.Now = func() time.Time {
		if first.CompareAndSwap(false, true) {
			close(started)
			<-release
		}
		return now
	}
	var released atomic.Bool
	unblock := func() {
		if released.CompareAndSwap(false, true) {
			close(release)
		}
	}
	t.Cleanup(unblock)
	return started, unblock
}

func awaitInventory(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("reader did not start")
	}
}

func toggleResponds(t *testing.T, api *API, id string) {
	t.Helper()
	done := make(chan error, 1)
	go func() { _, err := api.Toggle(int(view.Active), []string{id}, nil); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second): // the slow read is held until released; this only bounds a hang
		t.Fatal("editor waits for a slow read")
	}
}

func TestInventoryBuildDoesNotBlockEditsOrPublishStaleCache(t *testing.T) {
	t.Parallel()
	_, api, _ := fixture(t)
	b := api.Board()
	id := b.Cols[view.Active][1].ID
	started, release := pauseInventory(t, api)
	done := make(chan error, 1)
	go func() { _, err := api.Library("", nil, ""); done <- err }()
	awaitInventory(t, started)
	toggleResponds(t, api, id)
	// A full inventory in flight must not delay a one-copy details read either.
	copyDone := make(chan error, 1)
	go func() { _, err := api.LibraryCopy(id); copyDone <- err }()
	select {
	case err := <-copyDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("copy detail waits for full library")
	}
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if api.lib != nil {
		t.Fatal("old inventory was cached under a newer edit")
	}
	row, err := api.LibraryCopy(id)
	if err != nil || row == nil || row.InPlan {
		t.Fatalf("copy did not reflect edit: %+v, %v", row, err)
	}
}

func TestCopyLibraryBuildDoesNotBlockToggle(t *testing.T) {
	t.Parallel()
	_, api, _ := fixture(t)
	id := api.Board().Cols[view.Active][1].ID
	started, release := pauseInventory(t, api)
	done := make(chan error, 1)
	go func() { _, err := api.LibraryCopy(id); done <- err }()
	awaitInventory(t, started)
	toggleResponds(t, api, id)
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	row, err := api.LibraryCopy(id)
	if err != nil || row == nil || row.InPlan {
		t.Fatalf("stale copy cache: %+v, %v", row, err)
	}
}

func TestConcurrentLibraryReadsCoalesce(t *testing.T) {
	t.Parallel()
	_, api, _ := fixture(t)
	api.Board()
	started, release := pauseInventory(t, api)
	done := make(chan error, 2)
	libs := make(chan any, 2)
	for range 2 {
		go func() { lib, err := api.library(); libs <- lib; done <- err }()
	}
	awaitInventory(t, started)
	release()
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if first, second := <-libs, <-libs; first != second {
		t.Fatal("concurrent reads rebuilt the same inventory")
	}
}

func TestWorkspaceReadOvertakenByEditIsStale(t *testing.T) {
	t.Parallel()
	_, api, events := fixture(t)
	id := api.Board().Cols[view.Active][1].ID
	started, release := pauseInventory(t, api)
	api.Workspace(1)
	awaitInventory(t, started)
	toggleResponds(t, api, id)
	release()
	if r := environmentResult(t, events, 1); !r.Stale {
		t.Fatal("old workspace was published after an edit")
	}
	api.mu.Lock()
	st := api.boardState()
	api.mu.Unlock()
	if st.List.Index(id) >= 0 {
		t.Fatal("old workspace overwrote the editor state")
	}
	api.Workspace(2)
	if r := environmentResult(t, events, 2); r.Stale || r.Error != "" {
		t.Fatalf("fresh workspace: %+v", r)
	}
}

func TestScopedGUIReadsMatchFullLibraryAfterEdits(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	b := api.Board()
	id := b.Cols[view.Active][1].ID
	for step := range 3 {
		if step > 0 {
			col := view.Active
			if step == 2 {
				col = view.Inactive
			}
			if _, err := api.Toggle(int(col), []string{id}, nil); err != nil {
				t.Fatal(err)
			}
		}
		lib, err := e.App.Library()
		if err != nil {
			t.Fatal(err)
		}
		i := lib.Instance(id)
		data := view.LoadDetail(e.App, lib.St.Ix, i.Mod, false, false)
		wantFields := view.LibFields(e.App, lib, i, data)
		fields, err := api.LibraryFields(id)
		if err != nil || !reflect.DeepEqual(fields, wantFields) {
			t.Fatalf("fields differ after edit %d: %v", step, err)
		}
		for _, l := range lib.Lineages {
			for _, i := range l.Instances {
				refs := []string{i.Ref}
				if i.Mod != nil {
					refs = append(refs, i.Mod.ID)
				}
				for _, ref := range refs {
					got, err := api.LibraryCopy(ref)
					want := view.LibCopy(lib, ref)
					if err != nil || !reflect.DeepEqual(got, want) {
						t.Fatalf("copy %s: %+v, %v; want %+v", ref, got, err, want)
					}
					if i.Mod != nil {
						if got, want := api.RowOps(int(view.Active), ref), view.RowOps(lib, ref, view.Active); !reflect.DeepEqual(got, want) {
							t.Fatalf("row ops: %v != %v", got, want)
						}
					}
				}
			}
		}
	}
}

func TestCursorAndFilterReadsDoNotBlockToggle(t *testing.T) {
	t.Parallel()
	for _, page := range []string{"detail", "filter"} {
		t.Run(page, func(t *testing.T) {
			_, api, _ := fixture(t)
			id := api.Board().Cols[view.Active][1].ID
			started, release := pauseInventory(t, api)
			done := make(chan error, 1)
			go func() {
				var err error
				if page == "detail" {
					_, err = api.Detail(id, false, false, false)
				} else {
					_, err = api.BarFilter("", nil, true)
				}
				done <- err
			}()
			awaitInventory(t, started)
			toggleResponds(t, api, id)
			release()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
