package app_test

// M2 Loadout tests (04 §1.1–§4, §8). Every ModsConfig.xml written here lives in a temp dir.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func refOf(t *testing.T, e *testkit.Env, name string, kind source.Kind) string {
	t.Helper()
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ix.Mods {
		if m.Name == name && (kind == "" || m.Kind == kind) {
			return ix.Ref(m.ID)
		}
	}
	t.Fatalf("no mod %q (%s)", name, kind)
	return ""
}

func active(t *testing.T, e *testkit.Env) []string {
	t.Helper()
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, en := range st.List {
		if en.Instance == "" {
			out = append(out, "?"+en.Missing)
			continue
		}
		out = append(out, st.Ix.ByID[en.Instance].Name)
	}
	return out
}

func joined(l []string) string { return strings.Join(l, ", ") }

// loadoutEnv: Core, Harmony (workshop), A (workshop), B (local), and a ModsConfig.xml that
// lists Core, Harmony, A and a mod that is not installed.
func loadoutEnv(t *testing.T) *testkit.Env {
	e := setup(t, true)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony", Files: map[string]string{"Assemblies/0Harmony.dll": "MZ"}})
	a := testkit.Sample("tester.a", "A")
	a.Deps = []string{"brrainz.harmony"}
	e.AddWorkshop("1001", a)
	e.WriteMod(filepath.Join(e.Game, "Mods", "B"), testkit.Sample("tester.b", "B"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony", "tester.a", "gone.mod")
	return e
}

func TestLoadoutImportHealAndEdits(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	// Before any scan nothing resolves: entries are kept as missing, not dropped.
	if got := joined(active(t, e)); got != "?ludeon.rimworld, ?brrainz.harmony, ?tester.a, ?gone.mod" {
		t.Fatalf("before scan: %s", got)
	}
	scanNow(t, e)
	if got := joined(active(t, e)); got != "Core, Harmony, A, ?gone.mod" {
		t.Fatalf("after scan: %s", got)
	}
	if out := e.MustRun("status"); !strings.Contains(out, "loadout: 4 active (1 missing entries), same as v1 (import)") {
		t.Fatal(out)
	}
	// First use (04 §1.4): the first plan is ModsConfig.xml, imported as v1.
	p, err := e.App.CurrentPlan()
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != store.DefaultPlanName {
		t.Fatalf("first plan %q", p.Name)
	}
	if vs, _ := e.App.Store.Versions(p.ID); len(vs) != 1 || vs[0].Source != "import" || vs[0].Note != "ModsConfig.xml" {
		t.Fatalf("v1: %+v", vs)
	}
	if ps, _ := e.App.Plans(); len(ps) != 1 || !ps[0].Current || ps[0].Entries != 4 {
		t.Fatalf("plans: %+v", ps)
	}
	e.MustRun("enable", refOf(t, e, "B", ""), "--after", refOf(t, e, "Harmony", ""))
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("move", refOf(t, e, "A", ""), "--to", "2")
	if got := joined(active(t, e)); got != "Core, A, Harmony, B" {
		t.Fatalf("after edits: %s", got)
	}
	out := e.MustRun("loadout", "diff")
	for _, want := range []string{"diff v1 → workspace: 3 changes", "added", "removed", "moved"} {
		if !strings.Contains(out, want) {
			t.Fatalf("diff lacks %q:\n%s", want, out)
		}
	}
	e.MustRun("undo")
	if got := joined(active(t, e)); got != "Core, Harmony, B, A" {
		t.Fatalf("after undo: %s", got)
	}
	e.MustRun("redo")
	if got := joined(active(t, e)); got != "Core, A, Harmony, B" {
		t.Fatalf("after redo: %s", got)
	}
	if _, code := e.Run("redo"); code == 0 {
		t.Fatal("redo with an empty stack must fail")
	}
}

func TestEnableRefusals(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	e.WriteMod(filepath.Join(e.Workshop, "9999"), testkit.Sample("tester.residue", "Residue"))
	e.WriteMod(filepath.Join(e.SteamCMD, "8888"), testkit.Sample("tester.cmd", "CmdOnly"))
	dis := e.AddWorkshop("7777", testkit.Sample("tester.off", "Off"))
	e.Move(filepath.Join(dis, "About", "About.xml"), filepath.Join(dis, "About", "About.xml.disabled"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "A local"), testkit.Sample("tester.a", "A"))
	scanNow(t, e)
	for name, want := range map[string]string{
		"Residue": "residue", "Off": "unloadable: about-disabled",
	} {
		out, code := e.Run("enable", refOf(t, e, name, ""))
		if code == 0 || !strings.Contains(out, want) {
			t.Fatalf("enable %s: exit %d\n%s", name, code, out)
		}
	}
	// "tester.a" resolves to the local copy (the game gives non-workshop copies the plain
	// id). The workshop copy cannot be enabled next to it, only swapped in.
	ws := refOf(t, e, "A", source.Workshop)
	out, code := e.Run("enable", ws)
	if code == 0 || !strings.Contains(out, "archo swap") {
		t.Fatalf("duplicate packageId: exit %d\n%s", code, out)
	}
	e.MustRun("swap", refOf(t, e, "A", source.Local), ws)
	st, _ := e.App.Loadout()
	if got := st.Reg.Tokens(st.List, nil); joined(got) != "ludeon.rimworld, brrainz.harmony, tester.a_steam, gone.mod" {
		t.Fatalf("tokens after swap to the workshop copy: %v", got)
	}
}

func TestSaveSteamSuffixVersionAndBackup(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	e.WriteMod(filepath.Join(e.Game, "Mods", "A local"), testkit.Sample("tester.a", "A"))
	e.WriteModsConfig("1.5.4409 rev1118", "ludeon.rimworld", "brrainz.harmony", "tester.a_steam", "gone.mod")
	orig := e.ReadModsConfig()
	scanNow(t, e)
	// tester.a_steam resolves to the workshop copy because a local copy exists.
	st, _ := e.App.Loadout()
	if m := st.Ix.ByID[st.List[2].Instance]; m == nil || m.Kind != source.Workshop {
		t.Fatalf("tester.a_steam resolved to %+v", st.List[2])
	}
	out := e.MustRun("status")
	if !strings.Contains(out, "ModsConfig.xml was saved for 1.5; the game (1.6) resets it on start") {
		t.Fatal(out)
	}
	// The mismatch no longer blocks a launch: launching writes the file for 1.6 (08 §2).
	if out, _ := e.Run("launch", "--dry-run"); strings.Contains(out, "saved for") {
		t.Fatalf("launch blocked by the version mismatch:\n%s", out)
	}
	out = e.MustRun("save", "--dry-run")
	if e.ReadModsConfig() != orig || !strings.Contains(out, "1.5.4409 rev1118 → 1.6.4633 rev1261 (rewritten from 1.5 to 1.6") {
		t.Fatalf("dry run wrote or wrong output:\n%s", out)
	}
	e.MustRun("disable", "pkg:gone.mod")
	out = e.MustRun("save")
	if !strings.Contains(out, "saved v2 to ModsConfig.xml") {
		t.Fatal(out)
	}
	got := e.ReadModsConfig()
	for _, want := range []string{"<version>1.6.4633 rev1261</version>", "<li>tester.a_steam</li>", "<li>ludeon.rimworld.royalty</li>"} {
		if !strings.Contains(got, want) {
			t.Fatalf("written file lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "gone.mod") {
		t.Fatal("disabled missing entry was written")
	}
	if b, err := os.ReadFile(e.ModsConfig + loadout.BackupSuffix); err != nil || string(b) != orig {
		t.Fatal("backup is not the original file")
	}
	// Our own write is not an external change; the launch goes through now.
	scanNow(t, e)
	vs, _ := e.App.Store.Versions(mustPlan(t, e))
	if len(vs) != 2 || vs[0].Source != "user" {
		t.Fatalf("versions after rescan: %d, newest %s", len(vs), vs[0].Source)
	}
	e.WriteFile(filepath.Join(e.Steam, "steam.exe"), "")
	out = e.MustRun("launch")
	if len(e.Launched) != 1 || !strings.Contains(e.Launched[0], "RimWorldWin64.exe") && !strings.Contains(e.Launched[0], "steam") {
		t.Fatalf("launch: %v\n%s", e.Launched, out)
	}
}

func TestExternalChangeAndConflict(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	// Same list, different formatting (the game re-saving it): no new version.
	e.WriteFile(e.ModsConfig, strings.ReplaceAll(e.ReadModsConfig(), "    <li>", "\t\t<li>"))
	scanNow(t, e)
	if vs, _ := e.App.Store.Versions(mustPlan(t, e)); len(vs) != 1 {
		t.Fatalf("a reformatted but identical file created %d versions", len(vs))
	}
	// Clean workspace: the external change is loaded.
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.a")
	r, err := e.App.Scan(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Notices) != 1 || !strings.Contains(r.Notices[0], "loaded as plan:default v2") || joined(active(t, e)) != "Core, A" {
		t.Fatalf("notices %v, active %v", r.Notices, active(t, e))
	}
	// Dirty workspace: the external change is kept as a version but not loaded.
	e.MustRun("enable", refOf(t, e, "B", ""))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony")
	out := e.MustRun("status")
	if !strings.Contains(out, "not loaded because the plan changed since it was saved") || joined(active(t, e)) != "Core, A, B" {
		t.Fatalf("conflict:\n%s", out)
	}
	e.MustRun("loadout", "checkout", "v3")
	if joined(active(t, e)) != "Core, Harmony" {
		t.Fatalf("checkout: %v", active(t, e))
	}
	if out := e.MustRun("loadout", "log"); strings.Contains(out, "pending:") {
		t.Fatalf("conflict should clear on checkout:\n%s", out)
	}
	if st, _ := e.App.Loadout(); st.WS.Conflict != 0 {
		t.Fatalf("conflict mark left after checkout: v%d", st.WS.Conflict)
	}
	e.MustRun("save") // the checkout unblocks saving
}

func TestHistoryCheckoutUndoSource(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("save") // v2
	e.MustRun("enable", refOf(t, e, "B", ""))
	e.Advance(time.Hour)
	e.MustRun("save") // v3
	e.MustRun("loadout", "checkout", "v2")
	out := e.MustRun("save") // v4, restores v2
	if !strings.Contains(out, "source: undo (restores v2)") {
		t.Fatal(out)
	}
	vs, _ := e.App.Store.Versions(mustPlan(t, e))
	if vs[0].Source != "undo" || vs[0].Note != "restored v2" {
		t.Fatalf("newest version %+v", vs[0])
	}
	out = e.MustRun("loadout", "diff", "v3", "v4")
	if !strings.Contains(out, "diff v3 → v4: 1 change") || !strings.Contains(out, "removed") {
		t.Fatal(out)
	}
	if _, code := e.Run("loadout", "tag", "v2", "base"); code == 0 {
		t.Fatal("loadout tag is gone: named lists are plans (04 §1.1)")
	}
}

// V16 (04 §2.1): enable without a position puts a mod at the latest place its constraints
// allow — a library before the enabled mod that needs it, anything else at the end.
func TestEnablePlacesByConstraints(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	needs := testkit.Sample("tester.needs", "Needs")
	e.WriteMod(filepath.Join(e.Game, "Mods", "Needs"), needs)
	e.WriteFile(filepath.Join(e.Game, "Mods", "Needs", "About", "About.xml"), strings.Replace(needs.AboutXML(), "</ModMetaData>",
		"<modDependencies><li><packageId>tester.lib</packageId><displayName>Lib</displayName></li></modDependencies></ModMetaData>", 1))
	for _, m := range []string{"Lib", "Other", "Late"} {
		e.WriteMod(filepath.Join(e.Game, "Mods", m), testkit.Sample("tester."+strings.ToLower(m), m))
	}
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.needs", "tester.other")
	scanNow(t, e)
	e.MustRun("enable", refOf(t, e, "Late", ""), refOf(t, e, "Lib", ""))
	if got := active(t, e); !slices.Equal(got, []string{"Core", "Lib", "Needs", "Other", "Late"}) {
		t.Fatalf("enabled: %v", got)
	}
	// An explicit position still wins.
	e.MustRun("disable", refOf(t, e, "Lib", ""))
	e.MustRun("enable", refOf(t, e, "Lib", ""), "--at", "4")
	if got := active(t, e); !slices.Equal(got, []string{"Core", "Needs", "Other", "Lib", "Late"}) {
		t.Fatalf("--at: %v", got)
	}
}

// S13 through the CLI (04 §5.5): a community loadBottom and a user top rule move mods to the
// ends, `sort` explains them under "ends", and the community one can be ignored.
func TestSortTopBottom(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	for _, m := range []string{"A", "B", "C", "Perf"} {
		e.WriteMod(filepath.Join(e.Game, "Mods", m), testkit.Sample("tester."+strings.ToLower(m), m))
	}
	e.WriteFile(filepath.Join(e.Home, "cache", "communityRules.json"), `{"rules": {"tester.perf": {"loadBottom": {"value": true}}}}`)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.perf", "tester.a", "tester.b", "tester.c")
	scanNow(t, e)
	cr := refOf(t, e, "C", "")
	if out := e.MustRun("rule", "add", "top", cr); !strings.Contains(out, "top") {
		t.Fatal(out)
	}
	if _, code := e.Run("rule", "add", "top", cr, refOf(t, e, "A", "")); code == 0 {
		t.Fatal("top takes one reference")
	}
	out := e.MustRun("sort")
	for _, want := range []string{"ends (loadTop / loadBottom", "top    " + cr + "  rule r:1", "bottom " + refOf(t, e, "Perf", "") + "  community  (ignore: community:tester.perf:loadbottom)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("sort output lacks %q:\n%s", want, out)
		}
	}
	if got := active(t, e); !slices.Equal(got, []string{"Core", "C", "A", "B", "Perf"}) {
		t.Fatalf("sorted: %v", got)
	}
	e.MustRun("rule", "add", "ignore", "community:tester.perf:loadbottom")
	if out := e.MustRun("sort", "--dry-run"); strings.Contains(out, "community:tester.perf") {
		t.Fatalf("ignored loadBottom still listed:\n%s", out)
	}
}

// S2–S4 through the CLI: cycle evidence names each About.xml, an ignore rule resolves it,
// an attach rule keeps a language pack right after its mod.
func TestSortCycleIgnoreAttach(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	x := testkit.Sample("tester.x", "X")
	e.WriteMod(filepath.Join(e.Game, "Mods", "X"), x)
	e.WriteFile(filepath.Join(e.Game, "Mods", "X", "About", "About.xml"), strings.Replace(x.AboutXML(), "</ModMetaData>", "<loadAfter><li>tester.y</li></loadAfter></ModMetaData>", 1))
	y := testkit.Sample("tester.y", "Y")
	e.WriteMod(filepath.Join(e.Game, "Mods", "Y"), y)
	e.WriteFile(filepath.Join(e.Game, "Mods", "Y", "About", "About.xml"), strings.Replace(y.AboutXML(), "</ModMetaData>", "<loadAfter><li>tester.x</li></loadAfter></ModMetaData>", 1))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Orig"), testkit.Sample("tester.orig", "Orig"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Other"), testkit.Sample("tester.other", "Other"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "OrigZh"), testkit.Sample("tester.orig.zh", "Orig ZH"))
	e.WriteModsConfig("1.6.4633 rev1261", "tester.origzh", "tester.x", "ludeon.rimworld", "tester.orig", "tester.other", "tester.orig.zh", "tester.y")
	scanNow(t, e)
	out := e.MustRun("sort", "--dry-run")
	xr, yr := refOf(t, e, "X", ""), refOf(t, e, "Y", "")
	for _, want := range []string{"info order.cycle  " + xr + " " + yr, "evidence: decl " + xr + " About.xml loadAfter pkg:tester.y",
		"evidence: decl " + yr + " About.xml loadAfter pkg:tester.x", "(dry run; the workspace is unchanged)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("sort output lacks %q:\n%s", want, out)
		}
	}
	e.MustRun("rule", "add", "ignore", "decl:"+yr+":loadafter:tester.x", "--note", "Y works fine before X")
	e.MustRun("rule", "add", "attach", refOf(t, e, "Orig ZH", ""), refOf(t, e, "Orig", ""))
	out = e.MustRun("sort")
	if strings.Contains(out, "order.cycle") {
		t.Fatal(out)
	}
	got := active(t, e)
	pos := func(n string) int {
		for i, x := range got {
			if x == n {
				return i
			}
		}
		return -1
	}
	if pos("Core") != 0 || pos("Y") > pos("X") || pos("Orig ZH") != pos("Orig")+1 {
		t.Fatalf("sorted: %v", got)
	}
	// S1: sorting a sorted list changes nothing.
	out = e.MustRun("sort")
	if !strings.HasPrefix(out, "sort: 0 moved") {
		t.Fatal(out)
	}
	rules, _ := e.App.Store.Rules(mustEnv(t, e))
	if len(rules) != 2 {
		t.Fatalf("rules: %d", len(rules))
	}
	e.MustRun("rule", "remove", "r:1")
	if out := e.MustRun("sort", "--dry-run"); !strings.Contains(out, "order.cycle") {
		t.Fatal("removing the ignore rule must bring the cycle back")
	}
}

