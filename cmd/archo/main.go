// Command archo is RimArcho, a RimWorld mod manager: `archo` starts the TUI, `archo <command>`
// runs one CLI command (`archo help`), `archo mcp` serves MCP over stdio.
package main

import (
	"fmt"
	"github.com/ToumanLin/RimArcho/internal/accounts"
	"os"

	"github.com/ToumanLin/RimArcho/internal/procmem"
	"github.com/ToumanLin/RimArcho/internal/surface/cli"
)

func main() {
	if handled, code := accounts.Helper(os.Args[1:]); handled {
		os.Exit(code)
	}
	code := cli.Run(os.Args[1:], os.Stdout, os.Stderr)
	if os.Getenv("ARCHO_MEMSTATS") != "" {
		ws, priv, peak := procmem.MB()
		fmt.Fprintf(os.Stderr, "memstats: working set %.1f MB, private %.1f MB, peak working set %.1f MB\n", ws, priv, peak)
	}
	os.Exit(code)
}
