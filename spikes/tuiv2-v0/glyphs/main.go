// Spike (TUI v2 V0, 06 §3): are the icon glyphs and the compat-mode fallbacks one cell?
//
// For each code point it reports the Unicode East Asian Width class, go-runewidth with
// EastAsianWidth off (RimArcho's setting) and on, and, for each font, whether the font has
// the glyph and its advance width relative to the font's ASCII advance ("1.00" = exactly
// one cell; ≥ 1.5 = draws over the next cell). Fonts are read from their cmap and hmtx
// tables; nothing is installed.
//
//	go run ./spikes/tuiv2-v0/glyphs -fonts spikes/wezterm-v0/fonts/MapleMono-NF-CN-Regular.ttf,C:/Windows/Fonts/CascadiaMono.ttf
package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-runewidth"
	"golang.org/x/text/width"
)

type font struct {
	name  string
	cmap  map[rune]uint16
	adv   []uint16
	ascii float64
}

func (f *font) advance(r rune) (float64, bool) {
	g, ok := f.cmap[r]
	if !ok || g == 0 {
		return 0, false
	}
	a := f.adv[len(f.adv)-1]
	if int(g) < len(f.adv) {
		a = f.adv[g]
	}
	return float64(a) / f.ascii, true
}

func load(path string) (*font, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) < 12 {
		return nil, errors.New("short file")
	}
	if string(b[:4]) == "ttcf" { // a collection: use the first font
		off := binary.BigEndian.Uint32(b[12:])
		b = b[off:]
		_ = b
		bb, _ := os.ReadFile(path)
		return parse(path, bb, int(off))
	}
	return parse(path, b, 0)
}

func parse(path string, b []byte, base int) (*font, error) {
	u16 := func(o int) uint16 { return binary.BigEndian.Uint16(b[o:]) }
	u32 := func(o int) uint32 { return binary.BigEndian.Uint32(b[o:]) }
	n := int(u16(base + 4))
	tables := map[string]int{}
	for i := 0; i < n; i++ {
		rec := base + 12 + 16*i
		tables[string(b[rec:rec+4])] = int(u32(rec + 8))
	}
	f := &font{name: filepath.Base(path), cmap: map[rune]uint16{}}
	// cmap: prefer format 12 (full Unicode), else format 4.
	c := tables["cmap"]
	nsub := int(u16(c + 2))
	var f4, f12 int = -1, -1
	for i := 0; i < nsub; i++ {
		rec := c + 4 + 8*i
		off := c + int(u32(rec+4))
		switch u16(off) {
		case 12:
			f12 = off
		case 4:
			if f4 < 0 {
				f4 = off
			}
		}
	}
	switch {
	case f12 >= 0:
		groups := int(u32(f12 + 12))
		for i := 0; i < groups; i++ {
			g := f12 + 16 + 12*i
			start, end, gid := u32(g), u32(g+4), u32(g+8)
			for r := start; r <= end && r-start < 70000; r++ {
				f.cmap[rune(r)] = uint16(gid + (r - start))
			}
		}
	case f4 >= 0:
		segX2 := int(u16(f4 + 6))
		ends, starts := f4+14, f4+16+segX2
		deltas, ranges := starts+segX2, starts+2*segX2
		for s := 0; s < segX2/2; s++ {
			end, start := int(u16(ends+2*s)), int(u16(starts+2*s))
			delta, ro := int16(u16(deltas+2*s)), int(u16(ranges+2*s))
			for r := start; r <= end && r != 0xFFFF; r++ {
				var gid int
				if ro == 0 {
					gid = (r + int(delta)) & 0xFFFF
				} else {
					p := ranges + 2*s + ro + 2*(r-start)
					gid = int(u16(p))
					if gid != 0 {
						gid = (gid + int(delta)) & 0xFFFF
					}
				}
				f.cmap[rune(r)] = uint16(gid)
			}
		}
	default:
		return nil, errors.New("no usable cmap")
	}
	hm := int(u16(tables["hhea"] + 34))
	h := tables["hmtx"]
	for i := 0; i < hm; i++ {
		f.adv = append(f.adv, u16(h+4*i))
	}
	a, _ := f.advance('M')
	f.ascii = 1
	if g, ok := f.cmap['M']; ok && int(g) < len(f.adv) {
		f.ascii = float64(f.adv[g])
	}
	_ = a
	return f, nil
}

type glyph struct {
	label string
	r     rune
}

