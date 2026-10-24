package loadcost

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/fsscan"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

// Measure reads the effective load folders in priority order (highest first).
// PNG headers use pngCache; other measurements are returned for the caller to cache.
// XML and language totals include all layers. Sounds and DLLs use the first file
// at each case-insensitive relative path. Unreadable WAV headers fall back to
// file bytes as PCM16; AIFF-family files use that approximation without opening.
func Measure(ctx context.Context, root string, folders []string, pngCache string) (Mod, error) {
	var m Mod
	set, err := textures.CollectCached(ctx, root, folders, pngCache)
	if err != nil {
		return Mod{}, fmt.Errorf("measure textures in %q: %w", root, err)
	}
	if err := measureTextures(ctx, &m, set); err != nil {
		return Mod{}, err
	}
	seen := make(map[string]bool)
	for _, folder := range folders {
		tree, err := fsscan.Walk(ctx, folder, fsscan.Options{
			Workers: 4,
			SkipDir: func(rel, name string) bool {
				return ignored(name) || (!strings.Contains(rel, "/") && !assetDir(strings.ToLower(name)))
			},
			Keep: func(rel, name string) bool {
				dir, _, ok := strings.Cut(strings.ToLower(rel), "/")
				if !ok {
					return false
				}
				ext := strings.ToLower(filepath.Ext(name))
				switch dir {
				case "sounds":
					return soundExt(ext)
				case "defs", "patches":
					return ext == ".xml"
				case "assemblies":
					return ext == ".dll"
				case "languages":
					return true
				}
				return false
			},
			Sorted: true,
		})
		if errors.Is(err, fs.ErrNotExist) {
			continue // the game skips a load folder that does not exist
		}
		if err != nil {
			return Mod{}, fmt.Errorf("measure load folder %q: %w", folder, err)
		}
		for _, e := range tree.Entries {
			if err := ctx.Err(); err != nil {
				return Mod{}, err
			}
			rel := strings.ToLower(e.Rel)
			dir, _, _ := strings.Cut(rel, "/")
			f := Files{Count: 1, Bytes: e.Size}
			switch dir {
			case "defs":
				m.Defs.add(f)
			case "patches":
				m.Patches.add(f)
			case "languages":
				m.Languages.add(f)
			case "assemblies", "sounds":
				if seen[rel] {
					continue
				}
				seen[rel] = true
				if dir == "assemblies" {
					m.Assemblies.add(f)
					continue
				}
				ext := strings.ToLower(filepath.Ext(e.Rel))
				pcm := e.Size
				if ext == ".wav" {
					pcm = wavPCM16(ctx, filepath.Join(folder, filepath.FromSlash(e.Rel)), e.Size)
				}
				if ext == ".ogg" || ext == ".mp3" {
					pcm = int64(float64(e.Size) * Coef.CompressedPCM16)
				}
				if m.Sounds == nil {
					m.Sounds = make(map[string]Sound)
				}
				s := m.Sounds[ext]
				s.Files.add(f)
				if e.Size <= Coef.Sound16FileLimit {
					s.Small.add(f)
					s.SmallPCM16Bytes += pcm
				}
				if pcm <= Coef.Sound17PCM16Limit {
					s.Resident.add(f)
				}
				m.Sounds[ext] = s
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return Mod{}, err
	}
	return m, nil
}

func assetDir(name string) bool {
	switch name {
	case "sounds", "defs", "patches", "languages", "assemblies":
		return true
	}
	return false
}

func soundExt(ext string) bool {
	switch ext {
	case ".wav", ".ogg", ".mp3", ".aif", ".aiff", ".aifc":
		return true
	}
	return false
}

func ignored(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".svn", ".rimarcho":
		return true
	}
	return false
}

type textureFile struct {
	key, ext, path string
	size           int64
	prio           int
	png            *textures.Source
}

func measureTextures(ctx context.Context, m *Mod, set *textures.Set) error {
	// CollectCached retains every candidate for conversion. Merge by relative
	// filename before applying DDS shadowing, just as GetAllFilesForMod does.
	files := make(map[string]textureFile)
	add := func(f textureFile) {
		dir, _ := filepath.Split(f.key)
		for _, component := range strings.Split(dir, "/") {
			if ignored(component) {
				return
			}
		}
		key := f.key + f.ext
		if old, ok := files[key]; !ok || f.prio < old.prio {
			files[key] = f
		}
	}
	for _, s := range set.Sources {
		add(textureFile{s.Key, ".png", s.Path, s.Size, s.Prio, s})
	}
	for _, s := range set.Others {
		add(textureFile{s.Key, s.Ext, s.Path, s.Size, s.Prio, nil})
	}
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		ddsKey := f.key + ".dds"
		// The game's four-character extension trim leaves an extra dot for JPEG.
		if f.ext == ".jpeg" {
			ddsKey = f.key + "..dds"
		}
		if _, ok := files[ddsKey]; ok && f.ext != ".dds" {
			continue
		}
		count := Files{Count: 1, Bytes: f.size}
		switch f.ext {
		case ".png":
			if f.png == nil {
				// Invalid PNGs have no usable dimensions.
				continue
			}
			w, h := int64(f.png.W), int64(f.png.H)
			pixels := mipPixels(w, h)
			m.PNG.add(count)
			m.PNGPixels += w * h
			m.PNGPixelsMip += pixels
			if w <= 16 && h <= 16 {
				m.PNGRaw16.Count++
				m.PNGRaw16.PixelsMip += pixels
			}
			if w%4 != 0 || h%4 != 0 {
				m.PNGNot4 += pixels
			}
			if w < 512 && h < 512 {
				m.PNGSmall512Pixels += w * h
			}
		case ".dds":
			count.Bytes = ddsPayload(f.path, f.size)
			m.DDS.add(count)
		case ".jpg", ".jpeg":
			m.JPG.add(count)
		case ".psd":
			m.PSD.add(count)
		}
	}
	return nil
}

