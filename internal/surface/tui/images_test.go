package tui

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 06 §12.4: one new image per frame; the others follow without a new frame.
func TestImgWriterOneNewPerFrame(t *testing.T) {
	var buf bytes.Buffer
	w := &imgWriter{Writer: &buf}
	var ps []placement
	for i := 0; i < 3; i++ {
		ps = append(ps, placement{row: 1 + 10*i, col: 1, cols: 4, rows: 2, seq: []byte(fmt.Sprintf("<img%d>", i))})
	}
	w.set(ps)
	w.Write([]byte("F"))
	w.mu.Lock()
	got := buf.String()
	w.mu.Unlock()
	if strings.Count(got, "<img") != 1 || !strings.Contains(got, "<img0>") {
		t.Fatalf("first frame: %q", got)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		w.mu.Lock()
		got, n := buf.String(), len(w.prev)
		w.mu.Unlock()
		if n == 3 {
			if !strings.Contains(got, "<img2>") {
				t.Fatalf("drawn %d without the bytes", n)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of 3 images drawn", n)
		}
		time.Sleep(5 * time.Millisecond)
	}
	// the next frame redraws all three (the frame may have written over them)
	buf.Reset()
	w.Write([]byte("F"))
	if got := buf.String(); strings.Count(got, "<img") != 3 {
		t.Fatalf("redraw: %q", got)
	}
}

// 06 §12.4: the thumbnails of ±10 rows around the cursor are made ahead; the cache keeps
// the 64 most recently used images.
func TestPrefetchAndLRU(t *testing.T) {
	e, _ := fixture(t)
	for i := 1; i <= 30; i++ {
		dir := filepath.Join(e.Game, "Mods", fmt.Sprintf("P%02d", i))
		e.WriteMod(dir, testkit.Sample(fmt.Sprintf("tester.p%02d", i), fmt.Sprintf("P%02d", i)))
		previewPNG(t, filepath.Join(dir, "About", "Preview.png"), 320, 180)
	}
	e.MustRun("scan")
	m := direct(t, e, 160, 40)
	m.images = &imgWriter{Writer: io.Discard}
	press(m, "h")
	press(m, "g")
	for m.current().name != "P15" {
		press(m, "down")
	}
	for i := 0; i < 40; i++ { // what the program does after every message
		runCmd(m, m.imageCmds(m.visibleDetail()))
	}
	c := m.col()
	for d := -10; d <= 10; d++ {
		it := &c.items[c.view[c.cursor+d]]
		en := m.imgs[imgKey(previewURL(it.id), thumbCols, thumbRows)]
		if en == nil || en.state != imgOK {
			t.Fatalf("%s (%+d) not made ahead: %+v", it.name, d, en)
		}
	}
	if m.prefetching != "" || m.imgLoading != "" {
		t.Fatalf("still loading %q %q", m.prefetching, m.imgLoading)
	}
	// LRU: 70 entries, the 6 least recently used go
	m.imgs, m.imgUse, m.lastLines = map[string]*imgEntry{}, map[string]int{}, nil
	for i := 0; i < 70; i++ {
		k := fmt.Sprint("k", i)
		m.imgs[k] = &imgEntry{state: imgOK}
		m.touchImage(k)
	}
	m.touchImage("k0") // used again: stays
	m.evictImages()
	if len(m.imgs) != imgKeep || m.imgs["k0"] == nil || m.imgs["k1"] != nil || m.imgs["k6"] != nil || m.imgs["k7"] == nil {
		t.Fatalf("after eviction: %d entries, k0 %v k1 %v k6 %v k7 %v", len(m.imgs), m.imgs["k0"] != nil, m.imgs["k1"] != nil, m.imgs["k6"] != nil, m.imgs["k7"] != nil)
	}
}

// ui.previews off: no images at all.
func TestPreviewsOff(t *testing.T) {
	e, _ := fixture(t)
	e.MustRun("config", "set", "ui.previews", "off")
	m := direct(t, e, 160, 40)
	m.images = &imgWriter{Writer: io.Discard}
	if m.inlineImages() {
		t.Fatal("images on with ui.previews off")
	}
}
