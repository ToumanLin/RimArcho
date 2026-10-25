package app_test

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// planList is a plan's workspace by mod name ("?pkg" for missing entries), resolved in the
// current environment.
func planList(t *testing.T, e *testkit.Env, name string) string {
	t.Helper()
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.App.Store.PlanByName(ix.Env.ID, name)
	if err != nil || p == nil {
		t.Fatalf("no plan %q: %v", name, err)
	}
	ws, err := e.App.Store.Workspace(p.ID)
	if err != nil || ws == nil {
		t.Fatalf("plan %q has no workspace: %v", name, err)
	}
	var out []string
	for _, en := range loadout.Decode(ws.Entries) {
		if m := ix.ByID[en.Instance]; m != nil {
			out = append(out, m.Name)
		} else {
			out = append(out, "?"+en.Missing)
		}
	}
	return joined(out)
}

func planVersions(t *testing.T, e *testkit.Env, name string) int {
	t.Helper()
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	p, _ := e.App.Store.PlanByName(ix.Env.ID, name)
	if p == nil {
		t.Fatalf("no plan %q", name)
	}
	vs, err := e.App.Store.Versions(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	return len(vs)
}

// V6: an outside change goes to the plan that wrote the file, not the current one; an
// untouched writer loads it (undoable).
func TestV6ExternalGoesToWriter(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("save") // default v2 = Core, Harmony, A
	e.MustRun("plan", "new", "b", "--from", "current")
	e.MustRun("plan", "use", "b")
	e.MustRun("enable", refOf(t, e, "B", ""))
	bBefore := planList(t, e, "b")

	// The player edits the list in the game's mod menu.
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony")
	out := e.MustRun("status")
	if !strings.Contains(out, "ModsConfig.xml changed outside RimArcho: loaded as plan:default v3") {
		t.Fatalf("status:\n%s", out)
	}
	if got := planList(t, e, "default"); got != "Core, Harmony" {
		t.Fatalf("default after the outside change: %s", got)
	}
	if got := planList(t, e, "b"); got != bBefore || planVersions(t, e, "b") != 1 {
		t.Fatalf("plan b changed: %s, %d versions", got, planVersions(t, e, "b"))
	}
	if p, _ := e.App.CurrentPlan(); p.Name != "b" {
		t.Fatalf("current plan is %s", p.Name)
	}
	// Seen now: nothing more happens.
	if out := e.MustRun("status"); strings.Contains(out, "changed outside") {
		t.Fatalf("reported twice:\n%s", out)
	}
	// The load is a workspace edit of default that u takes back.
	e.MustRun("plan", "use", "default")
	e.MustRun("undo")
	if got := joined(active(t, e)); got != "Core, Harmony, A" {
		t.Fatalf("undo after the load: %s", got)
	}
	// The record now says the file holds default v3: a reformatting is no change.
	e.MustRun("redo")
	e.WriteFile(e.ModsConfig, strings.ReplaceAll(e.ReadModsConfig(), "    <li>", "\t\t<li>"))
	if out := e.MustRun("status"); strings.Contains(out, "changed outside") || planVersions(t, e, "default") != 3 {
		t.Fatalf("a reformatting counted:\n%s", out)
	}
}

// V7 and 05 I8: the writer was edited since the write — the change is kept as an external
// version, the plan is marked, and writes are refused until it is seen, loaded or ignored.
func TestV7ConflictOnEditedWriter(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("save") // default v2
	e.MustRun("enable", refOf(t, e, "B", ""))
	e.MustRun("plan", "new", "b", "--from", "v2")
	e.MustRun("plan", "use", "b")
	defBefore := planList(t, e, "default")

	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony")
	out := e.MustRun("status")
	for _, want := range []string{"kept as plan:default v3, not loaded", "pending: ModsConfig.xml changed outside RimArcho; plan:default v3 was not loaded",
		"archo plan use default, archo loadout diff v3 workspace, archo loadout checkout v3, archo loadout ignore-external"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status lacks %q:\n%s", want, out)
		}
	}
	if planList(t, e, "default") != defBefore || planVersions(t, e, "default") != 3 {
		t.Fatalf("default was loaded: %s", planList(t, e, "default"))
	}

	// Plan b would overwrite the unseen change: refused, the file stays.
	file := e.ReadModsConfig()
	e.WriteFile(filepath.Join(e.Steam, "steam.exe"), "")
	for _, args := range [][]string{{"save"}, {"save", "--dry-run"}, {"launch"}} {
		out, code := e.Run(args...)
		if code == 0 || !strings.Contains(out, "plan:default v3 was not loaded; nothing was written") ||
			!strings.Contains(out, "first archo plan use default, then see the difference: archo loadout diff v3 workspace; load it: archo loadout checkout v3; ignore it: archo loadout ignore-external") {
			t.Fatalf("%v: exit %d\n%s", args, code, out)
		}
	}
	if _, err := e.App.Save(false); !errors.Is(err, app.ErrExternalConflict) {
		t.Fatalf("Save error %v", err)
	}
	if e.ReadModsConfig() != file || len(e.Launched) != 0 {
		t.Fatal("a refused write changed ModsConfig.xml or launched")
	}

	// In the writer: log shows the mark; diff works; save is refused; ignore clears it.
	e.MustRun("plan", "use", "default")
	if out := e.MustRun("loadout", "log"); !strings.Contains(out, "pending: ") || !strings.Contains(out, "archo loadout ignore-external") {
		t.Fatalf("log:\n%s", out)
	}
	if out := e.MustRun("loadout", "diff", "v3", "workspace"); !strings.Contains(out, "diff v3 → workspace") {
		t.Fatalf("diff:\n%s", out)
	}
	if out, code := e.Run("save"); code == 0 || strings.Contains(out, "plan use") || !strings.Contains(out, "ignore it: archo loadout ignore-external") {
		t.Fatalf("save in the writer: exit %d\n%s", code, out)
	}
	if out := e.MustRun("loadout", "ignore-external"); !strings.Contains(out, "ignored v3 of plan:default") {
		t.Fatal(out)
	}
	if out := e.MustRun("loadout", "ignore-external"); !strings.Contains(out, "nothing to ignore") {
		t.Fatal(out)
	}
	if out := e.MustRun("loadout", "log"); strings.Contains(out, "pending:") || !strings.Contains(out, "v3") {
		t.Fatalf("log after ignore:\n%s", out)
	}
	if planList(t, e, "default") != defBefore {
		t.Fatal("ignore changed the list")
	}
	e.MustRun("save")
	if !strings.Contains(e.ReadModsConfig(), "tester.b") {
		t.Fatal("save after ignore did not write")
	}
}

