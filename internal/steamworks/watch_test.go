package steamworks

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWatchRequiresFolderAndIdleInstalledState(t *testing.T) {
	t.Parallel()
	folder := filepath.Join(t.TempDir(), "item")
	step, requests, pumps := 0, 0, 0
	now := time.Unix(0, 0)
	var out bytes.Buffer
	watchDownloads([]uint64{1319782555}, &out, downloadAPI{
		request:   func(id uint64) bool { requests++; return id == 1319782555 },
		callbacks: func() { pumps++ },
		sample: func(id uint64) (Progress, string) {
			state := uint32(ItemSubscribed | ItemInstalled)
			if step == 2 {
				state |= ItemDownloadPending
			}
			if step == 3 {
				state |= ItemDownloading
			}
			return Progress{ID: id, Got: int64(step) * 25, Total: 100, State: state}, folder
		},
	}, watchClock{now: func() time.Time { return now }, sleep: func(d time.Duration) {
		if strings.Contains(out.String(), "ITEM ") {
			t.Fatal("stale Installed, empty folder or active download completed early")
		}
		step++
		now = now.Add(d)
		if step == 1 {
			if err := os.Mkdir(folder, 0755); err != nil {
				t.Fatal(err)
			}
		}
		if step == 2 {
			if err := os.WriteFile(filepath.Join(folder, "mod"), []byte("mod"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}})
	r := Parse(out.String())
	if step != 4 || requests != 1 || pumps != 5 || !r.Done || len(r.Items) != 1 || r.Items[0].Result != "ok" {
		t.Fatalf("steps=%d requests=%d pumps=%d: %s", step, requests, pumps, out.String())
	}
}

func TestWatchStallIsPerItemAndIgnoresFlagChanges(t *testing.T) {
	t.Parallel()
	now := time.Unix(0, 0)
	step := 0
	var out bytes.Buffer
	watchDownloads([]uint64{1, 2}, &out, downloadAPI{
		request: func(id uint64) bool { return id != 2 }, callbacks: func() {},
		sample: func(id uint64) (Progress, string) {
			return Progress{ID: id, State: uint32(ItemInstalled | (step%2)*ItemDownloadPending)}, ""
		},
	}, watchClock{now: func() time.Time { return now }, sleep: func(d time.Duration) { now = now.Add(d); step++ }})
	r := Parse(out.String())
	if !r.Done || len(r.Items) != 2 || r.Items[0].Result != "failed:download" || r.Items[1].Result != "failed:download-stalled" || now.Sub(time.Unix(0, 0)) != DownloadStall {
		t.Fatalf("%s", out.String())
	}
}

func TestWatchOnlyEmitsChangesAndHasOverallLimit(t *testing.T) {
	t.Parallel()
	for _, moves := range []bool{false, true} {
		now := time.Unix(0, 0)
		var out bytes.Buffer
		var got int64
		watchDownloads([]uint64{1}, &out, downloadAPI{
			request: func(uint64) bool { return true }, callbacks: func() {},
			sample: func(uint64) (Progress, string) { return Progress{ID: 1, Got: got, State: ItemDownloadPending}, "" },
		}, watchClock{now: func() time.Time { return now }, sleep: func(time.Duration) {
			now = now.Add(time.Minute)
			if moves {
				got++
			}
		}})
		if !moves && strings.Count(out.String(), "PROGRESS ") != 1 {
			t.Fatalf("unchanged samples repeated: %s", out.String())
		}
		if moves && !strings.Contains(out.String(), "failed:download-limit") {
			t.Fatal("missing overall cap")
		}
	}
}

func TestProgressProtocol(t *testing.T) {
	p := Progress{ID: 1319782555, Got: 45, Total: 100, State: 37}
	if got, ok := ParseProgress(ProgressLine(p)); !ok || got != p {
		t.Fatalf("%+v %v", got, ok)
	}
	for _, bad := range []string{"PROGRESS 1 -1 10 5", "PROGRESS x 1 10 5", "PROGRESS 1 1 10 junk", "PROGRESS 1 1 10", "other output"} {
		if _, ok := ParseProgress(bad); ok {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestWatchStopsWithoutReportingPendingItemsDone(t *testing.T) {
	var out bytes.Buffer
	stopped := false
	watchDownloads([]uint64{1}, &out, downloadAPI{
		request: func(uint64) bool { return true }, callbacks: func() {}, stopped: func() bool { return stopped },
		sample: func(uint64) (Progress, string) { return Progress{ID: 1, State: ItemInstalled}, "" },
	}, watchClock{now: time.Now, sleep: func(time.Duration) { stopped = true }})
	if strings.Contains(out.String(), "ITEM ") || strings.Contains(out.String(), "DONE ") {
		t.Fatalf("pending download finished on stop: %s", out.String())
	}
}
