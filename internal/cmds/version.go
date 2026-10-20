package cmds

import (
	"runtime"

	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/render"
)

func init() {
	// `archo mcp` is served by the CLI surface before commands run; this entry documents it.
	register(&Command{Name: "mcp", Summary: "serve the MCP tools over stdio (for Claude Code, Codex and other agents)", MaxArgs: 0, Run: func(c *Ctx, a *Args) (*render.Doc, error) {
		return nil, &UsageError{Msg: "run `archo mcp` as the whole command line; an MCP client starts it"}
	}})
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
