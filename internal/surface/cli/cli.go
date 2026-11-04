// Package cli is the command-line surface: it opens the data directory, strips global
// flags and hands the rest to internal/cmds.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/surface/gui"
	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/surface/tui"
)

// Run executes argv (without the program name) and returns the exit code.
func Run(argv []string, stdout, stderr io.Writer) int {
	if len(argv) >= 2 && argv[0] == "account" && argv[1] == "login-check" {
		if len(argv) != 3 || argv[2] != "loverslab" {
			fmt.Fprintln(stderr, "usage: archo account login-check loverslab")
			return cmds.ExitUsage
		}
		log, err := accounts.LoginCheck()
		fmt.Fprintln(stdout, "Login helper log:", log)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return cmds.ExitError
		}
		fmt.Fprintln(stdout, "WebView2 window check completed")
		return cmds.ExitOK
	}

	if len(argv) >= 1 && argv[0] == "steam-worker" {
		// The short-lived Steam client call (06 §2.9, 08 §10); no data directory.
		return steamWorker(argv[1:], stdout)
	}
	if len(argv) == 1 && (argv[0] == "--version" || argv[0] == "-v") {
		argv = []string{"version"}
	}
	envName, rest, err := globalFlags(argv)
	var openPlan string
	if err == nil {
		openPlan, rest, err = openPlanFlag(rest)
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return cmds.ExitUsage
	}
	// `archo mcp [--tools core|all]` (05 §5.1); `archo mcp --help` runs as a command. A bad
	// flag fails before the data folder is opened.
	serveMCP := len(rest) > 0 && rest[0] == "mcp" && !(len(rest) > 1 && (rest[len(rest)-1] == "--help" || rest[len(rest)-1] == "-h"))
	var group mcp.Group
	if serveMCP {
		if group, err = mcpGroup(rest); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return cmds.ExitUsage
		}
	}
	// A database upgrade runs inside app.Open; show it (02 §7): RimArcho.exe has no console,
	// so it gets a small window; the TUI's line follows ui.lang.
	home := env.AppHome()
	tuiMode := buildinfo.GUI != "1" && (len(rest) == 0 || len(rest) == 1 && rest[0] == "tui")
	up := &upgradeReport{window: buildinfo.GUI == "1", lang: buildinfo.GUI == "1" || tuiMode, dbPath: filepath.Join(home, "archo.db"), stderr: stderr}
	ap, err := app.OpenWith(home, app.OpenOptions{Upgrade: up.progress})
	up.finish(err == nil)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		up.openFailed(err)
		return cmds.ExitError
	}
	defer ap.Close()
	ap.EnvName, ap.PlanName = envName, openPlan
	interactive := len(rest) == 0 || len(rest) == 1 && (rest[0] == "gui" || rest[0] == "tui")
	if interactive {
		// What an update left in the program folder (07 §5.1); never delays the first paint.
		go ap.CleanSelfUpdate()
	}
	if (len(rest) == 0 && buildinfo.GUI == "1") || (len(rest) == 1 && rest[0] == "gui") {
		// RimArcho.exe (a double click) or `archo gui` (06 §8.1).
		if err := runGUI(ap, len(rest) == 1); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return cmds.ExitError
		}
		return cmds.ExitOK
	}
	if len(rest) == 0 || (len(rest) == 1 && rest[0] == "tui") {
		// No command, or `archo tui`: run the TUI in the current terminal.
		// Scans run as a child process of this executable.
		err := tui.Run(tui.Options{App: ap, ScanOnStart: true})
		var restart *tui.Restart
		if errors.As(err, &restart) {
			ap.Close() // the new version may upgrade the database
			return restartTUI(restart.Exe, argv, stderr)
		}
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return cmds.ExitError
		}
		return cmds.ExitOK
	}
	cmds.Progress = func(line string) { fmt.Fprintln(stderr, line) }
	cmds.Stream = func(line string) { fmt.Fprintln(stdout, line) }
	if serveMCP {
		// MCP over stdio (05 §5): stdout carries only protocol messages.
		if err := mcp.Run(context.Background(), ap, group); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return cmds.ExitError
		}
		return cmds.ExitOK
	}
	ctx := context.Background()
	// Ctrl+C stops a test run, or an update waiting for the other RimArcho to close.
	if len(rest) >= 2 && rest[0] == "test" && rest[1] == "run" || rest[0] == "update" {
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(ctx, os.Interrupt)
		defer stop()
	}
	out, code := cmds.ExecuteContext(ctx, ap, rest)
	// A failed test outcome still prints its report to stdout.
	if code == cmds.ExitError && strings.HasPrefix(out, "error: ") || code == cmds.ExitUsage {
		io.WriteString(stderr, out)
	} else {
		io.WriteString(stdout, out)
	}
	return code
}

