package tui

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// eventLoop runs the model the way the program does: every command in a goroutine, its
// message back through Update, a frame after every message.
type eventLoop struct {
	m    *model
	msgs chan tea.Msg
}

func (l *eventLoop) run(c tea.Cmd) {
	if c == nil {
		return
	}
	go func() {
		msg := c()
		if b, ok := msg.(tea.BatchMsg); ok {
			for _, x := range b {
				l.run(x)
			}
			return
		}
		if msg != nil {
			l.msgs <- msg
		}
	}()
}

func (l *eventLoop) send(msg tea.Msg) {
	_, c := l.m.Update(msg)
	l.run(c)
	l.m.View()
}

// pump handles the messages that arrived, for at most d.
func (l *eventLoop) pump(d time.Duration, done func() bool) bool {
	end := time.Now().Add(d)
	for {
		if done != nil && done() {
			return true
		}
		left := time.Until(end)
		if left <= 0 {
			return false
		}
		select {
		case msg := <-l.msgs:
			l.send(msg)
		case <-time.After(min(left, time.Millisecond)):
		}
	}
}

// thumbReady: the current mod's thumbnail is on screen, or its block says there is none.
func (l *eventLoop) thumbReady() bool {
	m := l.m
	it := m.current()
	if it == nil || m.thumbShown != it.id {
		return false
	}
	e := m.imgs[imgKey(previewURL(it.id), thumbCols, thumbRows)]
	if e == nil || e.state == imgLoading {
		return false
	}
	return e.state == imgFailed || len(m.images.cur) > 0
}

// T14 (06 §12.6) on a copy of a real data directory: scrolling 500 rows at 30 rows a
// second, the thumbnail is on screen within 100 ms after stopping, and within 30 ms when
// it was made ahead.
//
//	RIMARCHO_REAL_COPY=<copy of %APPDATA%\RimArcho> go test ./internal/surface/tui -run T14 -v
func TestT14ThumbLatencyReal(t *testing.T) {
	src := os.Getenv("RIMARCHO_REAL_COPY")
	if src == "" {
		t.Skip("set RIMARCHO_REAL_COPY to a copy of a real data directory")
	}
	home := t.TempDir()
	for _, f := range []string{"archo.db", "archo.db-wal", "config.toml"} {
		copyFile(t, filepath.Join(src, f), filepath.Join(home, f))
	}
	// the thumbnail cache, as the player's data directory has it
	pngs, _ := filepath.Glob(filepath.Join(src, "cache", "previews", "*.png"))
	os.MkdirAll(filepath.Join(home, "cache", "previews"), 0o755)
	for _, p := range pngs {
		copyFile(t, p, filepath.Join(home, "cache", "previews", filepath.Base(p)))
	}
	a, err := app.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.Launcher = func(string, []string, string) error { t.Fatal("launch attempted"); return nil }
	a.AnalyzeRunner = app.InProcessAnalyze(a)
	m := New(Options{App: a, Scan: func(string) (string, error) { t.Fatal("scan"); return "", nil }}).(*model)
	m.images = &imgWriter{Writer: io.Discard}
	m.Init()
	l := &eventLoop{m: m, msgs: make(chan tea.Msg, 256)}
	l.send(tea.WindowSizeMsg{Width: 160, Height: 46})
	// the longest list
	best := cActive
	for k := range m.cols {
		if len(m.cols[k].view) > len(m.cols[best].view) {
			best = colKind(k)
		}
	}
	m.focus = best
	n := len(m.cols[best].view)
	t.Logf("%s: %d rows, %d cached thumbnails", best.title(), n, len(pngs))
	if n < 50 {
		t.Skip("too few mods")
	}
	var misses []time.Duration
	down := true
	imaged := 0 // stops on a mod that has a thumbnail
	for run := 0; run < 5; run++ {
		var tKey time.Time
		for i := 0; i < 100; i++ { // 5 × 100 = 500 rows at 30 rows a second
			c := m.col()
			if c.cursor == n-1 {
				down = false
			} else if c.cursor == 0 {
				down = true
			}
			k := tea.KeyMsg{Type: tea.KeyDown}
			if !down {
				k = tea.KeyMsg{Type: tea.KeyUp}
			}
			tKey = time.Now()
			l.send(k)
			if i < 99 {
				l.pump(33*time.Millisecond, nil)
			}
		}
		// from the last key: the thumbnail of the row it stopped on
		if !l.pump(2*time.Second, l.thumbReady) {
			t.Fatalf("run %d: no thumbnail 2 s after stopping on %s", run, m.current().name)
		}
		misses = append(misses, time.Since(tKey))
		if e := m.imgs[imgKey(previewURL(m.current().id), thumbCols, thumbRows)]; e.state == imgOK {
			imaged++
		}
	}
	// made ahead: rest until the neighbours are done, then one row on
	var hits []time.Duration
	for run := 0; run < 5; run++ {
		l.pump(3*time.Second, func() bool { return m.prefetching == "" && m.imgLoading == "" && l.thumbReady() })
		k := tea.KeyMsg{Type: tea.KeyDown}
		if m.col().cursor == n-1 {
			k = tea.KeyMsg{Type: tea.KeyUp}
		}
		t0 := time.Now()
		l.send(k)
		if !l.pump(2*time.Second, l.thumbReady) {
			t.Fatalf("no thumbnail one row on")
		}
		hits = append(hits, time.Since(t0))
		if e := m.imgs[imgKey(previewURL(m.current().id), thumbCols, thumbRows)]; e.state == imgOK {
			imaged++
		}
	}
	// let the loaders finish before the temp dir goes
	l.pump(5*time.Second, func() bool { return m.prefetching == "" && m.imgLoading == "" })
	worst := func(ds []time.Duration) time.Duration {
		s := append([]time.Duration{}, ds...)
		sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
		return s[len(s)-1]
	}
	t.Logf("after stopping: %v (worst %v); made ahead: %v (worst %v); %d of 10 stops on a mod with a thumbnail", misses, worst(misses), hits, worst(hits), imaged)
	if w := worst(misses); w > 100*time.Millisecond {
		t.Errorf("thumbnail %v after stopping, want ≤ 100 ms", w)
	}
	if w := worst(hits); w > 30*time.Millisecond {
		t.Errorf("made-ahead thumbnail %v, want ≤ 30 ms", w)
	}
}
