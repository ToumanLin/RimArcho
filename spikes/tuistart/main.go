// Spike: cold start to an interactive list (01 §5.2): open the data directory, build the
// TUI model, run Init (workspace load + rows) and render the first frame, then exit.
// Time the whole process from outside (process start included):
//
//	Measure-Command { tuistart.exe -home $env:APPDATA\RimArcho }
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/surface/tui"
)

func main() {
	home := flag.String("home", "", "RimArcho data directory")
	flag.Parse()
	t0 := time.Now()
	a, err := app.Open(*home)
	if err != nil {
		panic(err)
	}
	tui.TuneProcess(a)
	a.Launcher = func(string, []string, string) error { panic("launch attempted") }
	m := tui.New(tui.Options{App: a, Scan: func(string) (string, error) { panic("scan attempted") }})
	m.Init()
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	frame := mm.View()
	rows := strings.Count(frame, "\n")
	fmt.Printf("first frame after %d ms in-process (%d lines, list shown: %v)\n", time.Since(t0).Milliseconds(), rows, strings.Contains(frame, "Core"))
	// T1 (06 §8): time each keypress (Update + View) over the whole list: scrolling, paging
	// and typing a search.
	var ds []time.Duration
	press := func(k tea.KeyMsg) {
		t := time.Now()
		mm, _ = mm.Update(k)
		mm.View()
		ds = append(ds, time.Since(t))
	}
	run := func(r string) { press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)}) }
	for i := 0; i < 300; i++ {
		press(tea.KeyMsg{Type: tea.KeyDown})
	}
	for i := 0; i < 30; i++ {
		press(tea.KeyMsg{Type: tea.KeyPgDown})
	}
	run("/")
	for _, r := range "harmony" {
		run(string(r))
	}
	press(tea.KeyMsg{Type: tea.KeyBackspace})
	press(tea.KeyMsg{Type: tea.KeyEsc})
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	fmt.Printf("keypresses: %d, median %.1f ms, p95 %.1f ms, max %.1f ms\n", len(ds), ms(ds[len(ds)/2]), ms(ds[len(ds)*95/100]), ms(ds[len(ds)-1]))
	os.Exit(0)
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
