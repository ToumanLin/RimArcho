// Spike (TUI v2 V0): a visual width test. Each glyph is printed between bars, three
// times in a row, and every row ends with a bar at a fixed column; a glyph wider than
// one cell pushes or overlaps its bars. Run it inside WezTerm and Windows Terminal and
// screenshot the window.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
)

type g struct {
	name string
	r    rune
}

var nerd = []g{{"shield", 0xF132}, {"steam", 0xF1B6}, {"folder", 0xF07B}, {"clone", 0xF24D}, {"git", 0xF1D3},
	{"times-circle", 0xF057}, {"exclam-tri", 0xF071}, {"bug", 0xF188}, {"ban", 0xF05E}, {"circle", 0xF111},
	{"lock", 0xF023}, {"refresh", 0xF021}, {"link", 0xF0C1}, {"inbox", 0xF01C}, {"play", 0xF04B},
	{"floppy", 0xF0C7}, {"sort-asc", 0xF160}, {"search", 0xF002}}

var old = []g{{"◆ official", 0x25C6}, {"◎ workshop", 0x25CE}, {"▢ local", 0x25A2}, {"◇ copy", 0x25C7},
	{"± git", 0xB1}, {"✕ error", 0x2715}, {"※ log", 0x203B}, {"⊘ unload", 0x2298}, {"● unsaved", 0x25CF},
	{"↻ update", 0x21BB}, {"→ deps", 0x2192}, {"▤ staging", 0x25A4}, {"▶ play", 0x25B6}, {"↓ save", 0x2193},
	{"⇅ sort", 0x21C5}}

var proposed = []g{{"⬥ official", 0x2B25}, {"◉ workshop", 0x25C9}, {"▢ local", 0x25A2}, {"⬦ copy", 0x2B26},
	{"∆ git", 0x2206}, {"⨯ error", 0x2A2F}, {"! warn", '!'}, {"◍ log", 0x25CD}, {"◌ unload", 0x25CC},
	{"⬤ unsaved", 0x2B24}, {"⇣ update", 0x21E3}, {"› deps", 0x203A}, {"◫ staging", 0x25EB}, {"► play", 0x25BA},
	{"↲ save", 0x21B2}, {"↨ sort", 0x21A8}, {"▸ select", 0x25B8}, {"✓ ok", 0x2713}, {"❯ prompt", 0x276F},
	{"… busy", 0x2026}, {"· dot", 0xB7}}

func section(w *bufio.Writer, title string, gs []g) {
	fmt.Fprintf(w, "\x1b[1m%s\x1b[0m\n", title)
	for i := 0; i < len(gs); i += 3 {
		var line strings.Builder
		for j := i; j < i+3 && j < len(gs); j++ {
			x := gs[j]
			// "|g|ggg|" then pad to a fixed width with a final bar: 7 cells of glyph area.
			name := fmt.Sprintf("%-13s", fmt.Sprintf("%04X %s", x.r, x.name))
			if len([]rune(name)) > 13 {
				name = string([]rune(name)[:13])
			}
			line.WriteString(fmt.Sprintf("%s|%c|%c%c%c|  ", name, x.r, x.r, x.r, x.r))
		}
		fmt.Fprintln(w, line.String())
	}
	fmt.Fprintln(w, strings.Repeat("....:....|", 12))
	fmt.Fprintln(w)
}

func main() {
	hold := flag.Bool("hold", true, "wait for Enter before exiting")
	flag.Parse()
	w := bufio.NewWriter(os.Stdout)
	fmt.Fprintf(w, "TERM_PROGRAM=%s  (each row: name |g|ggg| — bars must line up in one column)\n\n", os.Getenv("TERM_PROGRAM"))
	section(w, "Nerd Font (Font Awesome 4 range)", nerd)
	section(w, "06 §3 compat column as specified", old)
	section(w, "Proposed compat column", proposed)
	fmt.Fprintln(w, "CJK check: |中文|Ab|  ⇒ the bar after 中文 must be 4 cells after the first bar")
	w.Flush()
	if *hold {
		bufio.NewReader(os.Stdin).ReadString('\n')
	}
}
