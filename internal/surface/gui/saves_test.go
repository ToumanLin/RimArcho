package gui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The Saves page end to end on the API: the listing arrives as an event, a save's detail
// is checked against its list, fixes go to that list as one undoable edit, and the file
// actions show what they touch before they act.
func TestSavesPage(t *testing.T) {
	e, api, events := fixture(t)
	lists, _ := savesDrain(events)
	now := e.Now()
	e.RecordLaunch(now.Add(-90 * time.Minute))
	e.WriteSave("Colony.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.a"}, ModTime: now.Add(-2 * time.Hour)})
	e.WriteSave("Colony 2.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.a", "tester.b", "tester.gone"},
		Names: []string{"Core", "Alpha", "Beta", "Gone Mod"}, SteamIDs: []string{"", "1001", "", "99"}, ModTime: now.Add(-time.Hour)})

	if _, err := api.SaveDetail("Colony"); err == nil {
		t.Fatal("a detail before the listing")
	}
	read := func() *view.SavesPage {
		t.Helper()
		api.SavesRead(7, "")
		select {
		case ev := <-lists:
			if !strings.HasPrefix(ev, `saves {"request":7,"page":`) {
				t.Fatalf("event %s", ev)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("no saves event")
		}
		api.saves.mu.Lock()
		defer api.saves.mu.Unlock()
		return view.SavesRows(api.saves.list)
	}
	p := read()
	if p.Saves != 2 || len(p.Colonies) != 1 || p.Colonies[0].Name != "Colony" {
		t.Fatalf("listing %+v", p)
	}
	if got := api.SavesSearch("gone mod"); !slices.Equal(got, []string{p.Colonies[0].Key}) {
		t.Fatalf("search %v", got)
	}

	issues := func(ref string) []string {
		t.Helper()
		d, err := api.SaveDetail(ref)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, is := range d.Issues {
			out = append(out, is.Head)
		}
		return out
	}
	if got := issues("Colony"); len(got) != 0 {
		t.Fatalf("Colony issues %q", got)
	}
	want := []string{"Beta is installed but not in the list", "Gone Mod is not installed"}
	if got := issues("Colony 2"); !slices.Equal(got, want) {
		t.Fatalf("Colony 2 issues %q, want %q", got, want)
	}

	// enable Beta only: one undoable edit to default, nothing to subscribe
	r, err := api.SaveFix("Colony 2", []string{"tester.b"}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Undo || len(r.WorkshopIDs) != 0 || r.Plan == 0 || r.Text != "default fixed from Colony 2: enabled Beta" {
		t.Fatalf("fix %+v", r)
	}
	if got := issues("Colony 2"); !slices.Equal(got, want[1:]) {
		t.Fatalf("after the fix %q", got)
	}
	if _, err := api.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := issues("Colony 2"); !slices.Equal(got, want) {
		t.Fatalf("after undo %q", got)
	}
	// all of them: the Workshop Mod goes back to the page to subscribe
	if r, err = api.SaveFix("Colony 2", nil, false, false); err != nil || !slices.Equal(r.WorkshopIDs, []string{"99"}) {
		t.Fatalf("fix all %+v %v", r, err)
	}

	// rename keeps the colony and refuses a taken name; copy lands next to it
	if p := api.SaveNameProblem("Colony", "Colony 2"); p != "a save named Colony 2 is already there" {
		t.Fatalf("name problem %q", p)
	}
	if _, err := api.SaveRename("Colony", "Colony 2"); err == nil {
		t.Fatal("renamed over another save")
	}
	rn, err := api.SaveRename("Colony", "Colony 1")
	if err != nil || rn.Ref == "" || rn.Text != "renamed Colony to Colony 1" {
		t.Fatalf("rename %+v %v", rn, err)
	}
	if _, err := os.Stat(filepath.Join(e.SavesDir(), "Colony 1.rws")); err != nil {
		t.Fatal(err)
	}
	read()
	cp, err := api.SaveCopy("Colony 1")
	if err != nil || !strings.HasPrefix(cp.Text, "copied Colony 1 to ") {
		t.Fatalf("copy %+v %v", cp, err)
	}
	p = read()
	if p.Saves != 3 {
		t.Fatalf("after copy %+v", p)
	}

	// backup and recycle preview first, then act
	b, err := api.SaveBackup([]string{"Colony 1"}, false)
	if err != nil || b.Preview == nil || len(b.Preview.Lines) != 1 || b.Text != "" {
		t.Fatalf("backup preview %+v %v", b, err)
	}
	if b, err = api.SaveBackup([]string{"Colony 1"}, true); err != nil || b.Failed || !strings.HasPrefix(b.Text, "backed up Colony 1 to ") {
		t.Fatalf("backup %+v %v", b, err)
	}
	var trashed []string
	e.App.TrashFunc = func(path string) error { trashed = append(trashed, path); return os.Remove(path) }
	tr, err := api.SaveTrash([]string{"Colony 2"}, false)
	if err != nil || tr.Preview == nil || tr.Preview.Button != "recycle" || len(trashed) != 0 {
		t.Fatalf("recycle preview %+v %v %v", tr, err, trashed)
	}
	if tr, err = api.SaveTrash([]string{"Colony 2"}, true); err != nil || tr.Failed || len(trashed) != 1 {
		t.Fatalf("recycle %+v %v %v", tr, err, trashed)
	}
	if _, err := api.SaveTrash(nil, false); err == nil {
		t.Fatal("recycled nothing")
	}

	// a new list from a save is named after the colony and opens in the editor
	read()
	nl, err := api.SaveNewList("Colony 1")
	if err != nil || !strings.HasPrefix(nl.Page, "plan:") {
		t.Fatalf("new list %+v %v", nl, err)
	}

	// leaving the page drops the listing
	api.leaveSaves()
	if _, err := api.SaveDetail("Colony 1"); err == nil || err.Error() != "the saves are still being read" {
		t.Fatalf("after leaving: %v", err)
	}
}
