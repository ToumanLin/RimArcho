package app_test

// A plan's game versions (04 §1.6, P1, P2, P4).

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func versionsEnv(t *testing.T) *testkit.Env {
	e := setup(t, true)
	h := testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony", Supported: []string{"1.4", "1.5", "1.6"}}
	e.AddWorkshop("2009463077", h)
	a := testkit.Sample("tester.a", "A")
	a.Supported = []string{"1.4", "1.5"}
	e.AddWorkshop("1001", a)
	b := testkit.Sample("tester.b", "B")
	b.Supported = []string{"1.3", "1.4", "1.5"}
	e.WriteMod(filepath.Join(e.Game, "Mods", "B"), b)
	c := testkit.Sample("tester.c", "C")
	c.Supported = []string{"1.2"}
	e.WriteMod(filepath.Join(e.Game, "Mods", "C"), c)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony", "tester.a", "tester.b")
	scanNow(t, e)
	return e
}

func gameVersions(t *testing.T, e *testkit.Env) *app.PlanVersions {
	t.Helper()
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	pv, err := e.App.PlanVersionsOf(st, st.Plan)
	if err != nil {
		t.Fatal(err)
	}
	return pv
}

func TestPlanVersionsDefault(t *testing.T) {
	t.Parallel()
	e := versionsEnv(t)
	pv := gameVersions(t, e)
	// Every mod lists 1.4 and 1.5; the plan was made on 1.6 (Core is not counted).
	if got := app.FormatVersions(pv.Allowed); got != "1.4–1.6" || pv.Custom {
		t.Fatalf("default: %s custom=%v (common %v, made %s)", got, pv.Custom, pv.Common, pv.Created)
	}
	if pv.Counted != 3 || pv.Missing["1.6"] != 2 || pv.Missing["1.5"] != 0 || pv.Missing["1.3"] != 2 {
		t.Fatalf("counts: %d %v", pv.Counted, pv.Missing)
	}
	if got := strings.Join(pv.Shown, " "); got != "1.0 1.1 1.2 1.3 1.4 1.5 1.6" {
		t.Fatalf("shown: %s", got)
	}
	// No version common to every mod: only the game the plan was made on.
	e.MustRun("enable", refOf(t, e, "C", ""))
	if got := app.FormatVersions(gameVersions(t, e).Allowed); got != "1.6" {
		t.Fatalf("no common version: %s", got)
	}
	out := e.MustRun("plan", "versions")
	for _, want := range []string{"plan:default game versions: 1.6 (default)", "no version every mod lists; made on 1.6", "1.6  3 mods do not list it  [allowed, the game now]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("plan versions lacks %q:\n%s", want, out)
		}
	}
}

func TestPlanVersionsChosen(t *testing.T) {
	t.Parallel()
	e := versionsEnv(t)
	mc := e.ReadModsConfig()
	out := e.MustRun("plan", "versions", "default", "1.5-1.6")
	if !strings.Contains(out, "game versions: 1.5–1.6 (chosen)") || !strings.Contains(out, "was: 1.4–1.6 (default)") {
		t.Fatal(out)
	}
	// A choice stays when the mods change (P2).
	e.MustRun("enable", refOf(t, e, "C", ""))
	if pv := gameVersions(t, e); app.FormatVersions(pv.Allowed) != "1.5–1.6" || !pv.Custom {
		t.Fatalf("after enabling C: %+v", pv.Allowed)
	}
	// Not a list edit: undo takes back the enable, not the versions.
	e.MustRun("undo")
	if pv := gameVersions(t, e); app.FormatVersions(pv.Allowed) != "1.5–1.6" {
		t.Fatalf("after undo: %v", pv.Allowed)
	}
	// A copy keeps the choice.
	e.MustRun("plan", "new", "copy", "--from", "current")
	e.MustRun("plan", "use", "copy")
	if pv := gameVersions(t, e); app.FormatVersions(pv.Allowed) != "1.5–1.6" || !pv.Custom {
		t.Fatalf("copy: %v", pv.Allowed)
	}
	// Choosing the default is the default; "default" goes back.
	e.MustRun("plan", "versions", "copy", "1.4", "1.5", "1.6")
	if gameVersions(t, e).Custom {
		t.Fatal("the default chosen by hand is stored as the default")
	}
	e.MustRun("plan", "versions", "default", "default")
	e.MustRun("plan", "use", "default")
	if pv := gameVersions(t, e); pv.Custom || app.FormatVersions(pv.Allowed) != "1.4–1.6" {
		t.Fatalf("back to default: %v", pv.Allowed)
	}
	if _, code := e.Run("plan", "versions", "default", "1.6-1.4"); code == 0 {
		t.Fatal("a backwards range is refused")
	}
	if _, code := e.Run("plan", "versions", "default", "new"); code == 0 {
		t.Fatal("a word is refused")
	}
	if e.ReadModsConfig() != mc {
		t.Fatal("game versions write no game file")
	}
}

// P4: a RimArcho list carries the versions; importing it as a new plan keeps them.
func TestPlanVersionsTravelInRimArchoList(t *testing.T) {
	t.Parallel()
	e := versionsEnv(t)
	e.MustRun("plan", "versions", "default", "1.5", "1.6")
	out := e.MustRun("loadout", "export", "--format", "archo")
	if !strings.Contains(out, `"versions": [`) || !strings.Contains(out, `"1.5"`) {
		t.Fatal(out)
	}
	f := filepath.Join(t.TempDir(), "x.archolist.json")
	if err := os.WriteFile(f, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	e.MustRun("plan", "new", "shared", "--from", f)
	p, err := e.App.FindPlan("shared")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Versions, []string{"1.5", "1.6"}) {
		t.Fatalf("imported plan versions: %v", p.Versions)
	}
}

func TestParseAndFormatVersions(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"1.6": "1.6", "1.5-1.7": "1.5–1.7", "1.4,1.6–1.7": "1.4, 1.6–1.7", "1.7 1.5 1.6": "1.5–1.7", "1.3 - 1.4": "1.3–1.4"} {
		vs, err := app.ParseVersions([]string{in})
		if err != nil || app.FormatVersions(vs) != want {
			t.Fatalf("%q → %v %v", in, vs, err)
		}
	}
}

// The workshop export (04 §9.3): Markdown links in load order, the rest named in a
// comment; it reads back as the installed mods with those workshop IDs, after the current
// plan's Core (04 §9.1).
func TestExportWorkshopLinks(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	out := e.MustRun("loadout", "export", "--format", "workshop")
	for _, want := range []string{"# default: 4 mods, 2 in the Workshop", "- [Harmony](https://steamcommunity.com/sharedfiles/filedetails/?id=2009463077)\n- [A](https://steamcommunity.com/sharedfiles/filedetails/?id=1001)", "# not in the Workshop: Core, gone.mod (not installed)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	f := filepath.Join(t.TempDir(), "links.md")
	if err := os.WriteFile(f, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	e.MustRun("plan", "new", "links", "--from", f)
	e.MustRun("plan", "use", "links")
	if got := joined(active(t, e)); got != "Core, Harmony, A" {
		t.Fatalf("read back: %s", got)
	}
}