func TestSortRestoresCoreWithPreviewAndUndo(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	e.WriteMod(filepath.Join(e.Game, "Data", "Royalty"), testkit.Sample("ludeon.rimworld.royalty", "Royalty"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "A"), testkit.Sample("tester.a", "A"))
	e.WriteModsConfig("1.6.4633 rev1261", "tester.a")
	scanNow(t, e)
	orig, err := os.ReadFile(e.ModsConfig)
	if err != nil {
		t.Fatal(err)
	}
	out := e.MustRun("sort", "--dry-run")
	if !strings.Contains(out, "1 added") || joined(active(t, e)) != "A" {
		t.Fatalf("preview: %s; active %v", out, active(t, e))
	}
	out = e.MustRun("sort")
	if !strings.Contains(out, "1 added") || joined(active(t, e)) != "Core, A" {
		t.Fatalf("sort: %s; active %v", out, active(t, e))
	}
	e.MustRun("undo")
	if got := joined(active(t, e)); got != "A" {
		t.Fatalf("undo: %s", got)
	}
	e.MustRun("redo")
	if got := joined(active(t, e)); got != "Core, A" {
		t.Fatalf("redo: %s", got)
	}
	if out := e.MustRun("sort"); !strings.HasPrefix(out, "sort: 0 moved") {
		t.Fatalf("repeat sort: %s", out)
	}
	data, err := os.ReadFile(e.ModsConfig)
	if err != nil || string(data) != string(orig) {
		t.Fatalf("sort wrote the game file: %v", err)
	}
}

