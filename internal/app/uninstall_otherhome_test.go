package app_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// Two RimArcho homes share one game (02 §2): uninstalling this one removes its own
// Monitor link and leaves the other home's links into that home's folders, even after a
// scan found them in Mods/ as Mods of their own.
func TestUninstallLeavesOtherHomesLinks(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.App.MonitorBundle = monitorBundle(t, e, "0.11.0")
	e.MustRun("monitor", "install")
	e.MustRun("save")
	own := filepath.Join(e.Game, "Mods", monitorLink)
	ownTarget, err := source.RealPath(own)
	if err != nil {
		t.Fatal(err)
	}
	// The other home's Monitor (a newer version) and a SteamCMD Mod of its own.
	other := map[string]string{
		"_Archo_monitor_ArchoMonitor-0.12.0": filepath.Join(e.Root, "OtherHome", "Local", "monitor", "ArchoMonitor-0.12.0"),
		"_Archo_steamcmd_7777":               filepath.Join(e.Root, "OtherHome", "Local", "steamcmd", "7777"),
	}
	for name, target := range other {
		copyAsExplorer(t, ownTarget, target)
		if err := source.CreateLink(filepath.Join(e.Game, "Mods", name), target); err != nil {
			t.Fatal(err)
		}
	}
	scanNow(t, e)
	p, err := e.App.PlanUninstall(app.UninstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, it := range p.Step(app.UnLinks).Items {
		got[filepath.Base(it.Path)] = it.Do
	}
	if got[monitorLink] != app.DoUnlink {
		t.Fatalf("this home's Monitor link: %q, want unlink (%v)", got[monitorLink], got)
	}
	for name := range other {
		if got[name] != app.DoLeave {
			t.Fatalf("%s points into another home: %q, want left alone (%v)", name, got[name], got)
		}
	}
	// The link check names them as not this home's instead of healthy links of its own.
	if out := e.MustRun("links", "check", "--all"); !strings.Contains(out, "_Archo_steamcmd_7777") {
		t.Fatalf("links check does not report the other home's link:\n%s", out)
	}
}
