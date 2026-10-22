// Package textures turns a mod's PNG textures into DDS files the game loads in their place
// (docs/research/贴图优化-DDS.md). It knows the game's rules (which files are textures,
// when a .dds shadows a .png, how load folders override each other), measures and
// estimates, and drives the external encoder todds; it never decides where files may be
// written (internal/app does).
package textures

import (
	"bytes"
	"encoding/binary"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The game's texture extensions (ModContentLoader<Texture2D>.AcceptableExtensionsTexture,
// 1.6 and 1.7). A .dds at the same path shadows the others, except .jpeg (the game cuts
// four characters off the name, so "x.jpeg" looks for "x..dds").
var textureExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".psd": true, ".dds": true}

// Source is one PNG the game would load.
type Source struct {
	Path  string // absolute
	Rel   string // relative to the mod root, forward slashes, original case
	Key   string // the game's content key: relative to its load folder, lower case, no extension
	Prio  int    // index of its load folder, 0 = highest priority
	Size  int64
	MTime int64 // unix nanoseconds
	W, H  int
	Alpha bool // an alpha channel (colour type 4 or 6) or a tRNS chunk: BC7, else BC1
	Mask  bool // *_m.png: a mask, never scaled
}

// DDS returns where the DDS of s goes: next to it, same name.
func (s *Source) DDS() string { return strings.TrimSuffix(s.Path, filepath.Ext(s.Path)) + ".dds" }

// Other is a texture file the encoder does not touch: a .dds already there, a .jpg, a
// .psd, or a .png whose content is not PNG.
type Other struct {
	Path string
	Rel  string
	Key  string
	Prio int
	Ext  string // ".dds", ".jpg", ".jpeg", ".psd", ".png" (not a PNG inside)
	Size int64
}

// Set is every texture file of a mod's load folders.
type Set struct {
	Root    string
	Sources []*Source
	Others  []*Other
}

// Collect lists the texture files under Textures/ of each load folder (folders: absolute,
// highest priority first, as content.Resolve returns them). Folders named Source (an
// author's working files) are skipped, as are links leaving the folder.
func Collect(root string, folders []string) (*Set, error) {
	s := &Set{Root: root}
	for prio, f := range folders {
		dir := filepath.Join(f, "Textures")
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			// the folder's own name may differ in case on another file system
			if alt := childFold(f, "Textures"); alt != "" {
				dir = alt
			} else {
				continue
			}
		}
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if p != dir && strings.EqualFold(d.Name(), "source") {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(p))
			if !textureExt[ext] {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			inFolder, _ := filepath.Rel(f, p)
			key := strings.ToLower(strings.TrimSuffix(filepath.ToSlash(inFolder), filepath.Ext(inFolder)))
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			if ext == ".png" {
				if h, ok := ReadPNGHeader(p); ok {
					s.Sources = append(s.Sources, &Source{Path: p, Rel: rel, Key: key, Prio: prio, Size: info.Size(),
						MTime: info.ModTime().UnixNano(), W: h.W, H: h.H, Alpha: h.Alpha,
						Mask: strings.HasSuffix(strings.ToLower(strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))), "_m")})
					return nil
				}
			}
			s.Others = append(s.Others, &Other{Path: p, Rel: rel, Key: key, Prio: prio, Ext: ext, Size: info.Size()})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(s.Sources, func(i, j int) bool { return s.Sources[i].Rel < s.Sources[j].Rel })
	sort.Slice(s.Others, func(i, j int) bool { return s.Others[i].Rel < s.Others[j].Rel })
	return s, nil
}

// AllFolders lists every folder of a mod that has a Textures/ child: the root, version
// folders, LoadFolders.xml entries and any other. A mod folder is often shared by games of
// different versions (one Workshop folder for 1.6 and 1.7), each loading other folders, and
// a .dds in one folder shadows the texture of the same key in all of them, so converting in
// place has to cover every folder any game might load. Textures/, Source/, .git and
// .rimarcho are not searched further; at most four levels deep.
func AllFolders(root string) []string {
	var out []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			if !e.IsDir() || e.Type()&fs.ModeSymlink != 0 {
				continue
			}
			n := strings.ToLower(e.Name())
			switch {
			case n == "textures":
				out = append(out, dir)
			case n == "source" || n == ".git" || n == ".rimarcho" || n == ".svn" || n == "about":
			case depth < 4:
				walk(filepath.Join(dir, e.Name()), depth+1)
			}
		}
	}
	walk(root, 0)
	sort.Strings(out)
	return out
}

// childFold finds a child of dir named name in any case ("" if none).
func childFold(dir, name string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range ents {
		if e.IsDir() && strings.EqualFold(e.Name(), name) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// PNGHeader is what the IHDR (and a tRNS before the image data) says.
type PNGHeader struct {
	W, H  int
	Alpha bool
}

var pngSig = []byte("\x89PNG\r\n\x1a\n")

// ReadPNGHeader reads a PNG's size and whether it has transparency, from the chunks before
// the image data; ok is false when the file is not a PNG.
func ReadPNGHeader(path string) (PNGHeader, bool) {
	f, err := os.Open(path)
	if err != nil {
		return PNGHeader{}, false
	}
	defer f.Close()
	return readPNGHeader(f)
}

func readPNGHeader(r io.Reader) (PNGHeader, bool) {
	var h PNGHeader
	sig := make([]byte, 8)
	if _, err := io.ReadFull(r, sig); err != nil || !bytes.Equal(sig, pngSig) {
		return h, false
	}
	seenIHDR := false
	for i := 0; i < 64; i++ {
		var hdr [8]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return h, seenIHDR
		}
		n := binary.BigEndian.Uint32(hdr[:4])
		typ := string(hdr[4:8])
		switch typ {
		case "IHDR":
			if n < 13 {
				return h, false
			}
			b := make([]byte, n)
			if _, err := io.ReadFull(r, b); err != nil {
				return h, false
			}
			h.W, h.H = int(binary.BigEndian.Uint32(b[0:4])), int(binary.BigEndian.Uint32(b[4:8]))
			ct := b[9]
			h.Alpha = ct == 4 || ct == 6
			seenIHDR = true
			if _, err := io.CopyN(io.Discard, r, 4); err != nil { // crc
				return h, false
			}
			continue
		case "tRNS":
			h.Alpha = true
		case "IDAT", "IEND":
			return h, seenIHDR && h.W > 0 && h.H > 0
		}
		if _, err := io.CopyN(io.Discard, r, int64(n)+4); err != nil {
			return h, seenIHDR
		}
	}
	return h, seenIHDR && h.W > 0 && h.H > 0
}