func TestSortEmptyListRestoresOnlyInstalledCore(t *testing.T) {
	t.Parallel()
	for _, installed := range []bool{true, false} {
		t.Run(map[bool]string{true: "installed", false: "missing"}[installed], func(t *testing.T) {
			e := setup(t, false)
			if !installed {
				e.Move(filepath.Join(e.Game, "Data", "Core"), filepath.Join(e.Root, "UnusedCore"))
			}
			e.WriteModsConfig("1.6.4633 rev1261")
			scanNow(t, e)
			e.MustRun("sort")
			want := ""
			if installed {
				want = "Core"
			}
			if got := joined(active(t, e)); got != want {
				t.Fatalf("active: %s, want %s", got, want)
			}
		})
	}
}

func TestLaunchPlans(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.WriteFile(filepath.Join(e.Steam, "steam.exe"), "")
	out := e.MustRun("launch", "--dry-run")
	if !strings.Contains(out, "steam.exe") || !strings.Contains(out, "-applaunch 294100") || len(e.Launched) != 0 {
		t.Fatalf("steam plan:\n%s", out)
	}
	if !strings.Contains(out, "-savedatafolder=") {
		t.Fatalf("non-default user data must be passed:\n%s", out)
	}
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("enable", refOf(t, e, "B", ""))
	// Edits since the last write do not block: the launch writes them (08 §2 "what was removed").
	if out, code := e.Run("launch", "--force"); code == 0 {
		t.Fatalf("launch --force must be gone:\n%s", out)
	}
	e.MustRun("launch")
	if len(e.Launched) != 1 || !strings.Contains(e.Launched[0], "RimWorldWin64.exe") {
		t.Fatalf("launched %v", e.Launched)
	}
	var n int
	e.App.Store.DB.QueryRow(`SELECT count(*) FROM launch`).Scan(&n)
	if n != 1 || e.App.Store.LastLaunch(mustEnv(t, e)).IsZero() {
		t.Fatal("launch not recorded")
	}
	_ = store.UndoLimit
}

