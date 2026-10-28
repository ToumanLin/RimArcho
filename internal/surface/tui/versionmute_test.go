package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestVersionMuteDetailRefresh(t *testing.T) {
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Version.txt"), "1.7.5000 rev1")
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.a", Name: "Alpha", Supported: []string{"1.6"}})
	e.WriteModsConfig("1.7.5000 rev1", "ludeon.rimworld", "tester.a")
	e.AddEnv(false)
	e.MustRun("scan")
	m := direct(t, e, 160, 40)
	press(m, "home")
	press(m, "down")
	if m.current().name != "Alpha" {
		t.Fatalf("cursor on %q", m.current().name)
	}
	runCmd(m, m.loadData(m.current().id, false, false))
	for _, step := range []struct {
		key, kind, text string
	}{
		{"M", view.ActVersionUnmute, "warnings about 1.7 are muted for this mod"},
		{"U", view.ActVersionMute, "does not list 1.7 as supported (lists 1.6)"},
	} {
		cmd, handled := m.versionDetailKey(step.key)
		if !handled {
			t.Fatalf("%s not handled", step.key)
		}
		// The cached detail must update immediately, before the async detail command.
		if m.dd.Version == nil || m.dd.Version.Actions[0].Kind != step.kind {
			t.Fatalf("stale detail after %s: %+v", step.key, m.dd.Version)
		}
		runCmd(m, cmd)
		if got := plainLines(m.detailLayout(detailOpts{width: 90})); !strings.Contains(got, step.text) {
			t.Fatalf("after %s: %s", step.key, got)
		}
	}
}
