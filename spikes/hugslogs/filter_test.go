package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJudge(t *testing.T) {
	seen := map[string]bool{}
	cases := []struct {
		name, raw, want, mm string
	}{
		{"1.6 with a problem", fakeLog("a", "1.6", true, 30), verdictKept, "1.6"},
		{"1.6 without a problem", fakeLog("b", "1.6", false, 30), dropNoProblems, "1.6"},
		{"1.5", fakeLog("c", "1.5", true, 30), dropVersion, "1.5"},
		{"cut off", fakeLog("d", "1.6", true, 3), dropTooShort, "1.6"},
		{"no mod list", "Loaded mods:\n\nLog file contents:\nRimWorld 1.6.4630 rev1\nx\n", "no-mod-list", ""},
		{"not a HugsLib log", "hello world", "no-log-section", ""},
	}
	for _, c := range cases {
		v, mm, h := judge("id", "2026-01-01T00:00:00Z", []byte(c.raw), seen)
		if v != c.want || mm != c.mm || h == "" {
			t.Errorf("%s: judge = %q %q %q, want %q %q", c.name, v, mm, h, c.want, c.mm)
		}
		if v == verdictKept {
			seen[h] = true
		}
	}
	if v, _, _ := judge("id2", "", []byte(cases[0].raw), seen); v != dropDuplicate {
		t.Errorf("same bytes again: %q, want %q", v, dropDuplicate)
	}
	for _, cls := range []string{"other", "mod-report", ""} {
		if isProblem(cls) {
			t.Errorf("isProblem(%q) = true", cls)
		}
	}
	for _, cls := range []string{"exception", "xref-missing", "patch-failed", "texture-missing"} {
		if !isProblem(cls) {
			t.Errorf("isProblem(%q) = false", cls)
		}
	}
}

// even: whole days per month until the quota; dropped logs are recorded in the index, not stored;
// a rerun skips judged gists; -max stops a run early.
func TestEven(t *testing.T) {
	// 3 gists a day over Jan and Feb 2026; gists whose number is divisible by 3 have no problem.
	type g struct{ id, created string }
	var gs []g
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 59 * 3 {
		gs = append(gs, g{fmt.Sprintf("%032x", i), start.Add(time.Duration(i) * 8 * time.Hour).Format(time.RFC3339)})
	}
	mux := http.NewServeMux()
	hits := 0
	mux.HandleFunc("/"+account+"/", func(w http.ResponseWriter, r *http.Request) {
		hits++
		id := filepath.Base(filepath.Dir(r.URL.Path))
		var n int
		fmt.Sscanf(id, "%x", &n)
		if n == 7 {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, fakeLog(id, "1.6", n%3 != 0, 30))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	rawBase, rawDelay = srv.URL, 0
	dir := t.TempDir()
	var idx strings.Builder
	for _, x := range gs {
		fmt.Fprintf(&idx, `{"id":%q,"file":"output_log.txt","created":%q,"listed":"x"}`+"\n", x.id, x.created)
	}
	os.WriteFile(filepath.Join(dir, "index.jsonl"), []byte(idx.String()), 0o644)

	// Quota 4 per month: day 15 gives 2 kept (3 gists, one without problems), day 1 two more.
	if err := even(dir, "", "", 4, 2, 0); err != nil {
		t.Fatal(err)
	}
	raws, _ := os.ReadDir(filepath.Join(dir, "raw"))
	if len(raws) != 8 {
		t.Fatalf("%d raw logs, want 8 (4 per month)", len(raws))
	}
	ix, _ := loadIndex(dir)
	counts := map[string]int{}
	for _, id := range ix.order {
		counts[ix.m[id].Verdict]++
	}
	if counts[verdictKept] != 8 || counts[dropNoProblems] != 4 || counts[dropGone] != 0 || len(ix.order) != len(gs) {
		t.Fatalf("verdicts %v, index %d lines", counts, len(ix.order))
	}
	// A rerun with the same quota downloads nothing.
	hits = 0
	if err := even(dir, "", "", 4, 2, 0); err != nil || hits != 0 {
		t.Fatalf("rerun: %d downloads, %v", hits, err)
	}
	// Raising the quota with -max 3 stops after 3 more kept logs.
	if err := even(dir, "", "", 30, 1, 3); err != nil {
		t.Fatal(err)
	}
	if raws, _ = os.ReadDir(filepath.Join(dir, "raw")); len(raws) != 11 {
		t.Fatalf("%d raw logs after -max 3, want 11", len(raws))
	}
	// Then the whole window: gist 7 is gone, a third has no problems.
	if err := even(dir, "", "", 1000, 2, 0); err != nil {
		t.Fatal(err)
	}
	ix, _ = loadIndex(dir)
	counts = map[string]int{}
	for _, id := range ix.order {
		counts[ix.m[id].Verdict]++
	}
	if counts[dropGone] != 1 || counts[dropNoProblems] != 59 || counts[verdictKept] != 59*3-60 {
		t.Fatalf("whole window: %v", counts)
	}
}
