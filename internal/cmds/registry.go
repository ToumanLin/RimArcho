// Package cmds is the single command set behind every surface (05 §1.1): argument parsing,
// execution against internal/app, and outline text output. CLI (and later MCP and the TUI
// command line) only translate their input into Execute calls.
package cmds

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Flag describes one --flag.
type Flag struct {
	Name  string
	Value string // placeholder if the flag takes a value; "" for booleans
	Multi bool   // may repeat
	Help  string
}

// Command is one entry of the command table (05 §4.2).
type Command struct {
	Name    string // words, e.g. "env list"
	Args    string // positional usage, e.g. "<ref>"
	Summary string
	Flags   []Flag
	MinArgs int
	MaxArgs int // -1 = unbounded
	Run     func(c *Ctx, a *Args) (*render.Doc, error)
}

// Usage renders the command's one-line usage.
func (c *Command) Usage() string {
	s := "archo " + c.Name
	if c.Args != "" {
		s += " " + c.Args
	}
	for _, f := range c.Flags {
		if f.Value != "" {
			s += fmt.Sprintf(" [--%s %s]", f.Name, f.Value)
		} else {
			s += fmt.Sprintf(" [--%s]", f.Name)
		}
	}
	return s
}

// Ctx is passed to every command.
type Ctx struct {
	App     *app.App
	Context context.Context
	// ExitCode reports an unsuccessful outcome with normal command output.
	ExitCode int

	st     *app.LoadoutState
	stErr  error
	stDone bool
}

// Loadout returns the workspace, loaded once per command (nil if unavailable).
func (c *Ctx) Loadout() *app.LoadoutState {
	if !c.stDone {
		c.st, c.stErr = c.App.Loadout()
		c.stDone = true
	}
	return c.st
}

// Args are parsed positional arguments and flags.
type Args struct {
	Pos   []string
	flags map[string][]string
}

// Bool reports whether a boolean flag was given.
func (a *Args) Bool(name string) bool { _, ok := a.flags[name]; return ok }

// Str returns a flag's last value ("" if absent).
func (a *Args) Str(name string) string {
	v := a.flags[name]
	if len(v) == 0 {
		return ""
	}
	return v[len(v)-1]
}

// List returns all values of a repeatable flag.
func (a *Args) List(name string) []string { return a.flags[name] }

var registry []*Command

func register(c *Command) { registry = append(registry, c) }

// order is the documented command order (05 §4.2); registration order depends on file names.
var commandOrder = []string{"env list", "env detect", "env add", "env set", "env use", "env rebind", "env remove", "scan", "status", "list", "find",
	"mod", "describe", "def", "patches", "harmony", "why", "log", "problems", "problems show", "problems ignore", "problems unignore", "files search", "source", "source search", "analyze", "changes", "library", "library remove-from-plans", "library move-to-locked", "enable", "disable", "move", "swap", "sort", "undo", "redo", "save", "loadout log", "loadout diff",
	"loadout checkout", "loadout ignore-external", "loadout complete-dependencies", "loadout replace-recommended", "loadout use-installed", "loadout export", "loadout import", "plan list", "plan new", "plan use", "plan rename", "plan remove", "plan cost", "stage list", "stage add", "stage drop", "stage restore", "pin", "copy-local", "import rimcrow", "alias", "note", "tag", "rule add", "rule list", "rule remove", "launch", "test run", "identity", "identity list", "identity why", "identity resolve", "lineage split", "lineage merge", "gc", "config get", "config set", "mcp", "gui", "version", "uninstall"}

// Commands returns the command table in documented order.
func Commands() []*Command {
	out := make([]*Command, 0, len(registry))
	for _, n := range commandOrder {
		for _, c := range registry {
			if c.Name == n {
				out = append(out, c)
			}
		}
	}
	for _, c := range registry {
		if !slices.Contains(commandOrder, c.Name) {
			out = append(out, c)
		}
	}
	return out
}

// lookup finds the command with the longest name matching the leading words.
func lookup(words []string) (*Command, []string) {
	var best *Command
	bestN := 0
	for _, c := range registry {
		parts := strings.Fields(c.Name)
		if len(parts) > len(words) || len(parts) <= bestN {
			continue
		}
		ok := true
		for i, p := range parts {
			if words[i] != p {
				ok = false
				break
			}
		}
		if ok {
			best, bestN = c, len(parts)
		}
	}
	if best == nil {
		return nil, words
	}
	return best, words[bestN:]
}

// UsageError is a bad invocation.
type UsageError struct {
	Msg string
	Cmd *Command
}

func (e *UsageError) Error() string {
	if e.Cmd != nil {
		return e.Msg + "\nusage: " + e.Cmd.Usage()
	}
	return e.Msg
}

