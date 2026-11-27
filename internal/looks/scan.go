package looks

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/ToumanLin/RimArcho/internal/fsscan"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

// The game's texture extensions with the order in which one of two formats of a key is
// shown as the texture (the other is File.Alt).
var texExt = map[string]int{".png": 0, ".dds": 1, ".jpg": 2, ".jpeg": 3, ".psd": 4}

const cacheName = "looks"

// cacheStamp is the version of cacheEnt; a change makes every cached Mod a miss.
var cacheStamp = [16]byte{'l', 'o', 'o', 'k', 's', 1}

// cacheEnt is what the cache keeps for one file, valid while size and mtime match.
type cacheEnt struct {
	S int64  `json:"s"`
	M int64  `json:"m"`
	F string `json:"f"`
	W int    `json:"w,omitempty"`
	H int    `json:"h,omitempty"`
	X string `json:"x,omitempty"`
}

// Counters of files opened for a header and for a hash; tests prove cache reuse with them.
var headerReads, hashReads atomic.Int64

// modScan is what one Mod contributes.
type modScan struct {
	mod      Mod
	keys     []string            // sorted
	tex      map[string]*File    // key -> texture, Alt set for a second format
	next     map[string]cacheEnt // Rel -> cache entry
	defs     []*rawDef
	warnings []string
}

func scanMod(ctx context.Context, m Mod, cache *fsscan.Cache) (*modScan, error) {
	s := &modScan{mod: m, tex: map[string]*File{}, next: map[string]cacheEnt{}}
	folders := uniqueFolders(m.Folders)
	if !m.Official {
		old := map[string]cacheEnt{}
		if cache != nil && !cache.Get(m.Root, cacheName, cacheStamp, &old) {
			old = map[string]cacheEnt{}
		}
		for _, f := range folders {
			if err := s.walkTextures(ctx, f, old); err != nil {
				return nil, err
			}
		}
		for k := range s.tex {
			s.keys = append(s.keys, k)
		}
		sort.Strings(s.keys)
	}
	if err := s.readDefs(ctx, folders); err != nil {
		return nil, err
	}
	return s, nil
}

func uniqueFolders(folders []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range folders {
		f = filepath.Clean(f)
		if k := strings.ToLower(f); !seen[k] {
			seen[k] = true
			out = append(out, f)
		}
	}
	return out
}

// childFold finds the folder named name, in any letter case, inside dir ("" if none).
func childFold(dir, name string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range ents {
		if !strings.EqualFold(e.Name(), name) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if e.IsDir() {
			return p
		}
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p
		}
	}
	return ""
}

// walkTextures adds the keys of one load folder that no higher-priority folder supplied.
// The walking rules are those of textures.Collect: Textures in any letter case, regular
// files only, folders named Source skipped.
func (s *modScan) walkTextures(ctx context.Context, folder string, old map[string]cacheEnt) error {
	dir := childFold(folder, "Textures")
	if dir == "" {
		return nil
	}
	found := map[string][]*File{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != dir && strings.EqualFold(d.Name(), "source") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if _, ok := texExt[ext]; !ok || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		in, _ := filepath.Rel(dir, p)
		key := strings.ToLower(filepath.ToSlash(strings.TrimSuffix(in, filepath.Ext(in))))
		rel, _ := filepath.Rel(s.mod.Root, p)
		if top := s.tex[key]; top != nil {
			// a higher-priority load folder supplies the key: this file is never shown, and
			// only its place is kept (a pack removes it with the Mod's texture, 10 §2)
			top.Under = append(top.Under, &File{Rel: filepath.ToSlash(rel), Tex: filepath.ToSlash(in), Path: p, Size: info.Size(),
				MTime: info.ModTime().UnixNano(), SameAs: -1, ext: ext})
			return nil
		}
		found[key] = append(found[key], &File{Rel: filepath.ToSlash(rel), Tex: filepath.ToSlash(in), Path: p, Size: info.Size(),
			MTime: info.ModTime().UnixNano(), SameAs: -1, ext: ext})
		return nil
	})
	if err != nil {
		return err
	}
	for key, files := range found {
		sort.Slice(files, func(i, j int) bool { return texExt[files[i].ext] < texExt[files[j].ext] })
		s.tex[key] = files[0]
		s.header(files[0], old)
		if len(files) > 1 {
			files[0].Alt = files[1]
			s.header(files[1], old)
		}
	}
	return ctx.Err()
}

// header fills format and size from the cache, or from the file when it changed.
func (s *modScan) header(f *File, old map[string]cacheEnt) {
	e, ok := old[f.Rel]
	if !ok || e.S != f.Size || e.M != f.MTime || e.F == "" {
		e = cacheEnt{S: f.Size, M: f.MTime}
		e.F, e.W, e.H = readHeader(f.Path, f.ext)
	}
	f.Format, f.W, f.H, f.cached = e.F, e.W, e.H, e.X
	s.next[f.Rel] = e
}

