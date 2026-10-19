// Spike: which cells does WezTerm actually cover with an OSC 1337 image drawn into an
// 18×9 rect, and does clearing the rect erase all of it? Draws solid magenta images of
// several aspect ratios at the rect, one at a time, and clears after each; a driver
// script screenshots every phase and measures the magenta bounding box.
//
// Handshake: the program appends "PHASE <name>" to -log, then waits until the file
// -go exists, deletes it and continues.
//
//	ghost.exe -log phases.log -go next.flag [-margin 0|1]
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"time"
)

const rectRow, rectCol, rectW, rectH = 3, 56, 18, 9

func solid(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	m := color.RGBA{255, 0, 255, 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, m)
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

// cellAspect is cell width / cell height for the bundled font in WezTerm (13 x 30 px at
// 11 pt on a 150% display; the ratio comes from the font, so it holds at other DPIs to
// within a pixel of rounding).
const cellAspect = 13.0 / 30.0

// fitCells pads a w x h magenta image with transparent pixels to exactly the pixel aspect
// of rectW x rows cells, where rows is the fewest rows (<= rectH) that hold the image at
// full rect width (or full height, for tall images).
func fitCells(w, h int) ([]byte, int) {
	boxW := float64(rectW) * cellAspect // in units of cell height
	rows := int(boxW*float64(h)/float64(w) + 0.999)
	if rows > rectH {
		rows = rectH
	}
	if rows < 1 {
		rows = 1
	}
	// canvas with the aspect of rectW x rows cells, image centred
	cw := w
	ch := int(float64(w) * float64(rows) / boxW)
	if ch < h {
		ch = h
		cw = int(float64(h) * boxW / float64(rows))
	}
	img := image.NewRGBA(image.Rect(0, 0, cw, ch))
	m := color.RGBA{255, 0, 255, 255}
	ox, oy := (cw-w)/2, (ch-h)/2
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(ox+x, oy+y, m)
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes(), rows
}

func main() {
	logf := flag.String("log", "", "phase log")
	gof := flag.String("go", "", "go flag file")
	margin := flag.Int("margin", 0, "extra rows/cols cleared around the rect")
	params := flag.String("params", "preserveAspectRatio=1", "extra image params")
	fit := flag.Bool("fit", false, "pad each image to an exact 18xN-cell aspect and draw it with preserveAspectRatio=0 (the fix)")
	flag.Parse()
	out := bufio.NewWriter(os.Stdout)
	lf, _ := os.OpenFile(*logf, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	phase := func(name string) {
		out.Flush()
		fmt.Fprintf(lf, "PHASE %s\n", name)
		for {
			if _, err := os.Stat(*gof); err == nil {
				os.Remove(*gof)
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	out.WriteString("\x1b[?1049h\x1b[?25l\x1b[2J")
	for r := 1; r <= 20; r++ {
		fmt.Fprintf(out, "\x1b[%d;1H%02d", r, r)
	}
	fmt.Fprintf(out, "\x1b[1;56Hrect %d;%d %dx%d margin %d", rectRow, rectCol, rectW, rectH, *margin)
	drawn := rectH // rows covered by the last draw (what the clear must erase)
	clear := func() {
		out.WriteString("\x1b7")
		for r := 0; r < drawn+*margin; r++ {
			fmt.Fprintf(out, "\x1b[%d;%dH\x1b[%dX", rectRow+r, rectCol, rectW+*margin)
		}
		out.WriteString("\x1b8")
	}
	sizes := []struct {
		name string
		w, h int
	}{
		{"tall-1x3", 200, 600}, {"portrait-2x3", 400, 600}, {"square", 512, 512},
		{"4x3", 640, 480}, {"16x9", 1280, 720}, {"wide-3x1", 900, 300},
		{"near-cell-aspect-234x270", 234, 270}, {"odd-1061x683", 1061, 683},
	}
	for _, s := range sizes {
		data, p := solid(s.w, s.h), *params
		drawn = rectH
		if *fit {
			data, drawn = fitCells(s.w, s.h)
			p = "preserveAspectRatio=0"
		}
		enc := base64.StdEncoding.EncodeToString(data)
		fmt.Fprintf(out, "\x1b7\x1b[?25h\x1b[%d;%dH\x1b]1337;File=inline=1;size=%d;width=%d;height=%d;%s;doNotMoveCursor=1:%s\x07\x1b[?25l\x1b8",
			rectRow, rectCol, len(data), rectW, drawn, p, enc)
		fmt.Fprintf(out, "\x1b[40;1H\x1b[2K%s rows=%d", s.name, drawn)
		phase("draw " + s.name)
		clear()
		phase("clear " + s.name)
	}
	out.WriteString("\x1b[?25h\x1b[?1049l")
	out.Flush()
	_ = strings.TrimSpace
}
