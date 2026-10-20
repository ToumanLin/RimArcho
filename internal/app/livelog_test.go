package app_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func appendFile(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func clusterSig(key string, count int, attr []*app.Attribution) string {
	var as []string
	for _, a := range attr {
		as = append(as, a.Instance+":"+a.Role)
	}
	sort.Strings(as)
	return fmt.Sprintf("%s x%d [%s]", key, count, strings.Join(as, " "))
}

// L1, L2: a log written in two parts, the split inside a stack, gives the same clusters,
// counts and attribution as the full analysis.
func TestLiveLogMatchesFullAnalysis(t *testing.T) {
	e := testkit.New(t)
	text := e.HarmonyLogFixture()
	path := filepath.Join(e.Root, "live", "Player.log")
	os.MkdirAll(filepath.Dir(path), 0o755)
	cut := strings.Index(text, "    - POSTFIX tester.modb")
	if cut < 0 {
		t.Fatal("fixture changed")
	}
	lv := e.App.FollowLog(path, "")
	appendFile(t, path, text[:cut])
	if _, err := lv.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	appendFile(t, path, text[cut:])
	if _, err := lv.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	if _, err := lv.Close(e.App); err != nil {
		t.Fatal(err)
	}
	full, err := e.App.AnalyzeLogText(text)
	if err != nil {
		t.Fatal(err)
	}
	var want, got []string
	for _, lc := range full.Clusters {
		want = append(want, clusterSig(lc.Key, lc.Count, lc.Attr))
	}
	for _, c := range lv.Clusters() {
		if c.Report == nil {
			t.Fatalf("cluster %s not attributed", c.Key)
		}
		got = append(got, clusterSig(c.Key, c.Count, c.Report.Attr))
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("live:\n%s\nfull:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// L3, L4: a restarted log starts over; a flood of one error is one cluster and the
// follower keeps no text.
func TestLiveLogResetAndFlood(t *testing.T) {
	e := setup(t, false)
	scanNow(t, e)
	path := filepath.Join(e.Root, "Player.log")
	lv := e.App.FollowLog(path, "")
	appendFile(t, path, "Exception in Old: System.Exception: old\n  at Foo.Bar.Baz () [0x0] in <0>:0\n\n")
	lv.Poll(e.App)
	os.WriteFile(path, []byte("RimWorld 1.6.4633 rev1261\n"), 0o644)
	if _, err := lv.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	if lv.Resets != 1 || len(lv.Clusters()) != 0 {
		t.Fatalf("after restart: resets %d clusters %d", lv.Resets, len(lv.Clusters()))
	}
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	const n = 50000
	var sb strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "Exception ticking Pawn%d: System.NullReferenceException: Object reference not set to an instance of an object\n  at Foo.Bar.Tick () [0x0] in <0>:0\n\n", i)
		if sb.Len() > 4<<20 {
			appendFile(t, path, sb.String())
			sb.Reset()
			if _, err := lv.Poll(e.App); err != nil {
				t.Fatal(err)
			}
		}
	}
	appendFile(t, path, sb.String())
	for lv.Lines < 3*n {
		if _, err := lv.Poll(e.App); err != nil {
			t.Fatal(err)
		}
	}
	lv.Close(e.App)
	cs := lv.Clusters()
	if len(cs) != 1 || cs[0].Count != n {
		t.Fatalf("flood: %d clusters, first count %d", len(cs), cs[0].Count)
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if grow := int64(after.HeapInuse) - int64(before.HeapInuse); grow > 20<<20 {
		t.Fatalf("heap grew %d MB following %d lines", grow>>20, lv.Lines)
	}
}

// L5: the RimCrow companion log supplies timestamps and levels; bad lines are counted.
func TestLiveLogCompanion(t *testing.T) {
	e := setup(t, false)
	scanNow(t, e)
	path, comp := filepath.Join(e.Root, "Player.log"), filepath.Join(e.Root, "RimCrow_Realtime.log")
	os.WriteFile(comp, []byte(`{"timestamp":"2026-09-29 19:35:21.500","level":"ERROR","message":"Exception in Thing: System.Exception: boom","context":{"relatedModIds":[]}}
not json
`), 0o644)
	lv := e.App.FollowLog(path, comp)
	appendFile(t, path, "Exception in Thing: System.Exception: boom\n  at Foo.Bar.Baz () [0x0] in <0>:0\n\n")
	if _, err := lv.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	cs := lv.Clusters()
	if len(cs) != 1 || cs[0].Level != "ERROR" || cs[0].At.Format("15:04:05") != "19:35:21" || lv.BadLines != 1 {
		t.Fatalf("clusters %d level %q at %v bad %d", len(cs), cs[0].Level, cs[0].At, lv.BadLines)
	}
}

// archo log --follow --once prints the clusters with their first attributed mod.
func TestLogFollowOnce(t *testing.T) {
	e := testkit.New(t)
	text := e.HarmonyLogFixture()
	path := filepath.Join(e.Root, "follow", "Player.log")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(text), 0o644)
	out := e.MustRun("log", "--follow", "--once", "--file", path)
	if !strings.Contains(out, "x500") || !strings.Contains(out, "→ ") {
		t.Fatalf("follow --once:\n%s", out)
	}
	if out := e.MustRun("log", "--follow", "--once", "--json", "--file", path); !strings.Contains(out, `"count":500`) {
		t.Fatalf("--json:\n%s", out)
	}
}
