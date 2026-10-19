package app_test

// Replays the real Player.log patch failures against the simulation (M3 acceptance, as the
// M0 xpathcompat spike did). Read-only on the game and mod folders; nothing is persisted.
// Runs only when RIMARCHO_REAL_HOME is set; RIMARCHO_LOG_VERSION selects the major.minor
// the log was produced with (default: the environment's).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rimarcho/rimarcho/internal/analyze"
	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/env"
)

func TestRealPlayerLogReplay(t *testing.T) {
	home := os.Getenv("RIMARCHO_REAL_HOME")
	if home == "" {
		t.Skip("set RIMARCHO_REAL_HOME to replay a real Player.log")
	}
	a, err := app.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.Launcher = func(string, []string, string) error { t.Fatal("launch attempted"); return nil }
	st, err := a.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	in, _, err := a.AnalyzeInput(st, os.Getenv("RIMARCHO_LOG_VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	logPath := env.PlayerLog(st.Ix.Env.UserDataRoot, in.MM)
	lf, err := analyze.ParseLogFailures(logPath)
	if err != nil {
		t.Skip("no Player.log for", in.MM, err)
	}
	li, _ := os.Stat(logPath)
	out, err := analyze.Run(in)
	if err != nil {
		t.Fatal(err)
	}
	dir := map[string]string{}
	for _, m := range in.Mods {
		dir[m.Instance] = m.Dir
	}
	logCnt := map[string]int{}
	for _, l := range lf {
		logCnt[l.Key()]++
	}
	agree, simOnly, stale := 0, 0, 0
	for _, p := range out.Patches {
		if p.Result != analyze.ResultFailed {
			continue
		}
		path := filepath.Join(dir[p.Instance], filepath.FromSlash(p.File))
		if k := analyze.FailKey(path, p.Desc); logCnt[k] > 0 {
			logCnt[k]--
			agree++
			continue
		}
		simOnly++
		changed := false
		if fi, err := os.Stat(path); err == nil && fi.ModTime().After(li.ModTime()) {
			changed = true
		}
		for _, r := range p.Reasons {
			// A target renamed by an update after the log (M0: sbz Neat Storage).
			if r == analyze.ReasonStaleName {
				changed = true
			}
		}
		if changed {
			stale++
		}
		t.Logf("simulated only: %s %s %v", path, p.Desc, p.Reasons)
	}
	logOnly := 0
	for _, l := range lf {
		if logCnt[l.Key()] > 0 {
			logCnt[l.Key()]--
			logOnly++
			t.Logf("log only: [%s] %s %s", l.Mod, l.Desc, l.File)
		}
	}
	rate := float64(simOnly+logOnly) / float64(max(1, out.Stats.TopOps))
	t.Logf("log %d failures, simulated %d agreeing, %d sim-only (%d explained by later updates), %d log-only; mismatch %.3f%% of %d ops; %d ms",
		len(lf), agree, simOnly, stale, logOnly, 100*rate, out.Stats.TopOps, out.Stats.TotalMs)
	if rate >= 0.005 {
		t.Fatalf("mismatch rate %.3f%% ≥ 0.5%%", 100*rate)
	}
	if logOnly > 0 {
		t.Fatalf("%d failures in the log were not simulated", logOnly)
	}
}
