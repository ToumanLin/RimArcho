package app_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/savefile"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// A failed full read is stored for the file as it is (12 §1.7): lists mark the save and
// do not count it as unread, the full read skips it, SaveFacts without reading gives the
// reason, and the save is read again once the file changes or when asked to read it.
func TestSaveStoredReadFailure(t *testing.T) {
	t.Parallel()
	e := savesFixture(t)
	spec := testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}, Ticks: 90000, ModTime: e.Now().Add(-time.Hour)}
	p := e.WriteSave("Landing.rws", spec)
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.App.Store.PutSaveFailure(env.PathKey(p), fi.Size(), fi.ModTime().UnixNano(), savefile.ParserVersion, "the device is not ready"); err != nil {
		t.Fatal(err)
	}
	l, err := e.App.ListSaves()
	if err != nil {
		t.Fatal(err)
	}
	s, err := l.Find("Landing")
	if err != nil {
		t.Fatal(err)
	}
	if !s.ReadFailed || s.Summary != nil || s.NeedsRead() {
		t.Fatalf("listed: failed %v summary %+v needs read %v", s.ReadFailed, s.Summary, s.NeedsRead())
	}
	if f, err := e.App.SaveFacts(s, false); f != nil || err != nil {
		t.Fatalf("facts without reading: %v %v", f, err)
	}
	if why, err := e.App.SaveReadFailure(s); why != "the device is not ready" || err != nil {
		t.Fatalf("failure %q %v", why, err)
	}
	r, err := e.App.IndexSaves(l.Saves(), nil)
	if err != nil || r.Unreadable != 1 || len(r.Errs) != 0 {
		t.Fatalf("index %+v %v", r, err)
	}
	// asked to read it (saves show): read again, and the failure is gone
	cp := *s
	if f, err := e.App.SaveFacts(&cp, true); err != nil || f == nil || cp.ReadFailed {
		t.Fatalf("read again: %v %v failed %v", f, err, cp.ReadFailed)
	}
	if l, err = e.App.ListSaves(); err != nil {
		t.Fatal(err)
	}
	if s, _ = l.Find("Landing"); s.ReadFailed || s.Summary == nil {
		t.Fatalf("after the new read: failed %v summary %+v", s.ReadFailed, s.Summary)
	}

	// a changed file is no longer the one that failed
	if err := e.App.Store.PutSaveFailure(env.PathKey(p), fi.Size(), fi.ModTime().UnixNano(), savefile.ParserVersion, "x"); err != nil {
		t.Fatal(err)
	}
	spec.Ticks, spec.ModTime = 180000, e.Now().Add(-30*time.Minute)
	e.WriteSave("Landing.rws", spec)
	if l, err = e.App.ListSaves(); err != nil {
		t.Fatal(err)
	}
	if s, _ = l.Find("Landing"); s.ReadFailed || !s.NeedsRead() {
		t.Fatalf("changed file: failed %v needs read %v", s.ReadFailed, s.NeedsRead())
	}
	if why, _ := e.App.SaveReadFailure(s); why != "" {
		t.Fatalf("changed file still failed: %q", why)
	}
}

// The child's progress names a save it could not read on a line of its own.
func TestSavesFailLine(t *testing.T) {
	t.Parallel()
	s := &app.Save{Ref: "sv:abcdef", Name: "Landing"}
	line := app.SavesIndexLine(app.SaveIndexProgress{Done: 1, Total: 2, Save: s, Err: "read failed\nat byte 9"})
	parts := strings.Split(line, "\n")
	if len(parts) != 2 {
		t.Fatalf("lines %q", line)
	}
	if done, total, ref, name, ok := app.ParseSavesIndexLine(parts[0]); !ok || done != 1 || total != 2 || ref != s.Ref || name != "Landing" {
		t.Fatalf("progress line %q", parts[0])
	}
	if ref, why, ok := app.ParseSavesFailLine(parts[1]); !ok || ref != s.Ref || why != "read failed at byte 9" {
		t.Fatalf("fail line %q: %q %q", parts[1], ref, why)
	}
	if _, _, _, _, ok := app.ParseSavesIndexLine(parts[1]); ok {
		t.Fatal("the fail line reads as a progress line")
	}
}
