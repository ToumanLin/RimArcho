package menubg

import (
	"archive/zip"
	"context"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

func game17Paths(t *testing.T, prefix string) (string, string) {
	t.Helper()
	game := os.Getenv("RIMARCHO_TEST_GAME17")
	if game == "" {
		t.Skip("set RIMARCHO_TEST_GAME17 to a read-only RimWorld 1.7 installation")
	}
	out := os.Getenv("RIMARCHO_TEST_GAME17_OUTPUT")
	if out == "" {
		out = filepath.Join("..", "..", "tmp", "menu-backgrounds-17")
	}
	out, err := filepath.Abs(out)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	cache, err := os.MkdirTemp(out, prefix)
	if err != nil {
		t.Fatal(err)
	}
	return game, cache
}

func TestGame17Split(t *testing.T) {
	game, cache := game17Paths(t, "all-")
	items, err := Prepare(context.Background(), game, cache, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 6 {
		t.Fatalf("want six backgrounds, got %d", len(items))
	}
	for _, it := range items {
		for i, path := range []string{it.Full, it.Thumb} {
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			img, err := jpeg.Decode(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			width := []int{1920, 480}[i]
			height := width * 9 / 16
			if it.Key == "game:ludeon.rimworld" {
				height = width * 5 / 8
			}
			if img.Bounds().Dx() != width || img.Bounds().Dy() != height {
				t.Fatalf("%s: %v", it.Key, img.Bounds())
			}
			t.Logf("%s: %v %s", it.Key, img.Bounds(), path)
		}
	}
}

// Run in a fresh compiled test process to separate Core's process peak from the
// other five images and from the Go build driver. Outputs remain under tmp/.
func TestGame17CoreMemory(t *testing.T) {
	game, cache := game17Paths(t, "core-")
	z, err := zip.OpenReader(filepath.Join(game, "Assets.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	z.RegisterDecompressor(93, func(r io.Reader) io.ReadCloser {
		d, err := zstd.NewReader(r, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(64<<20))
		if err != nil {
			return failedReader{err}
		}
		return d.IOReadCloser()
	})
	var core *zip.File
	for _, f := range z.File {
		if strings.EqualFold(f.Name, "Resources/Textures/UI/HeroArt/BGPlanet.png.dds.split") {
			if core != nil {
				t.Fatal("ambiguous Core entry")
			}
			core = f
		}
	}
	if core == nil {
		t.Fatal("split Core entry absent")
	}
	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)
	done, peakCh := make(chan struct{}), make(chan uint64, 1)
	go func() {
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		peak := baseline.HeapAlloc
		for {
			select {
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				peak = max(peak, m.HeapAlloc)
			case <-done:
				peakCh <- peak
				return
			}
		}
	}()
	it := Item{Full: filepath.Join(cache, "core-1920.jpg"), Thumb: filepath.Join(cache, "core-480.jpg")}
	start := time.Now()
	err = prepareSharedPair(context.Background(), game, Stamp(game), it, func(staged Item) error {
		return extractPair(context.Background(), core, staged)
	})
	elapsed := time.Since(start)
	close(done)
	peak := <-peakCh
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Core 8000x5000: elapsed=%s sampled Go peak heap=%.2f MiB baseline=%.2f MiB; full=%s thumb=%s", elapsed, float64(peak)/(1<<20), float64(baseline.HeapAlloc)/(1<<20), it.Full, it.Thumb)
}
