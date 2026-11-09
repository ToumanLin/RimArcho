package gui

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/savefile"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// awaitListing waits for the "saves" event of request n.
func awaitListing(t *testing.T, lists chan string, n uint64) SavesEvent {
	t.Helper()
	deadline := time.After(30 * time.Second)
	for {
		select {
		case ev := <-lists:
			var got SavesEvent
			if err := json.Unmarshal([]byte(strings.TrimPrefix(ev, "saves ")), &got); err != nil {
				t.Fatalf("listing: %s", ev)
			}
			if got.Request == n {
				return got
			}
		case <-deadline:
			t.Fatalf("no saves event for request %d", n)
		}
	}
}

// A listing asked for before the player left the page and finishing after is not kept:
// the page is not revived and the full read does not start with no page shown.
func TestSavesListingAfterLeave(t *testing.T) {
	e, api, events := fixture(t)
	lists, _ := savesDrain(events)
	e.WriteSave("Landing.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}, ModTime: e.Now().Add(-time.Hour)})
	started := make(chan struct{}, 1)
	api.saves.runner = func(context.Context, []string, func(string)) error { started <- struct{}{}; return nil }

	api.saves.read.mu.Lock() // the listing waits for the reader
	api.SavesRead(1, "")
	api.leaveSaves()
	api.saves.read.mu.Unlock()
	if ev := awaitListing(t, lists, 1); ev.Error != "stale" || ev.Page != nil {
		t.Fatalf("a listing finished after leaving: %+v", ev)
	}
	api.saves.mu.Lock()
	revived := api.saves.list != nil
	api.saves.mu.Unlock()
	if revived {
		t.Fatal("the listing revived the page's state after leaving")
	}
	select {
	case <-started:
		t.Fatal("the full read started with no page shown")
	case <-time.After(200 * time.Millisecond):
	}

	// the next visit lists and reads as usual
	if ev := awaitListingOf(t, api, lists, 2); ev.Page == nil {
		t.Fatalf("next visit: %+v", ev)
	}
	select {
	case <-started:
	case <-time.After(30 * time.Second):
		t.Fatal("the next visit's full read did not start")
	}
}

func awaitListingOf(t *testing.T, api *API, lists chan string, n uint64) SavesEvent {
	t.Helper()
	api.SavesRead(n, "")
	return awaitListing(t, lists, n)
}

// A save the run could not read: the child names it on a line of its own and exits with
// an error; the run still ends done, saying how many could not be read and why, not the
// child's closing hint.
func TestSavesRunUnreadable(t *testing.T) {
	e, api, events := fixture(t)
	lists, news := savesDrain(events)
	e.WriteSave("Landing.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}, ModTime: e.Now().Add(-time.Hour)})
	api.saves.runner = func(_ context.Context, _ []string, progress func(string)) error {
		progress("progress: saves 1/1 sv:abcdef Landing/Landing\nprogress: saves-failed sv:abcdef read Landing.rws: the device is not ready")
		return &app.SavesIndexError{Err: errors.New("exit status 1"), Why: "Landing: read Landing.rws: the device is not ready"}
	}
	awaitListingOf(t, api, lists, 1)
	end := awaitNews(t, news, "the run's end", func(ev SavesReadEvent) bool { return ev.Done })
	if end.Error != "" || end.Text != "read 0 saves; 1 could not be read: read Landing.rws: the device is not ready" {
		t.Fatalf("end %+v", end)
	}
	if tk := savesTask(api); tk == nil || tk.State != app.TaskDone {
		t.Fatalf("task %+v", tk)
	}
}

// A child that failed on its own after a save it could not read (its "error:" line, such
// as the database failing while it tidies) ends the run failed, not done.
func TestSavesRunFatalAfterUnreadable(t *testing.T) {
	e, api, events := fixture(t)
	lists, news := savesDrain(events)
	e.WriteSave("Landing.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}, ModTime: e.Now().Add(-time.Hour)})
	api.saves.runner = func(_ context.Context, _ []string, progress func(string)) error {
		progress("progress: saves 1/1 sv:abcdef Landing/Landing\nprogress: saves-failed sv:abcdef read Landing.rws: the device is not ready")
		return &app.SavesIndexError{Err: errors.New("exit status 1"), Why: "database is locked", Fatal: true}
	}
	awaitListingOf(t, api, lists, 1)
	end := awaitNews(t, news, "the run's end", func(ev SavesReadEvent) bool { return ev.Done })
	if end.Error == "" || !strings.Contains(end.Error, "database is locked") {
		t.Fatalf("end %+v", end)
	}
}

// A save whose read failed and is unchanged since is not counted as unread: no run starts
// on every visit, and the right card says why it could not be read.
func TestSavesStoredFailure(t *testing.T) {
	e, api, events := fixture(t)
	lists, _ := savesDrain(events)
	p := e.WriteSave("Landing.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}, ModTime: e.Now().Add(-time.Hour)})
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := api.a.Store.PutSaveFailure(env.PathKey(p), fi.Size(), fi.ModTime().UnixNano(), savefile.ParserVersion, "the device is not ready"); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 1)
	api.saves.runner = func(context.Context, []string, func(string)) error { started <- struct{}{}; return nil }
	awaitListingOf(t, api, lists, 1)
	select {
	case <-started:
		t.Fatal("a run started for a save whose read failed")
	case <-time.After(300 * time.Millisecond):
	}
	d, err := api.SaveDetail("Landing")
	if err != nil {
		t.Fatal(err)
	}
	if d.Reading || d.PlayNote != "the save could not be read: the device is not ready" {
		t.Fatalf("detail: reading %v note %q %v", d.Reading, d.PlayNote, err)
	}
}

// An empty .rws file is read once (a malformed save), then counted as read: no run on
// every visit.
func TestSavesEmptyFileReadOnce(t *testing.T) {
	e, api, events := fixture(t)
	lists, news := savesDrain(events)
	p := filepath.Join(e.SavesDir(), "Empty.rws")
	e.WriteFile(p, "")
	at := e.Now().Add(-time.Hour)
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
	awaitListingOf(t, api, lists, 1)
	awaitNews(t, news, "the first run's end", func(ev SavesReadEvent) bool { return ev.Done })
	started := make(chan struct{}, 1)
	api.saves.runner = func(context.Context, []string, func(string)) error { started <- struct{}{}; return nil }
	awaitListingOf(t, api, lists, 2)
	select {
	case <-started:
		t.Fatal("an empty save is read again on every visit")
	case <-time.After(300 * time.Millisecond):
	}
}

// A late flush taken before the run's end drops its progress line: the status line never
// shows "reading the saves n/m" after the end.
func TestSavesLateProgressDropped(t *testing.T) {
	_, api, events := fixture(t)
	_, news := savesDrain(events)
	st := &api.saves
	st.mu.Lock()
	st.line = "reading the saves 40/40  Landing"
	st.lastSent = st.flushes + 2 // an event taken after this one went first (the end)
	st.mu.Unlock()
	api.savesFlush(nil)
	select {
	case ev := <-news:
		if ev.Line != "" {
			t.Fatalf("a late flush sent its line after a newer event: %+v", ev)
		}
	case <-time.After(300 * time.Millisecond):
	}
	// in order, the line goes
	st.mu.Lock()
	st.line = "reading the saves 1/40  Landing"
	st.mu.Unlock()
	api.savesFlush(nil)
	if ev := awaitNews(t, news, "the line", func(ev SavesReadEvent) bool { return ev.Line != "" }); ev.Line != "reading the saves 1/40  Landing" {
		t.Fatalf("line %q", ev.Line)
	}
}

// An older selection reaching the reader after a newer one never replaces it: the newest
// selection is the one read next.
func TestSavesReadSelectedOrder(t *testing.T) {
	_, api, _ := fixture(t)
	st := &api.saves
	a := &app.Save{Ref: "sv:aaaaaa", Path: "a.rws"}
	b := &app.Save{Ref: "sv:bbbbbb", Path: "b.rws"}
	busy := &app.Save{Ref: "sv:cccccc", Path: "c.rws"}
	st.mu.Lock()
	st.list = &app.SaveList{}
	st.reading, st.readingAt = factsKey(busy), 1
	st.mu.Unlock()
	api.readSelected(b, 5) // the newer selection takes the reader's lock first
	api.readSelected(a, 3)
	st.mu.Lock()
	next := st.next
	st.mu.Unlock()
	if next != b {
		t.Fatalf("waiting: %v, want the newest selection", next)
	}
	// the newest selection being the one read drops an older one waiting
	api.readSelected(busy, 7)
	st.mu.Lock()
	next = st.next
	st.reading, st.list = "", nil
	st.mu.Unlock()
	if next != nil {
		t.Fatalf("still waiting: %v", next)
	}
}

// The selected file changed after the listing (the game saved over it): its read is kept
// under the listing's key, so the page shows it instead of reading it again until the
// next relisting.
func TestSavesChangedAfterListing(t *testing.T) {
	e, api, events := fixture(t)
	lists, news := savesDrain(events)
	spec := testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}, Ticks: 90000, ModTime: e.Now().Add(-time.Hour)}
	spec.Body = playBody([]float32{1000, 2000}, []float32{3, 3}, []string{"Mara"}, nil, 0, nil)
	e.WriteSave("Landing.rws", spec)
	api.saves.runner = func(context.Context, []string, func(string)) error { return nil } // reads nothing
	awaitListingOf(t, api, lists, 1)
	spec.Ticks, spec.ModTime = 180000, e.Now().Add(-30*time.Minute)
	e.WriteSave("Landing.rws", spec) // after the listing
	api.saves.mu.Lock()
	api.saves.listed = time.Now().Add(time.Hour) // no relisting during the test
	api.saves.mu.Unlock()
	d, err := api.SaveDetail("Landing")
	if err != nil || !d.Reading {
		t.Fatalf("first detail: reading %v %v", d.Reading, err)
	}
	awaitNews(t, news, "the selected save read", func(ev SavesReadEvent) bool { return len(ev.Refs) > 0 })
	if d, err = api.SaveDetail("Landing"); err != nil || d.Reading || d.Play == nil || d.Play.Day != "day 3" {
		t.Fatalf("after its read: reading %v play %+v %v", d.Reading, d.Play, err)
	}
}
