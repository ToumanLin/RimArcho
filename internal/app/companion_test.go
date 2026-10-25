package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// monitorLine is one ArchoMonitor.log line (03 §6.7.3), JSON-encoded so paths stay valid.
func monitorLine(t *testing.T, kind, level, msg, details string, count, total int, id string, ctx map[string]any) string {
	t.Helper()
	m := map[string]any{"timestamp": "2026-10-21 14:05:03.120", "level": level, "message": msg, "details": details, "count": count,
		"context": ctx, "source": "archo-monitor", "schema": 1}
	if kind != "" {
		m["kind"] = kind
	} else {
		m["id"], m["total"] = id, total
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b) + "\n"
}

func sessionLine(t *testing.T, id string) string {
	return monitorLine(t, "session", "INFO", "[Archo Monitor] session "+id, "", 1, 0, "", map[string]any{"session": id, "version": "0.9.0"})
}

func marker(id, path string) string {
	return "[Archo Monitor] 0.9.0 session " + id + ": " + path + "\n"
}

const unityHead = "Mono path[0] = 'x'\nRimWorld 1.6.4633 rev1261\n"

// L9, L6, L12: which companion log belongs to a Player.log.
func TestCompanionFor(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	root := filepath.Join(e.Root, "userdata")
	am, amPrev := filepath.Join(root, "ArchoMonitor.log"), filepath.Join(root, "ArchoMonitor-prev.log")
	e.WriteFile(am, sessionLine(t, "bbbbbbbbbbbb"))
	e.WriteFile(amPrev, sessionLine(t, "aaaaaaaaaaaa"))
	cur, prev := filepath.Join(root, "Player.log"), filepath.Join(root, "Player-prev.log")
	e.WriteFile(cur, unityHead+marker("bbbbbbbbbbbb", am))
	// The previous run's marker names ArchoMonitor.log: it was renamed since.
	e.WriteFile(prev, unityHead+marker("aaaaaaaaaaaa", am))
	if got := app.CompanionFor(cur); got != am {
		t.Fatalf("current: %q", got)
	}
	if got := app.CompanionFor(prev); got != amPrev {
		t.Fatalf("previous: %q", got)
	}
	// 1.7 writes Player.log under Logs/; the companion is in the data root above.
	logs17 := filepath.Join(root, "Logs", "Player.log")
	e.WriteFile(logs17, "=== Waylith log ===\n2026-10-21 14:05:01.000 INFO [Mods] "+marker("bbbbbbbbbbbb", ""))
	if got := app.CompanionFor(logs17); got != am {
		t.Fatalf("1.7: %q", got)
	}
	// A pack on 1.6: Player.log stays in the default folder, the marker names the pack's file.
	pack := filepath.Join(e.Root, "packs", "p", "userdata", "ArchoMonitor.log")
	e.WriteFile(pack, sessionLine(t, "cccccccccccc"))
	packLog := filepath.Join(e.Root, "default", "Player.log")
	e.WriteFile(packLog, unityHead+marker("cccccccccccc", pack))
	if got := app.CompanionFor(packLog); got != pack {
		t.Fatalf("pack: %q", got)
	}

	// No marker: RimCrow's log of the same generation; the old RMM name too (L6).
	other := filepath.Join(e.Root, "other")
	crow := filepath.Join(other, "RMM_Realtime.log")
	e.WriteFile(crow, `{"timestamp":"2026-10-21 14:05:03.120","level":"ERROR","message":"x","count":1,"context":{}}`+"\n")
	pl := filepath.Join(other, "Player.log")
	e.WriteFile(pl, unityHead)
	if got := app.CompanionFor(pl); got != crow {
		t.Fatalf("RMM_Realtime.log: %q", got)
	}
	// Written well after the Player.log: a later run's.
	old := time.Now().Add(-time.Hour)
	os.Chtimes(pl, old, old)
	if got := app.CompanionFor(pl); got != "" {
		t.Fatalf("a later run's companion: %q", got)
	}
}

