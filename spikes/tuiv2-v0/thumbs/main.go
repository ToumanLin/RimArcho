// Spike (TUI v2 V0, 06 §6.2): Preview.png thumbnails over the iTerm2 inline-image
// protocol (OSC 1337) beside a bubbletea list, in WezTerm.
//
// How it cooperates with bubbletea's renderer (v1.3 standard renderer):
//   - The renderer writes each frame with one Write call, skips unchanged lines, and
//     erases to end of line on lines it rewrites — which also erases image cells there.
//   - The program's output is a wrapper (imgOut) around stdout. Every frame Write is
//     wrapped in synchronized output (DEC 2026) and followed, in the same Write, by the
//     image work: erase the reserved rect when the selection changed, and (re)draw the
//     thumbnail while one is shown. Cursor is saved and restored around it (ESC 7 / ESC 8),
//     so the renderer's cursor bookkeeping is untouched, and made visible (?25h … ?25l)
//     around the image so ConPTY forwards the positioning CUP (see Write).
//   - Selecting another row clears the rect at once; the image is drawn only after the
//     cursor rests 150 ms (a tea.Tick carries the selection generation).
//
// Read-only on the mod folders. Run inside WezTerm:
//
//	thumbs.exe -root D:\SteamLibrary\steamapps\workshop\content\294100 -n 500 [-auto] [-log run.log]
package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	rectRow, rectCol = 3, 56 // 1-based screen position of the thumbnail rect
	rectW, rectH     = 18, 9 // cells
	dwell            = 150 * time.Millisecond
	thumbPx          = 360 // cached width (06 §6.2)
)

type mod struct {
	name, dir, preview string
}

// imgOut appends the image work to each frame (see the package comment).
type imgOut struct {
	*os.File
	mu      sync.Mutex
	clear   bool   // erase the rect after the next frame
	img     []byte // escape sequence that draws the current thumbnail; nil = none
	frames  int
	imgSent int
	dump    *os.File
	dumped  int
	bytes   int64
}

func (o *imgOut) Write(frame []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	var b bytes.Buffer
	if os.Getenv("THUMBS_NOSYNC") == "" {
		b.WriteString("\x1b[?2026h")
	}
	b.Write(frame)
	if o.clear || o.img == nil {
		b.WriteString("\x1b7")
		for r := 0; r < rectH; r++ {
			fmt.Fprintf(&b, "\x1b[%d;%dH\x1b[%dX", rectRow+r, rectCol, rectW)
		}
		b.WriteString("\x1b8")
		o.clear = false
	}
	if o.img != nil {
		// ConPTY (WezTerm on Windows) forwards OSC 1337 as passthrough but only emits its
		// own cursor moves while the cursor is visible; bubbletea hides it (?25l), so the
		// CUP before the image would be dropped and the image would land wherever ConPTY
		// last left the cursor. Show the cursor for the duration of the image write.
		b.WriteString("\x1b7\x1b[?25h")
		fmt.Fprintf(&b, "\x1b[%d;%dH", rectRow, rectCol)
		b.Write(o.img)
		b.WriteString("\x1b[?25l\x1b8")
		o.imgSent++
	}
	if os.Getenv("THUMBS_NOSYNC") == "" {
		b.WriteString("\x1b[?2026l")
	}
	if o.dump != nil && o.dumped < 6 && o.img != nil {
		o.dumped++
		d := b.String()
		if i := strings.Index(d, "\x1b]1337"); i >= 0 {
			if j := strings.Index(d[i:], ":"); j >= 0 {
				if k := strings.Index(d[i+j:], "\x07"); k >= 0 {
					d = d[:i+j+1] + "<b64>" + d[i+j+k:]
				}
			}
		}
		fmt.Fprintf(o.dump, "---- write %d (%d bytes)\n%q\n", o.frames, b.Len(), d)
	}
	o.frames++
	o.bytes += int64(b.Len())
	if _, err := o.File.Write(b.Bytes()); err != nil {
		return 0, err
	}
	return len(frame), nil
}

func (o *imgOut) set(clear bool, img []byte) {
	o.mu.Lock()
	o.clear = o.clear || clear
	o.img = img
	o.mu.Unlock()
}

type dwellMsg struct{ gen int }
type thumbMsg struct {
	gen  int
	seq  []byte
	info string
}
type autoMsg struct{ k string }

type model struct {
	mods      []mod
	cur, off  int
	h         int
	gen       int
	out       *imgOut
	info      string
	cache     map[string][]byte
	cacheInfo map[string]string
	log       *log.Logger
	quitting  bool
}

