package app_test

import (
	"sync"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// A scan reports every folder it reads, then saving; the window shows it (16 §2.15).
func TestScanProgress(t *testing.T) {
	e := setup(t, false)
	var mu sync.Mutex
	var got []app.ScanProgress
	r, err := e.App.ScanWith(app.ScanOptions{Progress: func(p app.ScanProgress) {
		mu.Lock()
		got = append(got, p)
		mu.Unlock()
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 3 {
		t.Fatalf("progress = %+v", got)
	}
	total := got[0].Total
	if got[0].Done != 0 || total == 0 || total > r.Stats.Live {
		t.Fatalf("first = %+v, live %d", got[0], r.Stats.Live)
	}
	last := got[len(got)-1]
	if !last.Saving || last.Total != total {
		t.Fatalf("last = %+v", last)
	}
	reads := got[1 : len(got)-1]
	if len(reads) != total {
		t.Fatalf("%d folder reports for %d folders", len(reads), total)
	}
	seen := map[int]bool{}
	for _, p := range reads {
		if p.Saving || p.Total != total || p.Done < 1 || p.Done > total || seen[p.Done] {
			t.Fatalf("read report %+v", p)
		}
		seen[p.Done] = true
	}
}

func TestScanProgressLine(t *testing.T) {
	for _, p := range []app.ScanProgress{{Done: 0, Total: 1200}, {Done: 37, Total: 1200}, {Saving: true, Done: 1200, Total: 1200}} {
		q, ok := app.ParseScanProgress(app.ScanProgressLine(p) + "\r\n")
		if !ok || q != p {
			t.Fatalf("%+v -> %q -> %+v %v", p, app.ScanProgressLine(p), q, ok)
		}
	}
	for _, l := range []string{"progress: defs: 3/9 files", "scan:4  1.6  2s", "progress: scan mods x/2"} {
		if _, ok := app.ParseScanProgress(l); ok {
			t.Fatalf("%q read as scan progress", l)
		}
	}
}
