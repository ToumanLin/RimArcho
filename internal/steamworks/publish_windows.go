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
	procs, err := findUGCProcs(f, len(r.Translations) > 0)
	if err != nil {
		return err
	}
	if err := checkSignedIn(f); err != nil {
		return err
	}
	p := &publisher{f: f, procs: procs, w: w, r: r}
	// Keep all C strings and the string array alive across flat API calls.
	defer func() { runtime.KeepAlive(p.strings) }()
	legal := false
	if p.r.ID == 0 {
		if legal, err = p.createItem(); err != nil {
			return err
		}
	}
	if p.h, err = p.start(); err != nil {
		return err
	}
	if err := p.setMainFields(); err != nil {
		return err
	}
	b, err := p.submit(p.r.Note)
	if err != nil {
		return err
	}
	legal = legal || b[4] != 0
	fmt.Fprintf(w, "PUBLISHED %d %d\n", p.r.ID, boolNumber(legal))
	if legal {
		legalMessage(w, p.r.ID)
	}
	if err := p.publishTranslations(); err != nil {
		return err
	}
	fmt.Fprintln(w, DoneLine(Publish, 1))
	return nil
}

// findUGCProcs resolves every ISteamUGC export the publish needs, so an old DLL fails before CreateItem.
func findUGCProcs(f *flat, translations bool) (map[string]*syscall.Proc, error) {
	procs := map[string]*syscall.Proc{}
	names := []string{"CreateItem", "StartItemUpdate", "SetItemContent", "SetItemPreview", "SetItemTags", "SetItemTitle", "SetItemDescription", "SetItemVisibility", "SubmitItemUpdate", "GetItemUpdateProgress"}
	if translations {
		names = append(names, "SetItemUpdateLanguage")
	}
	for _, name := range names {
		p, err := f.dll.FindProc("SteamAPI_ISteamUGC_" + name)
		if err != nil {
			return nil, fmt.Errorf("steam_api64.dll has no SteamAPI_ISteamUGC_%s (too old?)", name)
		}
		procs[name] = p
	}
	return procs, nil
}

// checkSignedIn fails unless Steam is running with a signed-in user.
func checkSignedIn(f *flat) error {
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
	return nil
}

// publisher holds one publish's flat API state: the resolved procs, the current update handle and the live C strings.
type publisher struct {
	f       *flat
	procs   map[string]*syscall.Proc
	w       io.Writer
	r       PublishRequest
	h       uintptr // current update handle
	strings []*byte
}

// cstr returns a C string for s that stays alive as long as the publisher.
func (p *publisher) cstr(s string) uintptr {
	b, _ := syscall.BytePtrFromString(s) // ReadPublishRequest rejects NULs.
	p.strings = append(p.strings, b)
	return uintptr(unsafe.Pointer(b))
}

// createItem creates a new Workshop item, reports it and persists its ID; it returns whether the legal agreement is pending.
func (p *publisher) createItem() (bool, error) {
	h, _, _ := p.procs["CreateItem"].Call(p.f.ugc, AppID, 0) // k_EWorkshopFileTypeCommunity
	// CreateItemResult_t: EResult at 0, PublishedFileId_t at 8, bool at 16; sizeof 24.
	b, err := p.f.waitUpload(uint64(h), 3403, 24, time.Minute, nil)
	if err != nil {
		return false, err
	}
	p.r.ID = binary.LittleEndian.Uint64(b[8:16])
	if p.r.ID == 0 {
		return false, errors.New("Steam created no Workshop item ID")
	}
	legal := b[16] != 0
	fmt.Fprintf(p.w, "CREATED %d %d\n", p.r.ID, boolNumber(legal))
	if legal {
		legalMessage(p.w, p.r.ID)
	}
	// Persist before any later operation can fail, so a retry updates this item.
	id := []byte(strconv.FormatUint(p.r.ID, 10) + "\n")
	for _, path := range []string{p.r.IDFile, filepath.Join(p.r.Content, "About", "PublishedFileId.txt")} {
		if err := os.WriteFile(path, id, 0o644); err != nil {
			return legal, fmt.Errorf("created item %d, but could not save %s: %w; reuse --item %d", p.r.ID, path, err, p.r.ID)
		}
	}
	return legal, nil
}

