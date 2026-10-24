package menubg

import (
	"context"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// PNGs are review artifacts only. Normal tests neither need nor redistribute game data.
func TestGame16(t *testing.T) {
	game := os.Getenv("RIMARCHO_TEST_GAME16")
	if game == "" {
		t.Skip("set RIMARCHO_TEST_GAME16 to a read-only RimWorld 1.6 installation")
	}
	out := os.Getenv("RIMARCHO_TEST_GAME16_OUTPUT")
	if out == "" {
		out = filepath.Join("..", "..", "tmp", "menu-backgrounds-16")
	}
	out, err := filepath.Abs(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	done, measured := make(chan struct{}), make(chan uint64, 1)
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		peak := before.HeapAlloc
		for {
			select {
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				peak = max(peak, m.HeapAlloc)
			case <-done:
				measured <- peak
				return
			}
		}
	}()
	start := time.Now()
	items, err := Prepare(context.Background(), game, cache, "en")
	elapsed := time.Since(start)
	close(done)
	peak := <-measured
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 6 {
		t.Fatalf("want all 6 backgrounds, got %+v", items)
	}
	keys := map[string]bool{}
	for _, it := range items {
		keys[it.Key] = true
		var full image.Image
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
			w := []int{1920, 480}[i]
			if img.Bounds().Dx() != w || img.Bounds().Dy() < 200 {
				t.Fatalf("bad image dimensions %s: %v", path, img.Bounds())
			}
			// A uniform/empty decode can have correct dimensions, too.
			first := img.At(0, 0)
			different := false
			for y := 0; y < img.Bounds().Dy(); y += 19 {
				for x := 0; x < w; x += 19 {
					if img.At(x, y) != first {
						different = true
					}
				}
			}
			if !different {
				t.Fatalf("empty image %s", path)
			}
			if i == 0 {
				full = img
			}
		}
		path := filepath.Join(out, strings.TrimPrefix(it.Key, "game:")+".png")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, full)
		closeErr := f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		t.Logf("%s: %v → %s", it.Key, full.Bounds(), path)
	}
	for _, pkg := range []string{"ludeon.rimworld", "ludeon.rimworld.royalty", "ludeon.rimworld.ideology", "ludeon.rimworld.biotech", "ludeon.rimworld.anomaly", "ludeon.rimworld.odyssey"} {
		if !keys["game:"+pkg] {
			t.Fatalf("missing %s", pkg)
		}
	}
	start = time.Now()
	again, err := Prepare(context.Background(), game, cache, "en")
	if err != nil || len(again) != len(items) {
		t.Fatalf("cache hit: %v %+v", err, again)
	}
	t.Logf("first extraction=%s; cached=%s; sampled Go peak heap=%.2f MiB (baseline %.2f MiB); PNGs=%s", elapsed, time.Since(start), float64(peak)/(1<<20), float64(before.HeapAlloc)/(1<<20), out)
}

// Run once in a fresh test process to measure first extraction without PNG review
// encoding or build-tool memory: -run ^$ -bench ^BenchmarkGame16Prepare$ -benchtime=1x.
func BenchmarkGame16Prepare(b *testing.B) {
	game := os.Getenv("RIMARCHO_TEST_GAME16")
	if game == "" {
		b.Skip("set RIMARCHO_TEST_GAME16")
	}
	b.ReportAllocs()
	for b.Loop() {
		items, err := Prepare(context.Background(), game, b.TempDir(), "en")
		if err != nil || len(items) != 6 {
			b.Fatalf("all backgrounds: %+v %v", items, err)
		}
	}
}
