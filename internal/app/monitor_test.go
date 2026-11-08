package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// monitorBundle writes a bundled Archo Monitor of a version, as monitor/build.ps1 makes it.
func monitorBundle(t *testing.T, e *testkit.Env, version string) string {
	t.Helper()
	dir := filepath.Join(e.Root, "bundle-"+version, "ArchoMonitor")
	e.WriteFile(filepath.Join(dir, "About", "About.xml"), `<?xml version="1.0" encoding="utf-8"?>
<ModMetaData>
  <name>Archo Monitor</name>
  <author>Touman Lin</author>
  <packageId>toumanlin.archomonitor</packageId>
  <modVersion>`+version+`</modVersion>
  <supportedVersions><li>1.5</li><li>1.6</li><li>1.7</li></supportedVersions>
  <modDependencies><li><packageId>brrainz.harmony</packageId><displayName>Harmony</displayName></li></modDependencies>
  <loadAfter><li>brrainz.harmony</li></loadAfter>
  <loadBefore><li>Ludeon.RimWorld</li></loadBefore>
</ModMetaData>
`)
	e.WriteFile(filepath.Join(dir, "LoadFolders.xml"), "<loadFolders><v1.6><li>/</li><li>1.6</li></v1.6></loadFolders>\n")
	e.WriteFile(filepath.Join(dir, "1.6", "Assemblies", "ArchoMonitor.dll"), "MZ "+version)
	return dir
}

const monName = "Archo Monitor"

// 19 §6: Workshop wins version ties, including on exFAT; an update from any source
// leaves one Monitor in the plan and removes an obsolete managed link on save.
func TestMonitorWorkshopSelection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, bundled, fs string
		wantWorkshop      bool
	}{
		{"tie", "0.9.0", "NTFS", true},
		{"tie on exFAT", "0.9.0", "exFAT", true},
		{"higher source version", "0.10.0", "NTFS", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := loadoutEnv(t)
			scanNow(t, e)
			e.MustRun("disable", "pkg:gone.mod")
			e.App.MonitorBundle = monitorBundle(t, e, tc.bundled)
			e.MustRun("monitor", "install")
			e.MustRun("save")
			link := filepath.Join(e.Game, "Mods", "_Archo_monitor_ArchoMonitor-"+tc.bundled)
			workshop := e.AddWorkshop("9999", testkit.Mod{PackageID: app.MonitorPackage, Name: monName})
			about, err := os.ReadFile(filepath.Join(monitorBundle(t, e, "0.9.0"), "About", "About.xml"))
			if err != nil {
				t.Fatal(err)
			}
			e.WriteFile(filepath.Join(workshop, "About", "About.xml"), string(about))
			scanNow(t, e)
			st, err := e.App.Loadout()
			if err != nil {
				t.Fatal(err)
			}
			// Reproduce an imported list that enabled both the local and _steam tokens.
			ws := *st.WS
			ws.Entries = append(st.List.Clone(), loadout.Entry{Instance: refID(t, e, monName, source.Workshop)}).Encode()
			if err := e.App.Store.SetWorkspace(&ws); err != nil {
				t.Fatal(err)
			}
			e.App.VolumeFS = func(string) string { return tc.fs }
			e.MustRun("monitor", "install")
			ms, err := e.App.MonitorState()
			if err != nil || ms.InPlan == nil {
				t.Fatalf("Monitor state: %+v, %v", ms, err)
			}
			if (ms.InPlan.Kind == source.Workshop) != tc.wantWorkshop || ms.Newest.Mod.ID != ms.InPlan.ID {
				t.Fatalf("wrong Monitor selected: %+v", ms)
			}
			if got := active(t, e); strings.Count(strings.Join(got, ","), monName) != 1 {
				t.Fatalf("Monitor not collapsed to one entry: %v", got)
			}
			if out := e.MustRun("problems", "--all"); strings.Contains(out, "pkg.duplicate-") {
				t.Fatalf("Monitor reported as duplicate:\n%s", out)
			}
			e.MustRun("save")
			if source.IsLink(link) == tc.wantWorkshop {
				t.Fatalf("managed link present=%v, Workshop selected=%v", source.IsLink(link), tc.wantWorkshop)
			}
			cfg := e.ReadModsConfig()
			if strings.Count(cfg, "<li>"+app.MonitorPackage) != 1 || strings.Contains(cfg, app.MonitorPackage+"_steam") {
				t.Fatalf("Monitor active more than once or using a stale token:\n%s", cfg)
			}
			if out := e.MustRun("status"); !strings.Contains(out, "ArchoMonitor "+tc.bundled+" in the plan") {
				t.Fatalf("status did not use modVersion:\n%s", out)
			}
		})
	}
}

