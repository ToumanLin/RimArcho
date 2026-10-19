// Command archo is RimArcho, a RimWorld mod manager: `archo` starts the TUI, `archo <command>`
// runs one CLI command (`archo help`), `archo mcp` serves MCP over stdio.
package main

import (
	"fmt"
	"os"

	"github.com/rimarcho/rimarcho/internal/procmem"
	"github.com/rimarcho/rimarcho/internal/surface/cli"
)

func main() {
	code := cli.Run(os.Args[1:], os.Stdout, os.Stderr)
	if os.Getenv("ARCHO_MEMSTATS") != "" {
		ws, priv, peak := procmem.MB()
		fmt.Fprintln(os.Stderr, fmt.Sprintf("memstats: working set %.1f MB, private %.1f MB, peak working set %.1f MB", ws, priv, peak))
	}
	os.Exit(code)
}
