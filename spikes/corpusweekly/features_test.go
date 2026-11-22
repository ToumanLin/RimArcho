package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Dropped logs go to the dropped list and count as seen; prune deletes only committed raw logs
// that are done with (features kept, dropped, or uploaded before the window).
func TestFeaturesAndPrune(t *testing.T) {
	dir := t.TempDir()
	corpusDir, features := filepath.Join(dir, "hugslogs"), filepath.Join(dir, "features")
	os.MkdirAll(filepath.Join(corpusDir, "raw"), 0o755)
	old := time.Now().UTC().AddDate(0, 0, -7*12) // 12 weeks ago: older than -keep 8, in a 26-week window
	ancient := time.Now().UTC().AddDate(0, 0, -7*40)
	recs := []map[string]any{
		{"id": "ok16", "status": "ok", "mm": "1.6", "hash": "h1"},
		{"id": "ok17", "status": "ok", "mm": "1.7", "hash": "h2"},
		{"id": "drop", "status": "dropped", "drop": "empty-log", "mm": "1.6", "hash": "h3"},
		{"id": "v15", "status": "dropped", "drop": "version-out-of-scope", "mm": "1.5", "hash": "h4"},
	}
	var parsed, idx []string
	for _, r := range recs {
		r["created"] = old.Format(time.RFC3339)
		j, _ := json.Marshal(r)
		parsed = append(parsed, string(j))
	}
	for _, id := range []string{"ok16", "ok17", "drop", "v15", "pending", "uncommitted"} {
		idx = append(idx, fmt.Sprintf(`{"id":%q,"created":%q}`, id, old.Format(time.RFC3339)))
	}
	idx = append(idx, fmt.Sprintf(`{"id":"ancient","created":%q}`, ancient.Format(time.RFC3339)))
	for _, id := range []string{"ok16", "ok17", "drop", "v15", "pending", "uncommitted", "ancient"} {
		os.WriteFile(filepath.Join(corpusDir, "raw", id+".txt"), []byte("x"), 0o644)
	}
	os.WriteFile(filepath.Join(corpusDir, "index.jsonl"), []byte(strings.Join(idx, "\n")+"\n"), 0o644)
	pp := filepath.Join(dir, "parsed.jsonl")
	os.WriteFile(pp, []byte(strings.Join(parsed, "\n")+"\n"), 0o644)
	silence(t)

	week := thisWeek()
	since := sinceOf(t, week, 26)
	if err := keepFeatures(pp, "", features, since); err != nil {
		t.Fatal(err)
	}
	seen := seenFiles(features)
	if len(seen) != 3 || !strings.HasSuffix(seen[2], "dropped.jsonl.gz") {
		t.Fatalf("seen files %v, want the 1.6 and 1.7 parsed files and the dropped list", seen)
	}
	tracked := filepath.Join(dir, "tracked.txt")
	os.WriteFile(tracked, []byte("hugslogs/raw/ok16.txt\nhugslogs/raw/ok17.txt\nhugslogs/raw/drop.txt\nhugslogs/raw/v15.txt\nhugslogs/raw/pending.txt\nhugslogs/raw/ancient.txt\n"), 0o644)
	if err := prune(corpusDir, features, tracked, since, 8, false); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]bool{"ok16": false, "ok17": false, "drop": false, "v15": false, "ancient": false, "pending": true, "uncommitted": true} {
		_, err := os.Stat(filepath.Join(corpusDir, "raw", id+".txt"))
		if (err == nil) != want {
			t.Errorf("%s: kept %v, want %v", id, err == nil, want)
		}
	}
}

func thisWeek() string {
	y, w := time.Now().UTC().ISOWeek()
	return fmt.Sprintf("%04d-W%02d", y, w)
}

func sinceOf(t *testing.T, week string, weeks int) string {
	s, err := weekStart(week)
	if err != nil {
		t.Fatal(err)
	}
	y, w := s.AddDate(0, 0, -7*(weeks-1)).ISOWeek()
	return fmt.Sprintf("%04d-W%02d", y, w)
}

func silence(t *testing.T) {
	old := os.Stdout
	null, _ := os.Open(os.DevNull)
	os.Stdout = null
	t.Cleanup(func() { os.Stdout = old; null.Close() })
}
