package menubg

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sharedGame(t *testing.T, version string, core, royalty byte) string {
	t.Helper()
	game := singleSplitGame(t)
	put(t, filepath.Join(game, "Version.txt"), []byte(version))
	put(t, filepath.Join(game, "Data/Royalty/About/About.xml"), []byte("<ModMetaData/>"))
	writeSplitZIP(t, game, 93, map[string][]byte{
		coreSplitName: splitFixture(t, 4, 4, 1, []byte{core}),
		"Resources/Textures/UI/MenuRoyalty.jpg.dds.split": splitFixture(t, 4, 4, 1, []byte{royalty}),
	}, false)
	return game
}

func TestSharedCacheByPackageAcrossZIPAndUnityInstalls(t *testing.T) {
	source, cache := sharedGame(t, "1.7.4990 rev1069", 9, 10), t.TempDir()
	first, err := Prepare(context.Background(), source, cache, "en")
	if err != nil || len(first) != 2 {
		t.Fatalf("initial: %v %v", first, err)
	}
	old := time.Unix(100, 0)
	for _, it := range first {
		for _, path := range []string{it.Full, it.Thumb} {
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, zipSource := range []bool{false, true} {
		current := singleSplitGame(t)
		put(t, filepath.Join(current, "Data/Royalty/About/About.xml"), []byte("<ModMetaData/>"))
		// A different resource path and unreadable asset sources must be irrelevant
		// on the warm path: no texture catalog can resolve these definitions.
		defs := defsPath(current)
		data, err := os.ReadFile(defs)
		if err != nil {
			t.Fatal(err)
		}
		put(t, defs, bytes.ReplaceAll(data, []byte("UI/Menu"), []byte("Unresolvable/")))
		if zipSource {
			put(t, filepath.Join(current, "Assets.zip"), []byte("not a zip"))
		} else {
			put(t, filepath.Join(current, "RimWorldWin64_Data/resources.assets"), []byte("not Unity"))
		}
		items, err := PrepareFrom(context.Background(), current, cache, "zh", []string{source})
		if err != nil || len(items) != 2 {
			t.Fatalf("warm: %v %v", items, err)
		}
		for i, it := range items {
			if it.Full != first[i].Full || it.Thumb != first[i].Thumb || it.Label != []string{"Core", "Royalty"}[i] {
				t.Fatal("not shared/localized", items)
			}
			st, err := os.Stat(it.Full)
			if err != nil || !st.ModTime().Equal(old) {
				t.Fatal("rewrote shared art", err)
			}
		}
		// Ownership remains local even though another install has DLC art cached.
		if err := os.RemoveAll(filepath.Join(current, "Data/Royalty")); err != nil {
			t.Fatal(err)
		}
		items, err = Prepare(context.Background(), current, cache, "en")
		if err != nil || len(items) != 1 || items[0].Key != "game:ludeon.rimworld" {
			t.Fatalf("unowned DLC leaked: %v %v", items, err)
		}
	}
	files, _ := os.ReadDir(cache)
	if len(files) != 4 {
		t.Fatalf("want exactly one pair per package, got %v", files)
	}
}

func TestSharedCacheNewestProviderAndOlderFallback(t *testing.T) {
	old := sharedGame(t, "1.9.100 rev9", 6, 5)
	newest := sharedGame(t, "1.10.100 rev1", 9, 10)
	current := singleSplitGame(t)
	put(t, filepath.Join(current, "Data/Royalty/About/About.xml"), []byte("<ModMetaData/>"))
	// Prefer numeric newest, independent of caller order or selected game. A
	// corrupt/missing Royalty candidate in the newest game falls back to older.
	writeSplitZIP(t, newest, 93, map[string][]byte{coreSplitName: splitFixture(t, 4, 4, 1, []byte{9})}, false)
	cache := t.TempDir()
	items, err := PrepareFrom(context.Background(), current, cache, "en", []string{old, newest, old})
	if err != nil || len(items) != 2 {
		t.Fatalf("providers: %v %v", items, err)
	}
	reference, err := Prepare(context.Background(), old, t.TempDir(), "en")
	if err != nil {
		t.Fatal(err)
	}
	coreRef, err := Prepare(context.Background(), newest, t.TempDir(), "en")
	if err != nil || len(coreRef) != 1 {
		t.Fatalf("newest reference: %v %v", coreRef, err)
	}
	for i, want := range []Item{coreRef[0], reference[1]} {
		gotBytes, err := os.ReadFile(items[i].Full)
		if err != nil {
			t.Fatal(err)
		}
		wantBytes, err := os.ReadFile(want.Full)
		if err != nil || !bytes.Equal(gotBytes, wantBytes) {
			t.Fatalf("provider %d incorrect: %v", i, err)
		}
	}
	// No cache for a DLC the current game doesn't own, even when another provider
	// has it; adding that DLC later extracts only its missing pair.
	cache = t.TempDir()
	if err := os.RemoveAll(filepath.Join(current, "Data/Royalty")); err != nil {
		t.Fatal(err)
	}
	items, err = PrepareFrom(context.Background(), current, cache, "en", []string{old})
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	files, _ := os.ReadDir(cache)
	if len(files) != 2 {
		t.Fatal("extracted unowned DLC", files)
	}
	put(t, filepath.Join(current, "Data/Royalty/About/About.xml"), []byte("<ModMetaData/>"))
	if err := os.Chtimes(items[0].Full, time.Unix(100, 0), time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	items, err = PrepareFrom(context.Background(), current, cache, "en", []string{old})
	if err != nil || len(items) != 2 {
		t.Fatal(items, err)
	}
	st, _ := os.Stat(items[0].Full)
	if !st.ModTime().Equal(time.Unix(100, 0)) {
		t.Fatal("adding DLC rewrote Core")
	}
}

func TestSharedCacheRepairsInvalidPairsAndManualRefresh(t *testing.T) {
	game, cache := sharedGame(t, "1.7", 9, 10), t.TempDir()
	items, err := Prepare(context.Background(), game, cache, "en")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := os.ReadFile(items[0].Full)
	if err != nil {
		t.Fatal(err)
	}
	// An old cache and unrelated files are ignored, never deleted or adopted.
	legacy := filepath.Join(cache, "menu-0123456789-1920.jpg")
	outside := filepath.Join(filepath.Dir(cache), "user-image.jpg")
	put(t, legacy, []byte("old"))
	put(t, outside, []byte("user"))
	for _, broken := range []string{"empty", "header-only", "wrong-dimensions", "missing"} {
		switch broken {
		case "empty":
			put(t, items[0].Full, nil)
		case "header-only":
			put(t, items[0].Full, initial[:len(initial)/2])
		case "wrong-dimensions":
			if err := writeJPEG(image.NewRGBA(image.Rect(0, 0, 7, 3)), items[0].Thumb); err != nil {
				t.Fatal(err)
			}
		case "missing":
			if err := os.Remove(items[0].Thumb); err != nil {
				t.Fatal(err)
			}
		}
		got, err := Prepare(context.Background(), game, cache, "en")
		if err != nil || len(got) != 2 || !validPair(got[0]) {
			t.Fatalf("repair %s: %v %v", broken, got, err)
		}
	}
	// Manual refresh: deleting this pair re-extracts it using the same stable key.
	writeSplitZIP(t, game, 93, map[string][]byte{coreSplitName: splitFixture(t, 4, 4, 1, []byte{6})}, false)
	for _, path := range []string{items[0].Full, items[0].Thumb} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Prepare(context.Background(), game, cache, "en")
	if err != nil || len(got) != 2 || got[0].Full != items[0].Full {
		t.Fatal("refresh", got, err)
	}
	changed, _ := os.ReadFile(got[0].Full)
	if bytes.Equal(changed, initial) {
		t.Fatal("manual refresh did not replace pixels")
	}
	for path, want := range map[string]string{legacy: "old", outside: "user"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("modified user/legacy file %s: %v", path, err)
		}
	}
}

func TestSharedCacheStagingRejectsChangedSourceAndCancellation(t *testing.T) {
	for _, cancelRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "source-changed", true: "canceled"}[cancelRun], func(t *testing.T) {
			game, cache := sharedGame(t, "1.7", 9, 10), t.TempDir()
			items, err := Prepare(context.Background(), game, cache, "en")
			if err != nil {
				t.Fatal(err)
			}
			original, _ := os.ReadFile(items[0].Full)
			if err := os.Remove(items[0].Thumb); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err = prepareSharedPair(ctx, game, Stamp(game), items[0], func(staged Item) error {
				img := image.NewUniform(color.RGBA{255, 0, 0, 255})
				small := image.NewRGBA(image.Rect(0, 0, 4, 4))
				for y := 0; y < 4; y++ {
					for x := 0; x < 4; x++ {
						small.Set(x, y, img.At(x, y))
					}
				}
				for _, path := range []string{staged.Full, staged.Thumb} {
					if err := writeJPEG(small, path); err != nil {
						return err
					}
				}
				if cancelRun {
					cancel()
				} else {
					put(t, filepath.Join(game, "Assets.zip"), []byte("updated source"))
				}
				return nil
			})
			if err == nil || (cancelRun && !errors.Is(err, context.Canceled)) {
				t.Fatal("published changed source", err)
			}
			data, _ := os.ReadFile(items[0].Full)
			if !bytes.Equal(data, original) {
				t.Fatal("overwrote existing member on failure")
			}
			files, _ := os.ReadDir(cache)
			for _, f := range files {
				if strings.HasSuffix(f.Name(), ".pending") || strings.HasSuffix(f.Name(), ".tmp") {
					t.Fatal("left staging file", f.Name())
				}
			}
		})
	}
}

func TestSharedJPEGValidationRejectsTruncatedScan(t *testing.T) {
	var out bytes.Buffer
	if err := jpeg.Encode(&out, image.NewRGBA(image.Rect(0, 0, 16, 16)), nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "broken.jpg")
	data := out.Bytes()
	// Preserve a readable header but truncate the compressed pixel scan.
	put(t, path, data[:len(data)-8])
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = jpeg.DecodeConfig(f)
	f.Close()
	if err != nil {
		t.Fatal("fixture must have a valid header", err)
	}
	if _, _, ok := validJPEG(path, 1920); ok {
		t.Fatal("accepted truncated JPEG scan")
	}
}