// Loading the unloaded external version (checkout) clears the mark and unblocks saving.
func TestIgnoreExternalCheckoutClears(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("save")
	e.MustRun("enable", refOf(t, e, "B", ""))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony")
	st, err := e.App.Loadout()
	if err != nil || st.WS.Conflict == 0 {
		t.Fatalf("no conflict: %v", err)
	}
	n := st.WS.Conflict
	e.MustRun("loadout", "checkout", "v"+strconv.Itoa(n))
	if st, _ := e.App.Loadout(); st.WS.Conflict != 0 || joined(active(t, e)) != "Core, Harmony" {
		t.Fatal("checkout did not load and clear")
	}
	e.MustRun("save")
}

// V8: two environments share a user data folder. The other environment's write is not an
// outside change; an outside change after it goes to the writer's plan in its own
// environment, resolved there.
func TestV8SharedUserData(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("save") // main/default v2

	other := filepath.Join(e.Root, "Other", "RimWorld")
	e.WriteFile(filepath.Join(other, "Version.txt"), "1.6.4633 rev1261\n")
	e.WriteMod(filepath.Join(other, "Data", "Core"), testkit.Mod{PackageID: "ludeon.rimworld", Name: "Core"})
	e.MustRun("env", "add", "second", "--game", other, "--user-data", e.UserData, "--workshop", e.Workshop)
	e.MustRun("env", "use", "second")
	r, err := e.App.Scan(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range r.Notices {
		if strings.Contains(n, "changed outside") {
			t.Fatalf("V8: %v", r.Notices)
		}
	}
	if out := e.MustRun("status"); strings.Contains(out, "changed outside") || strings.Contains(out, "pending:") {
		t.Fatalf("V8 status:\n%s", out)
	}
	secondVersions := planVersions(t, e, "default")

	// B exists only in main's Mods/: the change is resolved by main.
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony", "tester.a", "tester.b")
	out := e.MustRun("status")
	if !strings.Contains(out, "ModsConfig.xml changed outside RimArcho: loaded as env:main plan:default v3") {
		t.Fatalf("status in second:\n%s", out)
	}
	if planVersions(t, e, "default") != secondVersions {
		t.Fatal("second's plan got the change")
	}
	e.MustRun("env", "use", "main")
	if got := planList(t, e, "default"); got != "Core, Harmony, A, B" {
		t.Fatalf("main/default: %s", got)
	}
	if out := e.MustRun("status"); strings.Contains(out, "changed outside") {
		t.Fatalf("seen again in main:\n%s", out)
	}

	// A conflict in main's plan shows from second with --env in the commands, and second
	// cannot overwrite it.
	e.MustRun("disable", refOf(t, e, "B", ""))
	e.MustRun("env", "use", "second")
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	if out := e.MustRun("status"); !strings.Contains(out, "kept as env:main plan:default v4, not loaded") ||
		!strings.Contains(out, "archo --env main plan use default") {
		t.Fatalf("conflict from second:\n%s", out)
	}
	if out, code := e.Run("save"); code == 0 || !strings.Contains(out, "first archo --env main plan use default, then see the difference: archo --env main loadout diff v4 workspace") {
		t.Fatalf("save in second: exit %d\n%s", code, out)
	}
}

// No write record (a new folder, a lost record): the change goes to the current plan.
func TestExternalNoRecordGoesToCurrent(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e) // imported; seen, but never written
	e.MustRun("plan", "new", "b", "--from", "current")
	e.MustRun("plan", "use", "b")
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony")
	if out := e.MustRun("status"); !strings.Contains(out, "loaded as plan:b v2") {
		t.Fatalf("status:\n%s", out)
	}
	if planVersions(t, e, "default") != 1 || joined(active(t, e)) != "Core, Harmony" {
		t.Fatalf("default %d versions, b %v", planVersions(t, e, "default"), active(t, e))
	}
	// A deleted writer falls back to the current plan too.
	e.MustRun("save")
	e.MustRun("plan", "use", "default")
	e.MustRun("plan", "remove", "b", "--yes")
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	if out := e.MustRun("status"); !strings.Contains(out, "plan:default v2") || joined(active(t, e)) != "Core" {
		t.Fatalf("after deleting the writer:\n%s\n%v", out, active(t, e))
	}
}