// A subscribed Monitor works without a bundled copy or Monitor source, including
// default insertion, sorting and explicit addition on exFAT.
func TestMonitorWorkshopOnlyExFAT(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	e.App.VolumeFS = func(string) string { return "exFAT" }
	dir := e.AddWorkshop("9999", testkit.Mod{PackageID: app.MonitorPackage, Name: monName})
	about, err := os.ReadFile(filepath.Join(monitorBundle(t, e, "0.9.0"), "About", "About.xml"))
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(filepath.Join(dir, "About", "About.xml"), string(about))
	e.MustRun("scan")
	e.MustRun("disable", "pkg:gone.mod")
	if !contains(active(t, e), monName) {
		t.Fatal("Workshop-only Monitor was not added by default")
	}
	e.MustRun("disable", "pkg:"+app.MonitorPackage)
	e.MustRun("sort")
	if !contains(active(t, e), monName) {
		t.Fatal("sort did not add Workshop-only Monitor")
	}
	e.MustRun("disable", "pkg:"+app.MonitorPackage)
	if _, err := e.App.AddMonitor(); err != nil {
		t.Fatal(err)
	}
	e.MustRun("save")
	links, err := filepath.Glob(filepath.Join(e.Game, "Mods", "_Archo_*"))
	if err != nil || len(links) != 0 {
		t.Fatalf("Workshop-only Monitor needed links: %v, %v", links, err)
	}
}

func monitorDir(e *testkit.Env) string {
	return filepath.Join(e.Root, "LocalAppData", "RimArcho", "monitor")
}

