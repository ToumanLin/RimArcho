package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func logProblems(l []*app.Problem) int {
	n := 0
	for _, p := range l {
		if strings.HasPrefix(p.Code, "log.") {
			n++
		}
	}
	return n
}

// 03 §6.3: a log written before the Loadout's last change, or by another game version, is
// stale: its problems are kept apart (ProblemSet.Stale), not counted, and do not escalate.
func TestStaleLog(t *testing.T) {
	e := testkit.New(t)
	log := e.HarmonyLogFixture()
	ps := problems(t, e)
	if len(ps.Logs) != 1 || ps.Logs[0].Stale() || len(ps.Stale) != 0 || logProblems(ps.List) != 4 {
		t.Fatalf("current log: logs %+v, stale %d, listed %d", ps.Logs, len(ps.Stale), logProblems(ps.List))
	}
	for _, p := range ps.List {
		if strings.HasPrefix(p.Code, "log.") && p.Log != ps.Logs[0] {
			t.Fatalf("%s is not tied to its log", p.Ref)
		}
	}

	e.Advance(time.Hour)
	e.MustRun("disable", "pkg:tester.manual")
	ps = problems(t, e)
	r := ps.Logs[0]
	if !r.Before || r.OtherGame || !r.Stale() || len(ps.Stale) != 4 || logProblems(ps.List) != 0 {
		t.Fatalf("log before the change: %+v, stale %d, listed %d", r, len(ps.Stale), logProblems(ps.List))
	}
	for _, p := range ps.List {
		if p.Code == "harmony.skip-prefix" && p.Severity != "info" {
			t.Fatal("a stale log escalated skip-prefix")
		}
	}
	if _, code := e.Run("problems", "show", ps.Stale[0].Ref); code != 0 {
		t.Fatal("problems show cannot find a stale-log problem")
	}

	// Written after the change, but by another game version.
	p := filepath.Join(e.UserData, "Player.log")
	os.WriteFile(p, []byte(strings.Replace(log, "RimWorld 1.6.4633 rev1261", "RimWorld 1.5.4409 rev1118", 1)), 0o644)
	os.Chtimes(p, e.Now().Add(time.Hour), e.Now().Add(time.Hour))
	ps = problems(t, e)
	r = ps.Logs[0]
	if r.Before || !r.OtherGame || r.MM != "1.5" || r.EnvMM != "1.6" || len(ps.Stale) == 0 || logProblems(ps.List) != 0 {
		t.Fatalf("1.5 log: %+v, stale %d, listed %d", r, len(ps.Stale), logProblems(ps.List))
	}
}

// The attributed log is stored in the database: a new process reads it without parsing,
// and a cache-only problem list (status, mod) never parses the log.
func TestLogCache(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	p := filepath.Join(e.UserData, "Player.log")

	e.App.LogCacheOnly = true
	ps := problems(t, e)
	if len(ps.Logs) != 1 || !ps.Logs[0].Pending || logProblems(ps.List) != 0 {
		t.Fatalf("cache-only before any analysis: %+v, listed %d", ps.Logs, logProblems(ps.List))
	}
	if out := e.MustRun("status"); !strings.Contains(out, "not analysed yet") {
		t.Fatalf("status parsed the log:\n%s", out)
	}
	e.App.LogCacheOnly = false
	if n := logProblems(problems(t, e).List); n != 4 {
		t.Fatalf("log problems %d", n)
	}

	e.Open() // a new process
	rep, err := e.App.CachedLog(p)
	if err != nil {
		t.Fatalf("stored report: %v", err)
	}
	if len(rep.Clusters) == 0 || rep.Clusters[0].First == nil || rep.Clusters[0].Finding == nil || len(rep.Clusters[0].Attr) == 0 {
		t.Fatalf("stored report incomplete: %+v", rep.Clusters[0])
	}
	if out := e.MustRun("status"); strings.Contains(out, "not analysed yet") {
		t.Fatalf("status ignores the stored report:\n%s", out)
	}

	// A changed attribution input (the enabled list) invalidates the stored report.
	e.MustRun("disable", "pkg:tester.manual")
	e.Open()
	if _, err := e.App.CachedLog(p); !errors.Is(err, app.ErrLogNotCached) {
		t.Fatalf("after a Loadout change: %v", err)
	}
	if _, err := e.App.AnalyzeLog(p); err != nil {
		t.Fatal(err)
	}
	// So does a changed log.
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("one more line\n")
	f.Close()
	e.Open()
	if _, err := e.App.CachedLog(p); !errors.Is(err, app.ErrLogNotCached) {
		t.Fatalf("after the log grew: %v", err)
	}
}