// readHeader reads a texture's format, width and height without decoding pixels. The
// format follows the content; a file whose content is not recognised gets its extension
// and no size.
func readHeader(path, ext string) (format string, w, h int) {
	headerReads.Add(1)
	if ext == ".png" {
		if hd, ok := textures.ReadPNGHeader(path); ok {
			return "png", hd.W, hd.H
		}
	}
	format = strings.TrimPrefix(ext, ".")
	if format == "jpeg" {
		format = "jpg"
	}
	f, err := os.Open(path)
	if err != nil {
		return format, 0, 0
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 4096)
	head, _ := r.Peek(128)
	switch {
	case len(head) >= 128 && string(head[:4]) == "DDS ":
		return ddsHeader(head)
	case len(head) >= 26 && string(head[:4]) == "8BPS":
		return "psd", int(binary.BigEndian.Uint32(head[18:22])), int(binary.BigEndian.Uint32(head[14:18]))
	case len(head) >= 2 && head[0] == 0xFF && head[1] == 0xD8:
		w, h = jpegSize(r)
		return "jpg", w, h
	case len(head) >= 24 && string(head[1:4]) == "PNG":
		if hd, ok := textures.ReadPNGHeader(path); ok {
			return "png", hd.W, hd.H
		}
	}
	return format, 0, 0
}

// ddsHeader reads a DDS_HEADER: size, and the FourCC (DXT1, DXT5, …; DX10 for the extended
// header) or the uncompressed layout.
func ddsHeader(b []byte) (string, int, int) {
	h, w := int(binary.LittleEndian.Uint32(b[12:16])), int(binary.LittleEndian.Uint32(b[16:20]))
	flags := binary.LittleEndian.Uint32(b[80:84])
	kind := "RGB"
	switch {
	case flags&0x4 != 0: // DDPF_FOURCC
		kind = strings.TrimRight(string(b[84:88]), "\x00 ")
		for _, c := range kind {
			if c < '0' || c > 'z' {
				kind = "?"
				break
			}
		}
	case flags&0x1 != 0: // DDPF_ALPHAPIXELS
		kind = "RGBA"
	case flags&0x40 == 0: // not DDPF_RGB: luminance, alpha only, …
		kind = "?"
	}
	return "dds:" + kind, w, h
}

// jpegSize reads the frame size from the first SOF segment (0, 0 if there is none).
func jpegSize(r *bufio.Reader) (w, h int) {
	if _, err := r.Discard(2); err != nil {
		return 0, 0
	}
	for i := 0; i < 256; i++ {
		b, err := r.ReadByte()
		for err == nil && b != 0xFF {
			b, err = r.ReadByte()
		}
		for err == nil && b == 0xFF {
			b, err = r.ReadByte()
		}
		if err != nil {
			return 0, 0
		}
		if b == 0xD8 || b == 0x01 || (b >= 0xD0 && b <= 0xD7) {
			continue
		}
		var seg [7]byte
		if _, err := io.ReadFull(r, seg[:2]); err != nil {
			return 0, 0
		}
		n := int(binary.BigEndian.Uint16(seg[:2]))
		if b >= 0xC0 && b <= 0xCF && b != 0xC4 && b != 0xC8 && b != 0xCC {
			if _, err := io.ReadFull(r, seg[2:7]); err != nil {
				return 0, 0
			}
			return int(binary.BigEndian.Uint16(seg[5:7])), int(binary.BigEndian.Uint16(seg[3:5]))
		}
		if b == 0xD9 || b == 0xDA || n < 2 {
			return 0, 0
		}
		if _, err := r.Discard(n - 2); err != nil {
			return 0, 0
		}
	}
	return 0, 0
}

type hashJob struct {
	s *modScan
	f *File
}

// hashContested gives a content hash to every texture that another provider supplies for
// the same key with the same extension and size (21 §2.1). Hashes come from the cache when
// the file is unchanged.
func hashContested(ctx context.Context, scans []*modScan, workers int) error {
	type group struct {
		key, ext string
		size     int64
	}
	groups := map[group][]hashJob{}
	for _, s := range scans {
		for key, f := range s.tex {
			g := group{key, f.ext, f.Size}
			groups[g] = append(groups[g], hashJob{s, f})
		}
	}
	var jobs []hashJob
	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		for _, j := range g {
			if j.f.Hash = j.f.cached; j.f.Hash == "" {
				jobs = append(jobs, j)
			}
		}
	}
	err := parallel(ctx, len(jobs), workers, func(i int) error {
		jobs[i].f.Hash = hashFile(ctx, jobs[i].f.Path)
		return ctx.Err()
	})
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if e := j.s.next[j.f.Rel]; j.f.Hash != "" {
			e.X, j.f.cached = j.f.Hash, j.f.Hash
			j.s.next[j.f.Rel] = e
		}
	}
	return nil
}

// hashFile returns the first 16 bytes of the file's SHA-256 in hex ("" if it cannot be read:
// such a file is identical to nothing).
func hashFile(ctx context.Context, path string) string {
	hashReads.Add(1)
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 256<<10)
	for {
		if ctx.Err() != nil {
			return ""
		}
		n, err := f.Read(buf)
		h.Write(buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			return ""
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}
