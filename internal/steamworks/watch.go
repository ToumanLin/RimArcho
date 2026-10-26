package steamworks

import (
	"fmt"
	"io"
	"os"
	"time"
)

// Byte movement, rather than Installed or changing flags, resets the stall deadline.
// Steam can retain Installed in its manifest after the directory has been recycled.
const DownloadStall = 2 * time.Minute
const DownloadLimit = 6 * time.Hour

type downloadAPI struct {
	request   func(uint64) bool
	callbacks func()
	stopped   func() bool
	sample    func(uint64) (Progress, string)
}

type watchClock struct {
	now   func() time.Time
	sleep func(time.Duration)
}

func installedFolder(path string, state uint32) bool {
	if state&ItemInstalled == 0 || state&(ItemDownloading|ItemDownloadPending) != 0 {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	names, _ := f.Readdirnames(1)
	return len(names) > 0
}

func watchDownloads(ids []uint64, w io.Writer, api downloadAPI, clock watchClock) {
	type pending struct {
		last  Progress
		seen  bool
		high  int64
		moved time.Time
	}
	start := clock.now()
	waiting := make(map[uint64]*pending, len(ids))
	for _, id := range ids {
		if !api.request(id) {
			p, _ := api.sample(id)
			fmt.Fprintln(w, ItemLine(id, "failed:download", p.State))
		} else {
			waiting[id] = &pending{moved: start}
		}
	}
	for len(waiting) > 0 {
		if api.stopped != nil && api.stopped() {
			return
		}
		api.callbacks()
		now := clock.now()
		for _, id := range ids {
			item := waiting[id]
			if item == nil {
				continue
			}
			p, path := api.sample(id)
			p.ID = id
			if !item.seen || p != item.last {
				fmt.Fprintln(w, ProgressLine(p))
				item.last, item.seen = p, true
			}
			if p.Got > item.high {
				item.high, item.moved = p.Got, now
			}
			result := ""
			switch {
			case installedFolder(path, p.State):
				result = "ok"
			case now.Sub(start) >= DownloadLimit:
				result = "failed:download-limit"
			case now.Sub(item.moved) >= DownloadStall:
				result = "failed:download-stalled"
			}
			if result != "" {
				fmt.Fprintln(w, ItemLine(id, result, p.State))
				delete(waiting, id)
			}
		}
		if len(waiting) > 0 {
			clock.sleep(500 * time.Millisecond)
		}
	}
	fmt.Fprintln(w, DoneLine(Watch, len(ids)))
}
