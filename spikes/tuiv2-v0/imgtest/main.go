// Spike: where does WezTerm put an OSC 1337 inline image, and how big? Draws the same
// thumbnail with several parameter variants at fixed cell positions over a grid.
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"
	"strings"
)

func main() {
	src := flag.String("img", "", "an image file")
	alt := flag.Bool("alt", false, "use the alternate screen (as bubbletea does)")
	hide := flag.Bool("hide", false, "hide the cursor (as bubbletea does)")
	show := flag.Bool("show", false, "with -hide: show the cursor around each image (the fix under test)")
	flag.Parse()
	f, err := os.Open(*src)
	if err != nil {
		panic(err)
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		panic(err)
	}
	var pb bytes.Buffer
	png.Encode(&pb, img)
	enc := base64.StdEncoding.EncodeToString(pb.Bytes())
	w := bufio.NewWriter(os.Stdout)
	if *alt {
		w.WriteString("\x1b[?1049h")
	}
	if *hide {
		w.WriteString("\x1b[?25l")
	}
	w.WriteString("\x1b[2J\x1b[H")
	for r := 1; r <= 44; r++ {
		fmt.Fprintf(w, "\x1b[%d;1H%02d %s", r, r, strings.Repeat("....:....|", 15))
	}
	variants := []struct {
		row, col int
		params   string
	}{
		{3, 10, "width=18;height=9;preserveAspectRatio=1"},
		{3, 40, "width=18;height=9;preserveAspectRatio=1;doNotMoveCursor=1"},
		{3, 70, "width=18;height=9;preserveAspectRatio=0"},
		{15, 10, "width=18"},
		{15, 40, "height=9"},
		{15, 70, "width=180px;height=90px"},
		{27, 10, ""},
	}
	for _, v := range variants {
		if *show {
			w.WriteString("\x1b[?25h")
		}
		fmt.Fprintf(w, "\x1b[%d;%dH\x1b]1337;File=inline=1;size=%d;%s:%s\x07", v.row, v.col, pb.Len(), v.params, enc)
		if *show {
			w.WriteString("\x1b[?25l")
		}
		fmt.Fprintf(w, "\x1b[%d;%dH<%s>", v.row-1, v.col, v.params)
	}
	fmt.Fprintf(w, "\x1b[44;1Hdone")
	w.Flush()
	bufio.NewReader(os.Stdin).ReadString('\n')
}
