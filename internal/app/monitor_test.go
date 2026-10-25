package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
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

func monitorDir(e *testkit.Env) string {
	return filepath.Join(e.Root, "LocalAppData", "RimArcho", "monitor")
}

// 08 §9, 04 §1.5 (M1, M2, M4): the bundled monitor is copied, registered as src:monitor,
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

// 04 §1.5: monitor.default off adds nothing; plans are not marked, so turning it on later
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
	if code == 0 || !strings.Contains(out, "Archo Monitor reaches the game only through a link") || !strings.Contains(out, "archo disable pkg:toumanlin.archomonitor") {
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

// 08 §9: RIMARCHO_MONITOR points RimArcho at a locally built monitor.
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

// 06 §2.10: the log page's "add Archo Monitor" puts it back after Harmony in the current
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