func mipPixels(w, h int64) int64 {
	var pixels int64
	for {
		pixels += ((w + 3) / 4) * ((h + 3) / 4) * 16
		if w <= 1 && h <= 1 {
			return pixels
		}
		w, h = max(1, w/2), max(1, h/2)
	}
}

func ddsPayload(path string, size int64) int64 {
	header := int64(128)
	if f, err := os.Open(path); err == nil {
		var b [128]byte
		if _, err := io.ReadFull(f, b[:]); err == nil && size >= 148 && string(b[84:88]) == "DX10" {
			header = 148
		}
		f.Close()
	}
	return max(0, size-header)
}

func wavPCM16(ctx context.Context, path string, size int64) int64 {
	f, err := os.Open(path)
	if err != nil {
		return size
	}
	defer f.Close()
	var header [12]byte
	if _, err := io.ReadFull(f, header[:]); err != nil || string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		return size
	}
	end := min(size, int64(binary.LittleEndian.Uint32(header[4:8]))+8)
	var dataBytes, sampleBytes int64
	hasData := false
	for offset := int64(12); offset+8 <= end; {
		if ctx.Err() != nil {
			return size
		}
		var chunk [8]byte
		if _, err := f.ReadAt(chunk[:], offset); err != nil {
			return size
		}
		n := int64(binary.LittleEndian.Uint32(chunk[4:]))
		offset += 8
		if n > end-offset {
			return size
		}
		switch string(chunk[:4]) {
		case "fmt ":
			var format [16]byte
			if n < 16 {
				return size
			}
			if _, err := f.ReadAt(format[:], offset); err != nil {
				return size
			}
			// PCM, IEEE float, and extensible WAV have a sample width here.
			tag := binary.LittleEndian.Uint16(format[:2])
			bits := binary.LittleEndian.Uint16(format[14:16])
			if (tag != 1 && tag != 3 && tag != 0xfffe) || bits == 0 || bits%8 != 0 {
				return size
			}
			sampleBytes = int64(bits / 8)
		case "data":
			dataBytes += n
			hasData = true
		}
		offset += n + n%2 // RIFF chunks have even-byte alignment.
	}
	if sampleBytes == 0 || !hasData {
		return size
	}
	return dataBytes * 2 / sampleBytes
}
