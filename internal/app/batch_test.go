package app_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func ids(t *testing.T, e *testkit.Env, refs ...string) []string {
	t.Helper()
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range refs {
		m, err := ix.ResolveOne(r)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, m.ID)
	}
	return out
}

// 16 §3.4: a pick session's edits become one undo step; a reverted one leaves nothing.
func TestBatchCommitAndRevert(t *testing.T) {
	t.Parallel()
	e := stageEnv(t)
	before := names(t, e)
	b, err := e.App.BeginBatch()
	if err != nil {
		t.Fatal(err)
	}
	e.MustRun("enable", "pkg:tester.m31", "pkg:tester.m32")
	if _, err := e.App.MoveBlock(ids(t, e, "pkg:tester.m31", "pkg:tester.m32"), 2); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.MoveBlock(ids(t, e, "pkg:tester.m31", "pkg:tester.m32"), 3); err != nil {
		t.Fatal(err)
	}
	after := names(t, e)
	if !strings.HasPrefix(after, "Core M01 M31 M32 M02") {
		t.Fatalf("block move: %s", after)
	}
	if err := b.Commit("pick"); err != nil {
		t.Fatal(err)
	}
	e.MustRun("undo")
	if got := names(t, e); got != before {
		t.Fatalf("one undo:\n%s\nwant\n%s", got, before)
	}
	e.MustRun("redo")
	if got := names(t, e); got != after {
		t.Fatalf("redo: %s", got)
	}
	// Revert: list and staging as they were, and no undo step left behind.
	e.MustRun("undo")
	b, _ = e.App.BeginBatch()
	e.MustRun("stage", "add", "pkg:tester.m05")
	e.MustRun("enable", "pkg:tester.m33")
	if err := b.Revert(); err != nil {
		t.Fatal(err)
	}
	if got := names(t, e); got != before {
		t.Fatalf("revert: %s", got)
	}
	if out := e.MustRun("stage", "list"); !strings.Contains(out, "staging: 0") {
		t.Fatalf("staging after revert:\n%s", out)
	}
}

// The suggested position follows loadAfter; nothing declared means the end.
func TestSuggestAt(t *testing.T) {
	t.Parallel()
	e := stageEnv(t)
	dir := filepath.Join(e.Game, "Mods", "Late")
	m := testkit.Sample("tester.late", "Late")
	e.WriteMod(dir, m)
	e.WriteFile(filepath.Join(dir, "About", "About.xml"), strings.Replace(m.AboutXML(), "</ModMetaData>",
		"<loadAfter><li>tester.m10</li></loadAfter></ModMetaData>", 1))
	e.MustRun("scan")
	at, err := e.App.SuggestAt(ids(t, e, "pkg:tester.late"))
	if err != nil || at != 12 { // Core, M01…M10 are 1…11
		t.Fatalf("after M10: %d %v", at, err)
	}
	if at, _ := e.App.SuggestAt(ids(t, e, "pkg:tester.m31")); at != 32 {
		t.Fatalf("unconstrained: %d", at)
	}
	// Needing only a mod that loads before Core (Harmony) never suggests a place ahead of
	// the official content.
	h := testkit.Sample("brrainz.harmony", "Harmony")
	e.WriteMod(filepath.Join(e.Game, "Mods", "Harmony"), h)
	e.WriteFile(filepath.Join(e.Game, "Mods", "Harmony", "About", "About.xml"), strings.Replace(h.AboutXML(), "</ModMetaData>",
		"<loadBefore><li>Ludeon.RimWorld</li></loadBefore></ModMetaData>", 1))
	dir = filepath.Join(e.Game, "Mods", "NeedsH")
	m = testkit.Sample("tester.needsh", "NeedsH")
	e.WriteMod(dir, m)
	e.WriteFile(filepath.Join(dir, "About", "About.xml"), strings.Replace(m.AboutXML(), "</ModMetaData>",
		"<loadAfter><li>brrainz.harmony</li></loadAfter></ModMetaData>", 1))
	e.MustRun("scan")
	e.MustRun("enable", "pkg:brrainz.harmony", "--at", "1")
	if at, _ := e.App.SuggestAt(ids(t, e, "pkg:tester.needsh")); at < 3 {
		t.Fatalf("ahead of Core: %d", at)
	}
}
