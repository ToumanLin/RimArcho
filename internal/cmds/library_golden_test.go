package cmds_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Mod 库存 (06 §2.9): the list, filters, one lineage, removing from every plan, and moving a
// spare copy out of Mods/ into the lock library.
func TestGoldenLibrary(t *testing.T) {
	e := testkit.New(t)
	ws := e.AddWorkshop("5100", testkit.Sample("tester.lock", "Lockable"))
	e.AddWorkshop("5200", testkit.Sample("tester.idle", "Idle"))
	old := testkit.Sample("tester.old", "Oldie")
	old.Supported = []string{"1.4"}
	e.AddWorkshop("5400", old)
	e.WriteMod(filepath.Join(e.Workshop, "5500"), testkit.Sample("tester.gone", "Gone")) // unsubscribed
	e.Subscribe("5300")                                                                  // no folder
	e.CopyTree(ws, filepath.Join(e.Game, "Mods", "Lockable Copy"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.lock")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("tag", "add", "pkg:tester.idle", "QoL")
	var copyID string // the Mods/ copy: the same instance after the move
	copyRef := func(e *testkit.Env) string {
		ix, _ := e.App.Index()
		if copyID == "" {
			copyID = byName(e, "Lockable", func(m *store.Mod) bool { return m.Kind == source.Local }).ID
		}
		return ix.Ref(copyID)
	}
	lockRef := func(e *testkit.Env) []string { return []string{"library", "--lineage", ref(e, "Lockable")} }
	runGolden(t, "library", e, []step{
		cmd("library"),
		cmd("library", "--facets"),
		cmd("library", "--include", "state:unused", "--exclude", "source:workshop"),
		cmd("library", "--include", "state:residue", "--include", "state:unsupported"),
		cmd("library", "--search", "qol"),
		cmd("library", "--include", "tag:QoL", "--facets"),
		cmd("library", "--sort", "size", "--include", "source:workshop"),
		lockRef,
		cmd("library", "--lineage", "ws:5300"),
		cmd("library", "--include", "colour:red"),
		cmd("library", "--include", "state:hidden"),
		cmd("plan", "new", "other", "--from", "current"),
		func(e *testkit.Env) []string { return []string{"library", "move-to-locked", copyRef(e), "--dry-run"} },
		act(func(e *testkit.Env) { e.Advance(time.Hour) }),
		func(e *testkit.Env) []string { return []string{"library", "move-to-locked", copyRef(e)} },
		lockRef,
		cmd("library", "--include", "source:locked"),
		func(e *testkit.Env) []string { return []string{"library", "remove-from-plans", copyRef(e)} },
		func(e *testkit.Env) []string { return []string{"library", "remove-from-plans", copyRef(e)} },
		cmd("undo"),
		func(e *testkit.Env) []string { return []string{"library", "move-to-locked", ref(e, "Idle")} },
	})
}
