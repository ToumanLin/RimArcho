package app

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	_ "image/png"

	"github.com/ToumanLin/RimArcho/internal/source"
)

// Launch page backgrounds (06 §2.3, §2.14). The images come from three places:
//   - the game: every official mod's About/Preview.png (RimWorld's planet, the DLC covers);
//   - mods whose name has "background" in it: the images their VBE.BackgroundImageDefs
//     name (Vanilla Backgrounds Expanded, Shella Backgrounds, ...), or, with no such Def,
//     the large landscape images under Textures/;
//   - files the player picked.
// Only those few mod folders are read; nothing else is searched. The page never gets the
// original files: each image is decoded once, one at a time, scaled to BgFull and BgThumb
// wide and kept as JPEG in <home>/cache/backgrounds/, so a 4096 px PNG costs its decode
// once and then a few hundred KB on disk.

// Background is one image the launch page can show.
type Background struct {
	Key   string `json:"key"`   // "game:<packageId>", "mod:<lineage>/<defName>", "file:<path>"
	Label string `json:"label"` // its name
	Group string `json:"group"` // "" the game; else the mod's name
	Src   string `json:"-"`     // the file on disk
}

// The cached widths: the page's background and the settings' thumbnails and preview.
const (
	BgFull  = 1920
	BgThumb = 480
)

// BgDefault is the key of the default background: the game's own planet.
const BgDefault = "game:ludeon.rimworld"

// bgMinWidth: without Defs, a mod's texture counts as a background from this width on.
const bgMinWidth = 1280

// Backgrounds lists the game's and the background mods' images in the index's environment.
func (a *App) Backgrounds(ix *Index) []Background {
	return CollectBackgrounds(a.BackgroundTargets(ix))
}

// BackgroundTarget holds the small immutable input to filesystem discovery. GUI readers
// capture these from the existing index, without retaining or reloading every Mod.
type BackgroundTarget struct {
	Dir, Name, PackageID, LineageID string
	Official                        bool
}

func (a *App) BackgroundTargets(ix *Index) []BackgroundTarget {
	var out []BackgroundTarget
	for _, m := range ix.Mods {
		if !m.TombstoneAt.IsZero() || m.Residue || (m.Kind != source.Official && !strings.Contains(strings.ToLower(m.Name), "background")) {
			continue
		}
		out = append(out, BackgroundTarget{Dir: a.ModPath(ix, m), Name: m.Name, PackageID: m.PackageID, LineageID: m.LineageID, Official: m.Kind == source.Official})
	}
	return out
}

// CollectBackgrounds does the slow file reads without an App or database connection.
func CollectBackgrounds(targets []BackgroundTarget) []Background {
	var game, mods []Background
	seen := map[string]bool{}
	for _, m := range targets {
		dir := m.Dir
		if m.Official {
			src := findPreview(filepath.Join(dir, "About"))
			if src == "" {
				continue
			}
			label := m.Name
			if strings.EqualFold(m.PackageID, "ludeon.rimworld") {
				label = "RimWorld"
			}
			game = append(game, Background{Key: "game:" + strings.ToLower(m.PackageID), Label: label, Src: src})
			continue
		}
		if seen[m.LineageID] {
			continue
		}
		seen[m.LineageID] = true
		for _, b := range modBackgrounds(dir) {
			b.Key = "mod:" + m.LineageID + "/" + b.Key
			b.Group = m.Name
			mods = append(mods, b)
		}
	}
	// the planet first, then the DLC in release order
	sort.SliceStable(game, func(i, j int) bool { return gameRank(game[i].Key) < gameRank(game[j].Key) })
	sort.SliceStable(mods, func(i, j int) bool { return mods[i].Group < mods[j].Group })
	return append(game, mods...)
}

var gameOrder = []string{"rimworld", "royalty", "ideology", "biotech", "anomaly", "odyssey"}

func gameRank(key string) int {
	for i, n := range gameOrder {
		if key == "game:ludeon."+n {
			return i
		}
	}
	return len(gameOrder)
}

// modBackgrounds reads one background mod: its BackgroundImageDefs (Key = defName), or
// its large landscape textures (Key = the texture's path).
func modBackgrounds(dir string) []Background {
	var xmls []string
	tex := map[string]string{} // lower path under any Textures/ without extension → file
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch strings.ToLower(d.Name()) {
			case ".git", "source", "languages", "sounds", "assemblies", "about":
				if p != dir {
					return filepath.SkipDir
				}
			}
			return nil
		}
		low := strings.ToLower(filepath.ToSlash(p))
		switch filepath.Ext(low) {
		case ".xml":
			xmls = append(xmls, p)
		case ".png", ".jpg", ".jpeg":
			if i := strings.LastIndex(low, "/textures/"); i >= 0 {
				k := strings.TrimSuffix(low[i+len("/textures/"):], filepath.Ext(low))
				if _, ok := tex[k]; !ok {
					tex[k] = p
				}
			}
		}
		return nil
	})
	var out []Background
	have := map[string]bool{}
	for _, x := range xmls {
		for _, d := range bgDefs(x) {
			src := tex[strings.ToLower(strings.Trim(filepath.ToSlash(d.Path), "/"))]
			if src == "" || have[d.DefName] {
				continue
			}
			have[d.DefName] = true
			label := d.Label
			if label == "" {
				label = d.DefName
			}
			out = append(out, Background{Key: d.DefName, Label: label, Src: src})
		}
	}
	if len(out) > 0 {
		return out
	}
	keys := make([]string, 0, len(tex))
	for k := range tex {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		w, h := imageSize(tex[k])
		if w >= bgMinWidth && w > h {
			out = append(out, Background{Key: k, Label: filepath.Base(k), Src: tex[k]})
		}
	}
	return out
}

