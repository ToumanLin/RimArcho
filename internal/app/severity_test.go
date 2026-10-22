package app_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Severities recalibrated on the log corpus (03 §4.3, decisions-corpus §2 G8): official.order is
// a warning, order.cycle info; version.unsupported is a warning when the mod stops at the
// previous minor version and info when it lists only older ones.
func TestCorpusSeverities(t *testing.T) {
	e := setup(t, false)
	prev := testkit.Sample("tester.prev", "Prev")
	prev.Supported = []string{"1.4", "1.5"}
	older := testkit.Sample("tester.older", "Older")
	older.Supported = []string{"1.3", "1.4"}
	e.WriteMod(filepath.Join(e.Game, "Mods", "Prev"), prev)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Older"), older)
	for _, p := range [][2]string{{"X", "tester.y"}, {"Y", "tester.x"}} {
		m := testkit.Sample("tester."+strings.ToLower(p[0]), p[0])
		e.WriteMod(filepath.Join(e.Game, "Mods", p[0]), m)
		e.WriteFile(filepath.Join(e.Game, "Mods", p[0], "About", "About.xml"), strings.Replace(m.AboutXML(), "</ModMetaData>",
			"<loadAfter><li>"+p[1]+"</li></loadAfter></ModMetaData>", 1))
	}
	// A mod before Core: official.order.
	e.WriteModsConfig("1.6.4633 rev1261", "tester.prev", "ludeon.rimworld", "tester.older", "tester.x", "tester.y")
	scanNow(t, e)
	ps := problems(t, e)
	sev := func(code, subject string) string {
		for _, p := range byCode(ps.List, code) {
			if subject == "" || p.SubjectInstance() == instOf(t, e, subject) {
				return p.Severity
			}
		}
		t.Fatalf("no %s %s", code, subject)
		return ""
	}
	for _, c := range []struct{ code, subject, want string }{
		{"official.order", "", finding.Warning},
		{"order.cycle", "", finding.Info},
		{"version.unsupported", "pkg:tester.prev", finding.Warning},
		{"version.unsupported", "pkg:tester.older", finding.Info},
	} {
		if got := sev(c.code, c.subject); got != c.want {
			t.Errorf("%s %s: %s, want %s", c.code, c.subject, got, c.want)
		}
	}
	// The folded row counts them the right way round.
	if out := e.MustRun("problems"); !strings.Contains(out, "2 enabled mods do not list 1.6 (1 list 1.5, 1 older only)") {
		t.Errorf("version row:\n%s", out)
	}
}
