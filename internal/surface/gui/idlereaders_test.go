package gui

import (
	"runtime"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// The launch page asks for its memory comparison every 30 seconds. An unchanged refresh
// reuses the launch page's reader instead of rebuilding the index and list on a new
// connection (07 §8.3).
func TestHomeMemoryReusesLaunchReader(t *testing.T) {
	e, api, _ := fixture(t)
	api.Board()
	first, err := api.HomeMemory()
	if err != nil || first == nil {
		t.Fatalf("%+v %v", first, err)
	}
	reader := api.logRead.a
	if reader == nil || reader == e.App {
		t.Fatal("HomeMemory did not read on the launch page's reader")
	}
	perCall := func(f func()) uint64 {
		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		for range 10 {
			f()
		}
		runtime.ReadMemStats(&b)
		return (b.TotalAlloc - a.TotalAlloc) / 10
	}
	warm := perCall(func() {
		m, err := api.HomeMemory()
		if err != nil || m.Token != first.Token {
			t.Fatalf("%+v %v", m, err)
		}
	})
	if api.logRead.a != reader {
		t.Fatal("the launch reader was replaced")
	}
	// What each refresh used to do: a fresh session that reads the whole list.
	cold := perCall(func() {
		r, err := api.detachedReader()
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		r.WorkspaceSnapshot()
		r.OpenPlan, r.PlanName = 0, ""
		st, err := r.Loadout()
		if err != nil {
			t.Fatal(err)
		}
		view.BuildMemoryLaunch(r, st)
	})
	if warm*3 > cold {
		t.Fatalf("an unchanged refresh allocates %d bytes, a cold read %d", warm, cold)
	}
	// Other pages on the same reader still see the opened list, not the launch page's.
	h := api.Home()
	if h.Error != "" || h.Rows[0].Memory == nil || h.Rows[0].Memory.Token != first.Token {
		t.Fatalf("home %+v", h)
	}
}

func TestIdlePageReadersClose(t *testing.T) {
	e, api, _ := fixture(t)
	api.Board()
	if _, err := api.Library("", nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := api.PlanManage(0); err != nil {
		t.Fatal(err)
	}
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	id := ix.Mods[0].ID
	if _, err := api.Detail(id, false, false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := api.LibraryCopy(id); err != nil {
		t.Fatal(err)
	}
	readers := map[string]*logReader{"library": &api.libRead, "plans": &api.plansRead, "detail": &api.detailRead, "index": &api.indexRead}
	for name, w := range readers {
		if w.a == nil || w.timer == nil {
			t.Fatalf("%s reader not retained with an expiry", name)
		}
	}
	if api.copyRead.a == nil || api.copyRead.timer == nil {
		t.Fatal("copy reader not retained with an expiry")
	}
	if api.workspaceReader.timer != nil || api.logRead.timer != nil {
		t.Fatal("the editor and log readers must never expire: the window publishes their objects")
	}

	// A callback that waited behind a newer read keeps the reader.
	for name, w := range readers {
		api.expireReader(w)
		if w.a == nil {
			t.Fatalf("recently used %s reader closed", name)
		}
	}
	api.expireCopy()
	if api.copyRead.a == nil {
		t.Fatal("recently used copy reader closed")
	}

	idle := func(u *idleUse) { u.last = time.Now().Add(-readerIdle) }
	for name, w := range readers {
		w.mu.Lock()
		idle(&w.idleUse)
		w.mu.Unlock()
		api.expireReader(w)
		if w.a != nil || w.timer != nil {
			t.Fatalf("idle %s reader kept its connection", name)
		}
	}
	if api.lib != nil {
		t.Fatal("the library built on the closed reader is still cached")
	}
	api.copyRead.mu.Lock()
	idle(&api.copyRead.idleUse)
	api.copyRead.mu.Unlock()
	api.expireCopy()
	if api.copyRead.a != nil || api.copyRead.lib != nil {
		t.Fatal("idle copy reader kept its connection")
	}

	// The next visit reads again on new connections, with current data.
	e.MustRun("disable", "pkg:tester.a")
	lib, err := api.Library("", nil, "")
	if err != nil || len(lib.Rows) == 0 {
		t.Fatalf("%+v %v", lib, err)
	}
	if _, err := api.PlanManage(0); err != nil {
		t.Fatal(err)
	}
	if d, err := api.Detail(id, false, false, false); err != nil || d.ID != id {
		t.Fatalf("%+v %v", d, err)
	}
	if api.libRead.a == nil || api.plansRead.a == nil || api.detailRead.a == nil || api.indexRead.a == nil {
		t.Fatal("readers not reopened")
	}

	// Closing the window stops every expiry; a late callback does nothing.
	api.Close()
	for name, w := range readers {
		if w.a != nil || w.timer != nil {
			t.Fatalf("%s reader open after Close", name)
		}
		api.expireReader(w)
	}
	api.expireCopy()
}