func parse(c *Command, rest []string) (*Args, error) {
	a := &Args{flags: map[string][]string{}}
	for i := 0; i < len(rest); i++ {
		w := rest[i]
		if w == "--" {
			a.Pos = append(a.Pos, rest[i+1:]...)
			break
		}
		if !strings.HasPrefix(w, "--") || len(w) == 2 {
			a.Pos = append(a.Pos, w)
			continue
		}
		name, val, hasVal := strings.Cut(w[2:], "=")
		var f *Flag
		for j := range c.Flags {
			if c.Flags[j].Name == name {
				f = &c.Flags[j]
			}
		}
		if f == nil {
			return nil, &UsageError{Msg: fmt.Sprintf("unknown flag --%s", name), Cmd: c}
		}
		if f.Value == "" {
			if hasVal {
				return nil, &UsageError{Msg: fmt.Sprintf("--%s takes no value", name), Cmd: c}
			}
			a.flags[name] = append(a.flags[name], "")
			continue
		}
		if !hasVal {
			if i+1 >= len(rest) {
				return nil, &UsageError{Msg: fmt.Sprintf("--%s needs a value", name), Cmd: c}
			}
			i++
			val = rest[i]
		}
		if len(a.flags[name]) > 0 && !f.Multi {
			return nil, &UsageError{Msg: fmt.Sprintf("--%s given twice", name), Cmd: c}
		}
		a.flags[name] = append(a.flags[name], val)
	}
	if len(a.Pos) < c.MinArgs || (c.MaxArgs >= 0 && len(a.Pos) > c.MaxArgs) {
		return nil, &UsageError{Msg: "wrong number of arguments", Cmd: c}
	}
	return a, nil
}

// Exit codes.
const (
	ExitOK        = 0
	ExitError     = 1
	ExitUsage     = 2
	ExitAmbiguous = 3
)

// Execute runs one command line (without the program name) and returns its output text and
// exit code. Errors are rendered as text too, so every surface prints the same thing.
func Execute(ap *app.App, argv []string) (string, int) {
	return ExecuteContext(context.Background(), ap, argv)
}

// ExecuteContext is Execute with cancellation for commands that wait for a process.
func ExecuteContext(ctx context.Context, ap *app.App, argv []string) (string, int) {
	if len(argv) == 0 || argv[0] == "help" || argv[0] == "--help" || argv[0] == "-h" {
		return Help().String(), ExitOK
	}
	if ap.Uninstalled {
		return "error: RimArcho was uninstalled; close it\n", ExitError
	}
	c, rest := lookup(argv)
	if c == nil {
		return fmt.Sprintf("error: unknown command %q (archo help)\n", strings.Join(argv, " ")), ExitUsage
	}
	if len(rest) > 0 && (rest[len(rest)-1] == "--help" || rest[len(rest)-1] == "-h") {
		return CommandHelp(c).String(), ExitOK
	}
	a, err := parse(c, rest)
	if err != nil {
		return "error: " + err.Error() + "\n", ExitUsage
	}
	if v := a.Str("game-version"); v != "" {
		if !gameVersionRe.MatchString(v) {
			return "error: --game-version must look like 1.6\n", ExitUsage
		}
		prev := ap.GameVersion
		ap.GameVersion = v
		defer func() { ap.GameVersion = prev }()
	}
	cctx := &Ctx{App: ap, Context: ctx}
	doc, err := c.Run(cctx, a)
	if err != nil {
		var amb *app.Ambiguous
		if errors.As(err, &amb) {
			return ambiguousDoc(ap, amb, c).String(), ExitAmbiguous
		}
		var ue *UsageError
		if errors.As(err, &ue) {
			if ue.Cmd == nil {
				ue.Cmd = c
			}
			return render.ShellSafe("error: " + ue.Error() + "\n"), ExitUsage
		}
		// Errors suggest commands too ("x is not enabled (archo enable #x)").
		return render.ShellSafe("error: " + err.Error() + "\n"), ExitError
	}
	return doc.String(), cctx.ExitCode
}

// Help lists every command.
func Help() *render.Doc {
	d, root := render.New("archo — RimWorld mod manager (CLI; run archo without arguments for the TUI)")
	var rows [][]string
	for _, c := range Commands() {
		u := c.Name
		if c.Args != "" {
			u += " " + c.Args
		}
		rows = append(rows, []string{u, c.Summary})
	}
	root.Section("commands").AddColumns(rows)
	root.KV("references", "#<id> instance, L#<id> lineage, pkg:<packageId>, ws:<workshopId>, q:<n> question, src:<label> source, scan:<n>, pattern:<id> error pattern")
	root.KV("global flags", "--env <name> (use another environment for this command)")
	return d
}

var gameVersionRe = regexp.MustCompile(`^\d+\.\d+$`)

// gameVersionFlag is accepted by the analysis commands (03 §3).
var gameVersionFlag = Flag{Name: "game-version", Value: "<X.Y>", Help: "analyse for another game version than the environment's"}

// CommandHelp is `archo <command> --help`: the usage line, the summary and each flag.
func CommandHelp(c *Command) *render.Doc {
	d, root := render.New(c.Usage())
	root.Add(c.Summary)
	if len(c.Flags) > 0 {
		fl := root.Section("flags")
		for _, f := range c.Flags {
			l := "--" + f.Name
			if f.Value != "" {
				l += " " + f.Value
			}
			if f.Help != "" {
				l += "  " + f.Help
			}
			fl.Add(l)
		}
	}
	return d
}
