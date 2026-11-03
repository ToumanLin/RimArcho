package app_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/gametest"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func comparisonLog(names ...string) string {
	var b strings.Builder
	for _, name := range names {
		fmt.Fprintf(&b, "Exception: %s\n  at Compare.%s.Start()\n", name, name)
	}
	return b.String()
}

func runComparisonJSON(t *testing.T, e *testkit.Env, text string, flags ...string) app.TestRunReport {
	t.Helper()
	e.App.TestGameStart = fakeTestGame(t, "menu", text)
	args := append([]string{"test", "run", "--timeout", "5s", "--settle", "1ms", "--json"}, flags...)
	out, code := e.Run(args...)
	var r app.TestRunReport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("report (exit %d): %s: %v", code, out, err)
	}
	wantCode := 1
	if r.Success() {
		wantCode = 0
	}
	if code != wantCode {
		t.Fatalf("run outcome: exit %d, want %d: %s", code, wantCode, out)
	}
	saved, err := os.ReadFile(filepath.Join(r.RunDir, "report.json"))
	if err != nil || string(saved) != out {
		t.Fatalf("saved report differs: %v", err)
	}
	if strings.Contains(out, `"key"`) || strings.Contains(out, `"clusterKeys"`) {
		t.Fatal("comparison keys leaked into JSON")
	}
	return r
}

func TestTestRunComparison(t *testing.T) {
	t.Parallel()
	e := testRunEnv(t)
	settings := "XML error: unknown field in type PrefsData\n"
	info := "[SafePatcher]The xPath: missing not found\nGUI Error: You are pushing more GUIClips than you are popping. Make sure they are balanced.\n"
	first := runComparisonJSON(t, e, comparisonLog("Dropped", "Dropped", "Retained")+settings+info)
	if first.Baseline != nil || len(first.Gone) != 0 || first.ClustersTotal != 3 {
		t.Fatalf("first run: %+v", first)
	}
	for _, c := range first.Clusters {
		if c.Delta != "" {
			t.Fatalf("first run has delta: %+v", c)
		}
	}
	second := runComparisonJSON(t, e, comparisonLog("Retained", "Added"))
	b := second.Baseline
	if b == nil || b.RunDir != first.RunDir || b.Status != "menu" || b.New != 1 || b.Present != 1 || b.Gone != 2 {
		t.Fatalf("baseline: %+v", b)
	}
	deltas := map[string]string{}
	for _, c := range second.Clusters {
		deltas[c.Message] = c.Delta
	}
	if deltas["Exception: Retained"] != "present" || deltas["Exception: Added"] != "new" {
		t.Fatalf("deltas: %v", deltas)
	}
	if len(second.Gone) != 2 {
		t.Fatalf("gone: %+v", second.Gone)
	}
	for _, c := range second.Gone {
		if c.Mods == nil || len(c.Mods) != 0 || c.Confidence != "" || c.Delta != "" {
			t.Fatalf("gone attribution: %+v", c)
		}
		switch c.Message {
		case "Exception: Dropped":
			if c.Count != 2 {
				t.Fatalf("gone occurrences: %+v", c)
			}
		case strings.TrimSpace(settings):
			if c.Class != gametest.ClassSettings {
				t.Fatalf("gone settings: %+v", c)
			}
		default:
			t.Fatalf("unexpected gone cluster: %+v", c)
		}
	}
	e.App.TestGameStart = fakeTestGame(t, "menu", comparisonLog("Retained", "Third"))
	out, code := e.Run("test", "run", "--timeout", "5s", "--settle", "1ms")
	for _, want := range []string{
		"new exception x1: Exception: Third", "exception x1: Exception: Retained",
		fmt.Sprintf("compared with run %s (menu): 1 new, 1 present, 1 gone", filepath.Base(second.RunDir)),
		"gone: 1", "exception x1: Exception: Added",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q: %s", want, out)
		}
	}
	if code != 1 {
		t.Fatalf("errors must still fail: exit %d", code)
	}
	unchanged := runComparisonJSON(t, e, comparisonLog("Retained", "Third"))
	if unchanged.Baseline == nil || unchanged.Baseline.New != 0 || unchanged.Baseline.Present != 2 || unchanged.Baseline.Gone != 0 || unchanged.Success() {
		t.Fatalf("unchanged faulty run: %+v", unchanged)
	}
}

