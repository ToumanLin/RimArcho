//go:build windows && amd64

package steamworks

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

// PublishWorker uses only exported flat API functions, with Windows' 8-byte packing.
// Missing exports fail before CreateItem, so an old DLL cannot create an empty item.
func PublishWorker(r PublishRequest, w io.Writer) int {
	err := publishWorker(r, w)
	if err != nil {
		fmt.Fprintln(w, ErrorLine(err.Error()))
		return 3
	}
	return 0
}

func publishWorker(r PublishRequest, w io.Writer) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	f, err := load(filepath.Join(wd, "steam_api64.dll"))
	if err != nil {
		return err
	}
	defer f.shutdown.Call()
	procs := map[string]*syscall.Proc{}
	for _, name := range []string{"CreateItem", "StartItemUpdate", "SetItemContent", "SetItemPreview", "SetItemTags", "SetItemTitle", "SetItemDescription", "SetItemVisibility", "SubmitItemUpdate", "GetItemUpdateProgress"} {
		p, err := f.dll.FindProc("SteamAPI_ISteamUGC_" + name)
		if err != nil {
			return fmt.Errorf("steam_api64.dll has no SteamAPI_ISteamUGC_%s (too old?)", name)
		}
		procs[name] = p
	}
	user := accessor(f.dll, "SteamAPI_SteamUser_v", 25, 17, "SteamUser")
	loggedOn, err := f.dll.FindProc("SteamAPI_ISteamUser_BLoggedOn")
	if err != nil {
		return errors.New("steam_api64.dll has no SteamAPI_ISteamUser_BLoggedOn")
	}
	if user == 0 {
		return errors.New("Steam gave no ISteamUser; Steam must be running and signed in")
	}
	if ok, _, _ := loggedOn.Call(user); ok&0xff == 0 {
		return errors.New("Steam must be running and signed in before publishing")
	}
	// Keep all C strings and the string array alive across flat API calls.
	var strings []*byte
	cstr := func(s string) uintptr {
		p, _ := syscall.BytePtrFromString(s) // ReadPublishRequest rejects NULs.
		strings = append(strings, p)
		return uintptr(unsafe.Pointer(p))
	}
	defer func() { runtime.KeepAlive(strings) }()
	legal := false
	if r.ID == 0 {
		h, _, _ := procs["CreateItem"].Call(f.ugc, AppID, 0) // k_EWorkshopFileTypeCommunity
		// CreateItemResult_t: EResult at 0, PublishedFileId_t at 8, bool at 16; sizeof 24.
		b, err := f.waitUpload(uint64(h), 3403, 24, time.Minute, nil)
		if err != nil {
			return err
		}
		r.ID = binary.LittleEndian.Uint64(b[8:16])
		if r.ID == 0 {
			return errors.New("Steam created no Workshop item ID")
		}
		legal = b[16] != 0
		fmt.Fprintf(w, "CREATED %d %d\n", r.ID, boolNumber(legal))
		if legal {
			legalMessage(w, r.ID)
		}
		// Persist before any later operation can fail, so a retry updates this item.
		id := []byte(strconv.FormatUint(r.ID, 10) + "\n")
		for _, path := range []string{r.IDFile, filepath.Join(r.Content, "About", "PublishedFileId.txt")} {
			if err := os.WriteFile(path, id, 0o644); err != nil {
				return fmt.Errorf("created item %d, but could not save %s: %w; reuse --item %d", r.ID, path, err, r.ID)
			}
		}
	}
	h, _, _ := procs["StartItemUpdate"].Call(f.ugc, AppID, uintptr(r.ID))
	if h == 0 || h == ^uintptr(0) {
		return errors.New("Steam could not start the Workshop item update; check item ownership")
	}
	set := func(name string, value uintptr) error {
		ok, _, _ := procs[name].Call(f.ugc, h, value)
		if ok&0xff == 0 {
			return fmt.Errorf("Steam rejected %s; check the item's inputs and ownership", name)
		}
		return nil
	}
	for _, x := range []struct{ name, value string }{
		{"SetItemContent", r.Content}, {"SetItemTitle", r.Title},
		{"SetItemPreview", r.Preview}, {"SetItemDescription", r.Description},
	} {
		if x.value != "" {
			if err := set(x.name, cstr(x.value)); err != nil {
				return err
			}
		}
	}
	tag := cstr("v" + r.Version)
	// SteamParamStringArray_t: const char ** at 0, int32 count at 8, sizeof 16.
	tags := struct {
		values  *uintptr
		count   int32
		padding int32
	}{&tag, 1, 0}
	if ok, _, _ := procs["SetItemTags"].Call(f.ugc, h, uintptr(unsafe.Pointer(&tags)), 0); ok&0xff == 0 { // bAllowAdminTags = false
		return errors.New("Steam rejected SetItemTags; check the item's inputs and ownership")
	}
	runtime.KeepAlive(tags)
	if r.Visibility != nil {
		if err := set("SetItemVisibility", uintptr(*r.Visibility)); err != nil {
			return err
		}
	}
	call, _, _ := procs["SubmitItemUpdate"].Call(f.ugc, h, cstr(r.Note))
	var last string
	sample := func() {
		var got, total uint64
		status, _, _ := procs["GetItemUpdateProgress"].Call(f.ugc, h, uintptr(unsafe.Pointer(&got)), uintptr(unsafe.Pointer(&total)))
		line := fmt.Sprintf("UPLOAD %d %d %d", uint32(status), got, total)
		if line != last {
			fmt.Fprintln(w, line)
			last = line
		}
	}
	// SubmitItemUpdateResult_t: EResult at 0, bool at 4, PublishedFileId_t at 8; sizeof 16.
	b, err := f.waitUpload(uint64(call), 3404, 16, 30*time.Minute, sample)
	if err != nil {
		return err
	}
	legal = legal || b[4] != 0
	fmt.Fprintf(w, "PUBLISHED %d %d\n", r.ID, boolNumber(legal))
	if legal {
		legalMessage(w, r.ID)
	}
	fmt.Fprintln(w, DoneLine(Publish, 1))
	return nil
}

