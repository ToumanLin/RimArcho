package cmds_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// A failed update replaces nothing (08 §2.3 D2), so its output never says "updated" or
// shows the downloaded version; each dataset names its failure or the version it kept.
func TestDataUpdateFailureReportsKeptVersions(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	src := func(key string) string { return filepath.Join(e.Root, "sources", key+".json") }
	e.WriteFile(src("rules"), `{"timestamp":1,"rules":{}}`)
	e.WriteFile(src("replacements"), `{"version":"r1","rules":[]}`)
	e.MustRun("config", "set", "data.rules.url", src("rules"))
	e.MustRun("config", "set", "data.replacements.url", src("replacements"))
	e.MustRun("data", "update", "rules", "replacements")

	check := func(out string, code int, want ...string) {
		t.Helper()
		if code == cmds.ExitOK || strings.Contains(out, "community data updated") || strings.Contains(out, "version 2") {
			t.Fatalf("failure reported as an update (exit %d):\n%s", code, out)
		}
		for _, w := range want {
			if !strings.Contains(out, w) {
				t.Fatalf("missing %q:\n%s", w, out)
			}
		}
	}

	// One dataset fails to download: the other is not replaced either.
	e.WriteFile(src("rules"), `{"timestamp":2,"rules":{}}`)
	e.WriteFile(src("replacements"), `not json`)
	out, code := e.Run("data", "update", "rules", "replacements")
	check(out, code, "community data not updated", "rules: not updated; kept version 1", "replacements: failed: ")

	// Every download is valid, but replacing the stored file fails and is rolled back.
	e.WriteFile(src("replacements"), `{"version":"2","rules":[]}`)
	stored := filepath.Join(e.Home, "cache", "communityRules.json")
	if err := os.Remove(stored); err != nil {
		t.Fatal(err)
	}
	e.MkdirAll(stored)
	out, code = e.Run("data", "update", "rules")
	check(out, code, "community data not updated", "rules: not updated; kept version 1")
	if st := e.App.DataStatus()["rules"]; st == nil || st.Version != "1" {
		t.Fatalf("stored version changed: %+v", st)
	}
}