func TestTestRunComparisonLimits(t *testing.T) {
	t.Parallel()
	e := testRunEnv(t)
	var oldNames, newNames []string
	for i := range 35 {
		oldNames = append(oldNames, fmt.Sprintf("Old%d", i))
		newNames = append(newNames, fmt.Sprintf("New%d", i))
	}
	first := runComparisonJSON(t, e, comparisonLog(oldNames...))
	second := runComparisonJSON(t, e, comparisonLog(append(oldNames[30:], newNames...)...))
	b := second.Baseline
	if b == nil || b.RunDir != first.RunDir || b.New != 35 || b.Present != 5 || b.Gone != 30 || len(second.Clusters) != 30 || len(second.Gone) != 30 {
		t.Fatalf("counts beyond display limit: baseline %+v, listed %d/%d", b, len(second.Clusters), len(second.Gone))
	}
	third := runComparisonJSON(t, e, "")
	if third.Baseline == nil || third.Baseline.New != 0 || third.Baseline.Present != 0 || third.Baseline.Gone != 40 || len(third.Gone) != 30 || !third.Success() {
		t.Fatalf("gone display limit and clean outcome: %+v", third)
	}
}

func TestTestRunComparisonBaselineSelection(t *testing.T) {
	for _, kind := range []string{"first", "other-plan", "other-map", "blocked", "unreadable-log", "bad-report", "future", "old-report", "newest-eligible"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			e := testRunEnv(t)
			if kind == "first" {
				e.App.TestGameStart = fakeTestGame(t, "menu", comparisonLog("Retained"))
				out, code := e.Run("test", "run", "--timeout", "5s", "--settle", "1ms")
				if code != 1 || !strings.Contains(out, "compared with: no earlier run of this plan") || strings.Contains(out, "new exception") {
					t.Fatalf("first run text: exit %d: %s", code, out)
				}
				return
			}
			st, err := e.App.Loadout()
			if err != nil {
				t.Fatal(err)
			}
			runs := filepath.Join(e.Home, "tests", st.Ix.Env.ID, "runs")
			plan := app.PlanRef(st.Plan)
			oldDir := filepath.Join(runs, e.Now().Add(-time.Second).Format("20060102-150405"))
			writeRun := func(dir, plan, status string, args []string) {
				t.Helper()
				// Pre-change reports need no comparison fields or stored cluster keys.
				b, err := json.Marshal(map[string]any{"plan": plan, "status": status, "args": args})
				if err != nil {
					t.Fatal(err)
				}
				e.WriteFile(filepath.Join(dir, "report.json"), string(b))
				e.WriteFile(filepath.Join(dir, "Player.log"), comparisonLog("Retained"))
			}
			writeRun(oldDir, plan, "timeout", nil)
			switch kind {
			case "other-plan":
				writeRun(oldDir, "plan:another", "menu", nil)
			case "other-map":
				writeRun(oldDir, plan, "timeout", []string{"quicktest"})
			case "blocked":
				writeRun(oldDir, plan, "links-blocked", nil)
			case "unreadable-log":
				if err := os.Remove(filepath.Join(oldDir, "Player.log")); err != nil {
					t.Fatal(err)
				}
			case "bad-report":
				e.WriteFile(filepath.Join(oldDir, "report.json"), "invalid JSON")
			case "future":
				if err := os.Rename(oldDir, filepath.Join(runs, e.Now().Add(time.Hour).Format("20060102-150405"))); err != nil {
					t.Fatal(err)
				}
			case "newest-eligible":
				writeRun(filepath.Join(runs, e.Now().Add(-2*time.Second).Format("20060102-150405")), plan, "exited", nil)
				writeRun(filepath.Join(runs, e.Now().Format("20060102-150405")), "plan:another", "menu", nil)
			}
			r := runComparisonJSON(t, e, comparisonLog("Retained"))
			if kind == "old-report" || kind == "newest-eligible" {
				if r.Baseline == nil || r.Baseline.RunDir != oldDir || r.Baseline.Status != "timeout" || r.Baseline.Present != 1 || r.Baseline.New != 0 || r.Baseline.Gone != 0 {
					t.Fatalf("eligible baseline: %+v", r.Baseline)
				}
			} else if r.Baseline != nil {
				t.Fatalf("ineligible baseline: %+v", r.Baseline)
			}
		})
	}
}