// 19 §6, 07 §1.5 (M1, M2, M4): the bundled monitor is copied, registered as src:monitor,
// and added right after Harmony to every plan once, as one undoable edit; then it is a
// normal mod.
func TestMonitorInstallAndDefault(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("plan", "new", "other", "--from", "current")
	e.MustRun("plan", "new", "bare") // empty: no Harmony
	e.App.MonitorBundle = monitorBundle(t, e, "0.9.0")

	out := e.MustRun("monitor", "install")
	for _, want := range []string{"installed 0.9.0", "registered src:monitor", "plan:default: added after Harmony", "plan:other: added after Harmony", "plan:bare: not added: no Harmony"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	target := filepath.Join(monitorDir(e), "ArchoMonitor-0.9.0")
	if !source.HasAbout(target) {
		t.Fatalf("not installed at %s", target)
	}
	if got := active(t, e); !contains(got, monName) || strings.Join(got, ",") != strings.Join(insertAfter(got, "Harmony"), ",") {
		t.Fatalf("not right after Harmony: %v", got)
	}
	if out := e.MustRun("status"); !strings.Contains(out, "ArchoMonitor 0.9.0 in the plan (after Harmony)") {
		t.Fatalf("status:\n%s", out)
	}

	// M2: nothing to do the second time.
	out = e.MustRun("monitor", "install")
	if !strings.Contains(out, "up to date: 0.9.0") || strings.Contains(out, "plan:default") || strings.Contains(out, "registered") {
		t.Fatalf("second install:\n%s", out)
	}

	// M4: saving links it and writes it after Harmony.
	e.MustRun("save")
	link := filepath.Join(e.Game, "Mods", "_Archo_monitor_ArchoMonitor-0.9.0")
	if !source.IsLink(link) {
		t.Fatal("no _Archo_ link for the monitor")
	}
	cfg := e.ReadModsConfig()
	if h, m := strings.Index(cfg, "<li>brrainz.harmony</li>"), strings.Index(cfg, "<li>toumanlin.archomonitor</li>"); h < 0 || m < h {
		t.Fatalf("ModsConfig.xml:\n%s", cfg)
	}

	// One undoable edit; once removed it stays removed.
	e.MustRun("undo")
	if contains(active(t, e), monName) {
		t.Fatal("undo did not remove the monitor")
	}
	if out := e.MustRun("monitor", "install"); strings.Contains(out, "plan:default") {
		t.Fatalf("added again after the player removed it:\n%s", out)
	}
	if out := e.MustRun("monitor"); !strings.Contains(out, "not in the plan (archo enable") {
		t.Fatalf("monitor:\n%s", out)
	}

	// New plans get it at once (copying a plan without it included).
	out = e.MustRun("plan", "new", "fresh", "--from", "current")
	if !strings.Contains(out, "Archo Monitor added after Harmony") {
		t.Fatalf("new plan:\n%s", out)
	}
	e.MustRun("plan", "use", "fresh")
	if !contains(active(t, e), monName) {
		t.Fatalf("fresh: %v", active(t, e))
	}
	e.MustRun("undo")
	if contains(active(t, e), monName) {
		t.Fatal("the new plan's insertion is not undoable")
	}
}

// insertAfter is got with the monitor moved right after pkg (the expected order).
func insertAfter(got []string, pkg string) []string {
	var out []string
	for _, p := range got {
		if p == monName {
			continue
		}
		out = append(out, p)
		if p == pkg {
			out = append(out, monName)
		}
	}
	return out
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// M3: a new bundled version is installed next to the old one, joins its lineage and
// replaces it in the plans; undo goes back to the old version.
func TestMonitorUpdate(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.App.MonitorBundle = monitorBundle(t, e, "0.9.0")
	e.MustRun("monitor", "install")
	e.App.MonitorBundle = monitorBundle(t, e, "0.10.0")
	out := e.MustRun("monitor", "install")
	if !strings.Contains(out, "installed 0.10.0") || !strings.Contains(out, "plan:default: updated 0.9.0 → 0.10.0") {
		t.Fatalf("update:\n%s", out)
	}
	if !source.HasAbout(filepath.Join(monitorDir(e), "ArchoMonitor-0.9.0")) {
		t.Fatal("the old version folder is gone")
	}
	ms := named(t, e, "Archo Monitor")
	if len(ms) != 2 || ms[0].LineageID != ms[1].LineageID {
		t.Fatalf("%d instances, same lineage %v", len(ms), len(ms) == 2 && ms[0].LineageID == ms[1].LineageID)
	}
	if out := e.MustRun("status"); !strings.Contains(out, "ArchoMonitor 0.10.0 in the plan") {
		t.Fatalf("status:\n%s", out)
	}
	e.MustRun("undo")
	if out := e.MustRun("status"); !strings.Contains(out, "ArchoMonitor 0.9.0 in the plan") {
		t.Fatalf("after undo:\n%s", out)
	}
	if app.CompareVersions("0.10.0", "0.9.0") <= 0 || app.CompareVersions("0.9.0", "0.9.0-dev") <= 0 || app.CompareVersions("1.0.0-a", "1.0.0-b") >= 0 {
		t.Fatal("CompareVersions")
	}
}

// 19 §6 step 4: the first `archo scan` installs the monitor before reading, so one scan
// reads every folder once, the monitor's included; an update reads only the monitor's folder.
func TestMonitorScanOnePass(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	e.App.MonitorBundle = monitorBundle(t, e, "0.9.0")
	cur, err := e.App.Env()
	if err != nil {
		t.Fatal(err)
	}
	// newScans are the scans since the last call, each with the sources it read.
	seen := 0
	newScans := func() [][]string {
		t.Helper()
		scans, err := e.App.Store.Scans(cur.ID, 100)
		if err != nil {
			t.Fatal(err)
		}
		var out [][]string
		for _, sc := range scans[:len(scans)-seen] {
			var st app.ScanStats
			sc.DecodeStats(&st)
			var read []string
			for _, s := range st.Sources {
				read = append(read, s.Label)
			}
			out = append(out, read)
		}
		seen = len(scans)
		return out
	}
	live := func() int {
		t.Helper()
		n := 0
		for _, m := range allMods(t, e) {
			if m.Status == "live" {
				n++
			}
		}
		return n
	}

	out := e.MustRun("scan")
	for _, want := range []string{"src:monitor", "installed 0.9.0", "registered src:monitor", "plan:default: added after Harmony"} {
		if !strings.Contains(out, want) {
			t.Fatalf("first scan: missing %q:\n%s", want, out)
		}
	}
	if got := newScans(); len(got) != 1 || !slices.Contains(got[0], app.MonitorLabel) || len(got[0]) < 3 {
		t.Fatalf("first scan read %v; want one scan of every source", got)
	}
	if got := active(t, e); strings.Join(got, ",") != strings.Join(insertAfter(got, "Harmony"), ",") || !contains(got, monName) {
		t.Fatalf("not right after Harmony: %v", got)
	}
	before := live()

	// A new version on window open (`monitor install`): only src:monitor is read again.
	e.App.MonitorBundle = monitorBundle(t, e, "0.10.0")
	if out := e.MustRun("monitor", "install"); !strings.Contains(out, "plan:default: updated 0.9.0 → 0.10.0") {
		t.Fatalf("update:\n%s", out)
	}
	if got := newScans(); len(got) != 1 || !slices.Equal(got[0], []string{app.MonitorLabel}) {
		t.Fatalf("update read %v; want only src:monitor", got)
	}
	if n := live(); n != before+1 {
		t.Fatalf("%d live instances after the update, want %d (the other sources keep theirs)", n, before+1)
	}

	// `scan --game-only` with a new version reads the game's folders and the monitor's, once.
	e.App.MonitorBundle = monitorBundle(t, e, "0.11.0")
	if out := e.MustRun("scan", "--game-only"); !strings.Contains(out, "plan:default: updated 0.10.0 → 0.11.0") {
		t.Fatalf("game-only scan:\n%s", out)
	}
	got := newScans()
	if len(got) != 1 || !slices.Contains(got[0], app.MonitorLabel) || slices.Contains(got[0], "workshop") {
		t.Fatalf("game-only scan read %v; want one scan of the game's folders and src:monitor", got)
	}
	if n := live(); n != before+2 {
		t.Fatalf("%d live instances after the game-only scan, want %d", n, before+2)
	}
	if ms := named(t, e, monName); len(ms) != 3 || ms[0].LineageID != ms[2].LineageID {
		t.Fatalf("%d monitor instances, not one lineage", len(ms))
	}
}

// 07 §1.5: monitor.default off adds nothing; plans are not marked, so turning it on later
// still adds it.
func TestMonitorDefaultOff(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("config", "set", "monitor.default", "off")
	e.App.MonitorBundle = monitorBundle(t, e, "0.9.0")
	out := e.MustRun("monitor", "install")
	if !strings.Contains(out, "plan:default: not added: monitor.default is off") || contains(active(t, e), monName) {
		t.Fatalf("off:\n%s", out)
	}
	e.MustRun("config", "set", "monitor.default", "on")
	if out := e.MustRun("scan"); !strings.Contains(out, "plan:default: added after Harmony") {
		t.Fatalf("scan after on:\n%s", out)
	}
}

// M5: on exFAT the default skips the monitor; enabled by hand, saving refuses and names it.
func TestMonitorExFAT(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	e.App.VolumeFS = func(string) string { return "exFAT" }
	e.App.MonitorBundle = monitorBundle(t, e, "0.9.0")
	out := e.MustRun("monitor", "install")
	if !strings.Contains(out, "not added: Mods/ is on exFAT") || contains(active(t, e), monName) {
		t.Fatalf("exFAT:\n%s", out)
	}
	saved := app.CreateLink
	app.CreateLink = func(link, target string) error { return errors.New("the volume does not support junctions") }
	defer func() { app.CreateLink = saved }()
	e.MustRun("enable", "pkg:"+app.MonitorPackage, "--after", "pkg:brrainz.harmony")
	out, code := e.Run("save")
	if code == 0 || !strings.Contains(out, "this Archo Monitor copy needs a link") || !strings.Contains(out, "archo disable pkg:toumanlin.archomonitor") {
		t.Fatalf("save on exFAT: exit %d\n%s", code, out)
	}
	e.MustRun("disable", "pkg:"+app.MonitorPackage)
}

// M7: a development build with the same version replaces its folder, unless a game runs.
func TestMonitorDevReplace(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	b := monitorBundle(t, e, "0.9.0-dev")
	e.App.MonitorBundle = b
	e.MustRun("monitor", "install")
	dll := filepath.Join(b, "1.6", "Assemblies", "ArchoMonitor.dll")
	e.WriteFile(dll, "MZ rebuilt")
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42, Started: e.Now()} }
	if out := e.MustRun("monitor", "install"); !strings.Contains(out, "a game is running") {
		t.Fatalf("running game:\n%s", out)
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	if out := e.MustRun("monitor", "install"); !strings.Contains(out, "replaced ") {
		t.Fatalf("replace:\n%s", out)
	}
	got, _ := os.ReadFile(filepath.Join(monitorDir(e), "ArchoMonitor-0.9.0-dev", "1.6", "Assemblies", "ArchoMonitor.dll"))
	if string(got) != "MZ rebuilt" {
		t.Fatalf("installed dll %q", got)
	}
	// A release folder is never replaced.
	b2 := monitorBundle(t, e, "0.9.1")
	e.App.MonitorBundle = b2
	e.MustRun("monitor", "install")
	e.WriteFile(filepath.Join(b2, "1.6", "Assemblies", "ArchoMonitor.dll"), "MZ other")
	if out := e.MustRun("monitor", "install"); !strings.Contains(out, "a release folder is never replaced") {
		t.Fatalf("release:\n%s", out)
	}
}

// M8: without a bundled monitor nothing is installed and `monitor` says where it looked.
func TestMonitorNotBundled(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.App.MonitorBundle = filepath.Join(e.Root, "nowhere")
	out := e.MustRun("monitor")
	if !strings.Contains(out, "none found") || !strings.Contains(out, "nowhere") {
		t.Fatalf("monitor:\n%s", out)
	}
	if out := e.MustRun("monitor", "install"); strings.Contains(out, "installed") {
		t.Fatalf("install without a bundle:\n%s", out)
	}
	if _, err := os.Stat(monitorDir(e)); err == nil {
		t.Fatal("the monitor folder was created")
	}
}

// 19 §6: RIMARCHO_MONITOR points RimArcho at a locally built monitor.
func TestMonitorDevPath(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	b := monitorBundle(t, e, "0.0.0-dev")
	t.Setenv("RIMARCHO_MONITOR", b)
	e.App.MonitorBundle = ""
	got, tried := e.App.FindMonitor()
	if got == nil || got.Dir != b || got.Version != "0.0.0-dev" || tried[0] != b {
		t.Fatalf("found %+v (tried %v)", got, tried)
	}
}

// M6: a pack carries the monitor like any other mod (a copy, not a link).
func TestMonitorInPack(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("disable", "pkg:gone.mod")
	e.App.MonitorBundle = monitorBundle(t, e, "0.9.0")
	e.MustRun("monitor", "install")
	out := e.MustRun("pack", "build", "p")
	matches, _ := filepath.Glob(filepath.Join(e.App.Home, "packs", "p", "mods", "*", "About", "About.xml"))
	found := false
	for _, m := range matches {
		b, _ := os.ReadFile(m)
		found = found || strings.Contains(string(b), app.MonitorPackage)
	}
	if !found {
		t.Fatalf("no monitor copy in the pack (%v):\n%s", matches, out)
	}
}

// 16 §2.10: the log page's "add Archo Monitor" puts it back after Harmony in the current
// plan, also after the player removed it; one step that undo takes back.
func TestAddMonitor(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	if _, err := e.App.AddMonitor(); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("no monitor installed: %v", err)
	}
	e.App.MonitorBundle = monitorBundle(t, e, "0.9.0")
	e.MustRun("monitor", "install")
	if _, err := e.App.AddMonitor(); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("already in the plan: %v", err)
	}
	e.MustRun("disable", "pkg:toumanlin.archomonitor")
	before := active(t, e)
	m, err := e.App.AddMonitor()
	if err != nil || !app.IsMonitor(m) {
		t.Fatalf("add: %v %v", m, err)
	}
	if got := active(t, e); !contains(got, monName) || strings.Join(got, ",") != strings.Join(insertAfter(got, "Harmony"), ",") {
		t.Fatalf("not right after Harmony: %v", got)
	}
	if _, _, err := e.App.UndoRedo(false); err != nil || strings.Join(active(t, e), ",") != strings.Join(before, ",") {
		t.Fatalf("undo: %v %v", err, active(t, e))
	}
}