// start opens an update handle for the item.
func (p *publisher) start() (uintptr, error) {
	h, _, _ := p.procs["StartItemUpdate"].Call(p.f.ugc, AppID, uintptr(p.r.ID))
	if h == 0 || h == ^uintptr(0) {
		return 0, errors.New("Steam could not start the Workshop item update; check item ownership")
	}
	return h, nil
}

// set calls one SetItem* export on the current update handle.
func (p *publisher) set(name string, value uintptr) error {
	ok, _, _ := p.procs[name].Call(p.f.ugc, p.h, value)
	if ok&0xff == 0 {
		return fmt.Errorf("Steam rejected %s; check the item's inputs and ownership", name)
	}
	return nil
}

// submit submits the current update with note, printing each new UPLOAD progress line while it waits.
func (p *publisher) submit(note string) ([]byte, error) {
	call, _, _ := p.procs["SubmitItemUpdate"].Call(p.f.ugc, p.h, p.cstr(note))
	var last string
	sample := func() {
		var got, total uint64
		status, _, _ := p.procs["GetItemUpdateProgress"].Call(p.f.ugc, p.h, uintptr(unsafe.Pointer(&got)), uintptr(unsafe.Pointer(&total)))
		line := fmt.Sprintf("UPLOAD %d %d %d", uint32(status), got, total)
		if line != last {
			fmt.Fprintln(p.w, line)
			last = line
		}
	}
	// SubmitItemUpdateResult_t: EResult at 0, bool at 4, PublishedFileId_t at 8; sizeof 16.
	return p.f.waitUpload(uint64(call), 3404, 16, 30*time.Minute, sample)
}

// setMainFields sets content, title, preview, description, tags and visibility on the main update.
func (p *publisher) setMainFields() error {
	r := &p.r
	for _, x := range []struct{ name, value string }{
		{"SetItemContent", r.Content}, {"SetItemTitle", r.Title},
		{"SetItemPreview", r.Preview}, {"SetItemDescription", r.Description},
	} {
		if x.value != "" {
			if err := p.set(x.name, p.cstr(x.value)); err != nil {
				return err
			}
		}
	}
	if err := p.setTags(); err != nil {
		return err
	}
	if r.Visibility != nil {
		if err := p.set("SetItemVisibility", uintptr(*r.Visibility)); err != nil {
			return err
		}
	}
	return nil
}

// setTags passes the request's tags to SetItemTags as a SteamParamStringArray_t.
func (p *publisher) setTags() error {
	values := make([]uintptr, len(p.r.Tags)) // ReadPublishRequest requires at least one
	for i, t := range p.r.Tags {
		values[i] = p.cstr(t)
	}
	// SteamParamStringArray_t: const char ** at 0, int32 count at 8, sizeof 16.
	tags := struct {
		values  *uintptr
		count   int32
		padding int32
	}{&values[0], int32(len(values)), 0}
	if ok, _, _ := p.procs["SetItemTags"].Call(p.f.ugc, p.h, uintptr(unsafe.Pointer(&tags)), 0); ok&0xff == 0 { // bAllowAdminTags = false
		return errors.New("Steam rejected SetItemTags; check the item's inputs and ownership")
	}
	runtime.KeepAlive(tags)
	runtime.KeepAlive(values)
	return nil
}

// publishTranslations submits each translation as its own update.
// An update handle holds one language; its empty note keeps the change history to the main update's note.
func (p *publisher) publishTranslations() error {
	for _, t := range p.r.Translations {
		var err error
		if p.h, err = p.start(); err != nil {
			return err
		}
		for _, x := range []struct{ name, value string }{
			{"SetItemUpdateLanguage", t.Language}, {"SetItemTitle", p.r.Title}, {"SetItemDescription", t.Description},
		} {
			if err := p.set(x.name, p.cstr(x.value)); err != nil {
				return fmt.Errorf("%s translation: %w", t.Language, err)
			}
		}
		if _, err := p.submit(""); err != nil {
			return fmt.Errorf("%s translation: %w", t.Language, err)
		}
		fmt.Fprintf(p.w, "TRANSLATED %d %s\n", p.r.ID, t.Language)
	}
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
