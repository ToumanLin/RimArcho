package tui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"sync"

	"github.com/rimarcho/rimarcho/internal/app"
)

// Inline images (06 §6.2 method, §6.4 item 6). bubbletea draws text only; this writer
// wraps the program's output and, after every frame and in the same write, erases image
// rects that are gone (ECH) and draws the current ones with OSC 1337. Images are padded
// to exactly cols×rows cells and sent with preserveAspectRatio=0, so they cover exactly
// their rect; the cursor is shown while an image is written (ConPTY drops the positioning
// otherwise, 07 §6.2).

// cellAspect is cell width / height of the bundled font in WezTerm (07 §6.2).
const cellAspect = 13.0 / 30.0

// placement is one image on screen (1-based cell position).
type placement struct {
	row, col, cols, rows int
	seq                  []byte
}

func (p placement) key() string { return fmt.Sprintf("%d,%d,%d,%d", p.row, p.col, p.cols, p.rows) }

// imgWriter is the program's output in the WezTerm host.
type imgWriter struct {
	io.Writer
	mu   sync.Mutex
	cur  []placement
	prev []placement
}

// set replaces the images to show from the next frame on.
func (w *imgWriter) set(ps []placement) {
	w.mu.Lock()
	w.cur = ps
	w.mu.Unlock()
}

func (w *imgWriter) Write(frame []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var b bytes.Buffer
	b.WriteString("\x1b[?2026h")
	b.Write(frame)
	now := map[string]bool{}
	for _, p := range w.cur {
		now[p.key()] = true
	}
	for _, p := range w.prev {
		if now[p.key()] {
			continue
		}
		b.WriteString("\x1b7")
		for r := 0; r < p.rows; r++ {
			fmt.Fprintf(&b, "\x1b[%d;%dH\x1b[%dX", p.row+r, p.col, p.cols)
		}
		b.WriteString("\x1b8")
	}
	for _, p := range w.cur {
		b.WriteString("\x1b7\x1b[?25h")
		fmt.Fprintf(&b, "\x1b[%d;%dH", p.row, p.col)
		b.Write(p.seq)
		b.WriteString("\x1b[?25l\x1b8")
	}
	b.WriteString("\x1b[?2026l")
	w.prev = append(w.prev[:0], w.cur...)
	if _, err := w.Writer.Write(b.Bytes()); err != nil {
		return 0, err
	}
	return len(frame), nil
}

// imgRows is how many rows an image w×h takes at cols columns (at most maxRows).
func imgRows(w, h, cols, maxRows int) int {
	if w <= 0 || h <= 0 {
		return max(min(int(float64(cols)*cellAspect*9/16+0.5), maxRows), 1)
	}
	return max(min(int(float64(cols)*cellAspect*float64(h)/float64(w)+0.999), maxRows), 1)
}

// imgSeq builds the OSC 1337 sequence for a cached PNG at cols×rows cells: scaled to fit
// and padded with transparent pixels to exactly the cells' pixel aspect.
func imgSeq(path string, cols, rows int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	// The canvas has the cells' aspect and holds the image at its size, centred.
	box := float64(cols) * cellAspect / float64(rows) // width / height of the cells
	cw, ch := b.Dx(), b.Dy()
	if float64(b.Dx())/float64(b.Dy()) > box {
		ch = int(float64(b.Dx())/box + 0.5) // wider than the box: pad above and below
	} else {
		cw = int(float64(b.Dy())*box + 0.5) // taller: pad left and right
	}
	canvas := image.NewRGBA(image.Rect(0, 0, max(cw, 1), max(ch, 1)))
	ox, oy := (cw-b.Dx())/2, (ch-b.Dy())/2
	draw.Draw(canvas, image.Rect(ox, oy, ox+b.Dx(), oy+b.Dy()), src, b.Min, draw.Src)
	var pb bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&pb, canvas); err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("\x1b]1337;File=inline=1;size=%d;width=%d;height=%d;preserveAspectRatio=0;doNotMoveCursor=1:%s\x07",
		pb.Len(), cols, rows, base64.StdEncoding.EncodeToString(pb.Bytes()))), nil
}

// imgEntry is one description image at one width.
type imgEntry struct {
	state      int // imgLoading, imgOK, imgFailed
	w, h, rows int
	seq        []byte
	err        string
}

const (
	imgLoading = iota
	imgOK
	imgFailed
)

type imgLoadedMsg struct {
	key string
	e   imgEntry
}

// loadImage downloads (or reads from the cache) and encodes one image for cols columns.
func loadImage(a *app.App, url string, cols, maxRows int) func() imgLoadedMsg {
	return func() imgLoadedMsg {
		key := imgKey(url, cols)
		path, w, h, err := a.DescImage(url)
		if err != nil {
			return imgLoadedMsg{key, imgEntry{state: imgFailed, err: err.Error()}}
		}
		rows := imgRows(w, h, cols, maxRows)
		seq, err := imgSeq(path, cols, rows)
		if err != nil {
			return imgLoadedMsg{key, imgEntry{state: imgFailed, err: err.Error()}}
		}
		return imgLoadedMsg{key, imgEntry{state: imgOK, w: w, h: h, rows: rows, seq: seq}}
	}
}

func imgKey(url string, cols int) string { return fmt.Sprintf("%d|%s", cols, url) }