type bgDef struct {
	DefName string `xml:"defName"`
	Label   string `xml:"label"`
	Path    string `xml:"path"`
}

// bgDefs reads the BackgroundImageDefs of one Defs file; nil when it has none.
func bgDefs(path string) []bgDef {
	if st, err := os.Stat(path); err != nil || st.Size() > 4<<20 {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(b, []byte("BackgroundImageDef")) {
		return nil
	}
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.Strict = false
	var out []bgDef
	for {
		t, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := t.(xml.StartElement)
		if !ok || !strings.HasSuffix(se.Name.Local, "BackgroundImageDef") {
			continue
		}
		var d bgDef
		if dec.DecodeElement(&d, &se) == nil && d.DefName != "" && d.Path != "" {
			out = append(out, d)
		}
	}
	return out
}

func imageSize(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// bgMu: one background decode at a time, so the peak is one image's pixels.
var bgMu sync.Mutex

// releaseSoon gives the decoded pixels back to Windows once the decoding stops for a
// moment (a page of thumbnails is many decodes in a row; one release after them, 06 §10).
var (
	releaseMu    sync.Mutex
	releaseTimer *time.Timer
)

func releaseSoon() {
	releaseMu.Lock()
	defer releaseMu.Unlock()
	if releaseTimer != nil {
		releaseTimer.Stop()
	}
	releaseTimer = time.AfterFunc(2*time.Second, debug.FreeOSMemory)
}

// BackgroundImage returns the cached JPEG of src at width BgFull or BgThumb (no wider
// than the original), making both on first use from one decode.
func (a *App) BackgroundImage(src string, width int) (string, error) {
	st, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d", src, st.Size(), st.ModTime().UnixNano())))
	base := filepath.Join(a.cacheDir("backgrounds"), hex.EncodeToString(h[:10]))
	want := fmt.Sprintf("%s-%d.jpg", base, width)
	if _, err := os.Stat(want); err == nil {
		return want, nil
	}
	bgMu.Lock()
	defer bgMu.Unlock()
	if _, err := os.Stat(want); err == nil { // made while this one waited
		return want, nil
	}
	f, err := os.Open(src)
	if err != nil {
		return "", err
	}
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		f.Close()
		return "", fmt.Errorf("cannot read %s: %v", filepath.Base(src), err)
	}
	if cfg.Width < 16 || cfg.Height < 16 || cfg.Width*cfg.Height > 40e6 {
		f.Close()
		return "", errors.New("image size out of range")
	}
	f.Seek(0, io.SeekStart)
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %v", filepath.Base(src), err)
	}
	os.MkdirAll(filepath.Dir(base), 0o755)
	for _, w := range []int{BgFull, BgThumb} {
		if err := writeScaled(img, w, fmt.Sprintf("%s-%d.jpg", base, w)); err != nil {
			return "", err
		}
	}
	img = nil
	releaseSoon()
	return want, nil
}

func writeScaled(img image.Image, w int, path string) error {
	b := img.Bounds()
	if w > b.Dx() {
		w = b.Dx()
	}
	h := max(b.Dy()*w/b.Dx(), 1)
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	err = jpeg.Encode(out, scaleDown(img, w, h), &jpeg.Options{Quality: 86})
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// scaleDown is a box filter like ScaleBox, read straight from the decoded image (no full
// size RGBA copy): only the w×h result is allocated. Alpha is dropped onto black.
func scaleDown(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	px := pixelReader(src)
	for y := 0; y < h; y++ {
		y0, y1 := y*b.Dy()/h, max((y+1)*b.Dy()/h, y*b.Dy()/h+1)
		for x := 0; x < w; x++ {
			x0, x1 := x*b.Dx()/w, max((x+1)*b.Dx()/w, x*b.Dx()/w+1)
			var r, g, bl, n uint32
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					cr, cg, cb := px(b.Min.X+xx, b.Min.Y+yy)
					r, g, bl, n = r+cr, g+cg, bl+cb, n+1
				}
			}
			j := dst.PixOffset(x, y)
			dst.Pix[j], dst.Pix[j+1], dst.Pix[j+2], dst.Pix[j+3] = uint8(r/n), uint8(g/n), uint8(bl/n), 255
		}
	}
	return dst
}

// pixelReader reads 8-bit premultiplied RGB, fast for the types PNG and JPEG decode to.
func pixelReader(src image.Image) func(x, y int) (uint32, uint32, uint32) {
	switch s := src.(type) {
	case *image.NRGBA:
		return func(x, y int) (uint32, uint32, uint32) {
			i := s.PixOffset(x, y)
			a := uint32(s.Pix[i+3])
			return uint32(s.Pix[i]) * a / 255, uint32(s.Pix[i+1]) * a / 255, uint32(s.Pix[i+2]) * a / 255
		}
	case *image.RGBA:
		return func(x, y int) (uint32, uint32, uint32) {
			i := s.PixOffset(x, y)
			return uint32(s.Pix[i]), uint32(s.Pix[i+1]), uint32(s.Pix[i+2])
		}
	}
	return func(x, y int) (uint32, uint32, uint32) {
		r, g, b, _ := src.At(x, y).RGBA()
		return r >> 8, g >> 8, b >> 8
	}
}
