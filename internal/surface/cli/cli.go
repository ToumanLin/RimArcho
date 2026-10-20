// Package cli is the command-line surface: it opens the data directory, strips global
// flags and hands the rest to internal/cmds.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/host"
	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/surface/tui"
)

// Run executes argv (without the program name) and returns the exit code.
func Run(argv []string, stdout, stderr io.Writer) int {
	if len(argv) == 1 && (argv[0] == "--version" || argv[0] == "-v") {
		argv = []string{"version"}
	}
	envName, here, rest, err := globalFlags(argv)
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
	if wantsHost(rest, here) {
		// A bare `archo` (a double click): run the TUI in RimArcho's WezTerm host when it is
		// there or the player agrees to download it (06 §1.1).
		if launched, err := runHost(ap, envName, stdout); err != nil {
			fmt.Fprintln(stderr, "error:", err)
		} else if launched {
			return cmds.ExitOK
		}
	}
	if len(rest) == 0 || (len(rest) == 1 && rest[0] == "tui") {
		// No command, or `archo tui` (what the WezTerm host runs, 06 §1.1): the TUI.
		// Scans run as a child process of this executable.
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

// wantsHost: only a bare `archo` may start the WezTerm host; `archo tui` (what the host
// runs), `archo --here`, every CLI command and `archo mcp` never do (06 §1.1).
func wantsHost(rest []string, here bool) bool { return len(rest) == 0 && !here }

// runHost is replaced in tests.
var runHost = func(ap *app.App, envName string, out io.Writer) (bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	var args []string
	if envName != "" {
		args = []string{"--env", envName}
	}
	h := &host.Host{
		Home: ap.Home, ProgramDir: filepath.Dir(exe), Exe: exe, Args: args, API: host.GitHubAPI,
		Out: out, Getenv: os.Getenv, LookPath: exec.LookPath,
		Start:   func(name string, args ...string) error { return startAlive(ap.Home, name, args...) },
		Pref:    func() string { return ap.Store.Meta("cfg:terminal.host") },
		SetPref: func(v string) error { return ap.Store.SetMeta("cfg:terminal.host", v) },
	}
	for _, k := range []string{"ProgramW6432", "ProgramFiles", "LOCALAPPDATA"} {
		if d := os.Getenv(k); d != "" {
			if k == "LOCALAPPDATA" {
				d = filepath.Join(d, "Programs")
			}
			h.ProgramFiles = append(h.ProgramFiles, d)
		}
	}
	if st, err := os.Stdin.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
		h.In = os.Stdin // a console to ask on
	}
	return h.Run()
}

// globalFlags removes --env <name> / --env=<name> and --here from argv.
func globalFlags(argv []string) (envName string, here bool, rest []string, err error) {
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--":
			return envName, here, append(rest, argv[i:]...), nil
		case a == "--here" && len(rest) == 0:
			here = true // only before a command: `archo --here` (06 §1.1)
		case a == "--env":
			if i+1 >= len(argv) {
				return "", false, nil, fmt.Errorf("--env needs a name")
			}
			envName = argv[i+1]
			i++
		case strings.HasPrefix(a, "--env="):
			envName = strings.TrimPrefix(a, "--env=")
		default:
			rest = append(rest, a)
		}
	}
	return envName, here, rest, nil
}
