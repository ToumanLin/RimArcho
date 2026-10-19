// Package cmds is the single command set behind every surface (05 §1): argument parsing,
// execution against internal/app, and outline text output. CLI (and later MCP and the TUI
// command line) only translate their input into Execute calls.
package cmds

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/render"
)

// Flag describes one --flag.
type Flag struct {
	Name  string
	Value string // placeholder if the flag takes a value; "" for booleans
	Multi bool   // may repeat
	Help  string
}

// Command is one entry of the command table (05 §3).
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
	App *app.App
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

// Commands returns the command table in registration order.
func Commands() []*Command { return registry }

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
	if len(argv) == 0 || argv[0] == "help" || argv[0] == "--help" || argv[0] == "-h" {
		return Help().String(), ExitOK
	}
	c, rest := lookup(argv)
	if c == nil {
		return fmt.Sprintf("error: unknown command %q (archo help)\n", strings.Join(argv, " ")), ExitUsage
	}
	a, err := parse(c, rest)
	if err != nil {
		return "error: " + err.Error() + "\n", ExitUsage
	}
	doc, err := c.Run(&Ctx{App: ap}, a)
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
			return "error: " + ue.Error() + "\n", ExitUsage
		}
		return "error: " + err.Error() + "\n", ExitError
	}
	return doc.String(), ExitOK
}

// Help lists every command.
func Help() *render.Doc {
	d, root := render.New("archo — RimWorld mod manager (M1: environments, scanning, identity)")
	var rows [][]string
	for _, c := range registry {
		u := c.Name
		if c.Args != "" {
			u += " " + c.Args
		}
		rows = append(rows, []string{u, c.Summary})
	}
	root.Section("commands").AddColumns(rows)
	root.KV("references", "#<id> instance, L#<id> lineage, pkg:<packageId>, ws:<workshopId>, q:<n> question, src:<label> source, scan:<n>")
	root.KV("global flags", "--env <name> (use another environment for this command)")
	return d
}
