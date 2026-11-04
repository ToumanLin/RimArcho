package app_test

// Several plans per environment (04 §1, V9, X5): each plan has its own list, undo stack,
// versions and staging list; switching, creating and removing write no game file.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/store"
)

func TestPlanIsolationAndSwitch(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	b := refOf(t, e, "B", "")
	mc := e.ReadModsConfig()
	def := mustPlan(t, e)

	e.MustRun("plan", "new", "copy", "--from", "current")
	e.MustRun("plan", "new", "empty")
	e.MustRun("enable", b) // in default
	if mustPlan(t, e) != def {
		t.Fatal("plan new keeps the current plan")
	}

	e.MustRun("plan", "use", "copy")
	if got := joined(active(t, e)); got != "Core, Harmony, A, ?gone.mod" {
		t.Fatalf("copy: %s", got)
	}
	if _, code := e.Run("undo"); code == 0 {
		t.Fatal("copy's undo stack is its own (empty)")
	}
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("stage", "add", refOf(t, e, "A", ""))
	if got := joined(active(t, e)); got != "Core, Harmony" {
		t.Fatalf("copy after edits: %s", got)
	}
	h, err := e.App.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Undo) != 2 || len(h.Versions) != 1 || h.Versions[0].Note != "copied from default v1" {
		t.Fatalf("copy history: %d steps, versions %+v", len(h.Undo), h.Versions)
	}

	e.MustRun("plan", "use", "default")
	if got := joined(active(t, e)); got != "Core, Harmony, A, ?gone.mod, B" {
		t.Fatalf("default unchanged by copy's edits: %s", got)
	}
	if items, _, _ := e.App.Staged(); len(items) != 0 {
		t.Fatal("staging is per plan")
	}
	e.MustRun("undo")
	if got := joined(active(t, e)); got != "Core, Harmony, A, ?gone.mod" {
		t.Fatalf("default undo: %s", got)
	}
	if e.App.Store.StepCount(def, "redo") != 1 {
		t.Fatal("default redo")
	}

	e.MustRun("plan", "use", "empty")
	if got := active(t, e); len(got) != 0 {
		t.Fatalf("empty: %v", got)
	}
	if e.ReadModsConfig() != mc {
		t.Fatal("switching and editing plans wrote ModsConfig.xml")
	}

	// Saving writes the current plan, versions it in that plan and records it as writer.
	e.MustRun("plan", "use", "copy")
	copyID := mustPlan(t, e)
	e.MustRun("save")
	if vs, _ := e.App.Store.Versions(copyID); len(vs) != 2 || vs[0].N != 2 || vs[0].Source != "user" {
		t.Fatalf("copy versions: %+v", vs)
	}
	if n := e.App.Store.VersionCount(def); n != 1 {
		t.Fatalf("default versions: %d", n)
	}
	st, _ := e.App.Store.ModsConfigStateAt(e.UserData)
	if st.WriterPlan != copyID || st.WriterVersion != 2 {
		t.Fatalf("writer: %+v", st)
	}
	// Our own write is not an external change for the other plans either.
	e.MustRun("plan", "use", "default")
	scanNow(t, e)
	if n := e.App.Store.VersionCount(def); n != 1 {
		t.Fatalf("default got a version from copy's write: %d", n)
	}

	// Rename keeps the history; remove takes everything; the last plan stays.
	e.MustRun("plan", "rename", "copy", "raid")
	p, _ := e.App.FindPlan("raid")
	if p == nil || p.ID != copyID || e.App.Store.VersionCount(copyID) != 2 {
		t.Fatal("rename")
	}
	e.MustRun("plan", "use", "raid")
	e.MustRun("plan", "remove", "raid", "--yes")
	if mustPlan(t, e) == copyID {
		t.Fatal("removing the current plan selects another")
	}
	if e.App.Store.VersionCount(copyID) != 0 || e.App.Store.Meta(store.StageKey(copyID)) != "" {
		t.Fatal("removed plan's versions and staging")
	}
	e.MustRun("plan", "remove", "empty", "--yes")
	if out, code := e.Run("plan", "remove", "default", "--yes"); code == 0 || !strings.Contains(out, "only plan") {
		t.Fatalf("the last plan cannot be removed: %s", out)
	}
}

// X5: importing a list as a new plan adds a plan with an import v1; the current plan stays.
func TestLoadoutImportNewPlan(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	list := filepath.Join(e.Root, "Night.txt")
	e.WriteFile(list, "ludeon.rimworld\ntester.b\n")
	before := joined(active(t, e))
	cur := mustPlan(t, e)
	e.MustRun("loadout", "import", list, "--new")
	e.MustRun("loadout", "import", list, "--new")
	if mustPlan(t, e) != cur || joined(active(t, e)) != before {
		t.Fatal("import --new changed the current plan")
	}
	p, err := e.App.FindPlan("Night (2)")
	if err != nil {
		t.Fatal(err)
	}
	vs, _ := e.App.Store.Versions(p.ID)
	if len(vs) != 1 || vs[0].Source != "import" || vs[0].Note != "Night.txt" {
		t.Fatalf("imported plan v1: %+v", vs)
	}
	e.MustRun("plan", "use", "Night")
	if got := joined(active(t, e)); got != "Core, B" {
		t.Fatalf("imported plan list: %s", got)
	}
	e.MustRun("plan", "new", "old", "--from", "v1")
	if _, code := e.Run("plan", "new", "night"); code == 0 {
		t.Fatal("plan names are unique, ignoring case")
	}
}
