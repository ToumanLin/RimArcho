package app

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/looks"
)

// Texture pictures of the looks view (16 §4.4, 21 §6). The Mod folder is only read: a
// thumbnail, or a DDS turned into a PNG, is kept in <home>/cache/looktex/.

// LookTexWidths are the thumbnail boxes the looks view asks for; 0 is the original.
var LookTexWidths = []int{96, 192, 512}

// LookTexture returns the file to serve for the texture f at width w, and its media type.
// w 0 is the original: a PNG or JPEG as the Mod ships it, a DDS turned into a PNG. Another
// w is a PNG scaled down to fit a w×w box (never enlarged; a PNG or JPEG that already fits
// is served as it is). The PNG twin is shown when the Mod has both formats (looks.Shown).
// ErrNoPreview: no decoder reads the format (BC7 and other DX10 DDS, PSD), or w is no
// width of LookTexWidths. f comes from a looks index: no path is built from anything else.
func (a *App) LookTexture(f *looks.File, w int) (path, mime string, err error) {
	if f = looks.Shown(f); f == nil {
		return "", "", ErrNoPreview
	}
	ok := w == 0
	for _, x := range LookTexWidths {
		ok = ok || w == x
	}
	if !ok {
		return "", "", ErrNoPreview
	}
	dds := f.Format != "png" && f.Format != "jpg"
	if !dds && (w == 0 || (f.W > 0 && f.H > 0 && f.W <= w && f.H <= w)) {
		if f.Format == "jpg" {
			return f.Path, "image/jpeg", nil
		}
		return f.Path, "image/png", nil
	}
	st, err := os.Stat(f.Path)
	if err != nil {
		return "", "", err
	}
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d", f.Path, st.Size(), st.ModTime().UnixNano())))
	cache := filepath.Join(a.cacheDir("looktex"), fmt.Sprintf("%s-%d.png", hex.EncodeToString(sum[:10]), w))
	if _, _, ok := cachedPNG(cache); ok {
		return cache, "image/png", nil
	}
	decodeSlots <- struct{}{}
	defer func() { <-decodeSlots }()
	if _, _, ok := cachedPNG(cache); ok { // made while this one waited
		return cache, "image/png", nil
	}
	img, err := decodeTexture(f.Path, dds)
	if err != nil {
		return "", "", err
	}
	b := img.Bounds()
	if w > 0 && (b.Dx() > w || b.Dy() > w) {
		tw, th := w, max(b.Dy()*w/b.Dx(), 1)
		if b.Dy() > b.Dx() {
			tw, th = max(b.Dx()*w/b.Dy(), 1), w
		}
		img = ScaleBox(img, tw, th)
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img); err != nil {
		return "", "", err
	}
	if err := writeCacheFile(cache, buf.Bytes()); err != nil {
		return "", "", err
	}
	img = nil
	releaseSoon()
	return cache, "image/png", nil
}

// decodeTexture decodes a texture file once a decode slot is held. A picture over 40
// megapixels is refused before its pixels are allocated. A DDS is turned upright: files
// for the game hold their rows bottom first (10 §2).
func decodeTexture(path string, dds bool) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if dds {
		img, err := looks.DecodeDDS(file)
		if err != nil {
			return nil, decodeError{err}
		}
		looks.FlipV(img)
		return img, nil
	}
	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return nil, decodeError{err}
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width*cfg.Height > looks.MaxPixels {
		return nil, errImageSize
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(file)
	if err != nil {
		return nil, decodeError{err}
	}
	return img, nil
}
