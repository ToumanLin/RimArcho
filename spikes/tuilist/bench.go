package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/ToumanLin/RimArcho/spikes/internal/procmem"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func runes(s string) []tea.KeyMsg {
	var out []tea.KeyMsg
	for _, r := range s {
		out = append(out, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return out
}

type stats []float64

func (s stats) String() string {
	c := append(stats(nil), s...)
	sort.Float64s(c)
	p := func(q float64) float64 { return c[int(q*float64(len(c)-1))] }
	sum := 0.0
	for _, v := range c {
		sum += v
	}
	return fmt.Sprintf("mean=%.3f n=%d p50=%.3f p95=%.3f p99=%.3f max=%.3f ms", sum/float64(len(c)), len(c), p(.5), p(.95), p(.99), c[len(c)-1])
}

type align struct {
	lines, bad int
	samples    []string
}

func (a *align) check(view string) {
	lines := strings.Split(view, "\n")
	want := -1
	for i, l := range lines[:len(lines)-1] { // last line is the footer
		w := runewidth.StringWidth(ansiRe.ReplaceAllString(l, ""))
		if want < 0 {
			want = w
		}
		a.lines++
		if w != want {
			a.bad++
			if len(a.samples) < 5 {
				a.samples = append(a.samples, fmt.Sprintf("line %d width %d != %d: %q", i, w, want, l))
			}
		}
	}
}

func step(m tea.Model, msg tea.Msg, st *stats, al *align) tea.Model {
	t := time.Now()
	m, _ = m.Update(msg)
	v := m.View()
	*st = append(*st, float64(time.Since(t).Microseconds())/1000)
	al.check(v)
	return m
}

func benchRows(n int, out io.Writer) {
	fmt.Fprintf(out, "=== %d rows, window 120x40 ===\n", n)
	var m tea.Model = newModel(n)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	var scroll, search stats
	al := &align{}
	keys := []tea.KeyMsg{{Type: tea.KeyDown}, {Type: tea.KeyPgDown}, {Type: tea.KeyRunes, Runes: []rune{'j'}}, {Type: tea.KeyPgUp}, {Type: tea.KeyUp}, {Type: tea.KeyRunes, Runes: []rune{'k'}}}
	for i := 0; i < 1000; i++ {
		m = step(m, keys[(i/7)%len(keys)], &scroll, al)
		if i%3 == 0 {
			m = step(m, keys[2], &scroll, al)
		}
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}, &search, al)
	typed := runes("机械族 Mech")
	for _, k := range typed {
		m = step(m, k, &search, al)
	}
	for range typed {
		m = step(m, tea.KeyMsg{Type: tea.KeyBackspace}, &search, al)
	}
	for _, k := range runes("原版扩展：") {
		m = step(m, k, &search, al)
	}
	for i := 0; i < 5; i++ {
		m = step(m, tea.KeyMsg{Type: tea.KeyBackspace}, &search, al)
	}
	ws, priv, peak := procmem.MB()
	fmt.Fprintf(out, "scroll: %s\nsearch: %s\n", scroll, search)
	res := "PASS"
	if al.bad > 0 {
		res = "FAIL"
	}
	fmt.Fprintf(out, "alignment: %s (%d rendered list lines checked, %d offending)\n", res, al.lines, al.bad)
	for _, s := range al.samples {
		fmt.Fprintln(out, "  ", s)
	}
	fmt.Fprintf(out, "memory MiB: ws=%.1f private=%.1f peak_ws=%.1f\n", ws, priv, peak)
	fmt.Fprintf(out, "runewidth.EastAsianWidth=%v\n\n", runewidth.EastAsianWidth)
}

// notifier is a tea output writer that signals when a marker appears.
type notifier struct {
	mu     sync.Mutex
	marker string
	last   string
	ch     chan struct{}
}

func (n *notifier) Write(p []byte) (int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.last = string(p)
	if n.marker != "" && strings.Contains(string(p), n.marker) {
		n.marker = ""
		n.ch <- struct{}{}
	}
	return len(p), nil
}

func (n *notifier) expect(s string) { n.mu.Lock(); n.marker = s; n.mu.Unlock() }

// e2e runs a real tea.Program (real renderer, 60fps flush) with piped input and
// measures key-write -> footer text appears in rendered output.
func e2e(out io.Writer, nrows int) {
	pr, pw := io.Pipe()
	nt := &notifier{ch: make(chan struct{}, 1)}
	p := tea.NewProgram(newModel(nrows), tea.WithInput(pr), tea.WithOutput(nt), tea.WithoutSignals())
	done := make(chan struct{})
	go func() { p.Run(); close(done) }()
	time.Sleep(300 * time.Millisecond)
	var st stats
	wait := func(marker string, send func()) {
		nt.expect(marker)
		t := time.Now()
		send()
		select {
		case <-nt.ch:
			st = append(st, float64(time.Since(t).Microseconds())/1000)
		case <-time.After(2 * time.Second):
			nt.mu.Lock()
			fmt.Fprintf(out, "  timeout waiting for %q last=%q\n", marker, nt.last)
			nt.mu.Unlock()
		}
	}
	for i := 0; i < 100; i++ {
		wait(fmt.Sprintf("pos %d/", i+2), func() { pw.Write([]byte("j")) })
	}
	wait("search[/]: _", func() { pw.Write([]byte("/")) })
	q := ""
	for _, r := range "机械族" {
		q += string(r)
		// piped UTF-8 is mis-decoded by bubbletea on Windows, so CJK uses p.Send
		wait("search[/]: "+q+"_", func() { p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}) })
	}
	fmt.Fprintf(out, "e2e (real renderer, piped input, %d rows) key->frame: %s\n", nrows, st)
	p.Quit()
	<-done
}

func runBench(withE2E bool) {
	var sb strings.Builder
	w := io.MultiWriter(os.Stdout, &sb)
	benchRows(500, w)
	benchRows(5000, w)
	if withE2E {
		e2e(w, 500)
		e2e(w, 5000)
	}
	fmt.Fprintln(w, "Pass criterion: every p99/max <= 50 ms (Update+View).")
	os.WriteFile("RESULT.txt", []byte(sb.String()), 0o644)
}
