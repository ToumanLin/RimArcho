// Spike: Bubble Tea 500-row CJK list; -bench runs headless latency benchmark.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	bench := flag.Bool("bench", false, "headless benchmark (writes RESULT.txt)")
	e2eFlag := flag.Bool("e2e", false, "include end-to-end program run in bench")
	rows := flag.Int("rows", 500, "row count")
	flag.Parse()
	if *bench {
		runBench(*e2eFlag)
		return
	}
	if _, err := tea.NewProgram(newModel(*rows), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
