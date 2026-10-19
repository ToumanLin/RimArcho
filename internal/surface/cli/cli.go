// Package cli is the command-line surface: it opens the data directory, strips global
// flags and hands the rest to internal/cmds.
package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/cmds"
	"github.com/rimarcho/rimarcho/internal/env"
	"github.com/rimarcho/rimarcho/internal/surface/mcp"
	"github.com/rimarcho/rimarcho/internal/surface/tui"
)

// Run executes argv (without the program name) and returns the exit code.
func Run(argv []string, stdout, stderr io.Writer) int {
	if len(argv) == 1 && (argv[0] == "--version" || argv[0] == "-v") {
		argv = []string{"version"}
	}
	envName, rest, err := globalFlags(argv)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return cmds.ExitUsage
	}
	ap, err := app.Open(env.AppHome())
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return cmds.ExitError
	}
	defer ap.Close()
	ap.EnvName = envName
	if len(rest) == 0 {
		// No command: the TUI (06). Scans run as a child process of this executable.
		if err := tui.Run(tui.Options{App: ap, ScanOnStart: true}); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return cmds.ExitError
		}
		return cmds.ExitOK
	}
	cmds.Progress = func(line string) { fmt.Fprintln(stderr, line) }
	if rest[0] == "mcp" && len(rest) == 1 {
		// MCP over stdio (05 §4): stdout carries only protocol messages.
		if err := mcp.Run(context.Background(), ap); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return cmds.ExitError
		}
		return cmds.ExitOK
	}
	out, code := cmds.Execute(ap, rest)
	if code == cmds.ExitError || code == cmds.ExitUsage {
		io.WriteString(stderr, out)
	} else {
		io.WriteString(stdout, out)
	}
	return code
}

// globalFlags removes --env <name> / --env=<name> from argv.
func globalFlags(argv []string) (envName string, rest []string, err error) {
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--":
			return envName, append(rest, argv[i:]...), nil
		case a == "--env":
			if i+1 >= len(argv) {
				return "", nil, fmt.Errorf("--env needs a name")
			}
			envName = argv[i+1]
			i++
		case strings.HasPrefix(a, "--env="):
			envName = strings.TrimPrefix(a, "--env=")
		default:
			rest = append(rest, a)
		}
	}
	return envName, rest, nil
}
