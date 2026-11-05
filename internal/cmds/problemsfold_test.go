package cmds_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// problems --all with more info rows than the limit lists info as one row per code; --code
// lists that code's rows (MCP review round 2 P12). Not parallel: it lowers a package limit.
func TestProblemsAllFoldsInfo(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	ids := []string{"ludeon.rimworld"}
	for _, pair := range [][2]string{{"a", "b"}, {"c", "d"}} {
		for i, id := range pair {
			m := testkit.Sample("tester."+id, strings.ToUpper(id))
			dir := filepath.Join(e.Game, "Mods", strings.ToUpper(id))
			e.WriteMod(dir, m)
			e.WriteFile(filepath.Join(dir, "About", "About.xml"), strings.Replace(m.AboutXML(), "</ModMetaData>",
				fmt.Sprintf("<loadAfter><li>tester.%s</li></loadAfter></ModMetaData>", pair[1-i]), 1))
			ids = append(ids, "tester."+id)
		}
	}
	e.WriteModsConfig("1.6.4633 rev1261", ids...)
	e.MustRun("scan")
	full := e.MustRun("problems", "--all", "--no-analyze")
	if strings.Count(full, "info order.cycle  ") != 2 {
		t.Fatalf("expected two cycles:\n%s", full)
	}
	old := *cmds.InfoFoldLimit
	*cmds.InfoFoldLimit = 1
	defer func() { *cmds.InfoFoldLimit = old }()
	out := e.MustRun("problems", "--all", "--no-analyze")
	if !strings.Contains(out, "info order.cycle  2 problems  (archo problems --code order.cycle; MCP: list_problems code=order.cycle)") ||
		strings.Count(out, "info order.cycle  ") != 1 {
		t.Fatalf("folded:\n%s", out)
	}
	if code := e.MustRun("problems", "--code", "order.cycle", "--no-analyze"); strings.Count(code, "info order.cycle  ") != 2 || strings.Contains(code, "2 problems  (") {
		t.Fatalf("--code is folded:\n%s", code)
	}
}
