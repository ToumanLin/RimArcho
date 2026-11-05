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
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// monitorLine is one ArchoMonitor.log line (06 §7.3), JSON-encoded so paths stay valid.
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

// writeLog writes a file, creating its folder; the companion tests need no environment.
func writeLog(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// L9, L6, L12: which companion log belongs to a Player.log.
func TestCompanionFor(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root := filepath.Join(dir, "userdata")
	am, amPrev := filepath.Join(root, "ArchoMonitor.log"), filepath.Join(root, "ArchoMonitor-prev.log")
	writeLog(t, am, sessionLine(t, "bbbbbbbbbbbb"))
	writeLog(t, amPrev, sessionLine(t, "aaaaaaaaaaaa"))
	cur, prev := filepath.Join(root, "Player.log"), filepath.Join(root, "Player-prev.log")
	writeLog(t, cur, unityHead+marker("bbbbbbbbbbbb", am))
	// The previous run's marker names ArchoMonitor.log: it was renamed since.
	writeLog(t, prev, unityHead+marker("aaaaaaaaaaaa", am))
	if got := app.CompanionFor(cur); got != am {
		t.Fatalf("current: %q", got)
	}
	if got := app.CompanionFor(prev); got != amPrev {
		t.Fatalf("previous: %q", got)
	}
	// 1.7 writes Player.log under Logs/; the companion is in the data root above.
	logs17 := filepath.Join(root, "Logs", "Player.log")
	writeLog(t, logs17, "=== Waylith log ===\n2026-10-21 14:05:01.000 INFO [Mods] "+marker("bbbbbbbbbbbb", ""))
	if got := app.CompanionFor(logs17); got != am {
		t.Fatalf("1.7: %q", got)
	}
	// A pack on 1.6: Player.log stays in the default folder, the marker names the pack's file.
	pack := filepath.Join(dir, "packs", "p", "userdata", "ArchoMonitor.log")
	writeLog(t, pack, sessionLine(t, "cccccccccccc"))
	packLog := filepath.Join(dir, "default", "Player.log")
	writeLog(t, packLog, unityHead+marker("cccccccccccc", pack))
	if got := app.CompanionFor(packLog); got != pack {
		t.Fatalf("pack: %q", got)
	}

	// No marker: RimCrow's log of the same generation; the old RMM name too (L6).
	other := filepath.Join(dir, "other")
	crow := filepath.Join(other, "RMM_Realtime.log")
	writeLog(t, crow, `{"timestamp":"2026-10-21 14:05:03.120","level":"ERROR","message":"x","count":1,"context":{}}`+"\n")
	pl := filepath.Join(other, "Player.log")
	writeLog(t, pl, unityHead)
	at := time.Now().Add(time.Second)
	for _, p := range []string{pl, crow} {
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
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
	// The problem carries the file for the chip in Can it run (16 §2.4).
	if out := e.MustRun("log", "--file", pl, "--cluster", "E1"); !strings.Contains(out, "ArchoMonitor.log") {
		t.Fatalf("archo log:\n%s", out)
	}
}

// L5, L12: live tracking rejects an empty RimCrow log, then follows this run's
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
	if lv.Companion != "" {
		t.Fatalf("before Player.log exists: %q", lv.Companion)
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
	// The Monitor pairing message can also be an informational cluster when a parent
	// directory contains "errors". This test counts actual errors, independent of TMP.
	problemClusters := func() []*app.LiveCluster {
		var out []*app.LiveCluster
		for _, c := range lv.Clusters() {
			if !logs.Informational(c.Class) {
				out = append(out, c)
			}
		}
		return out
	}
	cs := problemClusters()
	if len(cs) != 1 || cs[0].Level != "ERROR" || cs[0].At.Format("15:04:05") != "14:05:03" {
		t.Fatalf("clusters %d", len(cs))
	}
	e.Advance(6 * time.Second)
	if _, err := lv.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	cs = problemClusters()
	if len(cs) != 2 || !cs[1].OnlyCompanion || cs[1].Count != 50 || cs[1].Report == nil {
		t.Fatalf("monitor-only cluster: %d clusters", len(cs))
	}
}

// The monitor writes an error before Player.log finishes it: one cluster, not one per log.
// Player.log's entry can stay open past the wait (its next line comes late), or arrive after
// the monitor's entry has become a cluster of its own (VM run 2026-10-28 listed it twice).
func TestLiveLogMonitorFirst(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	scanNow(t, e)
	root := filepath.Join(e.Root, "userdata")
	path := filepath.Join(root, "Player.log")
	am := filepath.Join(root, "ArchoMonitor.log")
	e.WriteFile(am, sessionLine(t, "aaaaaaaaaaaa"))
	e.WriteFile(path, unityHead+marker("aaaaaaaaaaaa", am))
	lv := e.App.FollowRun(path, root)
	poll := func() []*app.LiveCluster {
		t.Helper()
		if _, err := lv.Poll(e.App); err != nil {
			t.Fatal(err)
		}
		var out []*app.LiveCluster
		for _, c := range lv.Clusters() {
			if !logs.Informational(c.Class) {
				out = append(out, c)
			}
		}
		return out
	}
	poll()
	xref := "Could not resolve cross-reference to Verse.SoundDef named Reload_Standard (wanter=soundInteract)"
	appendFile(t, am, monitorLine(t, "", "ERROR", xref, "  at Verse.Log.Error ()\n  at Verse.DirectXmlCrossRefLoader.ResolveAllWantedCrossReferences ()", 1, 1, "55555555", map[string]any{}))
	appendFile(t, path, xref+"\n")
	if cs := poll(); len(cs) != 0 {
		t.Fatalf("the entry is still open: %d clusters", len(cs))
	}
	e.Advance(6 * time.Second)
	if cs := poll(); len(cs) != 0 {
		t.Fatalf("Player.log is still writing it, past the wait: %d clusters", len(cs))
	}
	appendFile(t, path, "Some later message\n")
	cs := poll()
	if len(cs) != 1 || cs[0].OnlyCompanion || cs[0].Count != 1 || cs[0].Report == nil {
		t.Fatalf("open entry: %d clusters", len(cs))
	}
	// Player.log shows it only after the monitor's entry became a cluster of its own.
	tick := "Exception ticking Pawn7: System.NullReferenceException: Object reference not set to an instance of an object"
	appendFile(t, am, monitorLine(t, "", "ERROR", tick, "", 3, 3, "66666666", map[string]any{}))
	poll()
	e.Advance(6 * time.Second)
	cs = poll()
	if len(cs) != 2 || !cs[1].OnlyCompanion || cs[1].Count != 3 {
		t.Fatalf("monitor-only: %d clusters", len(cs))
	}
	appendFile(t, path, tick+"\n  at Foo.Bar.Tick () [0x0] in <0>:0\n\n")
	cs = poll()
	if len(cs) != 2 || cs[1].OnlyCompanion || cs[1].Count != 3 || cs[1].Report == nil || cs[1].Report.First.Top() == nil {
		t.Fatalf("late Player.log entry: %d clusters", len(cs))
	}
	// later occurrences in Player.log join it too
	appendFile(t, path, strings.Repeat(tick+"\n  at Foo.Bar.Tick () [0x0] in <0>:0\n\n", 4))
	cs = poll()
	if len(cs) != 2 || cs[1].Count != 5 {
		t.Fatalf("more occurrences: %d clusters, count %d", len(cs), cs[len(cs)-1].Count)
	}
	if _, err := lv.Close(e.App); err != nil {
		t.Fatal(err)
	}
	if cs := poll(); len(cs) != 2 {
		t.Fatalf("after exit: %d clusters", len(cs))
	}
}

// A line Player.log has as noise is in Player.log: the monitor's entry for it (with a stack)
// is not an error of its own, as in the full analysis (VM run 2026-10-28 listed "Translation
// data … 9 errors" under ArchoMonitor.log while the game ran). Since the translation count is
// known noise (06 §1) both copies are one informational noise cluster.
func TestLiveLogMonitorNoise(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	scanNow(t, e)
	root := filepath.Join(e.Root, "userdata")
	path := filepath.Join(root, "Player.log")
	am := filepath.Join(root, "ArchoMonitor.log")
	e.WriteFile(am, sessionLine(t, "aaaaaaaaaaaa"))
	e.WriteFile(path, unityHead+marker("aaaaaaaaaaaa", am))
	lv := e.App.FollowRun(path, root)
	if _, err := lv.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	noise := "Translation data for language English has 9 errors. Generate translation report for more info."
	appendFile(t, am, monitorLine(t, "", "ERROR", noise, "  at Verse.Log.Error ()\n  at Verse.LanguageDatabase.InitAllMetadata ()", 1, 1, "77777777", map[string]any{}))
	appendFile(t, path, noise+"\nUnloading 5 Unused Serialized files (Serialized files now loaded: 5)\n")
	for _, d := range []time.Duration{0, 6 * time.Second} {
		e.Advance(d)
		if _, err := lv.Poll(e.App); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := lv.Close(e.App); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range lv.Clusters() {
		if strings.Contains(c.Template, "Translation data") || c.Report != nil && strings.HasPrefix(c.Report.First.Message, "Translation data") {
			n++
			if c.OnlyCompanion || c.Class != logs.ClassNoise || !logs.Informational(c.Class) {
				t.Fatalf("noise listed as an error: only %v class %s", c.OnlyCompanion, c.Class)
			}
		}
	}
	if n > 1 {
		t.Fatalf("noise split into %d clusters", n)
	}
}

// Files from another run must never become the companion merely because their names fit.
func TestCompanionRunWindow(t *testing.T) {
	t.Parallel()
	const crowLine = `{"timestamp":"2026-10-21 14:01:00.000","message":"ready","level":"INFO"}` + "\n"
	start := time.Date(2026, 10, 21, 14, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		name, crowName, crow, monitor, playerMarker, want string
		crowAt                                            time.Time
	}{
		{name: "stale empty", crowAt: start.Add(-72 * time.Hour)},
		{name: "fresh empty", crowAt: start.Add(time.Minute)},
		{name: "stale nonempty", crow: crowLine, crowAt: start.Add(-72 * time.Hour)},
		{name: "before start", crow: crowLine, crowAt: start.Add(-time.Second)},
		{name: "after run", crow: crowLine, crowAt: start.Add(3 * time.Minute)},
		{name: "same run", crow: crowLine, crowAt: start.Add(time.Minute), want: "RimCrow_Realtime.log"},
		{name: "legacy same run", crowName: "RMM_Realtime.log", crow: crowLine, crowAt: start.Add(time.Minute), want: "RMM_Realtime.log"},
		{name: "monitor priority", crow: crowLine, crowAt: start.Add(time.Minute), monitor: "aaaaaaaaaaaa", playerMarker: "aaaaaaaaaaaa", want: "ArchoMonitor.log"},
		{name: "monitor before marker", crow: crowLine, crowAt: start.Add(time.Minute), monitor: "aaaaaaaaaaaa", want: "ArchoMonitor.log"},
		{name: "mismatched monitor", crow: crowLine, crowAt: start.Add(time.Minute), monitor: "bbbbbbbbbbbb", playerMarker: "aaaaaaaaaaaa", want: "RimCrow_Realtime.log"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			player := filepath.Join(root, "Logs", "Player.log")
			if err := os.MkdirAll(filepath.Dir(player), 0o755); err != nil {
				t.Fatal(err)
			}
			write := func(path, content string, at time.Time) {
				t.Helper()
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(path, at, at); err != nil {
					t.Fatal(err)
				}
			}
			monitor := filepath.Join(root, "ArchoMonitor.log")
			head := "=== Waylith log ===\nStarted:   " + start.Format("2006-01-02 15:04:05.000 -07:00") + "\n"
			if tc.playerMarker != "" {
				head += marker(tc.playerMarker, monitor)
			}
			write(player, head, start.Add(time.Minute))
			name := tc.crowName
			if name == "" {
				name = "RimCrow_Realtime.log"
			}
			write(filepath.Join(root, name), tc.crow, tc.crowAt)
			if tc.monitor != "" {
				write(monitor, sessionLine(t, tc.monitor), start.Add(time.Minute))
			}
			want := ""
			if tc.want != "" {
				want = filepath.Join(root, tc.want)
			}
			if got := app.CompanionFor(player); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
			lv := (&app.App{}).FollowRun(player, root)
			if lv.Companion != want {
				t.Fatalf("live got %q, want %q", lv.Companion, want)
			}
		})
	}
}

func TestCompanionUniqueSession(t *testing.T) {
	t.Parallel()
	for _, currentMarked := range []bool{false, true} {
		t.Run(map[bool]string{false: "previous marker wins", true: "duplicate marker prefers current"}[currentMarked], func(t *testing.T) {
			root := t.TempDir()
			cur, prev := filepath.Join(root, "Player.log"), filepath.Join(root, "Player-prev.log")
			monitor := filepath.Join(root, "ArchoMonitor.log")
			writeLog(t, monitor, sessionLine(t, "aaaaaaaaaaaa"))
			head := unityHead
			if currentMarked {
				head += marker("aaaaaaaaaaaa", monitor)
			}
			writeLog(t, cur, head)
			alias := filepath.Join(root, "monitor-alias.log")
			if err := os.Link(monitor, alias); err != nil {
				t.Fatal(err)
			}
			writeLog(t, prev, unityHead+marker("aaaaaaaaaaaa", alias))
			at := time.Now().Add(time.Second)
			for _, p := range []string{cur, monitor} {
				if err := os.Chtimes(p, at, at); err != nil {
					t.Fatal(err)
				}
			}
			gotCur, gotPrev := app.CompanionFor(cur), app.CompanionFor(prev)
			if currentMarked {
				if gotCur != monitor || gotPrev != "" {
					t.Fatalf("current %q previous %q", gotCur, gotPrev)
				}
			} else if gotCur != "" || gotPrev != alias {
				t.Fatalf("current %q previous %q", gotCur, gotPrev)
			}
		})
	}
}

func TestFollowRunDiscoversMonitor(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	player := filepath.Join(e.Root, "Player.log")
	monitor := filepath.Join(e.Root, "ArchoMonitor.log")
	e.WriteFile(player, unityHead)
	at := time.Now().Add(time.Second)
	if err := os.Chtimes(player, at, at); err != nil {
		t.Fatal(err)
	}
	lv := e.App.FollowRun(player, e.Root)
	if lv.Companion != "" {
		t.Fatalf("before companion: %q", lv.Companion)
	}
	crow := filepath.Join(e.Root, "RimCrow_Realtime.log")
	e.WriteFile(crow, `{"message":"ready","level":"INFO"}`+"\n")
	if err := os.Chtimes(crow, at, at); err != nil {
		t.Fatal(err)
	}
	if _, err := lv.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	if lv.Companion != crow {
		t.Fatalf("RimCrow appeared: %q", lv.Companion)
	}
	e.WriteFile(monitor, sessionLine(t, "aaaaaaaaaaaa"))
	if err := os.Chtimes(monitor, at, at); err != nil {
		t.Fatal(err)
	}
	if _, err := lv.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	if lv.Companion != monitor || lv.CompanionLog().Session != "aaaaaaaaaaaa" {
		t.Fatalf("monitor appeared: %q", lv.Companion)
	}
}

// An unknown start is conservative, and the previous run bounds reused creation times.
func TestCompanionConservativeStart(t *testing.T) {
	t.Parallel()
	for _, hasPrev := range []bool{false, true} {
		t.Run(map[bool]string{false: "no start evidence", true: "previous run boundary"}[hasPrev], func(t *testing.T) {
			root := t.TempDir()
			end := time.Date(2026, 10, 21, 14, 5, 0, 0, time.Local)
			player, crow := filepath.Join(root, "Player.log"), filepath.Join(root, "RimCrow_Realtime.log")
			head := unityHead
			if hasPrev {
				head += "Started:   " + end.Add(-time.Hour).Format("2006-01-02 15:04:05.000 -07:00") + "\n"
			}
			writeLog(t, player, head)
			writeLog(t, crow, `{"timestamp":"2026-10-21 14:05:00.000","message":"ready"}`+"\n")
			if err := os.Chtimes(player, end, end); err != nil {
				t.Fatal(err)
			}
			if hasPrev {
				prev := filepath.Join(root, "Player-prev.log")
				writeLog(t, prev, unityHead)
				at := end.Add(-time.Second)
				if err := os.Chtimes(prev, at, at); err != nil {
					t.Fatal(err)
				}
			}
			for _, at := range []time.Time{end.Add(-time.Minute), end} {
				if err := os.Chtimes(crow, at, at); err != nil {
					t.Fatal(err)
				}
				want := ""
				if at.Equal(end) {
					want = crow
				}
				if got := app.CompanionFor(player); got != want {
					t.Fatalf("mtime %v: got %q, want %q", at, got, want)
				}
			}
		})
	}
}