// L8, L10, L13: the full analysis takes files, counts and times from ArchoMonitor.log; an
// error only the monitor saw becomes a cluster of its own.
func TestLogAnalysisWithMonitor(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	ix, _ := e.App.Index()
	var aDir string
	for _, m := range ix.Mods {
		if m.Name == "A" {
			aDir = ix.ModDir(m)
		}
	}
	root := filepath.Join(e.Root, "userdata")
	am := filepath.Join(root, "ArchoMonitor.log")
	file := filepath.Join(aDir, "Defs", "ThingDefs", "a.xml")
	e.WriteFile(am, sessionLine(t, "aaaaaaaaaaaa")+
		monitorLine(t, "", "ERROR", "Config error in a_Thing: has no label", "  at Verse.Log.Error ()", 1, 1, "11111111",
			map[string]any{"relatedModIds": []string{"tester.a"}, "relatedFiles": []string{file}, "inferredType": "DefConfigError", "roles": map[string]string{"tester.a": "xml"}, "def": "a_Thing"})+
		monitorLine(t, "", "ERROR", "Config error in a_Thing: has no label", "", 8, 9, "11111111", map[string]any{})+
		monitorLine(t, "", "ERROR", "Exception ticking Pawn7: System.NullReferenceException: Object reference not set to an instance of an object", "  at Foo.Bar.Tick ()", 1234, 1234, "22222222", map[string]any{}))
	pl := filepath.Join(root, "Player.log")
	e.WriteFile(pl, unityHead+marker("aaaaaaaaaaaa", am)+"Config error in a_Thing: has no label\n(Filename: C:/x.cpp Line: 0)\n\n")
	rep, err := e.App.AnalyzeLog(pl)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Companion != "ArchoMonitor.log" {
		t.Fatalf("companion %q", rep.Companion)
	}
	var cfg, tick *app.LogCluster
	for _, lc := range rep.Clusters {
		switch {
		case strings.HasPrefix(lc.First.Message, "Config error"):
			cfg = lc
		case strings.HasPrefix(lc.First.Message, "Exception ticking"):
			tick = lc
		}
	}
	if cfg == nil || tick == nil {
		t.Fatalf("clusters %d", len(rep.Clusters))
	}
	if cfg.Count != 9 || cfg.Level != "ERROR" || cfg.At.IsZero() || cfg.Confidence != finding.Certain {
		t.Fatalf("config error: count %d level %q at %v confidence %s", cfg.Count, cfg.Level, cfg.At, cfg.Confidence)
	}
	if len(cfg.Attr) == 0 || cfg.Attr[0].Role != app.RoleSource || !strings.Contains(strings.Join(cfg.Attr[0].Evidence, ";"), "log file (ArchoMonitor.log): Defs/ThingDefs/a.xml (while loading a_Thing)") {
		t.Fatalf("attribution %+v", cfg.Attr)
	}
	if !tick.OnlyCompanion || tick.Count != 1234 || tick.First.Top() == nil {
		t.Fatalf("monitor-only: only %v count %d", tick.OnlyCompanion, tick.Count)
	}
	// The problem carries the file for the chip in 能不能跑 (06 §2.4).
	if out := e.MustRun("log", "--file", pl, "--cluster", "E1"); !strings.Contains(out, "ArchoMonitor.log") {
		t.Fatalf("archo log:\n%s", out)
	}
}

// L5, L12: live tracking follows RimCrow's log until Player.log names this run's
// ArchoMonitor.log; an error only the monitor saw shows up after a short wait.
func TestLiveLogMonitor(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	scanNow(t, e)
	root := filepath.Join(e.Root, "packs", "p", "userdata")
	e.WriteFile(filepath.Join(root, "RimCrow_Realtime.log"), "")
	path := filepath.Join(e.Root, "default", "Player.log")
	e.MkdirAll(filepath.Dir(path))
	lv := e.App.FollowRun(path, root)
	if !strings.HasSuffix(lv.Companion, "RimCrow_Realtime.log") {
		t.Fatalf("before the marker: %q", lv.Companion)
	}
	am := filepath.Join(root, "ArchoMonitor.log")
	e.WriteFile(am, sessionLine(t, "aaaaaaaaaaaa"))
	appendFile(t, path, unityHead+marker("aaaaaaaaaaaa", am))
	if _, err := lv.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	if lv.Companion != am {
		t.Fatalf("after the marker: %q", lv.Companion)
	}
	appendFile(t, am, monitorLine(t, "", "ERROR", "Exception in Thing: System.Exception: boom", "", 1, 1, "33333333", map[string]any{})+
		monitorLine(t, "", "ERROR", "Exception ticking Pawn7: System.NullReferenceException: x", "  at Foo.Bar.Tick ()", 50, 50, "44444444", map[string]any{}))
	appendFile(t, path, "Exception in Thing: System.Exception: boom\n  at Foo.Bar.Baz () [0x0] in <0>:0\n\n")
	if _, err := lv.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	cs := lv.Clusters()
	if len(cs) != 1 || cs[0].Level != "ERROR" || cs[0].At.Format("15:04:05") != "14:05:03" {
		t.Fatalf("clusters %d", len(cs))
	}
	e.Advance(6 * time.Second)
	if _, err := lv.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	cs = lv.Clusters()
	if len(cs) != 2 || !cs[1].OnlyCompanion || cs[1].Count != 50 || cs[1].Report == nil {
		t.Fatalf("monitor-only cluster: %d clusters", len(cs))
	}
}
