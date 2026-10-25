//go:build windows && amd64

package steamworks

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Call-result ids (k_iSteamRemoteStorageCallbacks + n) of SubscribeItem / UnsubscribeItem.
const (
	cbSubscribed   = 1313 // RemoteStorageSubscribePublishedFileResult_t
	cbUnsubscribed = 1315 // RemoteStorageUnsubscribePublishedFileResult_t
	resultOK       = 1    // k_EResultOK
)

// flat is the part of the flat C API the worker uses.
type flat struct {
	dll                                                    *syscall.DLL
	runCallbacks, shutdown                                 *syscall.Proc
	subscribe, unsubscribe, itemState, completed, callRslt *syscall.Proc
	ugc, utils                                             uintptr
}

// Worker runs one action in this process (the `archo steam-worker` subcommand) and returns
// the exit code. The current folder must hold steam_api64.dll and steam_appid.txt.
func Worker(action string, ids []uint64, w io.Writer) int {
	say := func(s string) { fmt.Fprintln(w, s) }
	if action != Subscribe && action != Unsubscribe && action != State && action != Download {
		say(ErrorLine("unknown action " + action))
		return 2
	}
	wd, _ := os.Getwd()
	f, err := load(filepath.Join(wd, "steam_api64.dll"))
	if err != nil {
		say(ErrorLine(err.Error()))
		return 3
	}
	defer f.shutdown.Call()
	if action == Download {
		p, err := f.dll.FindProc("SteamAPI_ISteamUGC_DownloadItem")
		if err != nil {
			say(ErrorLine(err.Error()))
			return 3
		}
		for _, id := range ids {
			ok, _, _ := p.Call(f.ugc, uintptr(id), 1)
			result := "ok"
			if ok&0xff == 0 {
				result = "failed:download"
			}
			say(ItemLine(id, result, f.state(id)))
		}
		say(DoneLine(action, len(ids)))
		return 0
	}
	if action == State {
		for _, id := range ids {
			say(StateLine(id, f.state(id)))
		}
		say(DoneLine(action, len(ids)))
		return 0
	}
	call, expect := f.subscribe, uintptr(cbSubscribed)
	if action == Unsubscribe {
		call, expect = f.unsubscribe, cbUnsubscribed
	}
	handles := make([]uint64, len(ids))
	for k, id := range ids {
		h, _, _ := call.Call(f.ugc, uintptr(id))
		handles[k] = uint64(h)
	}
	// Wait for every answer: at least 2 s, half a second an item, at most 15 s (as RimCrow).
	wait := min(max(2*time.Second, time.Duration(len(ids))*500*time.Millisecond), 15*time.Second)
	results := make([]string, len(ids))
	left := len(ids)
	for end := time.Now().Add(wait); left > 0 && time.Now().Before(end); {
		f.runCallbacks.Call()
		for k, h := range handles {
			if results[k] != "" || h == 0 {
				continue
			}
			if r, ok := f.result(h, expect); ok {
				results[k] = r
				left--
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Steam finishes its side of the call before the process goes (RimCrow waits the same).
	time.Sleep(500 * time.Millisecond)
	f.runCallbacks.Call()
	for k, id := range ids {
		st := f.state(id)
		r := results[k]
		switch {
		case handles[k] == 0:
			r = "failed:no-call"
		case r == "":
			// no answer in time: the item's state says whether it happened
			subscribed := st&ItemSubscribed != 0
			if subscribed == (action == Subscribe) {
				r = "ok"
			} else {
				r = "timeout"
			}
		}
		say(ItemLine(id, r, st))
	}
	say(DoneLine(action, len(ids)))
	return 0
}

func (f *flat) state(id uint64) uint32 {
	r, _, _ := f.itemState.Call(f.ugc, uintptr(id))
	return uint32(r)
}

// result reads a finished call: ok, failed:<EResult>, or not finished yet.
func (f *flat) result(h uint64, expect uintptr) (string, bool) {
	var failed byte
	done, _, _ := f.completed.Call(f.utils, uintptr(h), uintptr(unsafe.Pointer(&failed)))
	if done&0xff == 0 {
		return "", false
	}
	// { EResult m_eResult; PublishedFileId_t m_nPublishedFileId; }, 8-byte packing on Windows
	var buf [16]byte
	got, _, _ := f.callRslt.Call(f.utils, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), expect, uintptr(unsafe.Pointer(&failed)))
	if got&0xff == 0 || failed != 0 {
		return "failed:io", true
	}
	res := int32(uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24)
	if res == resultOK {
		return "ok", true
	}
	return fmt.Sprintf("failed:%d", res), true
}

// load opens the DLL, starts the Steam API and finds the interfaces. The accessors are
// versioned (SteamAPI_SteamUGC_v020 …): the first one the DLL has is taken.
func load(path string) (*flat, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no steam_api64.dll in %s", filepath.Dir(path))
	}
	dll, err := syscall.LoadDLL(path)
	if err != nil {
		return nil, fmt.Errorf("could not load %s: %v", path, err)
	}
	f := &flat{dll: dll}
	need := func(name string) *syscall.Proc {
		p, e := dll.FindProc(name)
		if e != nil && err == nil {
			err = fmt.Errorf("steam_api64.dll has no %s (too old?)", name)
		}
		return p
	}
	f.runCallbacks, f.shutdown = need("SteamAPI_RunCallbacks"), need("SteamAPI_Shutdown")
	f.subscribe, f.unsubscribe = need("SteamAPI_ISteamUGC_SubscribeItem"), need("SteamAPI_ISteamUGC_UnsubscribeItem")
	f.itemState = need("SteamAPI_ISteamUGC_GetItemState")
	f.completed, f.callRslt = need("SteamAPI_ISteamUtils_IsAPICallCompleted"), need("SteamAPI_ISteamUtils_GetAPICallResult")
	if err != nil {
		return nil, err
	}
	if err := initAPI(dll); err != nil {
		return nil, err
	}
	if f.ugc = accessor(dll, "SteamAPI_SteamUGC_v", 25, 10, "SteamUGC"); f.ugc == 0 {
		f.shutdown.Call()
		return nil, fmt.Errorf("steam_api64.dll gave no ISteamUGC")
	}
	if f.utils = accessor(dll, "SteamAPI_SteamUtils_v", 12, 7, "SteamUtils"); f.utils == 0 {
		f.shutdown.Call()
		return nil, fmt.Errorf("steam_api64.dll gave no ISteamUtils")
	}
	return f, nil
}

// initAPI: SteamAPI_InitFlat (SDK 1.58 and later, 0 = OK, the reason in a buffer), else
// the older SteamAPI_Init (true = OK).
func initAPI(dll *syscall.DLL) error {
	const hint = "is Steam running and signed in?"
	if p, err := dll.FindProc("SteamAPI_InitFlat"); err == nil {
		var msg [1024]byte
		r, _, _ := p.Call(uintptr(unsafe.Pointer(&msg[0])))
		if int32(r) != 0 {
			text := strings.TrimRight(string(msg[:]), "\x00")
			return fmt.Errorf("Steam API did not start (%d %s); %s", int32(r), text, hint)
		}
		return nil
	}
	p, err := dll.FindProc("SteamAPI_Init")
	if err != nil {
		return fmt.Errorf("steam_api64.dll has neither SteamAPI_InitFlat nor SteamAPI_Init")
	}
	if r, _, _ := p.Call(); r&0xff == 0 {
		return fmt.Errorf("Steam API did not start; %s", hint)
	}
	return nil
}

func accessor(dll *syscall.DLL, prefix string, hi, lo int, plain string) uintptr {
	for v := hi; v >= lo; v-- {
		if p, err := dll.FindProc(fmt.Sprintf("%s%03d", prefix, v)); err == nil {
			if r, _, _ := p.Call(); r != 0 {
				return r
			}
		}
	}
	if p, err := dll.FindProc(plain); err == nil {
		r, _, _ := p.Call()
		return r
	}
	return 0
}
