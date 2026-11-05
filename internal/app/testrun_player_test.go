package app_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// writePlayerLog puts the player's own Player.log where `archo log` reads it by default.
func writePlayerLog(e *testkit.Env, text string) string {
	e.RecordLaunch(e.Now().Add(-30 * time.Minute))
	p := filepath.Join(e.UserData, "Player.log")
	e.WriteFile(p, "RimWorld 1.6.4633 rev1261\n"+text)
	os.Chtimes(p, e.Now(), e.Now())
	return p
}

func testRunWith(t *testing.T, e *testkit.Env, mode, text string) *app.TestRunReport {
	t.Helper()
	e.App.TestGameStart = fakeTestGame(t, mode, text)
	r, err := e.App.TestRun(context.Background(), app.TestRunOptions{Timeout: 5 * time.Second, Settle: time.Millisecond})
	if err != nil && r == nil {
		t.Fatal(err)
	}
	return r
}

// The first run of a plan has no previous run, but it is compared with the player's log:
// the fault that is gone is listed as fixed with its attribution, and the noise that
// stays is marked as already there (09 §7).
func TestTestRunComparesWithPlayerLog(t *testing.T) {
	t.Parallel()
	e := codeModEnv(t)
	path := writePlayerLog(e, staticCtorLog+comparisonLog("Noise"))
	r := testRunWith(t, e, "menu", comparisonLog("Noise", "Added"))
	if r.Baseline != nil || len(r.Gone) != 0 {
		t.Fatalf("first run has a baseline: %+v", r.Baseline)
	}
	p := r.PlayerLog
	if p == nil || p.Path != path || p.Present != 1 || p.Absent != 1 || p.Fixed != 1 || p.Unreached != 0 || p.Error != "" {
		t.Fatalf("player log: %+v", p)
	}
	before := map[string]string{}
	for _, c := range r.Clusters {
		before[c.Message] = c.Before
	}
	if before["Exception: Noise"] != "present" || before["Exception: Added"] != "absent" {
		t.Fatalf("before: %v", before)
	}
	if len(r.Fixed) != 1 || !strings.Contains(r.Fixed[0].Message, "static constructor of ModA.Boot") ||
		!slices.Equal(r.Fixed[0].Mods, []string{"Mod A"}) || r.Fixed[0].Confidence != finding.Likely {
		t.Fatalf("fixed: %+v", r.Fixed)
	}
	if r.Success() {
		t.Fatal("the comparison must not change success")
	}
	saved, err := os.ReadFile(filepath.Join(r.RunDir, "report.json"))
	if err != nil || !strings.Contains(string(saved), `"player_log"`) || !strings.Contains(string(saved), `"before": "present"`) {
		t.Fatalf("saved report: %v\n%s", err, saved)
	}

	// The CLI text says the same; a player's log the run has nothing left of is all fixed.
	e.App.TestGameStart = fakeTestGame(t, "menu", comparisonLog("Noise"))
	out, code := e.Run("test", "run", "--timeout", "5s", "--settle", "1ms")
	for _, want := range []string{
		"before: in the player's log",
		"compared with the player's log " + path + ": 1 present, 0 absent, 1 fixed",
		"fixed: 1", "mods: Mod A (likely)",
		"gone: 1", "exception x1: Exception: Added",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if code != 1 {
		t.Fatalf("remaining noise must still fail: exit %d", code)
	}
}

// gone keeps the attribution the earlier run's report listed (09 §7).
func TestTestRunGoneKeepsAttribution(t *testing.T) {
	t.Parallel()
	e := codeModEnv(t)
	first := testRunWith(t, e, "menu", staticCtorLog+comparisonLog("Noise"))
	if first.PlayerLog != nil || len(first.Fixed) != 0 {
		t.Fatalf("no player's log, yet compared: %+v", first.PlayerLog)
	}
	second := testRunWith(t, e, "menu", comparisonLog("Noise"))
	if second.Baseline == nil || second.Baseline.Gone != 1 || len(second.Gone) != 1 {
		t.Fatalf("baseline %+v, gone %+v", second.Baseline, second.Gone)
	}
	if g := second.Gone[0]; !slices.Equal(g.Mods, []string{"Mod A"}) || g.Confidence != finding.Likely {
		t.Fatalf("gone attribution: %+v", g)
	}
	out, _ := e.Run("test", "run", "--timeout", "5s", "--settle", "1ms", "--json")
	var third app.TestRunReport
	if err := json.Unmarshal([]byte(out), &third); err != nil || third.Baseline == nil || third.Baseline.Gone != 0 || strings.Contains(out, `"player_log"`) {
		t.Fatalf("third run: %v: %s", err, out)
	}
	if out, _ := e.Run("test", "run", "--timeout", "5s", "--settle", "1ms"); !strings.Contains(out, "compared with the player's log: none found") {
		t.Fatalf("missing note without a player's log:\n%s", out)
	}
}

// A run that stops before its target shows nothing about the player's errors it did not
// reach: none of them is reported fixed.
func TestTestRunUnreachedPlayerClusters(t *testing.T) {
	t.Parallel()
	e := testRunEnv(t)
	writePlayerLog(e, comparisonLog("Early", "Later"))
	r := testRunWith(t, e, "exit", comparisonLog("Early"))
	if r.Status != "exited" {
		t.Fatalf("status %q", r.Status)
	}
	if p := r.PlayerLog; p == nil || p.Present != 1 || p.Fixed != 0 || p.Unreached != 1 || len(r.Fixed) != 0 {
		t.Fatalf("player log %+v, fixed %+v", p, r.Fixed)
	}
}
