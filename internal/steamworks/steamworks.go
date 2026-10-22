// Package steamworks subscribes to and unsubscribes from Workshop items, and reads their
// state, through the Steam client (06 §2.9, 08 §10). The calls run in a short-lived worker
// process (`archo steam-worker <action> <ids>`) started in a runtime folder that holds
// steam_api64.dll (copied from the game install) and steam_appid.txt: Steam then sees
// RimWorld only for the second the call takes, and a crash in the DLL never reaches the
// window. The worker uses the flat C API through syscall, so there is no cgo.
//
// The worker writes one line per fact on stdout, which Parse reads:
//
//	ERROR <message>                  Steam could not be reached (not running, no DLL…)
//	ITEM <id> <result> <state>       subscribe / unsubscribe: ok, failed:<EResult>, timeout
//	STATE <id> <state>               state: the item's EItemState flags
//	DONE <action> <n>
package steamworks

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// AppID is RimWorld's Steam app id.
const AppID = 294100

// Actions.
const (
	Subscribe   = "subscribe"
	Unsubscribe = "unsubscribe"
	State       = "state"
)

// EItemState flags (ISteamUGC::GetItemState).
const (
	ItemSubscribed      = 1
	ItemLegacy          = 2
	ItemInstalled       = 4
	ItemNeedsUpdate     = 8
	ItemDownloading     = 16
	ItemDownloadPending = 32
)

// Item is one Workshop item's answer.
type Item struct {
	ID     uint64
	Result string // ok, failed:<EResult>, timeout ("" for state)
	State  uint32
}

// Report is what the worker said.
type Report struct {
	Error string // Steam could not be reached
	Items []Item
	Done  bool
}

// Parse reads the worker's output; other lines (the DLL's own messages) are skipped.
func Parse(out string) *Report {
	r := &Report{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 {
			continue
		}
		switch f[0] {
		case "ERROR":
			r.Error = strings.TrimSpace(strings.TrimPrefix(sc.Text(), "ERROR"))
		case "ITEM":
			if len(f) == 4 {
				id, _ := strconv.ParseUint(f[1], 10, 64)
				st, _ := strconv.ParseUint(f[3], 10, 32)
				r.Items = append(r.Items, Item{ID: id, Result: f[2], State: uint32(st)})
			}
		case "STATE":
			if len(f) == 3 {
				id, _ := strconv.ParseUint(f[1], 10, 64)
				st, _ := strconv.ParseUint(f[2], 10, 32)
				r.Items = append(r.Items, Item{ID: id, State: uint32(st)})
			}
		case "DONE":
			r.Done = true
		}
	}
	return r
}

// Line formats are shared by the worker and the tests' fake.

func ErrorLine(msg string) string { return "ERROR " + msg }
func ItemLine(id uint64, result string, state uint32) string {
	return fmt.Sprintf("ITEM %d %s %d", id, result, state)
}
func StateLine(id uint64, state uint32) string { return fmt.Sprintf("STATE %d %d", id, state) }
func DoneLine(action string, n int) string     { return fmt.Sprintf("DONE %s %d", action, n) }

// StateText says an item's state in words.
func StateText(s uint32) string {
	var parts []string
	for _, x := range []struct {
		bit  uint32
		word string
	}{{ItemSubscribed, "subscribed"}, {ItemInstalled, "installed"}, {ItemNeedsUpdate, "needs update"}, {ItemDownloading, "downloading"}, {ItemDownloadPending, "download pending"}, {ItemLegacy, "legacy"}} {
		if s&x.bit != 0 {
			parts = append(parts, x.word)
		}
	}
	if len(parts) == 0 {
		return "not subscribed"
	}
	return strings.Join(parts, ", ")
}