func boolNumber(b bool) int {
	if b {
		return 1
	}
	return 0
}
func legalMessage(w io.Writer, id uint64) {
	fmt.Fprintf(w, "LEGAL Accept the Steam Workshop legal agreement on https://steamcommunity.com/sharedfiles/filedetails/?id=%d; the item stays hidden until accepted\n", id)
}

// Like subscribe, poll ISteamUtils and retrieve the call result by callback ID.
func (f *flat) waitUpload(h uint64, callback uintptr, size int, limit time.Duration, progress func()) ([]byte, error) {
	if h == 0 {
		return nil, errors.New("Steam did not accept the Workshop API call")
	}
	for end := time.Now().Add(limit); time.Now().Before(end); time.Sleep(250 * time.Millisecond) {
		f.runCallbacks.Call()
		if progress != nil {
			progress()
		}
		var failed byte
		done, _, _ := f.completed.Call(f.utils, uintptr(h), uintptr(unsafe.Pointer(&failed)))
		if done&0xff == 0 {
			continue
		}
		if failed != 0 {
			return nil, errors.New("Steam Workshop call failed while communicating with Steam")
		}
		b := make([]byte, size)
		got, _, _ := f.callRslt.Call(f.utils, uintptr(h), uintptr(unsafe.Pointer(&b[0])), uintptr(size), callback, uintptr(unsafe.Pointer(&failed)))
		if got&0xff == 0 || failed != 0 {
			return nil, errors.New("could not read Steam Workshop call result")
		}
		if result := int32(binary.LittleEndian.Uint32(b[:4])); result != resultOK {
			return nil, errors.New(PublishResultMessage(result))
		}
		return b, nil
	}
	return nil, errors.New("Steam Workshop upload timed out; check the item page before retrying")
}