func (m *model) Init() tea.Cmd { return m.selected() }

func (m *model) selected() tea.Cmd {
	m.gen++
	m.out.set(true, nil) // clear now; draw after the dwell
	m.info = "…"
	g := m.gen
	return tea.Tick(dwell, func(time.Time) tea.Msg { return dwellMsg{g} })
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.h = msg.Height
	case tea.KeyMsg:
		return m, m.key(msg.String())
	case autoMsg:
		return m, m.key(msg.k)
	case dwellMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		md := m.mods[m.cur]
		g := m.gen
		return m, func() tea.Msg {
			seq, info := m.thumb(md)
			return thumbMsg{g, seq, info}
		}
	case thumbMsg:
		if msg.gen != m.gen {
			return m, nil // the cursor moved on while decoding
		}
		m.out.set(false, msg.seq)
		m.info = msg.info
		if m.log != nil {
			m.log.Printf("draw gen=%d row=%d %s", msg.gen, m.cur, msg.info)
		}
	}
	return m, nil
}

func (m *model) key(k string) tea.Cmd {
	prev := m.cur
	switch k {
	case "q", "ctrl+c":
		m.quitting = true
		return tea.Quit
	case "down", "j":
		m.cur = min(m.cur+1, len(m.mods)-1)
	case "up", "k":
		m.cur = max(m.cur-1, 0)
	case "pgdown":
		m.cur = min(m.cur+m.page(), len(m.mods)-1)
	case "pgup":
		m.cur = max(m.cur-m.page(), 0)
	case "G", "end":
		m.cur = len(m.mods) - 1
	case "g", "home":
		m.cur = 0
	}
	if m.cur != prev {
		return m.selected()
	}
	return nil
}

func (m *model) page() int { return max(m.h-4, 1) }

// thumb loads, scales (360 px wide) and encodes a preview as an OSC 1337 sequence.
func (m *model) thumb(md mod) ([]byte, string) {
	if md.preview == "" {
		return nil, "no Preview.png"
	}
	if s, ok := m.cache[md.preview]; ok {
		return s, m.cacheInfo[md.preview] + " (cached)"
	}
	t0 := time.Now()
	f, err := os.Open(md.preview)
	if err != nil {
		return nil, err.Error()
	}
	src, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return nil, "decode: " + err.Error()
	}
	b := src.Bounds()
	w := min(thumbPx, b.Dx())
	h := b.Dy() * w / max(b.Dx(), 1)
	dst := scale(src, w, h)
	var pb bytes.Buffer
	(&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&pb, dst)
	enc := base64.StdEncoding.EncodeToString(pb.Bytes())
	seq := []byte(fmt.Sprintf("\x1b]1337;File=inline=1;size=%d;width=%d;height=%d;preserveAspectRatio=1;doNotMoveCursor=1:%s\x07", pb.Len(), rectW, rectH, enc))
	info := fmt.Sprintf("%dx%d → %dx%d, %d KB png, %d ms", b.Dx(), b.Dy(), w, h, pb.Len()/1024, time.Since(t0).Milliseconds())
	m.cache[md.preview], m.cacheInfo[md.preview] = seq, info
	return seq, info
}

