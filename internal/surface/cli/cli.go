// Package cli is the command-line surface: it opens the data directory, strips global
// flags and hands the rest to internal/cmds.
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/ToumanLin/RimArcho/internal/netcfg"
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
	"github.com/ToumanLin/RimArcho/internal/machine"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/surface/tui"
	"github.com/ToumanLin/RimArcho/internal/view"
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
		// The short-lived Steam client call (16 §2.9, 11 §2); no data directory.
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
	// Maintainer dry runs assemble local release files without creating a player home.
	if cmds.WorkshopPublishOffline(rest) {
		out, code := cmds.ExecuteContext(context.Background(), &app.App{}, rest)
		if code == cmds.ExitOK {
			io.WriteString(stdout, out)
		} else {
			io.WriteString(stderr, out)
		}
		return code
	}
	// `archo mcp [--tools core|all]` (14 §1); `archo mcp --help` runs as a command. A bad
	// flag fails before the data folder is opened.
	serveMCP := len(rest) > 0 && rest[0] == "mcp" && !(len(rest) > 1 && (rest[len(rest)-1] == "--help" || rest[len(rest)-1] == "-h"))
	var group mcp.Group
	if serveMCP {
		if group, err = mcpGroup(rest); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return cmds.ExitUsage
		}
	}
	// A database upgrade runs inside app.Open; show it (04 §4): RimArcho.exe has no console,
	// so it gets a small window; the TUI's line follows ui.lang.
	home, err := env.AppHome()
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		if buildinfo.GUI == "1" {
			showOpenError("RimArcho", err.Error())
		}
		return cmds.ExitError
	}
	netcfg.Install() // proxy and GitHub mirror for every request of this process (17 §2.2)
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
		// What an update left in the program folder (17 §5.1); never delays the first paint.
		go ap.CleanSelfUpdate()
	}
	if (len(rest) == 0 && buildinfo.GUI == "1") || (len(rest) == 1 && rest[0] == "gui") {
		// RimArcho.exe (a double click) or `archo gui` (16 §8.1).
		if err := runGUI(ap, len(rest) == 1); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			if buildinfo.GUI == "1" { // no console: say it in a message box (16 §8.1)
				showOpenError("RimArcho", windowFailureText(err))
			}
			return cmds.ExitError
		}
		return cmds.ExitOK
	}
	if program, err := ap.SelfProgramDir(); err == nil && app.SelfUpdateSupported() {
		if _, ok := selfupdate.InWorkshopFolder(program); ok {
			fmt.Fprintln(stderr, "warning: Steam can replace this Workshop folder; open RimArcho.exe to move RimArcho")
		}
	}
	if len(rest) == 0 || (len(rest) == 1 && rest[0] == "tui") {
		// No command, or `archo tui`: run the TUI in the current terminal.
		// Scans run as a child process of this executable.
		if !terminalDrawsTUI() {
			return noTUI(ap, stderr)
		}
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
		// MCP over stdio (14): stdout carries only protocol messages.
		if err := mcp.Run(context.Background(), ap, group); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return cmds.ExitError
		}
		return cmds.ExitOK
	}
	ctx := context.Background()
	// Ctrl+C stops a test run, or an update waiting for the other RimArcho to close.
	if len(rest) >= 2 && rest[0] == "test" && rest[1] == "run" || rest[0] == "update" || len(rest) >= 2 && rest[0] == "workshop" && rest[1] == "publish" {
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
// its exit code (17 §5.1). Windows cannot replace a running process, so this one waits; the
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

// Tests replace these.
var (
	webView2Installed = accounts.WebView2Installed
	underWine         = machine.Wine
	terminalDrawsTUI  = machine.TerminalDrawsTUI
	ownConsole        = machine.OwnConsole
)

// windowUnavailable is why the window cannot open on this machine; its text is complete.
type windowUnavailable struct{ text string }

func (e *windowUnavailable) Error() string { return e.text }

// guiPreflight checks what the window needs before Wails starts (16 §8.1). Without a
// WebView2 runtime Wails creates no window and only logs, so the process would run unseen.
// Under Wine an installed WebView2 is given its chance.
func guiPreflight() error {
	if runtime.GOOS != "windows" || webView2Installed() {
		return nil
	}
	if underWine() {
		return &windowUnavailable{view.T("RimArcho's window needs Microsoft Edge WebView2, which does not run under Wine or Proton.\n\nUse archo.exe from a Linux terminal instead (wine archo.exe): the same manager as a text interface.")}
	}
	return &windowUnavailable{view.T("RimArcho's window needs the Microsoft Edge WebView2 Runtime, which is not installed.\n\nInstall it from https://developer.microsoft.com/microsoft-edge/webview2/ and start RimArcho again. archo.exe, the text interface, works without it.")}
}

// windowFailureText is the message box text for a window that did not open.
func windowFailureText(err error) string {
	var u *windowUnavailable
	if errors.As(err, &u) {
		return u.text
	}
	return view.Tf("RimArcho could not open its window:\n%s", err.Error())
}

// noTUI explains, in plain lines, a console that would show the TUI's escape sequences as
// text (Wine's console window, 16 §9.2). A double click owns its console, which closes
// when archo exits, so it waits for Enter.
func noTUI(ap *app.App, stderr io.Writer) int {
	view.SetLang(cmds.Setting(ap, "ui.lang"))
	fmt.Fprintln(stderr, view.T("This console cannot draw RimArcho's text interface."))
	if underWine() {
		fmt.Fprintln(stderr, view.T("Under Wine or Proton, run archo.exe from a Linux terminal: wine archo.exe"))
	} else {
		fmt.Fprintln(stderr, view.T("For the window, open RimArcho.exe."))
	}
	fmt.Fprintln(stderr, view.T("Commands still work here: archo help"))
	if ownConsole() {
		fmt.Fprintln(stderr, view.T("Press Enter to close."))
		bufio.NewReader(os.Stdin).ReadString('\n')
	}
	return cmds.ExitError
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
// list without making it current (07 §1.3; the GUI's analysis and pack build children).
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
// Steam runtime folder. Maintainer uploads instead pass `publish <request.json>`.
// It is not a listed command; the public commands live under `archo workshop`.
func steamWorker(argv []string, stdout io.Writer) int {
	if len(argv) > 0 && argv[0] == steamworks.Publish {
		if len(argv) != 2 {
			fmt.Fprintln(stdout, steamworks.ErrorLine("usage: archo steam-worker publish <request.json>"))
			return cmds.ExitUsage
		}
		r, err := steamworks.ReadPublishRequest(argv[1])
		if err != nil {
			fmt.Fprintln(stdout, steamworks.ErrorLine(err.Error()))
			return cmds.ExitUsage
		}
		return steamworks.PublishWorker(r, stdout)
	}
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

// mcpGroup reads `archo mcp [--tools core|all]` (14 §1); a bad flag or value is a usage error.
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
