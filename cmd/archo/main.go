// Command archo is RimArcho: a RimWorld mod manager. M1 ships the CLI only; the TUI (no
// arguments) and `archo mcp` arrive in later milestones.
package main

import (
	"fmt"
	"os"

	"github.com/rimarcho/rimarcho/internal/procmem"
	"github.com/rimarcho/rimarcho/internal/surface/cli"
)

func main() {
	if len(os.Args) == 1 {
		fmt.Fprintln(os.Stderr, "archo: the TUI is not built yet (milestone M2); run `archo help` for the CLI")
		os.Exit(2)
	}
	code := cli.Run(os.Args[1:], os.Stdout, os.Stderr)
	if os.Getenv("ARCHO_MEMSTATS") != "" {
		ws, priv, peak := procmem.MB()
		fmt.Fprintln(os.Stderr, fmt.Sprintf("memstats: working set %.1f MB, private %.1f MB, peak working set %.1f MB", ws, priv, peak))
	}
	os.Exit(code)
}
