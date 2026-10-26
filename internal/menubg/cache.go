package menubg

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// One decode lane across environments: small outputs and four source rows,
// plus a bounded payload buffer for split BC7 entries. Completed JPEG pairs
// survive restarts and language changes.
var decodeMu sync.Mutex

// prepareZIP opens an archive only for expansions whose shared pairs are missing.
func prepareZIP(ctx context.Context, game, cache string, defs []expansion) ([]Item, error) {
	stamp := Stamp(game)
	archive, err := zip.OpenReader(filepath.Join(game, "Assets.zip"))
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	archive.RegisterDecompressor(93, func(r io.Reader) io.ReadCloser {
		d, err := zstd.NewReader(r, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(64<<20))
		if err != nil {
			return failedReader{err}
		}
		return d.IOReadCloser()
	})
	var out []Item
	var failures []error
	for _, e := range catalogDefs(game, defs, archive.File) {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		it := cacheItem(cache, e.def)
		if err := prepareSharedPair(ctx, game, stamp, it, func(staged Item) error {
			return extractPair(ctx, e.file, staged)
		}); err != nil {
			failures = append(failures, fmt.Errorf("%s (%s): %w; using About/Preview.png if available", e.def.Name, e.file.Name, err))
			continue
		}
		out = append(out, it)
	}
	return out, errors.Join(failures...)
}

type failedReader struct{ err error }

func (r failedReader) Read([]byte) (int, error) { return 0, r.err }
func (r failedReader) Close() error             { return nil }

func extractPair(ctx context.Context, f *zip.File, it Item) error {
	if f.Flags&1 != 0 || f.UncompressedSize64 > maxMenuTexture {
		return errors.New("encrypted or oversized menu texture")
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	checked := &textureReader{r: contextReader{ctx, r}}
	var d *DDS
	if strings.HasSuffix(strings.ToLower(f.Name), ".dds.split") {
		d, err = readSplitDDS(ctx, checked, f.UncompressedSize64)
	} else {
		d, err = ReadDDS(checked)
	}
	if err != nil {
		return err
	}
	images, err := d.Scaled(ctx, 1920, 480)
	if err != nil {
		return err
	}
	// Consume unused mipmaps too: Open's checksum reader reports a bad CRC at EOF.
	if _, err := io.Copy(io.Discard, contextReader{ctx, d.r}); err != nil {
		return err
	}
	if checked.err != nil {
		return checked.err
	}
	for i, path := range []string{it.Full, it.Thumb} {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writeJPEG(images[i], path); err != nil {
			return err
		}
	}
	return nil
}

// Preserve terminal checksum errors even when an exact-length DDS read consumes
// the accompanying bytes and io.ReadFull suppresses that error.
type textureReader struct {
	r   io.Reader
	err error
}

func (r *textureReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if err != nil && err != io.EOF && r.err == nil {
		r.err = err
	}
	return n, err
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
