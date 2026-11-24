package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/updatechannel"
)

// subscribeMonitor puts a Workshop-subscribed Archo Monitor of a version into the home.
func subscribeMonitor(t *testing.T, e *testkit.Env, item, version string) string {
	t.Helper()
	dir := e.AddWorkshop(item, testkit.Mod{PackageID: app.MonitorPackage, Name: monName})
	about, err := os.ReadFile(filepath.Join(monitorBundle(t, e, version), "About", "About.xml"))
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(filepath.Join(dir, "About", "About.xml"), string(about))
	return dir
}

// 19 §6 step 1 (#21): a Workshop-subscribed Monitor of the same or a newer version is used
// as it is; the bundled one is not copied into the Monitor directory. An older one keeps
// the bundled copy.
func TestMonitorWorkshopSkipsBundledCopy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, workshop, bundled string
		item                    string // the Workshop folder's name
		scanFirst               bool   // the Workshop copy is scanned before the check
		wantCopy                bool
	}{
		{"same version", "0.9.0", "0.9.0", "9999", true, false},
		{"newer Workshop", "0.10.0", "0.9.0", "9999", true, false},
		{"older Workshop", "0.8.0", "0.9.0", "9999", true, true},
		{"not scanned yet", "0.9.0", "0.9.0", updatechannel.MonitorWorkshopItem, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := loadoutEnv(t)
			scanNow(t, e)
			e.MustRun("disable", "pkg:gone.mod")
			subscribeMonitor(t, e, tc.item, tc.workshop)
			if tc.scanFirst {
				scanNow(t, e)
			}
			e.App.MonitorBundle = monitorBundle(t, e, tc.bundled)
			out := e.MustRun("scan")
			target := filepath.Join(monitorDir(e), "ArchoMonitor-"+tc.bundled)
			if _, err := os.Stat(target); (err == nil) != tc.wantCopy {
				t.Fatalf("bundled copy present=%v, want %v\n%s", err == nil, tc.wantCopy, out)
			}
			if !tc.wantCopy && !strings.Contains(out, "using the Workshop copy "+tc.workshop) {
				t.Fatalf("scan did not say it uses the Workshop copy:\n%s", out)
			}
			ms, err := e.App.MonitorState()
			if err != nil || ms.InPlan == nil {
				t.Fatalf("Monitor state: %+v, %v", ms, err)
			}
			if (ms.InPlan.Kind == source.Workshop) == tc.wantCopy {
				t.Fatalf("plan uses %s, Workshop wanted=%v", ms.InPlan.Kind, !tc.wantCopy)
			}
			// A second check changes nothing: no copy, no edit.
			again := e.MustRun("monitor", "install")
			if strings.Contains(again, "installed ") || strings.Contains(again, "one undoable edit") {
				t.Fatalf("second check changed something:\n%s", again)
			}
		})
	}
}

// #21: a local copy installed before the subscription stays where it is, and the plans that
// used it move to the Workshop copy (lineages merged, one undoable edit).
func TestMonitorWorkshopAfterLocalCopy(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("disable", "pkg:gone.mod")
	e.App.MonitorBundle = monitorBundle(t, e, "0.9.0")
	e.MustRun("monitor", "install")
	local := filepath.Join(monitorDir(e), "ArchoMonitor-0.9.0")
	if _, err := os.Stat(local); err != nil {
		t.Fatal(err)
	}
	subscribeMonitor(t, e, "9999", "0.9.0")
	out := e.MustRun("scan")
	if !strings.Contains(out, "now uses the workshop copy") && !strings.Contains(out, "now uses the Workshop copy") {
		t.Fatalf("plan did not move to the Workshop copy:\n%s", out)
	}
	if _, err := os.Stat(local); err != nil {
		t.Fatalf("the local copy was removed: %v", err)
	}
	ms, err := e.App.MonitorState()
	if err != nil || ms.InPlan == nil || ms.InPlan.Kind != source.Workshop {
		t.Fatalf("Monitor state: %+v, %v", ms, err)
	}
	e.MustRun("undo")
	ms, err = e.App.MonitorState()
	if err != nil || ms.InPlan == nil || ms.InPlan.Kind == source.Workshop {
		t.Fatalf("undo did not bring the local copy back: %+v, %v", ms, err)
	}
}