// 07 §1.5, 08 §1.1: with monitor.default on, every sort puts Archo Monitor back after Harmony,
// also after the player took it out; off, a sort leaves it out; a list without Harmony
// says why it was not added.
func TestSortAddsMonitor(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.App.MonitorBundle = monitorBundle(t, e, "0.9.0")
	e.MustRun("monitor", "install")
	e.MustRun("undo") // the player took it out; install alone does not add it again
	if contains(active(t, e), monName) {
		t.Fatal("undo did not remove the monitor")
	}

	r, err := e.App.Sort(false)
	if err != nil {
		t.Fatal(err)
	}
	got := active(t, e)
	if !contains(got, monName) || strings.Join(got, ",") != strings.Join(insertAfter(got, "Harmony"), ",") {
		t.Fatalf("sort did not put the monitor after Harmony: %v", got)
	}
	if r.MonitorSkipped != "" {
		t.Fatalf("skipped: %q", r.MonitorSkipped)
	}
	e.MustRun("undo") // one step with the sort
	if contains(active(t, e), monName) {
		t.Fatal("the sort's insertion is not undone with it")
	}

	e.MustRun("config", "set", "monitor.default", "off")
	e.MustRun("sort")
	if contains(active(t, e), monName) {
		t.Fatal("sort added the monitor with monitor.default off")
	}

	e.MustRun("config", "set", "monitor.default", "on")
	e.MustRun("plan", "new", "bare")
	e.MustRun("plan", "use", "bare")
	if out := e.MustRun("sort"); !strings.Contains(out, "Archo Monitor not added: "+app.MonitorNoHarmony) {
		t.Fatalf("no Harmony:\n%s", out)
	}
}

