package cmds_test

import (
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Locked versions (04 §7): pin, the upstream update warning, pin --update, unpin.
func TestGoldenPin(t *testing.T) {
	e := testkit.New(t)
	m := testkit.Sample("tester.lock", "Lockable")
	m.Files = map[string]string{"Defs/a.xml": "<Defs><ThingDef><defName>A</defName></ThingDef></Defs>"}
	ws := e.AddWorkshop("5100", m)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.lock")
	e.AddEnv(false)
	e.MustRun("scan")
	isCopy := func(rel string) func(*store.Mod) bool {
		return func(m *store.Mod) bool { return filepath.Base(m.Rel) == rel }
	}
	refTo := func(e *testkit.Env, rel string) string {
		ix, _ := e.App.Index()
		return ix.Ref(byName(e, "Lockable", isCopy(rel)).ID)
	}
	runGolden(t, "pin", e, []step{
		func(e *testkit.Env) []string { return []string{"pin", refTo(e, "5100"), "--dry-run"} },
		func(e *testkit.Env) []string { return []string{"pin", refTo(e, "5100")} },
		cmd("problems", "--code", "mod", "--min-severity", "info"),
		func(e *testkit.Env) []string { return []string{"pin", refTo(e, "5100_local"), "--update"} },
		act(func(e *testkit.Env) {
			e.WriteFile(filepath.Join(ws, "Defs", "b.xml"), "<Defs><ThingDef><defName>B</defName></ThingDef></Defs>")
		}),
		cmd("scan"),
		cmd("problems", "--code", "mod", "--min-severity", "info"),
		func(e *testkit.Env) []string { return []string{"pin", refTo(e, "5100"), "--update", "--dry-run"} },
		func(e *testkit.Env) []string { return []string{"pin", refTo(e, "5100_local"), "--update"} },
		func(e *testkit.Env) []string { return []string{"unpin", refTo(e, "5100_local"), "--dry-run"} },
		func(e *testkit.Env) []string { return []string{"unpin", refTo(e, "5100_local")} },
		func(e *testkit.Env) []string { return []string{"unpin", refTo(e, "5100_local")} },
		cmd("undo"),
		cmd("list", "--active"),
	})
}
