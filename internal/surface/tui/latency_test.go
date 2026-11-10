package tui

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

type latencyOffline struct{}

func (latencyOffline) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("synthetic latency measurement is offline")
}

// TestPageSwitchLatency measures first visits and repeat visits, including page commands
// and their resulting frames. The supplied home must be synthetic (16 §10).
func TestPageSwitchLatency(t *testing.T) {
	home := os.Getenv("RIMARCHO_TUI_LATENCY_HOME")
	if home == "" {
		t.Skip("set RIMARCHO_TUI_LATENCY_HOME to a synthetic home")
	}
	a, err := app.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.HTTP = &http.Client{Transport: latencyOffline{}}
	a.MonitorBundle = app.MonitorNone
	a.ToddsNoPath = true
	a.LockBase = t.TempDir()
	a.Launcher = func(string, []string, string) error { t.Fatal("launch attempted"); return nil }
	a.SteamSignedIn = func() (string, bool) { return "", false }
	a.WorkspaceSnapshot()
	TuneProcess(a)
	m := New(Options{App: a, Analyze: func(string, func(string)) error { return nil }, Scan: func(string) (string, error) { t.Fatal("scan attempted"); return "", nil }}).(*model)
	m.Init()
	m.w, m.h = 160, 46
	_ = m.View()
	t.Logf("inventory: %d copies", len(m.st.Ix.Mods))
	pageSwitchLatency(t, m)
}

func pageSwitchLatency(t *testing.T, m *model) {
	t.Helper()
	// The timer installed by Update is not a page read. Measure its handler directly;
	// drain every page command and measure each reply's handler and frame as well.
	var ignored, replyMax time.Duration
	var drain func(tea.Cmd)
	drain = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		start := time.Now()
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				drain(c)
			}
			return
		}
		// Cursor debounce and automatic analysis are independent of page data.
		switch msg.(type) {
		case detailTick, analyzeDoneMsg, analyzeLineMsg:
			ignored += time.Since(start)
			return
		}
		start = time.Now()
		_, next := m.update(msg)
		_ = m.View()
		replyMax = max(replyMax, time.Since(start))
		drain(next)
	}
	for round := 0; round < 4; round++ {
		for _, key := range "!@#$%^&*" {
			m.panel, m.prompt = nil, nil // close the previous chooser/text panel
			ignored, replyMax = 0, 0
			t0 := time.Now()
			_, cmd := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
			update := time.Since(t0)
			v0 := time.Now()
			_ = m.View()
			frame := time.Since(v0)
			drain(cmd)
			ready := time.Since(t0) - ignored
			t.Logf("round=%d key=%c update=%.3f view=%.3f reply=%.3f ready=%.3f ms", round, key, float64(update)/1e6, float64(frame)/1e6, float64(replyMax)/1e6, float64(ready)/1e6)
			if len(m.st.Ix.Mods) <= 1355 && max(update+frame, replyMax) > 50*time.Millisecond {
				t.Errorf("%c: key/frame %v, reply/frame %v > 50 ms", key, update+frame, replyMax)
			}
			if round > 0 && len(m.st.Ix.Mods) <= 1355 && ready > 50*time.Millisecond {
				t.Errorf("%c: repeat visit including page commands %v > 50 ms", key, ready)
			}
		}
	}
}

func TestPageReadSnapshots(t *testing.T) {
	for _, page := range []int{1, 2, 6, 7} {
		t.Run(fmt.Sprint(page), func(t *testing.T) {
			e, _ := fixture(t)
			m := direct(t, e, 160, 46)
			m.opts.Analyze = func(string, func(string)) error { return nil }
			cmd := m.goPage(page)
			if cmd == nil {
				t.Fatal("first visit must read in the background")
			}
			msg := cmd()
			m.goPage(4)
			m.prob.base = nil
			m.prob.readKey = "newer page"
			m.say(sayInfo, "newer page")
			_, next := m.update(msg)
			if next != nil || m.tab != tabList || m.home.data != nil || m.libr.page != nil || m.texturePanel != nil || len(m.prob.base) != 0 || m.prob.readKey != "newer page" || m.status != "newer page" {
				t.Fatal("late page read changed the newer page")
			}
			finishPage(m, m.goPage(page))
			if page == 1 && m.home.data == nil || page == 6 && m.libr.page == nil || page == 7 && m.texturePanel == nil || page == 2 && len(m.prob.base) == 0 {
				t.Fatal("current page read was not applied")
			}
			if page != 2 && m.goPage(page) != nil {
				t.Fatal("unchanged page was rebuilt")
			}
			before := len(m.st.List)
			if page == 6 {
				e.MustRun("disable", "pkg:tester.a") // a change the editor has not reloaded
			}
			if err := e.App.Store.SetMeta("test:page-generation", "changed"); err != nil {
				t.Fatal(err)
			}
			cmd = m.goPage(page)
			if cmd == nil {
				t.Fatal("changed state reused a stale page")
			}
			if page == 6 {
				finishPage(m, cmd)
				if len(m.libCache.St.List) != before-1 || len(m.st.List) != before {
					t.Fatal("adopted library lost the worker's newer list")
				}
				_ = m.View() // copy fields remain usable after the worker session closes
			}
		})
	}
}

