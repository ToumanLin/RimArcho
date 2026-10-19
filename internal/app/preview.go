package app

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rimarcho/rimarcho/internal/store"
)

// Thumbnails (06 §6.2): the mod's About/Preview.png, scaled to 360 px wide and cached by
// snapshot hash in <home>/cache/previews/. The mod folder is only read.

// PreviewWidth is the cached thumbnail width in pixels.
const PreviewWidth = 360

// ErrNoPreview: the mod has no About/Preview.png.
var ErrNoPreview = errors.New("no Preview.png")

// Preview returns the cached thumbnail of a mod (making it on first use) and its size.
func (a *App) Preview(ix *Index, m *store.Mod) (path string, w, h int, err error) {
	key := hex.EncodeToString(m.SnapHash)
	if key == "" {
		key = "i-" + m.ID // no snapshot yet: per instance until the next scan
	}
	path = filepath.Join(a.cacheDir("previews"), key+".png")
	if f, err := os.Open(path); err == nil {
		cfg, err := png.DecodeConfig(f)
		f.Close()
		if err == nil {
			return path, cfg.Width, cfg.Height, nil
		}
	}
	src := findPreview(filepath.Join(a.ModPath(ix, m), "About"))
	if src == "" {
		return "", 0, 0, ErrNoPreview
	}
	f, err := os.Open(src)
	if err != nil {
		return "", 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(io.LimitReader(f, 64<<20))
	if err != nil {
		return "", 0, 0, fmt.Errorf("cannot read Preview.png: %v", err)
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width*cfg.Height > 40e6 {
		return "", 0, 0, errors.New("Preview.png size out of range")
	}
	f.Seek(0, io.SeekStart)
	img, _, err := image.Decode(io.LimitReader(f, 64<<20)) // some Preview.png files are JPEGs
	if err != nil {
		return "", 0, 0, fmt.Errorf("cannot read Preview.png: %v", err)
	}
	b := img.Bounds()
	tw := min(b.Dx(), PreviewWidth)
	th := max(b.Dy()*tw/b.Dx(), 1)
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, ScaleBox(img, tw, th)); err != nil {
		return "", 0, 0, err
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return "", 0, 0, err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", 0, 0, err
	}
	return path, tw, th, nil
}

// findPreview finds Preview.png in an About folder, ignoring case.
func findPreview(dir string) string {
	es, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range es {
		if !e.IsDir() && strings.EqualFold(e.Name(), "Preview.png") {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}
