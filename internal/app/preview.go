package app

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/store"
)

// Thumbnails (16 §4.4): the mod's About/Preview.png, scaled to 360 px wide and cached by
// snapshot hash in <home>/cache/previews/. The mod folder is only read.

// PreviewWidth is the cached thumbnail width in pixels.
const PreviewWidth = 360

// ErrNoPreview: the mod has no About/Preview.png.
var ErrNoPreview = errors.New("no Preview.png")

// PreviewTarget is what making a thumbnail needs from the index: two paths. Capture it
// while the index is locked; PreviewOf then reads and decodes without the lock.
type PreviewTarget struct{ cache, about string }

// PreviewTargetOf returns the thumbnail's cache path and the mod's About folder.
func (a *App) PreviewTargetOf(ix *Index, m *store.Mod) PreviewTarget {
	key := hex.EncodeToString(m.SnapHash)
	if key == "" {
		key = "i-" + m.ID // no snapshot yet: per instance until the next scan
	}
	return PreviewTarget{
		cache: filepath.Join(a.cacheDir("previews"), key+".png"),
		about: filepath.Join(a.ModPath(ix, m), "About"),
	}
}

// Preview returns the cached thumbnail of a mod (making it on first use) and its size.
func (a *App) Preview(ix *Index, m *store.Mod) (path string, w, h int, err error) {
	return a.PreviewOf(a.PreviewTargetOf(ix, m))
}

// PreviewOf is Preview from a captured target.
func (a *App) PreviewOf(t PreviewTarget) (path string, w, h int, err error) {
	if w, h, ok := cachedPNG(t.cache); ok {
		return t.cache, w, h, nil
	}
	src := findPreview(t.about)
	if src == "" {
		return "", 0, 0, ErrNoPreview
	}
	w, h, err = makeCachedPNG(t.cache, PreviewWidth, func() (io.ReadSeeker, func(), error) {
		f, err := os.Open(src)
		if err != nil {
			return nil, nil, err
		}
		// some Preview.png files are JPEGs; none is near 64 MB
		return io.NewSectionReader(f, 0, 64<<20), func() { f.Close() }, nil
	})
	var bad decodeError
	switch {
	case errors.Is(err, errImageSize):
		return "", 0, 0, errors.New("Preview.png size out of range")
	case errors.As(err, &bad):
		return "", 0, 0, fmt.Errorf("cannot read Preview.png: %v", bad.error)
	case err != nil:
		return "", 0, 0, err
	}
	return t.cache, w, h, nil
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