func (m *model) View() string {
	if m.quitting {
		return ""
	}
	page := m.page()
	if m.cur < m.off {
		m.off = m.cur
	}
	if m.cur >= m.off+page {
		m.off = m.cur - page + 1
	}
	md := m.mods[m.cur]
	right := make([]string, m.h)
	right[0] = "\x1b[1m" + trunc(md.name, 50) + "\x1b[0m"
	for i := 1; i < 1+rectH+1; i++ {
		right[i] = strings.Repeat(" ", rectW) // the reserved rect (rows 2..10 of the column)
	}
	right[rectH+2] = "preview: " + trunc(m.info, 60)
	right[rectH+3] = fmt.Sprintf("row %d/%d · frames %d · image draws %d · %.1f MB out", m.cur+1, len(m.mods), m.out.frames, m.out.imgSent, float64(m.out.bytes)/(1<<20))
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\x1b[38;2;165;180;252mthumbnail spike\x1b[0m  %d mods, %d with Preview.png\n", len(m.mods), countPreviews(m.mods)))
	for i := 0; i < m.h-2; i++ {
		row := m.off + i
		left := ""
		if row < len(m.mods) && i < page {
			mark := "  "
			if row == m.cur {
				mark = "▸ "
			}
			icon := " "
			if m.mods[row].preview != "" {
				icon = "▫"
			}
			left = fmt.Sprintf("%s%4d %s %s", mark, row+1, icon, trunc(m.mods[row].name, 40))
		}
		r := ""
		if i < len(right) {
			r = right[i]
		}
		b.WriteString(pad(left, rectCol-1) + r)
		if i < m.h-3 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func countPreviews(ms []mod) int {
	n := 0
	for _, m := range ms {
		if m.preview != "" {
			n++
		}
	}
	return n
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func pad(s string, n int) string {
	w := len([]rune(s))
	if w >= n {
		return string([]rune(s)[:n])
	}
	return s + strings.Repeat(" ", n-w)
}

var nameRe = regexp.MustCompile(`(?is)<name>\s*(.*?)\s*</name>`)

func load(roots []string, n int) []mod {
	var out []mod
	for _, root := range roots {
		ents, _ := os.ReadDir(root)
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(root, e.Name())
			about := filepath.Join(dir, "About")
			name := e.Name()
			for _, fn := range []string{"About.xml", "About.xml.disabled"} {
				if b, err := os.ReadFile(filepath.Join(about, fn)); err == nil {
					if mm := nameRe.FindSubmatch(b); mm != nil {
						name = string(mm[1])
					}
					break
				}
			}
			md := mod{name: name, dir: dir}
			if aes, err := os.ReadDir(about); err == nil {
				for _, a := range aes {
					if strings.EqualFold(a.Name(), "Preview.png") {
						md.preview = filepath.Join(about, a.Name())
					}
				}
			}
			out = append(out, md)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].name) < strings.ToLower(out[j].name) })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func main() {
	root := flag.String("root", `D:\SteamLibrary\steamapps\workshop\content\294100`, "mod folders (comma-separated)")
	n := flag.Int("n", 500, "rows")
	auto := flag.Bool("auto", false, "drive the list itself: fast scroll, dwell, page, then quit")
	logPath := flag.String("log", "", "event log")
	flag.Parse()
	m := &model{mods: load(strings.Split(*root, ","), *n), h: 46, out: &imgOut{File: os.Stdout}, cache: map[string][]byte{}, cacheInfo: map[string]string{}}
	if d := os.Getenv("THUMBS_DUMP"); d != "" {
		m.out.dump, _ = os.Create(d)
	}
	if *logPath != "" {
		f, _ := os.Create(*logPath)
		defer f.Close()
		m.log = log.New(f, "", log.Lmicroseconds)
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(m.out))
	if *auto {
		go drive(p, m)
	}
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("frames %d, image draws %d, %.1f MB written\n", m.out.frames, m.out.imgSent, float64(m.out.bytes)/(1<<20))
}

// drive is the scripted scroll test. Checkpoints (for screenshots) are written to the log
// and held for 2 s.
func drive(p *tea.Program, m *model) {
	key := func(k string) { p.Send(autoMsg{k}) }
	mark := func(s string) {
		if m.log != nil {
			m.log.Printf("CHECKPOINT %s", s)
		}
		time.Sleep(2 * time.Second)
	}
	time.Sleep(2 * time.Second)
	mark("start: first row")
	for i := 0; i < 499; i++ { // fast scroll: 20 ms per row, faster than the dwell
		key("down")
		time.Sleep(20 * time.Millisecond)
	}
	mark("after fast scroll to row 500")
	for i := 0; i < 40; i++ { // slow scroll: every row dwells (250 ms)
		key("up")
		time.Sleep(250 * time.Millisecond)
	}
	mark("after slow scroll")
	// Find a row without a preview and stop there: the rect must be empty.
	for i := m.cur; i >= 0; i-- {
		if m.mods[i].preview == "" {
			for m.cur > i {
				key("up")
				time.Sleep(10 * time.Millisecond)
			}
			break
		}
	}
	time.Sleep(400 * time.Millisecond)
	mark("on a row without Preview.png")
	key("pgup")
	time.Sleep(400 * time.Millisecond)
	mark("after pgup")
	key("home")
	time.Sleep(400 * time.Millisecond)
	mark("home")
	key("q")
}

// scale is a box filter (averages the source pixels under each target pixel).
func scale(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0, y1 := b.Min.Y+y*b.Dy()/h, b.Min.Y+(y+1)*b.Dy()/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0, x1 := b.Min.X+x*b.Dx()/w, b.Min.X+(x+1)*b.Dx()/w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a, n uint32
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					cr, cg, cb, ca := src.At(xx, yy).RGBA()
					r, g, bl, a, n = r+cr, g+cg, bl+cb, a+ca, n+1
				}
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/n>>8), uint8(g/n>>8), uint8(bl/n>>8), uint8(a/n>>8)
		}
	}
	return dst
}