// The ignore reference printed by sort must be accepted verbatim by rule add ignore, for
// every constraint kind (a dependency edge once printed a field name that never matched).
func TestPrintedIgnoreRefsRoundTrip(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	p := testkit.Sample("tester.p", "P")
	p.Deps = []string{"tester.q"}
	e.WriteMod(filepath.Join(e.Game, "Mods", "P"), p)
	q := testkit.Sample("tester.q", "Q")
	e.WriteMod(filepath.Join(e.Game, "Mods", "Q"), q)
	e.WriteFile(filepath.Join(e.Game, "Mods", "Q", "About", "About.xml"), strings.Replace(q.AboutXML(), "</ModMetaData>", "<loadAfter><li>tester.p</li></loadAfter></ModMetaData>", 1))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.q", "tester.p")
	scanNow(t, e)
	out := e.MustRun("sort", "--dry-run")
	var refs []string
	for _, l := range strings.Split(out, "\n") {
		if i := strings.Index(l, "(ignore: "); i >= 0 {
			rest := l[i+len("(ignore: "):]
			refs = append(refs, rest[:strings.Index(rest, ")")])
		}
	}
	if len(refs) != 2 {
		t.Fatalf("want 2 ignorable edges:\n%s", out)
	}
	for _, r := range refs {
		e.MustRun("rule", "add", "ignore", r)
		res, err := e.App.Sort(true)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Result.Cycles) != 0 || res.Result.Ignored != 1 {
			t.Fatalf("ignore %s: cycles %d ignored %d", r, len(res.Result.Cycles), res.Result.Ignored)
		}
		e.MustRun("rule", "remove", "r:"+map[bool]string{true: "1", false: "2"}[r == refs[0]])
	}
}
