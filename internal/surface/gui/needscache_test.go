package gui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestNeedsCacheKeyInputs(t *testing.T) {
	e, s, _ := fixture(t)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.needsLocked()
	key := func() string { _, k, _, _ := s.needsKeyLocked(); return k }
	base := key()
	if base != key() {
		t.Fatal("stable inputs missed cache")
	}
	check := func(name string, change func(), restore func()) {
		t.Helper()
		before := key()
		change()
		if key() == before {
			t.Fatalf("%s did not invalidate", name)
		}
		restore()
	}
	check("game version", func() { e.App.GameVersion = "1.5" }, func() { e.App.GameVersion = "" })
	check("snapshot", func() {
		st, _ := e.App.Loadout()
		st.Ix.Mods[0].SnapHash = append([]byte{1}, st.Ix.Mods[0].SnapHash...)
	}, func() {})
	plan, err := e.App.NewPlan("Other", "current")
	if err != nil {
		t.Fatal(err)
	}
	check("open list", func() { e.App.OpenPlan = plan.Plan.ID }, func() { e.App.OpenPlan = 0 })
	st, _ := e.App.Loadout()
	path := filepath.Join(st.Ix.Env.UserDataRoot, "Player.log")
	if err = os.WriteFile(path, []byte("test log"), 0600); err != nil {
		t.Fatal(err)
	}
	check("log mtime", func() {
		now := time.Now().Add(time.Second)
		if err := os.Chtimes(path, now, now); err != nil {
			t.Fatal(err)
		}
	}, func() {})
	check("list edit", func() {
		_, err = e.App.Disable([]string{"pkg:tester.a"})
		if err != nil {
			t.Fatal(err)
		}
	}, func() { _, _, _ = e.App.UndoRedo(false) })
	check("analysis/database", func() {
		if err := e.App.Store.SetMeta("test.needs", "changed"); err != nil {
			t.Fatal(err)
		}
	}, func() {})
	// A separate connection must invalidate the same API, including its scan index.
	r, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	check("external writer", func() {
		if err := r.Store.SetMeta("test.needs.external", "changed"); err != nil {
			t.Fatal(err)
		}
	}, func() {})
}

