package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/ToumanLin/RimArcho/spikes/internal/jsonl"
)

// An incremental parse (two weekly runs, the second with -seen) gives the same records as one
// full parse of the same logs, duplicates included (05 §4.6).
func TestStatsIncremental(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "raw"), 0o755)
	var idx []byte
	add := func(id, created, body string) {
		os.WriteFile(rawPath(dir, id), []byte(body), 0o644)
		j, _ := json.Marshal(indexEntry{ID: id, Created: created})
		idx = append(append(idx, j...), '\n')
	}
	for i := range 6 {
		add(fmt.Sprintf("a%d", i), fmt.Sprintf("2026-11-0%dT10:00:00Z", i+1), fakeLog(fmt.Sprint(i), "1.6", i%2 == 0, 5))
	}
	add("dupe", "2026-11-12T10:00:00Z", fakeLog("1", "1.6", false, 5)) // same content as a1
	add("b1", "2026-11-13T10:00:00Z", fakeLog("b1", "1.7", true, 5))
	add("old", "2026-10-01T10:00:00Z", fakeLog("old", "1.6", false, 5))
	os.WriteFile(filepath.Join(dir, "index.jsonl"), idx, 0o644)
	silence(t)

	full := filepath.Join(dir, "full.jsonl.gz")
	if err := stats(dir, full, "2026-11-01", ""); err != nil {
		t.Fatal(err)
	}
	// The first week, then the rest with the first week's records as seen.
	week1 := filepath.Join(dir, "w1.jsonl.gz")
	if err := stats(dir, week1, "2026-11-01", ""); err != nil {
		t.Fatal(err)
	}
	keepBefore(t, week1, "2026-11-07")
	week2 := filepath.Join(dir, "w2.jsonl.gz")
	if err := stats(dir, week2, "2026-11-01", week1); err != nil {
		t.Fatal(err)
	}
	got, want := lines(t, week1+","+week2), lines(t, full)
	if len(got) != len(want) {
		t.Fatalf("incremental %d records, full %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("record %d differs:\n inc  %s\n full %s", i, got[i], want[i])
		}
	}
	for _, l := range want {
		var r Record
		json.Unmarshal([]byte(l), &r)
		if r.ID == "old" {
			t.Error("a log created before -since was parsed")
		}
		if r.ID == "dupe" && r.Drop != "duplicate-content" {
			t.Errorf("dupe: %s/%s, want duplicate-content", r.Status, r.Drop)
		}
	}
}

func silence(t *testing.T) {
	old := os.Stdout
	null, _ := os.Open(os.DevNull)
	os.Stdout = null
	t.Cleanup(func() { os.Stdout = old; null.Close() })
}

// keepBefore rewrites a records file to the logs created before day (the first weekly run).
func keepBefore(t *testing.T, path, day string) {
	var keep []byte
	jsonl.Each(path, func(b []byte) error {
		var r Record
		if json.Unmarshal(b, &r) == nil && r.Created < day {
			keep = append(keep, b...)
		}
		return nil
	})
	w, err := createRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(keep)
	w.Close()
}

func lines(t *testing.T, list string) []string {
	var out []string
	if err := jsonl.Each(list, func(b []byte) error {
		out = append(out, string(b))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}
