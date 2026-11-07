// Command monitoricon writes Archo Monitor's About/ModIcon.png and About/Preview.png from
// RimArcho's icon (16 §8.6), so no PNG is kept in the repository. monitor/build.ps1 runs it.
//
//	go run ./scripts/monitoricon -o monitor/out/ArchoMonitor/About
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/icon"
)

func main() {
	out := flag.String("o", "monitor/out/ArchoMonitor/About", "the mod's About folder")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for name, size := range map[string]int{"ModIcon.png": 256, "Preview.png": 512} {
		if err := os.WriteFile(filepath.Join(*out, name), icon.PNG(size), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
