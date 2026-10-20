package cmds

import (
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Hidden mods (10 §2): folders another manager hid by renaming About.xml.
func init() {
	dry := []Flag{{Name: "dry-run", Help: "show what would change"}}
	register(&Command{Name: "unhide", Args: "<ref>", Summary: "rename About.xml.disabled back to About.xml so the game sees the mod again", MinArgs: 1, MaxArgs: 1, Flags: dry,
		Run: func(c *Ctx, a *Args) (*render.Doc, error) { return fileOpDoc(a, "unhide", c.App.Unhide) }})
	register(&Command{Name: "unpin", Args: "<ref>", Summary: "restore a pinned mod's workshop copy and use it instead of the pinning copy", MinArgs: 1, MaxArgs: 1, Flags: dry,
		Run: func(c *Ctx, a *Args) (*render.Doc, error) { return fileOpDoc(a, "unpin", c.App.Unpin) }})
	register(&Command{Name: "trash", Args: "<ref>", Summary: "move a hidden or unsubscribed workshop folder to the recycle bin", MinArgs: 1, MaxArgs: 1, Flags: dry,
		Run: func(c *Ctx, a *Args) (*render.Doc, error) { return fileOpDoc(a, "trash", c.App.Trash) }})
}

func fileOpDoc(a *Args, name string, f func(ref string, dryRun bool) (*app.FileOp, error)) (*render.Doc, error) {
	op, err := f(a.Pos[0], a.Bool("dry-run"))
	if err != nil {
		return nil, err
	}
	head := map[string]string{"rename": "renamed", "trash": "moved to the recycle bin"}[op.Op]
	if op.DryRun {
		head = "dry run, nothing changed: would be " + head
	}
	d, root := render.New(head)
	switch op.Op {
	case "rename":
		root.KV("from", op.Path)
		root.KV("to", op.To)
	case "trash":
		root.KV("folder", op.Path)
	}
	if op.Swap != nil {
		root.KV("workspace", fmt.Sprintf("%d change unsaved: the workshop copy replaces the pinning copy", len(op.Swap.Changes)))
		d.AddNext("archo save")
	}
	if op.Scan != nil {
		root.KV("rescanned", fmt.Sprintf("scan:%d", op.Scan.ScanID))
	}
	if op.DryRun {
		d.AddNext("archo " + name + " " + a.Pos[0])
	}
	return d, nil
}