func TestNeedsEditSharesIncrementalSnapshotAndUndo(t *testing.T) {
	e, s, _ := fixture(t)
	// This test inspects the synchronous result; no background publication may race it.
	s.needsClosed = true
	b := s.Board()
	before := app.ListFingerprint(b.St.List)
	initial := s.needsLocked()
	if s.needsLocked() != initial {
		t.Fatal("identical read rebuilt needs")
	}
	var dep view.Action
	for _, it := range initial.Items {
		if it.Target != "" && len(it.Actions) > 0 && it.Actions[0].Kind == view.ActEnable {
			dep = it.Actions[0]
		}
	}
	if dep.Kind == "" {
		t.Fatal("fixture missing dependency action")
	}
	if _, err := s.DoAction(dep, 0); err != nil {
		t.Fatal(err)
	}
	c, _ := s.cachedNeedsLocked()
	if !c.n.Updating {
		t.Fatal("stale content must be marked updating")
	}
	for _, it := range c.n.Items {
		if it.Target != "" && it.Target == "brrainz.harmony" {
			t.Fatal("enable did not remove dependency")
		}
	}
	st, _ := e.App.Loadout()
	h, _ := st.Ix.ResolveOne(dep.Refs[0])
	a, _ := st.Ix.ResolveOne("pkg:tester.a")
	if st.Position(h.ID) >= st.Position(a.ID) {
		t.Fatal("dependency not placed before consumer")
	}
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	st, _ = e.App.Loadout()
	if app.ListFingerprint(st.List) != before {
		t.Fatal("undo did not restore list")
	}
	id := a.ID
	result, err := s.Toggle(int(view.Active), []string{id}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Problems == nil || !result.Problems.Updating {
		t.Fatal("edit omitted problem summary")
	}
	c, _ = s.cachedNeedsLocked()
	if result.Problems.Revision != c.n.Revision || result.Problems.Count != c.n.Count {
		t.Fatal("editor and needs snapshots differ")
	}
}

func TestNeedsCacheBoundedAndWorkspaceReused(t *testing.T) {
	_, s, events := fixture(t)
	s.Board()
	if !s.Workspace(1) {
		t.Fatal("workspace rejected")
	}
	r := environmentResult(t, events, 1)
	if !s.Workspace(2) {
		t.Fatal("workspace rejected")
	}
	next := environmentResult(t, events, 2)
	if r.Board.Problems.Revision != next.Board.Problems.Revision || r.Shell.Env != next.Shell.Env {
		t.Fatal("cached workspace differs")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < 12; i++ {
		s.a.GameVersion = fmt.Sprintf("1.%d", i)
		s.needsLocked()
	}
	if len(s.needsCache) > 8 {
		t.Fatal("unbounded needs cache")
	}
}

func TestNeedsIncrementalKeepsContentThroughEnableUndo(t *testing.T) {
	e, s, _ := fixture(t)
	s.needsClosed = true
	_ = s.needsLocked()
	c, _ := s.cachedNeedsLocked()
	st := c.ps.State
	a, _ := st.Ix.ResolveOne("pkg:tester.a")
	h, _ := st.Ix.ResolveOne("pkg:brrainz.harmony")
	content := &app.Problem{Finding: &finding.Finding{Layer: "patch", Code: "patch.no-match", Severity: finding.Warning, Key: "test", Subjects: []string{finding.Inst(a.ID), finding.Inst(h.ID)}, Claim: "target inactive"}}
	c.ps.List = append(c.ps.List, content)
	c.base = c.ps
	if _, err := s.DoAction(view.Action{Kind: view.ActEnable, Refs: []string{st.Ix.Ref(h.ID)}}, 0); err != nil {
		t.Fatal(err)
	}
	c, _ = s.cachedNeedsLocked()
	for _, it := range c.n.Items {
		if it.Target == h.PackageID {
			t.Fatal("stale patch dependency survived enable")
		}
	}
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	c, _ = s.cachedNeedsLocked()
	found := false
	for _, p := range c.base.List {
		found = found || p == content
	}
	if !found {
		t.Fatal("incremental pass discarded reusable content")
	}
	// The next view again includes the unresolved patch without re-reading any facts.
	ps, err := e.App.IncrementalProblems(c.base)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, p := range ps.List {
		found = found || p.Finding == content.Finding
	}
	if !found {
		t.Fatal("undo lost cached content finding")
	}
}

func TestNeedsBackgroundCannotPublishAnOlderEdit(t *testing.T) {
	e, s, _ := fixture(t)
	e.App.AnalyzeRunner = func(string, func(string)) error { return errors.New("fixture: analysis intentionally unavailable") }
	s.emit = func(string, any) {}
	b := s.Board()
	a, _ := b.St.Ix.ResolveOne("pkg:tester.a")
	if _, err := s.Toggle(int(view.Active), []string{a.ID}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Toggle(int(view.Inactive), []string{a.ID}, nil); err != nil {
		t.Fatal(err)
	}
	s.needsJobs.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	st, _ := e.App.Loadout()
	c, _ := s.cachedNeedsLocked()
	if c.list != app.ListFingerprint(st.List) || c.n.ListVersion != c.list {
		t.Fatal("background worker published an older list")
	}
	if c.n.Plan != st.Plan.ID || c.n.Environment != st.Ix.Env.ID {
		t.Fatal("snapshot lacks navigation identity")
	}
}

func TestNeedsDetachedLinkActionsUseReceivingSession(t *testing.T) {
	e, s, _ := fixture(t)
	s.needsClosed = true
	// Consume a different token in the main session before the reader starts at 1.
	e.App.RememberLinkRepair(&app.LinkRepairPlan{})
	target := filepath.Join(e.Root, "missing-target")
	link := filepath.Join(e.Game, "Mods", "_Archo_env_old-id")
	e.Junction(link, target)
	n := s.Needs()
	var action view.Action
	for _, it := range n.Items {
		for _, a := range it.Actions {
			if a.Kind == view.ActRepairLinks {
				action = a
				break
			}
		}
	}
	if action.Kind == "" {
		t.Fatal("missing repair action")
	}
	if _, err := s.DoAction(action, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("detached token did not execute its own confirmed preview")
	}
}