// M9 (VM 2026-10-28): the game was launched before the monitor check finished (the first
// scan took 27 s), and the check then put the newer Archo Monitor into the plan while the
// game ran. A monitor version swap is not a list change: the run's log is still counted
// (06 §4). The old copy's _Archo_ link, left in Mods/ until the next save removes it, does
// not make the plan's new copy look unloadable; the next launch links the new one.
func TestMonitorUpdateAfterLaunch(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.MustRun("env", "set", "main", "--launch", "direct")
	mods := filepath.Join(e.Game, "Mods")
	e.App.MonitorBundle = monitorBundle(t, e, "0.9.0-dev")
	e.MustRun("monitor", "install")
	e.Advance(time.Minute)
	e.MustRun("launch")
	oldLink := filepath.Join(mods, "_Archo_monitor_ArchoMonitor-0.9.0-dev")
	if !source.IsLink(oldLink) {
		t.Fatal("the launch did not link Archo Monitor 0.9.0-dev")
	}
	launched := launches(t, e)[0]

	// The check finishes while the game runs and swaps in 0.9.1.
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42, Started: launched.Started} }
	e.App.MonitorBundle = monitorBundle(t, e, "0.9.1")
	if out := e.MustRun("monitor", "install"); !strings.Contains(out, "plan:default: updated 0.9.0-dev → 0.9.1") {
		t.Fatalf("update while the game runs:\n%s", out)
	}

	// The game writes its log, then exits.
	e.Advance(time.Minute)
	p := filepath.Join(e.UserData, "Player.log")
	e.WriteFile(p, testkit.FixtureLog(filepath.Join(mods, "Compat", "Patches", "Compat.xml")))
	os.Chtimes(p, e.Now(), e.Now())
	e.App.GameProbe = func(string) *app.GameRun { return nil }

	r := logRun(t, e)
	if r.Stale() || r.Source == nil || r.Source.Launch == nil || r.Source.Launch.ID != launched.ID {
		t.Fatalf("the run's log after the monitor update: %+v %+v", r, r.Source)
	}
	out := e.MustRun("problems")
	if strings.Contains(out, "stale log") || !strings.Contains(out, "current (from the launch of plan:default at ") {
		t.Fatalf("problems after the monitor update:\n%s", out)
	}
	for _, f := range problems(t, e).List {
		if f.Code == "mod.unloadable" {
			t.Fatalf("the old monitor's link counts against the new one: %s %s", f.Code, f.Claim)
		}
	}

	// The next launch links 0.9.1 and removes the old link.
	e.Advance(time.Minute)
	e.MustRun("launch")
	if source.IsLink(oldLink) || !source.IsLink(filepath.Join(mods, "_Archo_monitor_ArchoMonitor-0.9.1")) {
		t.Fatal("the next launch did not swap the monitor links")
	}

	// Any other change since the launch still makes its log stale.
	e.Advance(time.Minute)
	e.WriteFile(p, testkit.FixtureLog(filepath.Join(mods, "Compat", "Patches", "Compat.xml")))
	os.Chtimes(p, e.Now(), e.Now())
	if r := logRun(t, e); r.Stale() {
		t.Fatalf("the new launch's log: %+v", r)
	}
	e.MustRun("disable", "pkg:tester.manual")
	if r := logRun(t, e); !r.Changed {
		t.Fatalf("a real change since the launch: %+v", r)
	}
}