// keyLatency sends navigation and filter keys to a model and returns the time of each
// key's Update plus the View that follows it (what the player waits for, 16 §11 T1).
func keyLatency(t *testing.T, m *model) map[string][]time.Duration {
	t.Helper()
	out := map[string][]time.Duration{}
	step := func(what string, msg tea.Msg) {
		t0 := time.Now()
		m.update(msg)
		_ = m.View()
		out[what] = append(out[what], time.Since(t0))
	}
	key := func(s string) tea.Msg {
		switch s {
		case "down":
			return tea.KeyMsg{Type: tea.KeyDown}
		case "up":
			return tea.KeyMsg{Type: tea.KeyUp}
		case "pgdown":
			return tea.KeyMsg{Type: tea.KeyPgDown}
		case "esc":
			return tea.KeyMsg{Type: tea.KeyEsc}
		case "bs":
			return tea.KeyMsg{Type: tea.KeyBackspace}
		}
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	for _, col := range []string{"h", "l"} { // not enabled, then enabled
		step("focus", key(col))
		for i := 0; i < 200; i++ {
			step("scroll", key("down"))
		}
		for i := 0; i < 10; i++ {
			step("page", key("pgdown"))
		}
		step("ends", key("end"))
		step("ends", key("home"))
		step("filter", key("/"))
		for _, r := range "harmony" {
			step("filter", key(string(r)))
		}
		for i := 0; i < 7; i++ {
			step("filter", key("bs"))
		}
		for _, r := range "中文" {
			step("filter", key(string(r)))
		}
		step("filter", key("esc"))
	}
	return out
}

func report(t *testing.T, lat map[string][]time.Duration) time.Duration {
	var worst time.Duration
	names := make([]string, 0, len(lat))
	for k := range lat {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		ds := lat[k]
		var sum, mx time.Duration
		for _, d := range ds {
			sum += d
			mx = max(mx, d)
		}
		worst = max(worst, mx)
		t.Logf("%-7s %4d keys  mean %6.2f ms  max %6.2f ms", k, len(ds), float64(sum)/float64(len(ds))/1e6, float64(mx)/1e6)
	}
	return worst
}

// 16 §11 T1 on a synthetic environment of 1354 mods (485 enabled, CJK names among them).
func TestT1KeyLatency1354(t *testing.T) {
	if os.Getenv("RIMARCHO_SCALE_TESTS") == "" {
		t.Skip("set RIMARCHO_SCALE_TESTS=1: builds and scans 1354 mods (about a minute)")
	}
	e := testkit.New(t)
	active := []string{"ludeon.rimworld"}
	for i := 1; i <= 1353; i++ {
		pkg := fmt.Sprintf("tester.m%04d", i)
		name := fmt.Sprintf("Mod %04d Harmony Patch Collection", i)
		if i%7 == 0 {
			name = fmt.Sprintf("中文汉化补丁 %04d（简体）", i)
		}
		e.WriteMod(filepath.Join(e.Game, "Mods", fmt.Sprintf("M%04d", i)), testkit.Sample(pkg, name))
		if i <= 484 {
			active = append(active, pkg)
		}
	}
	e.WriteModsConfig("1.6.4633 rev1261", active...)
	e.AddEnv(false)
	e.MustRun("scan")
	m := direct(t, e, 160, 46)
	m.opts.Analyze = func(string, func(string)) error { return nil }
	TuneProcess(e.App)
	if n := len(m.cols[cActive].items) + len(m.cols[cInactive].items); n != 1354 {
		t.Fatalf("%d mods in the columns", n)
	}
	if worst := report(t, keyLatency(t, m)); worst > 50*time.Millisecond {
		t.Fatalf("slowest key %v > 50 ms", worst)
	}
	pageSwitchLatency(t, m)
}

// The same on a real data directory (a copy: the TUI syncs the workspace into it).
//
//	RIMARCHO_REAL_COPY=<copy of %APPDATA%\RimArcho> go test ./internal/surface/tui -run T1Real -v
func TestT1KeyLatencyReal(t *testing.T) {
	home := os.Getenv("RIMARCHO_REAL_COPY")
	if home == "" {
		t.Skip("set RIMARCHO_REAL_COPY to a copy of a real data directory")
	}
	a, err := app.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.Launcher = func(string, []string, string) error { t.Fatal("launch attempted"); return nil }
	a.AnalyzeRunner = app.InProcessAnalyze(a) // never a child: that would be this test binary
	TuneProcess(a)
	m := New(Options{App: a, Scan: func(string) (string, error) { t.Fatal("scan"); return "", nil }}).(*model)
	t0 := time.Now()
	m.Init()
	m.update(tea.WindowSizeMsg{Width: 160, Height: 46})
	_ = m.View()
	t.Logf("first frame: %v (%d enabled, %d not enabled)", time.Since(t0), len(m.cols[cActive].items), len(m.cols[cInactive].items))
	if worst := report(t, keyLatency(t, m)); worst > 50*time.Millisecond {
		t.Fatalf("slowest key %v > 50 ms", worst)
	}
	// search (16 §3.7): the first content term loads the content kinds once
	for _, q := range []string{"type:assembly", "type:xml-only has:problem", "harmony", "author:oskar -tag:ui", "id:2009463077", "hrmny"} {
		c := &m.cols[cInactive]
		c.filter = q
		t0 := time.Now()
		m.refilterCol(cInactive, "")
		t.Logf("search %-28q %4d of %d in %v", q, len(c.view), len(c.items), time.Since(t0))
	}
}
