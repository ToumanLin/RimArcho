// Spike: TUI resident memory in a lean process (no test framework, no output capture).
// Runs the real TUI model headless against a data directory, scans once through the
// archo binary as a child process, browses (navigation and search keys only), then idles.
//
//	go run ./spikes/tuimem -home %APPDATA%\RimArcho -exe bin\archo.exe -idle 120s
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/surface/tui"
	"github.com/rimarcho/rimarcho/spikes/internal/procmem"
)

func main() {
	home := flag.String("home", "", "RimArcho data directory")
	exe := flag.String("exe", "", "archo binary for the child scan")
	idle := flag.Duration("idle", 2*time.Minute, "idle time after browsing")
	browse := flag.Bool("browse", true, "scan and browse before idling")
	cache := flag.Int("cache", 0, "PRAGMA cache_size for the TUI connection (0: default)")
	gc := flag.Int("gc", 0, "GOGC (0: default)")
	problems := flag.Bool("problems", true, "open the Problems view (child analysis) and Inspect")
	logSel := flag.Bool("log", true, "press l and analyse the current Player.log in the TUI process (restored afterwards)")
	flag.Parse()
	a, err := app.Open(*home)
	if err != nil {
		panic(err)
	}
	tui.TuneProcess(a)
	a.Launcher = func(string, []string, string) error { panic("launch attempted") }
	report := func(what string) {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		ws, priv, peak := procmem.MB()
		fmt.Printf("%-24s working set %5.1f MB  private %5.1f MB  peak %5.1f MB  (go heap in use %.1f MB)\n", what, ws, priv, peak, float64(ms.HeapInuse)/(1<<20))
	}
	if *cache != 0 {
		a.Store.DB.Exec(fmt.Sprintf("PRAGMA cache_size=%d", *cache))
	}
	if *gc != 0 {
		debug.SetGCPercent(*gc)
	}
	report("opened")
	if _, err := a.Loadout(); err != nil {
		panic(err)
	}
	report("workspace loaded")
	st0, _ := a.Loadout()
	selKey, seenKey := "log.selected:"+st0.Ix.Env.ID, "log.seen:"+st0.Ix.Env.ID
	sel0, seen0 := a.Store.Meta(selKey), a.Store.Meta(seenKey)
	defer func() { a.Store.SetMeta(selKey, sel0); a.Store.SetMeta(seenKey, seen0) }()
	m := tui.New(tui.Options{App: a, Scan: func(string) (string, error) {
		out, err := exec.Command(*exe, "scan").CombinedOutput()
		return string(out), err
	}, Analyze: func(env string, progress func(string)) error {
		return app.ChildAnalyzeExe(*exe, a.Home, env, "", progress)
	}})
	p := tea.NewProgram(m, tea.WithInput(nil), tea.WithOutput(io.Discard))
	go func() {
		key := func(s string) { p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}) }
		p.Send(tea.WindowSizeMsg{Width: 120, Height: 30})
		time.Sleep(2 * time.Second)
		report("list shown")
		if *browse {
			key("R")
			time.Sleep(8 * time.Second)
			for i := 0; i < 60; i++ {
				key("j")
				time.Sleep(150 * time.Millisecond)
			}
			key("/")
			key("har")
			p.Send(tea.KeyMsg{Type: tea.KeyEnter})
			p.Send(tea.KeyMsg{Type: tea.KeyEsc})
			for _, k := range []string{"G", "g", "f", "f", "f", "f"} {
				key(k)
			}
			time.Sleep(time.Second)
			report("after scan + browsing")
		}
		if *problems {
			key("2")
			time.Sleep(15 * time.Second) // child analysis when stale
			report("problems view shown")
			for i := 0; i < 40; i++ {
				key("j")
				if i%5 == 0 {
					p.Send(tea.KeyMsg{Type: tea.KeyEnter})
				}
				time.Sleep(100 * time.Millisecond)
			}
			key("o")
			time.Sleep(500 * time.Millisecond)
			p.Send(tea.KeyMsg{Type: tea.KeyTab})
			p.Send(tea.KeyMsg{Type: tea.KeyTab})
			time.Sleep(500 * time.Millisecond)
			p.Send(tea.KeyMsg{Type: tea.KeyEsc})
			key("1")
			time.Sleep(time.Second)
			report("after problems + inspect")
		}
		if *logSel {
			key("l")
			time.Sleep(500 * time.Millisecond)
			p.Send(tea.KeyMsg{Type: tea.KeyEnter})
			time.Sleep(5 * time.Second)
			report("log analysed (l)")
			for i := 0; i < 20; i++ {
				key("j")
				time.Sleep(100 * time.Millisecond)
			}
			key("1")
			time.Sleep(time.Second)
		}
		step := *idle / 4
		for i := 1; i <= 4; i++ {
			time.Sleep(step)
			report(fmt.Sprintf("idle %v", time.Duration(i)*step))
		}
		p.Quit()
	}()
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
