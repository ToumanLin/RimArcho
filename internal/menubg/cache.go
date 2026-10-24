package menubg

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// One decode lane across environments: only the small outputs and four source
// rows are live. Completed JPEG pairs survive restarts and language changes.
var decodeMu sync.Mutex

// Prepare performs the slow, cancellable work in the caller's background goroutine.
// Errors leave the affected image out; the GUI uses plain until a pair is ready.
// It must not be called while holding the GUI mutex (06 §3.1, 06 §2.14).
func Prepare(ctx context.Context, game, cache, lang string) ([]Item, error) {
	stamp := Stamp(game)
	archive, err := zip.OpenReader(filepath.Join(game, "Assets.zip"))
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	// archive/zip reads the local header (including variable name/extra fields) to
	// locate the compressed bytes, and verifies size and CRC-32 at EOF for all methods.
	archive.RegisterDecompressor(93, func(r io.Reader) io.ReadCloser {
		d, err := zstd.NewReader(r, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(64<<20))
		if err != nil {
			return failedReader{err}
		}
		return d.IOReadCloser()
	})
	entries, err := catalog(game, lang, archive.File)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return nil, err
	}
	var out []Item
	var failures []error
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		hash := sha256.Sum256([]byte("menu-v1|" + stamp + "|" + e.file.Name))
		base := filepath.Join(cache, fmt.Sprintf("menu-%x", hash[:12]))
		it := Item{Key: "game:" + strings.ToLower(e.def.Mod), Label: e.def.Label, Full: base + "-1920.jpg", Thumb: base + "-480.jpg"}
		if err := preparePair(ctx, e.file, it); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", e.def.Name, err))
			continue
		}
		out = append(out, it)
	}
	// Do not publish a pair under a signature that changed during extraction.
	if stamp != Stamp(game) {
		return nil, errors.New("Assets.zip changed while reading menu backgrounds")
	}
	return out, errors.Join(failures...)
}

type failedReader struct{ err error }

func (r failedReader) Read([]byte) (int, error) { return 0, r.err }
func (r failedReader) Close() error             { return nil }

func preparePair(ctx context.Context, f *zip.File, it Item) error {
	decodeMu.Lock()
	defer decodeMu.Unlock()
	if regular(it.Full) && regular(it.Thumb) {
		return nil
	}
	defer debug.FreeOSMemory() // return each image's scratch/output memory (06 §2.14)
	return extractPair(ctx, f, it)
}

func regular(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular() && st.Size() > 0
}

func extractPair(ctx context.Context, f *zip.File, it Item) error {
	if f.Flags&1 != 0 || f.UncompressedSize64 > 128<<20 {
		return errors.New("encrypted or oversized menu texture")
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	d, err := ReadDDS(r)
	if err != nil {
		return err
	}
	images, err := d.Scaled(ctx, 1920, 480)
	if err != nil {
		return err
	}
	// Consume unused mipmaps too: Open's checksum reader reports a bad CRC at EOF.
	if _, err := io.Copy(io.Discard, contextReader{ctx, r}); err != nil {
		return err
	}
	for i, path := range []string{it.Full, it.Thumb} {
		if err := writeJPEG(images[i], path); err != nil {
			return err
		}
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func writeJPEG(img image.Image, path string) error {
	f, err := os.Create(path + ".tmp")
	if err != nil {
		return err
	}
	defer os.Remove(path + ".tmp")
	err = jpeg.Encode(f, img, &jpeg.Options{Quality: 86})
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}
