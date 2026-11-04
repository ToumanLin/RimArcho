package app_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

var cursorLine = regexp.MustCompile(`(?m)^  cursor: (\S+)$`)

func cursorOf(t *testing.T, out string) string {
	t.Helper()
	m := cursorLine.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no cursor in:\n%s", out)
	}
	return m[1]
}

func appendTo(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(s)
	f.Close()
}

// pawn2 is the fixture's E4 entry (testkit.HarmonyLogFixture).
const pawn2 = "Exception ticking Pawn2: System.ArgumentOutOfRangeException: Index was out of range.\n" +
	"[Ref 4D5E6F]\n" +
	"  at ModA.CompBroken.CompTick () [0x00000] in <00000000000000000000000000000000>:0\n" +
	"  at Verse.ThingWithComps.Tick () [0x00000] in <00000000000000000000000000000000>:0\n" +
	"  at (wrapper dynamic-method) Verse.Pawn.Verse.Pawn.Tick_Patch1(Verse.Pawn)\n" +
	"    - PREFIX tester.moda.harmony: Boolean ModA.Pawn_Tick:Prefix(Verse.Pawn __instance)\n" +
	"  at Verse.TickList.Tick () [0x00000] in <00000000000000000000000000000000>:0\n\n"

// L7: archo log (MCP analyze_log) returns a cursor; with since=<cursor> only the clusters
// that are new or grew after it; a restarted game makes the cursor stale and the old run is
// then Player-prev.log.
func TestLogSince(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.MustRun("analyze")
	pl := filepath.Join(e.UserData, "Player.log")
	c := cursorOf(t, e.MustRun("log"))

	out := e.MustRun("log", "--since", c)
	if !strings.Contains(out, ": 0 new or grown since the cursor, 4 clusters in all") || !strings.Contains(out, "(nothing new)") || cursorOf(t, out) != c {
		t.Fatalf("nothing changed:\n%s", out)
	}

	// the game writes on: one more of E4 and a new exception
	appendTo(t, pl, pawn2+
		"Exception ticking Pawn3: System.InvalidOperationException: Collection was modified.\n"+
		"  at ModB.Thing.Tick () [0x00000] in <00000000000000000000000000000000>:0\n\n")
	out = e.MustRun("log", "--since", c)
	if !strings.Contains(out, ": 2 new or grown since the cursor, 5 clusters in all") ||
		!strings.Contains(out, "+1  log:E4 harmony-stack x2") || !strings.Contains(out, "new  log:E5 exception x1  Exception ticking Pawn3") ||
		strings.Contains(out, "log:E3") {
		t.Fatalf("after appending:\n%s", out)
	}
	c2 := cursorOf(t, out)
	if c2 == c {
		t.Fatal("the cursor did not move")
	}
	if out := e.MustRun("log", "--since", c2); !strings.Contains(out, "(nothing new)") {
		t.Fatalf("since the new cursor:\n%s", out)
	}

	// the game restarts: Player.log becomes Player-prev.log, a new Player.log starts
	prev := filepath.Join(e.UserData, "Player-prev.log")
	if err := os.Rename(pl, prev); err != nil {
		t.Fatal(err)
	}
	e.WriteFile(pl, "Mono path[0] = 'x'\nRimWorld 1.6.4633 rev1261\n"+pawn2)
	out = e.MustRun("log", "--since", c)
	if !strings.Contains(out, "the cursor is stale, 1 clusters") || !strings.Contains(out, "the run the cursor is from is now Player-prev.log") ||
		!strings.Contains(out, "new  log:E1") || !strings.Contains(out, "archo log --prev --since "+c) {
		t.Fatalf("after a restart:\n%s", out)
	}
	if out := e.MustRun("log", "--prev", "--since", c); !strings.Contains(out, ": 2 new or grown since the cursor") {
		t.Fatalf("the old run, since the cursor:\n%s", out)
	}
	// a second restart replaces Player-prev.log too
	os.Rename(pl, prev)
	e.WriteFile(pl, "Mono path[0] = 'x'\nRimWorld 1.6.4633 rev1261\n")
	if out := e.MustRun("log", "--prev", "--since", c); !strings.Contains(out, "Player-prev.log was replaced by a newer run") {
		t.Fatalf("after two restarts:\n%s", out)
	}

	for _, bad := range [][]string{{"log", "--since", "yesterday"}, {"log", "--since", c, "--cluster", "E1"}, {"log", "--since", "log1.zz.123"}} {
		if _, code := e.Run(bad...); code == 0 {
			t.Fatalf("archo %v: no error", bad)
		}
	}
}

// L7 with Archo Monitor: the cursor covers ArchoMonitor.log too, so an entry only the
// monitor saw (the game stopped writing Player.log) and a count that grew there show up.
func TestLogSinceMonitor(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	root := filepath.Join(e.Root, "userdata")
	am := filepath.Join(root, "ArchoMonitor.log")
	tick := "Exception ticking Pawn7: System.NullReferenceException: Object reference not set to an instance of an object"
	e.WriteFile(am, sessionLine(t, "aaaaaaaaaaaa")+
		monitorLine(t, "", "ERROR", "Config error in a_Thing: has no label", "  at Verse.Log.Error ()", 1, 1, "11111111", map[string]any{})+
		monitorLine(t, "", "ERROR", tick, "  at Foo.Bar.Tick ()", 1234, 1234, "22222222", map[string]any{}))
	pl := filepath.Join(root, "Player.log")
	e.WriteFile(pl, unityHead+marker("aaaaaaaaaaaa", am)+"Config error in a_Thing: has no label\n(Filename: C:/x.cpp Line: 0)\n\n")
	rep, err := e.App.AnalyzeLog(pl)
	if err != nil {
		t.Fatal(err)
	}
	cur, err := app.ParseLogCursor(rep.Cursor)
	if err != nil || cur.CompSize == 0 {
		t.Fatalf("cursor %q: %v", rep.Cursor, err)
	}
	s, err := e.App.Since(rep, cur)
	if err != nil || s.Stale != "" || len(s.New) != 0 {
		t.Fatalf("unchanged: %+v %v", s, err)
	}
	appendTo(t, am, monitorLine(t, "", "ERROR", tick, "", 6, 1240, "22222222", map[string]any{})+
		monitorLine(t, "", "ERROR", "Exception drawing Pawn7: System.Exception: late", "  at Foo.Bar.Draw ()", 3, 3, "33333333", map[string]any{}))
	rep2, err := e.App.AnalyzeLog(pl)
	if err != nil {
		t.Fatal(err)
	}
	s, err = e.App.Since(rep2, cur)
	if err != nil || s.Stale != "" || len(s.New) != 2 {
		t.Fatalf("after the monitor wrote on: %+v %v", s, err)
	}
	for _, lc := range s.New {
		switch {
		case strings.HasPrefix(lc.First.Message, "Exception ticking"):
			if lc.Count != 1240 || s.Then[lc.Key] != 1234 {
				t.Fatalf("tick: now %d, then %d", lc.Count, s.Then[lc.Key])
			}
		case strings.HasPrefix(lc.First.Message, "Exception drawing"):
			if !lc.OnlyCompanion || s.Then[lc.Key] != 0 {
				t.Fatalf("draw: only %v then %d", lc.OnlyCompanion, s.Then[lc.Key])
			}
		default:
			t.Fatalf("unexpected %s", lc.First.Message)
		}
	}
}