// restartTUI runs the updated archo in this terminal with the same arguments and returns
// its exit code (07 §5.1). Windows cannot replace a running process, so this one waits; the
// new one leaves the old program file for the next start to remove.
func restartTUI(exe string, argv []string, stderr io.Writer) int {
	c := exec.Command(exe, argv...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	c.Env = append(os.Environ(), app.UpdatedFromEnv+"="+strconv.Itoa(os.Getpid()))
	if err := c.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		fmt.Fprintln(stderr, "error: RimArcho was updated, but the new version did not start:", err)
		return cmds.ExitError
	}
	return cmds.ExitOK
}

// runGUI opens the GUI. `archo gui` hands over to RimArcho.exe next to it when there is
// one (no console stays open), else it runs the GUI in this process.
func runGUI(ap *app.App, fromConsole bool) error {
	if fromConsole && runtime.GOOS == "windows" {
		if exe, err := os.Executable(); err == nil {
			sib := filepath.Join(filepath.Dir(exe), "RimArcho.exe")
			if !strings.EqualFold(sib, exe) {
				if _, err := os.Stat(sib); err == nil {
					var args []string
					if ap.EnvName != "" {
						args = []string{"--env", ap.EnvName}
					}
					c := exec.Command(sib, args...)
					c.Env = append(os.Environ(), "RIMARCHO_HOME="+ap.Home)
					return c.Start()
				}
			}
		}
	}
	return gui.Run(gui.Options{App: ap})
}

// globalFlags removes --env <name> / --env=<name> and --here from argv.
func globalFlags(argv []string) (envName string, rest []string, err error) {
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--":
			return envName, append(rest, argv[i:]...), nil
		case a == "--here" && len(rest) == 0:
			// Accepted before a command for compatibility with old shortcuts; running here is always the default.
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

// openPlanFlag removes --open-plan <name> / --open-plan=<name>: the command acts on that
// list without making it current (04 §1.3; the GUI's analysis and pack build children).
func openPlanFlag(argv []string) (plan string, rest []string, err error) {
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--":
			return plan, append(rest, argv[i:]...), nil
		case a == "--open-plan":
			if i+1 >= len(argv) {
				return "", nil, fmt.Errorf("--open-plan needs a list name")
			}
			plan = argv[i+1]
			i++
		case strings.HasPrefix(a, "--open-plan="):
			plan = strings.TrimPrefix(a, "--open-plan=")
		default:
			rest = append(rest, a)
		}
	}
	return plan, rest, nil
}

// steamWorker runs `archo steam-worker <action> <id>…`, started by app.Workshop in the
// Steam runtime folder. It is not a listed command: players and agents use
// `archo workshop subscribe|unsubscribe|state`.
func steamWorker(argv []string, stdout io.Writer) int {
	if len(argv) < 2 {
		fmt.Fprintln(stdout, steamworks.ErrorLine("usage: archo steam-worker subscribe|unsubscribe|state|download|watch <workshop id>…"))
		return cmds.ExitUsage
	}
	var ids []uint64
	for _, a := range argv[1:] {
		var id uint64
		if _, err := fmt.Sscan(a, &id); err != nil || id == 0 {
			fmt.Fprintln(stdout, steamworks.ErrorLine("not a Workshop id: "+a))
			return cmds.ExitUsage
		}
		ids = append(ids, id)
	}
	return steamworks.Worker(argv[0], ids, stdout)
}

// mcpGroup reads `archo mcp [--tools core|all]` (05 §5.1); a bad flag or value is a usage error.
func mcpGroup(rest []string) (mcp.Group, error) {
	v, err := cmds.MCPTools(rest)
	if err != nil {
		return "", err
	}
	g, err := mcp.ParseGroup(v)
	if err != nil {
		c, _ := cmds.Find([]string{"mcp"})
		return "", &cmds.UsageError{Msg: err.Error(), Cmd: c}
	}
	return g, nil
}
