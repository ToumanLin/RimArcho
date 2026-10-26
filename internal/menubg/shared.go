package menubg

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/env"
)

const sharedVersion = "menu-shared-v1"

// InventoryStamp watches only ownership metadata, never art source files. The
// worker uses it to notice a game rebind, new definitions or installed DLC.
func InventoryStamp(game string) string {
	return fileStamp(filepath.Join(game, "Data")) + "|" + fileStamp(defsPath(game))
}

func defsPath(game string) string {
	return filepath.Join(game, "Data", "Core", "Defs", "Misc", "ExpansionDefs", "ExpansionDefs.xml")
}

// ownedExpansions keeps visibility local to the selected installation. Cached
// art or art in another installation cannot grant ownership of an expansion.
func ownedExpansions(game, lang string) ([]expansion, error) {
	defs, err := expansions(game, lang)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(defs, func(d expansion) bool {
		st, err := os.Stat(filepath.Join(game, "Data", d.Name))
		return d.Name != "Core" && (err != nil || !st.IsDir())
	}), nil
}

func cacheItem(cache string, d expansion) Item {
	id := strings.ToLower(d.Mod)
	hash := sha256.Sum256([]byte(id))
	base := filepath.Join(cache, fmt.Sprintf("%s-%x", sharedVersion, hash[:12]))
	return Item{Key: "game:" + id, Label: d.Label, Full: base + "-1920.jpg", Thumb: base + "-480.jpg"}
}

// Cached reads only small game metadata and shared JPEGs. No ZIP/Unity catalog
// or source is opened, even when one or all pairs need extraction.
func Cached(ctx context.Context, game, cache, lang string) ([]Item, error) {
	defs, err := ownedExpansions(game, lang)
	if err != nil {
		return nil, err
	}
	var out []Item
	for _, d := range defs {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		it := cacheItem(cache, d)
		if validPair(it) {
			out = append(out, it)
		}
	}
	return out, nil
}

// Prepare fills shared pairs from one installation. Use PrepareFrom when other
// registered installations can supply missing art (06 §2.14).
func Prepare(ctx context.Context, game, cache, lang string) ([]Item, error) {
	return PrepareFrom(ctx, game, cache, lang, nil)
}

// PrepareFrom prefers the newest installed game for missing pairs, falling back
// to older sources on failure. Existing pairs are independent of every source
// stamp. Call outside the GUI mutex; this includes filesystem discovery.
func PrepareFrom(ctx context.Context, game, cache, lang string, games []string) ([]Item, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	defs, err := ownedExpansions(game, lang)
	if err != nil {
		return nil, err
	}
	available := map[string]Item{}
	missing := map[string]bool{}
	for _, d := range defs {
		it := cacheItem(cache, d)
		if validPair(it) {
			available[it.Key] = it
		} else {
			missing[it.Key] = true
		}
	}
	// Labels and order always belong to the selected game.
	result := func() []Item {
		var out []Item
		for _, d := range defs {
			it := cacheItem(cache, d)
			if _, ok := available[it.Key]; ok {
				out = append(out, it)
			}
		}
		return out
	}
	if len(missing) == 0 {
		return result(), nil
	}
	if err := os.MkdirAll(cache, 0755); err != nil {
		return result(), err
	}
	var failures []error
	for _, source := range newestGames(append(slices.Clone(games), game)) {
		if err := ctx.Err(); err != nil {
			return result(), err
		}
		sourceDefs, err := ownedExpansions(source, "en")
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", source, err))
			continue
		}
		sourceDefs = slices.DeleteFunc(sourceDefs, func(d expansion) bool { return !missing[cacheItem(cache, d).Key] })
		if len(sourceDefs) == 0 {
			continue
		}
		var items []Item
		if _, statErr := os.Stat(filepath.Join(source, "Assets.zip")); os.IsNotExist(statErr) {
			items, err = prepareUnity(ctx, source, cache, sourceDefs)
		} else {
			items, err = prepareZIP(ctx, source, cache, sourceDefs)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", source, err))
		}
		for _, it := range items {
			delete(missing, it.Key)
			available[it.Key] = it
		}
		if len(missing) == 0 {
			break
		}
	}
	if len(missing) == 0 {
		return result(), nil
	}
	return result(), errors.Join(failures...)
}

func newestGames(games []string) []string {
	type source struct {
		root    string
		version []int
	}
	var sources []source
	seen := map[string]bool{}
	for _, game := range games {
		root, err := filepath.Abs(game)
		if err != nil || game == "" {
			continue
		}
		key := env.PathKey(root)
		if seen[key] {
			continue
		}
		seen[key] = true
		v := env.ParseVersion(env.ReadGameVersion(root))
		sources = append(sources, source{root, []int{v.Major, v.Minor, v.Build, v.Rev}})
	}
	slices.SortStableFunc(sources, func(a, b source) int { return slices.Compare(b.version, a.version) })
	var out []string
	for _, s := range sources {
		out = append(out, s.root)
	}
	return out
}

// Staging prevents a changing source or failed extraction from replacing shared
// art. The decode lock also serializes shared writers across game environments.
func prepareSharedPair(ctx context.Context, game, stamp string, it Item, extract func(Item) error) error {
	decodeMu.Lock()
	defer decodeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if validPair(it) {
		return nil
	}
	defer debug.FreeOSMemory()
	staged := it
	staged.Full, staged.Thumb = it.Full+".pending", it.Thumb+".pending"
	defer os.Remove(staged.Full)
	defer os.Remove(staged.Thumb)
	if err := extract(staged); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if Stamp(game) != stamp {
		return errors.New("menu art source changed during extraction; retry after the game update finishes")
	}
	if err := os.Rename(staged.Full, it.Full); err != nil {
		return err
	}
	return os.Rename(staged.Thumb, it.Thumb)
}

type jpegValidation struct {
	stamp         string
	width, height int
}

var jpegChecks = struct {
	sync.Mutex
	files map[string]jpegValidation
}{files: map[string]jpegValidation{}}

// Decode changed JPEGs completely, rather than accepting nonempty or truncated
// files. Successful validations are memoized by path/size/mtime, so settings
// reads do not keep decoding the same cached images.
func validJPEG(path string, maxWidth int) (int, int, bool) {
	jpegChecks.Lock()
	defer jpegChecks.Unlock()
	stamp := fileStamp(path)
	if v, ok := jpegChecks.files[path]; ok && v.stamp == stamp {
		return v.width, v.height, v.width <= maxWidth
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() < 4 || st.Size() > 32<<20 {
		return 0, 0, false
	}
	cfg, err := jpeg.DecodeConfig(f)
	if err != nil || cfg.Width < 1 || cfg.Width > maxWidth || cfg.Height < 1 || cfg.Height > 16384 || int64(cfg.Width)*int64(cfg.Height) > 40e6 {
		return 0, 0, false
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return 0, 0, false
	}
	if _, err = jpeg.Decode(f); err != nil || fileStamp(path) != stamp {
		return 0, 0, false
	}
	jpegChecks.files[path] = jpegValidation{stamp, cfg.Width, cfg.Height}
	return cfg.Width, cfg.Height, true
}

func validPair(it Item) bool {
	w, h, ok := validJPEG(it.Full, 1920)
	if !ok {
		return false
	}
	tw, th, ok := validJPEG(it.Thumb, 480)
	return ok && tw == min(480, w) && th == max(1, h*tw/w)
}
