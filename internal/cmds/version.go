package cmds

import (
	"runtime"

	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// mcpCommand documents `archo mcp` and parses its flags (MCPTools).
var mcpCommand = &Command{Name: "mcp", Summary: "serve the MCP tools over stdio (for Claude Code, Codex and other agents)", MaxArgs: 0,
	Flags: []Flag{{Name: "tools", Value: "core|all", Help: "which tools to serve: core (default: diagnosis, plan edits, save, launch, test) or all (adds the player's management tools: packs, subscriptions, library, textures, memory, maintenance, identity, hidden mods, links, player data, uninstall preview)"}},
	Run: func(c *Ctx, a *Args) (*render.Doc, error) {
		return nil, &UsageError{Msg: "run `archo mcp` as the whole command line; an MCP client starts it"}
	}}

// MCPTools parses `archo mcp [--tools core|all]` (argv starts with "mcp") and returns the
// --tools value, "" when absent. The MCP surface checks the value itself.
func MCPTools(argv []string) (string, error) {
	a, err := parse(mcpCommand, argv[1:])
	if err != nil {
		return "", err
	}
	return a.Str("tools"), nil
}

func init() {
	// `archo mcp` is served by the CLI surface before commands run; this entry documents it.
	register(mcpCommand)
	register(&Command{Name: "gui", Summary: "open the desktop window (RimArcho.exe does this on a double click)", MaxArgs: 0, Run: func(c *Ctx, a *Args) (*render.Doc, error) {
		return nil, &UsageError{Msg: "run `archo gui` as the whole command line"}
	}})
	register(&Command{Name: "version", Summary: "print the RimArcho version", MaxArgs: 0, Run: func(c *Ctx, a *Args) (*render.Doc, error) {
		d, root := render.New("archo " + buildinfo.String())
		root.KV("go", runtime.Version()+" "+runtime.GOOS+"/"+runtime.GOARCH)
		root.KV("data", c.App.Home)
		return d, nil
	}})
}
