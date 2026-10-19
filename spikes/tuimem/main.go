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
	flag.Parse()
	debug.SetMemoryLimit(tui.MemoryLimit)
	a, err := app.Open(*home)
	if err != nil {
		panic(err)
	}
	a.Launcher = func(string, []string, string) error { panic("launch attempted") }
	report := func(what string) {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		ws, priv, peak := procmem.MB()
		fmt.Printf("%-24s working set %5.1f MB  private %5.1f MB  peak %5.1f MB  (go heap in use %.1f MB)\n", what, ws, priv, peak, float64(ms.HeapInuse)/(1<<20))
	}
	report("opened")
	if _, err := a.Loadout(); err != nil {
		panic(err)
	}
	report("workspace loaded")
	m := tui.New(tui.Options{App: a, Scan: func(string) (string, error) {
		out, err := exec.Command(*exe, "scan").CombinedOutput()
		return string(out), err
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