func main() {
	fonts := flag.String("fonts", "", "comma-separated font files")
	set := flag.String("set", "all", "nerd, compat, candidates or all")
	scan := flag.Bool("scan", false, "list every symbol (U+2000–U+2BFF) that is Neutral width, 1 cell even with EastAsianWidth on, and exactly 1 cell in the first font")
	flag.Parse()
	var fs []*font
	for _, p := range strings.Split(*fonts, ",") {
		f, err := load(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, p, err)
			os.Exit(1)
		}
		fs = append(fs, f)
	}
	var gs []glyph
	if *scan {
		for r := rune(0x2000); r <= 0x2BFF; r++ {
			k := width.LookupRune(r).Kind()
			wide := runewidth.Condition{EastAsianWidth: true}
			if k != width.Neutral || wide.RuneWidth(r) != 1 {
				continue
			}
			if a, ok := fs[0].advance(r); ok && a > 0.99 && a < 1.01 {
				gs = append(gs, glyph{fmt.Sprintf("scan %c", r), r})
			}
		}
		*set = ""
	}
	if *set == "all" || *set == "nerd" {
		gs = append(gs, nerd...)
	}
	if *set == "all" || *set == "compat" {
		gs = append(gs, compat...)
	}
	if *set == "proposed" {
		gs = append(gs, proposed...)
	}
	if *set == "all" || *set == "candidates" {
		gs = append(gs, candidates...)
	}
	fmt.Printf("%-28s %-7s %-5s %-4s %-4s", "glyph", "U+", "EAW", "rw", "rwEA")
	for _, f := range fs {
		fmt.Printf("  %-22s", f.name)
	}
	fmt.Println()
	for _, g := range gs {
		eaw := map[width.Kind]string{width.EastAsianAmbiguous: "A", width.EastAsianWide: "W", width.EastAsianFullwidth: "F",
			width.EastAsianNarrow: "Na", width.EastAsianHalfwidth: "H", width.Neutral: "N"}[width.LookupRune(g.r).Kind()]
		narrow := runewidth.Condition{EastAsianWidth: false}
		wide := runewidth.Condition{EastAsianWidth: true}
		fmt.Printf("%-28s %-7X %-5s %-4d %-4d", g.label, g.r, eaw, narrow.RuneWidth(g.r), wide.RuneWidth(g.r))
		for _, f := range fs {
			if a, ok := f.advance(g.r); ok {
				fmt.Printf("  %-22s", fmt.Sprintf("%c  adv %.2f", g.r, a))
			} else {
				fmt.Printf("  %-22s", "missing")
			}
		}
		fmt.Println()
	}
}

var nerd = []glyph{
	{"nf shield (official)", 0xF132}, {"nf steam (workshop)", 0xF1B6}, {"nf folder (local)", 0xF07B},
	{"nf clone (local copy)", 0xF24D}, {"nf git", 0xF1D3}, {"nf times-circle (error)", 0xF057},
	{"nf exclamation-tri (warn)", 0xF071}, {"nf bug (log error)", 0xF188}, {"nf ban (unloadable)", 0xF05E},
	{"unsaved ●", 0x25CF}, {"nf lock", 0xF023}, {"nf refresh", 0xF021}, {"nf link (deps)", 0xF0C1},
	{"nf inbox (staging)", 0xF01C}, {"nf play", 0xF04B}, {"nf floppy (save)", 0xF0C7},
	{"nf sort-amount-asc", 0xF160}, {"nf search", 0xF002}, {"selected ▸", 0x25B8}, {"ok ✓", 0x2713},
	{"fail ✗", 0x2717}, {"busy …", 0x2026}, {"dot ·", 0x00B7}, {"prompt ❯", 0x276F},
}

var compat = []glyph{
	{"compat official ◆", 0x25C6}, {"compat workshop ◎", 0x25CE}, {"compat local ▢", 0x25A2},
	{"compat copy ◇", 0x25C7}, {"compat git ±", 0x00B1}, {"compat error ✕", 0x2715}, {"compat warn !", '!'},
	{"compat log ※", 0x203B}, {"compat unloadable ⊘", 0x2298}, {"compat unsaved ●", 0x25CF},
	{"compat refresh ↻", 0x21BB}, {"compat deps →", 0x2192}, {"compat staging ▤", 0x25A4},
	{"compat play ▶", 0x25B6}, {"compat save ↓", 0x2193}, {"compat sort ⇅", 0x21C5}, {"compat search /", '/'},
}

