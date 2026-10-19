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
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/surface/tui"
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
	os.Exit(0)
}
