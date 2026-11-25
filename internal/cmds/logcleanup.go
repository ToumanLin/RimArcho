package cmds

import (
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

func logCleanupCmd(c *Ctx, a *Args) (*render.Doc, error) {
	trim := a.Bool("trim")
	if trim && a.Bool("delete") {
		return nil, &UsageError{Msg: "choose either --trim or --delete"}
	}
	for _, flag := range []string{"file", "text", "cluster", "follow", "since", "select", "all", "once", "json"} {
		if a.Bool(flag) {
			return nil, &UsageError{Msg: fmt.Sprintf("--%s cannot be combined with --trim or --delete", flag)}
		}
	}
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	path, prev := c.App.LogPaths(st)
	if a.Bool("prev") {
		path = prev
	}
	if path == "" {
		return nil, app.ErrLogCleanupTarget
	}
	if a.Bool("yes") && !a.Bool("dry-run") {
		if a.Str("token") == "" {
			return nil, &UsageError{Msg: "--yes requires --token from the preview"}
		}
		if err := c.App.ApplyLogCleanup(path, a.Str("token"), trim); err != nil {
			return nil, err
		}
		verb := "deleted"
		if trim {
			verb = "trimmed"
		}
		d, root := render.New("game log " + verb)
		root.KV("file", path)
		return d, nil
	}
	p, err := c.App.PreviewLogCleanup(path, trim)
	if err != nil {
		return nil, err
	}
	verb := "delete"
	if trim {
		verb = "trim"
	}
	d, root := render.New("preview " + verb + " game log")
	root.KV("file", p.Path)
	root.KV("size", fmt.Sprintf("%d bytes", p.Size))
	if trim {
		root.KV("remove", fmt.Sprintf("%d bytes from the middle", p.Omitted))
		root.KV("remaining", fmt.Sprintf("%d bytes", p.Remaining))
	} else {
		root.KV("delete", "permanently delete this file")
	}
	root.KV("token", p.Token)
	next := "archo log --" + verb
	if a.Bool("prev") {
		next += " --prev"
	}
	d.AddNext(next + " --yes --token " + p.Token)
	return d, nil
}
