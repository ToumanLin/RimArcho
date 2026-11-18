package cmds

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Archo Agent (15 §7): install, configure and start the separate agent download.
func init() {
	register(&Command{Name: "agent status", Summary: "show whether Archo Agent is installed and how its model is set (15 §7)", MaxArgs: 0, Run: agentStatusCmd})
	register(&Command{Name: "agent install", Summary: "download, verify and install the latest Archo Agent, or install a downloaded bundle zip",
		Flags:   []Flag{{Name: "from", Value: "zip", Help: "install this bundle zip (and the .sha256 beside it, when there is one) instead of downloading"}},
		MaxArgs: 0, Run: agentInstallCmd})
	register(&Command{Name: "agent remove", Summary: "delete the installed Archo Agent; its settings, key and conversations stay", MaxArgs: 0, Run: agentRemoveCmd})
	register(&Command{Name: "agent key", Summary: "store the model API key in Windows Credential Manager, read from one line of standard input; --clear removes it",
		Flags: []Flag{{Name: "clear", Help: "remove the stored key"}}, MaxArgs: 0, Run: agentKeyCmd})
	register(&Command{Name: "agent run", Summary: "start Archo Agent in this terminal (the GUI opens it in a window of its own)",
		Flags: []Flag{{Name: "window", Help: "open it in a new console window and return"}}, MaxArgs: 0, Run: agentRunCmd})
}

func agentStatusCmd(c *Ctx, a *Args) (*render.Doc, error) {
	s := c.App.AgentStatus()
	title := "Archo Agent: not installed"
	if s.Installed {
		title = "Archo Agent " + s.Manifest.Version
	}
	d, root := render.New(title)
	if s.Installed {
		root.KV("folder", s.Dir)
		root.KV("pi", s.Manifest.Pi)
		root.KV("node", s.Manifest.Node)
	}
	root.KV("api", s.API)
	root.KV("base_url", orNone(s.BaseURL))
	root.KV("model", orNone(s.Model))
	root.KV("thinking", s.Thinking)
	root.KV("key", map[bool]string{true: "stored in Credential Manager", false: "(none)"}[s.HasKey])
	root.KV("config", s.ConfigDir)
	switch {
	case !s.Installed:
		d.AddNext("archo agent install")
	case s.BaseURL == "" || s.Model == "":
		d.AddNext("archo config set agent.base_url <url>")
		d.AddNext("archo config set agent.model <model>")
	case !s.HasKey:
		d.AddNext("archo agent key")
	default:
		d.AddNext("archo agent run")
	}
	return d, nil
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func agentInstallCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ctx := c.Context
	if ctx == nil {
		ctx = context.Background()
	}
	last := ""
	before := c.App.AgentStatus()
	m, err := c.App.InstallAgent(ctx, a.Str("from"), func(phase string, frac float64) {
		if Progress != nil && phase != last {
			last = phase
			Progress("progress: " + phase)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("cannot install Archo Agent: %w", err)
	}
	head := "Archo Agent " + m.Version + " installed"
	if a.Str("from") == "" && before.Installed && before.Manifest.Version == m.Version {
		head = "Archo Agent " + m.Version + " is up to date; nothing downloaded"
	}
	d, root := render.New(head)
	root.KV("pi", m.Pi)
	root.KV("node", m.Node)
	root.KV("folder", c.App.AgentBundlesDir())
	d.AddNext("archo agent status")
	return d, nil
}

func agentRemoveCmd(c *Ctx, a *Args) (*render.Doc, error) {
	if err := c.App.RemoveAgent(); err != nil {
		return nil, err
	}
	d, _ := render.New("Archo Agent removed")
	return d, nil
}

func agentKeyCmd(c *Ctx, a *Args) (*render.Doc, error) {
	if a.Bool("clear") {
		if err := app.SetAgentKey(""); err != nil {
			return nil, err
		}
		d, _ := render.New("API key removed")
		return d, nil
	}
	// one line from standard input, so the key never appears in the command line or shell history
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		fmt.Fprint(os.Stderr, "API key (input is shown; paste it and press Enter): ")
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	key := strings.TrimSpace(line)
	if key == "" {
		if err != nil {
			return nil, &UsageError{Msg: "no key on standard input"}
		}
		return nil, &UsageError{Msg: "the key is empty; use --clear to remove it"}
	}
	if err := app.SetAgentKey(key); err != nil {
		return nil, fmt.Errorf("cannot store the key: %w", err)
	}
	d, _ := render.New("API key stored in Windows Credential Manager")
	d.AddNext("archo agent run")
	return d, nil
}

func agentRunCmd(c *Ctx, a *Args) (*render.Doc, error) {
	window := a.Bool("window")
	if err := c.App.RunAgent(window, os.Stdin, os.Stdout, os.Stderr); err != nil {
		return nil, err
	}
	if window {
		d, _ := render.New("Archo Agent opened in a new window")
		return d, nil
	}
	d, _ := render.New("Archo Agent closed")
	return d, nil
}
