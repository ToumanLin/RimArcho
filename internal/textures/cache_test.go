package textures

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestHeaderCacheTracksFiles(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "Textures", "a.png")
	b := filepath.Join(root, "Textures", "b.png")
	cache := filepath.Join(t.TempDir(), "headers.json")
	writePNG(t, a, 32, 64, false)
	writePNG(t, b, 64, 32, true)
	reads := 0
	header := func(p string) (PNGHeader, bool) { reads++; return ReadPNGHeader(p) }
	read := func(wantReads int) *Set {
		t.Helper()
		reads = 0
		got, err := collectCached(context.Background(), root, []string{root}, cache, header)
		if err != nil || reads != wantReads {
			t.Fatalf("reads %d want %d, err %v", reads, wantReads, err)
		}
		plain, err := Collect(root, []string{root})
		if err != nil || !reflect.DeepEqual(got, plain) {
			t.Fatalf("cached collection differs from direct read: %v", err)
		}
		return got
	}
	read(2)
	read(0) // an unchanged file is not opened
	info, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	writePNG(t, a, 128, 64, true)
	// Even an update with the same timestamp is noticed when its size changes.
	if err := os.Chtimes(a, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	read(1)
	// A timestamp-only change invalidates the header too.
	if err := os.Chtimes(a, time.Now(), info.ModTime().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	read(1)
	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	}
	c := filepath.Join(root, "Textures", "c.png")
	writeFile(t, c, "invalid PNG")
	if s := read(1); len(s.Sources) != 1 || len(s.Others) != 1 {
		t.Fatalf("set: %+v", s)
	}
	read(0) // invalid PNGs are cached too
	writePNG(t, c, 16, 16, false)
	read(1)
	writeFile(t, cache, "{damaged")
	read(2)
	read(0)
}

func TestHeaderCacheCancellationDoesNotPublishPartialResults(t *testing.T) {
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "Textures", "a.png"), 32, 32, false)
	writePNG(t, filepath.Join(root, "Textures", "b.png"), 32, 32, false)
	cache := filepath.Join(t.TempDir(), "headers.json")
	ctx, cancel := context.WithCancel(context.Background())
	_, err := collectCached(ctx, root, []string{root}, cache, func(p string) (PNGHeader, bool) {
		cancel()
		return ReadPNGHeader(p)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error: %v", err)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("partial cache written: %v", err)
	}
	if _, err := CollectCached(ctx, root, nil, cache); !errors.Is(err, context.Canceled) {
		t.Fatalf("empty canceled read: %v", err)
	}
}
