package textures

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

type cachedHeader struct {
	Size   int64
	MTime  int64
	Header PNGHeader
	Valid  bool
}

// CollectCached still checks the directory entries and each file's size and mtime.
// Only the PNG header is reused: Steam can replace, add or remove files without a scan.
// A cache file belongs to one mod, so full-library reads retain only one mod's headers.
func CollectCached(ctx context.Context, root string, folders []string, cachePath string) (*Set, error) {
	return collectCached(ctx, root, folders, cachePath, ReadPNGHeader)
}

func collectCached(ctx context.Context, root string, folders []string, cachePath string, readHeader func(string) (PNGHeader, bool)) (*Set, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	old := map[string]cachedHeader{}
	if f, err := os.Open(cachePath); err == nil {
		// A damaged or oversized cache is just a miss.
		if json.NewDecoder(io.LimitReader(f, 8<<20)).Decode(&old) != nil {
			old = nil
		}
		f.Close()
	}
	next := map[string]cachedHeader{}
	changed := false
	s, err := collect(ctx, root, folders, func(path string, info os.FileInfo) (PNGHeader, bool) {
		h, ok := old[path]
		if !ok || h.Size != info.Size() || h.MTime != info.ModTime().UnixNano() {
			h = cachedHeader{Size: info.Size(), MTime: info.ModTime().UnixNano()}
			h.Header, h.Valid = readHeader(path)
			changed = true
		}
		next[path] = h
		return h.Header, h.Valid
	})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if (changed || len(old) != len(next)) && ctx.Err() == nil {
		// Cache failures never prevent reading or converting the mod.
		if os.MkdirAll(filepath.Dir(cachePath), 0o755) == nil {
			if f, err := os.CreateTemp(filepath.Dir(cachePath), "png-*.tmp"); err == nil {
				name := f.Name()
				err = json.NewEncoder(f).Encode(next)
				if closeErr := f.Close(); err == nil {
					err = closeErr
				}
				if err == nil {
					_ = os.Rename(name, cachePath)
				}
				_ = os.Remove(name)
			}
		}
	}
	return s, nil
}
