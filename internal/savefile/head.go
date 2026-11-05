// Package savefile reads RimWorld saves (.rws) without writing them (10 §1). ReadHead is
// the cheap tier (10 §1.7 L0): the <meta> block at the start and the last bytes of the file.
package savefile

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

// ErrNoMeta: the file has no <meta><modIds>, so it is not a usable save.
var ErrNoMeta = errors.New("no <meta><modIds> in the save")

// Head is what a save says about itself before the game data (10 §1.3), plus whether the
// file ends like a finished save.
type Head struct {
	GameVersion string   `json:"gameVersion"`
	ModIDs      []string `json:"modIds"`   // lower-case packageIds in load order
	ModNames    []string `json:"modNames"` // one per ModIDs entry when the save has them
	SteamIDs    []string `json:"steamIds"` // Workshop IDs per ModIDs entry; "0" or "" for none
	// Complete: the last non-space bytes are </savegame>. A file cut off while the game
	// wrote it (often during a crash) is not complete (10 §4.3).
	Complete bool `json:"complete"`
}

// Name is the save's name for packageId i, falling back to the id.
func (h *Head) Name(i int) string {
	if i < len(h.ModNames) && h.ModNames[i] != "" {
		return h.ModNames[i]
	}
	return h.ModIDs[i]
}

// SteamID is the Workshop ID the save recorded for packageId i ("" for none).
func (h *Head) SteamID(i int) string {
	if i < len(h.SteamIDs) && h.SteamIDs[i] != "0" {
		return h.SteamIDs[i]
	}
	return ""
}

// File is a save on disk as read at one moment.
type File struct {
	Size    int64
	ModTime time.Time
}

// ReadHead reads a save's <meta> and checks its end. The error is ErrNoMeta for a file
// without a Mod list, or the read error. The returned File is the size and modification
// time seen before reading; Changed reports whether they moved while reading (the game is
// writing the file).
func ReadHead(path string) (*Head, File, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, File{}, false, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, File{}, false, err
	}
	before := File{Size: fi.Size(), ModTime: fi.ModTime()}
	h, err := ReadMeta(bufio.NewReader(f))
	if err != nil {
		return nil, before, false, err
	}
	h.Complete = endsComplete(f, before.Size)
	changed := false
	if fi2, err := os.Stat(path); err != nil || fi2.Size() != before.Size || !fi2.ModTime().Equal(before.ModTime) {
		changed = true
	}
	return h, before, changed, nil
}

// endsComplete reports whether the last non-space bytes of the file are </savegame>.
func endsComplete(f *os.File, size int64) bool {
	const tail = 64
	off := max(size-tail, 0)
	b := make([]byte, size-off)
	if _, err := f.ReadAt(b, off); err != nil && err != io.EOF {
		return false
	}
	return bytes.HasSuffix(bytes.TrimRight(b, " \t\r\n\x00"), []byte("</savegame>"))
}

// ReadMeta reads <savegame><meta>: gameVersion, modIds, modSteamIds and modNames, and
// stops at </meta> (saves are tens of MB; the meta block comes first). Parsing is not
// strict and ignores the declared character set.
func ReadMeta(r io.Reader) (*Head, error) {
	d := xml.NewDecoder(r)
	d.Strict = false
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	h := &Head{}
	var path []string
	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := t.(type) {
		case xml.StartElement:
			path = append(path, t.Name.Local)
			// Each <li> opens a slot, so an empty one keeps the lists aligned.
			if l := h.list(path); l != nil {
				*l = append(*l, "")
			}
		case xml.EndElement:
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
			if t.Name.Local == "meta" {
				return finish(h)
			}
		case xml.CharData:
			n := len(path)
			if n >= 2 && path[n-1] == "gameVersion" && path[n-2] == "meta" {
				h.GameVersion += strings.TrimSpace(string(t))
				continue
			}
			if l := h.list(path); l != nil && len(*l) > 0 {
				(*l)[len(*l)-1] += strings.TrimSpace(string(t))
			}
		}
	}
	return finish(h)
}

// list is the slice an <li> at path belongs to (nil when it is not one of meta's lists).
func (h *Head) list(path []string) *[]string {
	n := len(path)
	if n < 3 || path[n-1] != "li" || path[n-3] != "meta" {
		return nil
	}
	switch path[n-2] {
	case "modIds":
		return &h.ModIDs
	case "modNames":
		return &h.ModNames
	case "modSteamIds":
		return &h.SteamIDs
	}
	return nil
}

func finish(h *Head) (*Head, error) {
	ids := h.ModIDs[:0]
	names, steam := h.ModNames, h.SteamIDs
	h.ModNames, h.SteamIDs = nil, nil
	for i, id := range h.ModIDs {
		if id = strings.ToLower(id); id == "" {
			continue // an empty id names nothing; drop its name and Workshop ID with it
		}
		ids = append(ids, id)
		if i < len(names) {
			h.ModNames = append(h.ModNames, names[i])
		}
		if i < len(steam) {
			h.SteamIDs = append(h.SteamIDs, steam[i])
		}
	}
	h.ModIDs = ids
	if len(h.ModIDs) == 0 {
		return nil, ErrNoMeta
	}
	return h, nil
}
