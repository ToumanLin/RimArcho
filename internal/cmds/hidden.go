package cmds

import (
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Hidden mods (08 §6): folders another manager hid by renaming About.xml.
func init() {
	dry := []Flag{{Name: "dry-run", Help: "show what would change"}}
	register(&Command{Name: "unhide", Args: "<ref>", Summary: "rename About.xml.disabled back to About.xml so the game sees the mod again", MinArgs: 1, MaxArgs: 1, Flags: dry,
		Run: func(c *Ctx, a *Args) (*render.Doc, error) { return fileOpDoc(c, a, "unhide", c.App.Unhide) }})
	register(&Command{Name: "unpin", Args: "<ref>", Summary: "use a locked mod's upstream again in the current plan (a RimCrow-style lock also gets its About.xml back); the copy stays", MinArgs: 1, MaxArgs: 1, Flags: dry,
		Run: func(c *Ctx, a *Args) (*render.Doc, error) { return fileOpDoc(c, a, "unpin", c.App.Unpin) }})
	register(&Command{Name: "trash", Args: "<ref>", Summary: "move a hidden or unsubscribed workshop folder to the recycle bin", MinArgs: 1, MaxArgs: 1, Flags: dry,
		Run: func(c *Ctx, a *Args) (*render.Doc, error) { return fileOpDoc(c, a, "trash", c.App.Trash) }})
}

func fileOpDoc(c *Ctx, a *Args, name string, f func(ref string, dryRun bool) (*app.FileOp, error)) (*render.Doc, error) {
	op, err := f(a.Pos[0], a.Bool("dry-run"))
	if err != nil {
		return nil, err
	}
	if op.Op == "unpin" {
		return unpinCopyDoc(c, a, op)
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

// unpinCopyDoc reports unpinning a locked version RimArcho made (04 §7.5): no files change.
func unpinCopyDoc(c *Ctx, a *Args, op *app.FileOp) (*render.Doc, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	l := op.Locked
	cp, up := modLabel(ix, l.Copy.ID), ix.Ref(l.Upstream.ID)
	var d *render.Doc
	var root *render.Node
	switch {
	case !op.InPlan:
		d, root = render.New(fmt.Sprintf("nothing changed: the current plan does not use %s", cp))
	case op.DryRun:
		d, root = render.New(fmt.Sprintf("dry run, nothing changed: would use %s instead of %s in the current plan", up, cp))
		d.AddNext("archo unpin " + a.Pos[0])
	default:
		d, root = render.New(fmt.Sprintf("unpinned: the current plan uses %s instead of %s", up, cp))
		d.AddNext("archo save")
	}
	root.KV("copy", fmt.Sprintf("stays at %s (no files change)", op.Path))
	root.KV("upstream", fmt.Sprintf("%s at %s", up, op.To))
	return d, nil
}
