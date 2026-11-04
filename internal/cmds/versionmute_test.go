package cmds_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestVersionMuteCLI(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Version.txt"), "1.7.5000 rev1")
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.a", Name: "Alpha", Supported: []string{"1.6"}})
	e.WriteModsConfig("1.7.5000 rev1", "ludeon.rimworld", "tester.a")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("problems", "mute-version", "pkg:tester.a")
	for _, flags := range [][]string{nil, {"--all"}, {"--code", "version.unsupported"}, {"--subject", "pkg:tester.a"}} {
		got := e.MustRun(append([]string{"problems", "--no-analyze"}, flags...)...)
		if strings.Contains(got, "Alpha") || !strings.Contains(got, "ignored-version-warnings: 1 version warnings") {
			t.Fatalf("default flags %v: %s", flags, got)
		}
	}
	got := e.MustRun("problems", "--no-analyze", "--include-muted", "--code", "version.unsupported")
	if !strings.Contains(got, "Alpha") || !strings.Contains(got, "[version warning ignored]") {
		t.Fatalf("include muted: %s", got)
	}
	got = e.MustRun("problems", "--no-analyze", "--include-muted", "--game-version", "1.8")
	if strings.Contains(got, "[version warning ignored]") || !strings.Contains(got, "1.8") {
		t.Fatalf("1.8: %s", got)
	}
	e.MustRun("problems", "unmute-version", "pkg:tester.a", "--game-version", "1.7")
	got = e.MustRun("problems", "--no-analyze", "--code", "version.unsupported")
	if !strings.Contains(got, "Alpha") || strings.Contains(got, "[version warning ignored]") {
		t.Fatalf("unmuted: %s", got)
	}
	if got, code := e.Run("problems", "mute-version", "pkg:tester.a", "--game-version", "1.7.1"); code == 0 {
		t.Fatalf("invalid version accepted: %s", got)
	}
}
