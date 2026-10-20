package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// keyLatency sends navigation and filter keys to a model and returns the time of each
// key's Update plus the View that follows it (what the player waits for, 06 §11 T1).
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
		step("ends", key("G"))
		step("ends", key("g"))
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

// 06 §11 T1 on a synthetic environment of 1354 mods (485 enabled, CJK names among them).
func TestT1KeyLatency1354(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and scans 1354 mods")
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
	if n := len(m.cols[cActive].items) + len(m.cols[cInactive].items); n != 1354 {
		t.Fatalf("%d mods in the columns", n)
	}
	if worst := report(t, keyLatency(t, m)); worst > 50*time.Millisecond {
		t.Fatalf("slowest key %v > 50 ms", worst)
	}
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
	// search (06 §5.5): the first content term loads the content kinds once
	for _, q := range []string{"type:assembly", "type:xml-only has:problem", "harmony", "author:oskar -tag:ui", "id:2009463077", "hrmny"} {
		c := &m.cols[cInactive]
		c.filter = q
		t0 := time.Now()
		m.refilterCol(cInactive, "")
		t.Logf("search %-28q %4d of %d in %v", q, len(c.view), len(c.items), time.Since(t0))
	}
}