// Replacement candidates: Neutral or Narrow width, likely in Cascadia Mono.
var candidates = []glyph{
	{"cand ◈ U+25C8", 0x25C8}, {"cand ⬥ U+2B25", 0x2B25}, {"cand ♦ U+2666", 0x2666}, {"cand ⌂ U+2302", 0x2302},
	{"cand ¤ U+00A4", 0x00A4}, {"cand ⊙ U+2299", 0x2299}, {"cand ⊚ U+229A", 0x229A}, {"cand ⊕ U+2295", 0x2295},
	{"cand ∘ U+2218", 0x2218}, {"cand ⌗ U+2317", 0x2317}, {"cand ⎘ U+2398", 0x2398}, {"cand ⧉ U+29C9", 0x29C9},
	{"cand ⊞ U+229E", 0x229E}, {"cand ⊡ U+22A1", 0x22A1}, {"cand ⋄ U+22C4", 0x22C4}, {"cand ⟲ U+27F2", 0x27F2},
	{"cand ⟳ U+27F3", 0x27F3}, {"cand ↺ U+21BA", 0x21BA}, {"cand ⇢ U+21E2", 0x21E2}, {"cand › U+203A", 0x203A},
	{"cand ⊳ U+22B3", 0x22B3}, {"cand ⏵ U+23F5", 0x23F5}, {"cand ⇩ U+21E9", 0x21E9}, {"cand ⤓ U+2913", 0x2913},
	{"cand ⇵ U+21F5", 0x21F5}, {"cand ⇳ U+21F3", 0x21F3}, {"cand ⌕ U+2315", 0x2315}, {"cand ∅ U+2205", 0x2205},
	{"cand ⦸ U+29B8", 0x29B8}, {"cand ⊗ U+2297", 0x2297}, {"cand ✗ U+2717", 0x2717}, {"cand ⨯ U+2A2F", 0x2A2F},
	{"cand × U+00D7", 0x00D7}, {"cand ⚠ U+26A0", 0x26A0}, {"cand ‼ U+203C", 0x203C}, {"cand ¡ U+00A1", 0x00A1},
	{"cand ⁂ U+2042", 0x2042}, {"cand ⌁ U+2301", 0x2301}, {"cand ◌ U+25CC", 0x25CC}, {"cand ⦁ U+2981", 0x2981},
	{"cand • U+2022", 0x2022}, {"cand ⬤ U+2B24", 0x2B24}, {"cand ⏺ U+23FA", 0x23FA}, {"cand ⊟ U+229F", 0x229F},
	{"cand ⊠ U+22A0", 0x22A0}, {"cand ⌸ U+2338", 0x2338}, {"cand ⍐ U+2350", 0x2350}, {"cand ⇉ U+21C9", 0x21C9},
	{"cand ⧖ U+29D6", 0x29D6}, {"cand ⌘ U+2318", 0x2318}, {"cand ⎇ U+2387", 0x2387}, {"cand ⑂ U+2442", 0x2442},
	{"cand ∓ U+2213", 0x2213}, {"cand ⌿ U+233F", 0x233F}, {"cand ↯ U+21AF", 0x21AF}, {"cand ⁜ U+205C", 0x205C},
	{"cand ¶ U+00B6", 0x00B6}, {"cand ⚑ U+2691", 0x2691}, {"cand ⬡ U+2B21", 0x2B21}, {"cand ⬢ U+2B22", 0x2B22},
	{"cand ⏏ U+23CF", 0x23CF}, {"cand ⋯ U+22EF", 0x22EF}, {"cand ⇣ U+21E3", 0x21E3}, {"cand ⇡ U+21E1", 0x21E1},
	{"cand ↕ U+2195", 0x2195}, {"cand ⇕ U+21D5", 0x21D5}, {"cand ↧ U+21A7", 0x21A7}, {"cand ⌄ U+2304", 0x2304},
}

// proposed is the corrected compat column (and the nerd-mode unsaved dot) for 06 §3.
var proposed = []glyph{
	{"official ⬥", 0x2B25}, {"workshop ◉", 0x25C9}, {"local ▢", 0x25A2}, {"local copy ⬦", 0x2B26},
	{"git ∆", 0x2206}, {"error ⨯", 0x2A2F}, {"warn !", '!'}, {"log error ◍", 0x25CD}, {"unloadable ◌", 0x25CC},
	{"unsaved ⬤", 0x2B24}, {"unsaved (nerd) fa-circle", 0xF111}, {"update ⇣", 0x21E3}, {"deps ›", 0x203A},
	{"staging ◫", 0x25EB}, {"play ►", 0x25BA}, {"save ↲", 0x21B2}, {"sort ↨", 0x21A8}, {"search /", '/'},
	{"selected ▸", 0x25B8}, {"ok ✓", 0x2713}, {"fail (compat) ⨯", 0x2A2F}, {"prompt ❯", 0x276F},
	{"busy … (ambiguous, kept)", 0x2026}, {"dot · (ambiguous, kept)", 0xB7},
}
