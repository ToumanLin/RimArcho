package cmds

import (
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Locked versions (04 §7.2, §7.4): pin copies a mod into Mods/ and uses the copy in the
// current plan; copy-local is the old name, kept as an alias.
func init() {
	flags := []Flag{{Name: "name", Value: "<folder>", Help: "default <folder>_local"}, {Name: "git", Help: "make the copy a git repository with a baseline commit"},
		{Name: "update", Help: "remake the locked version from the upstream's current content, in a new folder"}, {Name: "move-to-library", Help: "move a locked copy made in Mods/ before the lock library existed into the library"},
		{Name: "reason", Value: "<text>", Help: "why it is locked: kept in the copy's note"},
		{Name: "dry-run", Help: "show what would be copied"}}
	register(&Command{Name: "pin", Args: "<ref>", Summary: "lock a mod: copy it into the lock library (origin marker) and use the copy in the current plan", MinArgs: 1, MaxArgs: 1, Run: pinCmd, Flags: flags})
	register(&Command{Name: "copy-local", Args: "<ref>", Summary: "alias of pin", MinArgs: 1, MaxArgs: 1, Run: pinCmd, Flags: flags})
}

func pinCmd(c *Ctx, a *Args) (*render.Doc, error) {
	if a.Bool("move-to-library") {
		r, err := c.App.MoveToLibrary(a.Pos[0], a.Bool("dry-run"))
		if err != nil {
			return nil, err
		}
		ix, err := c.App.Index()
		if err != nil {
			return nil, err
		}
		head := "moved to the lock library"
		if r.DryRun {
			head = "dry run, nothing changed: would move to the lock library"
		}
		d, root := render.New(fmt.Sprintf("%s %s", modLabel(ix, r.Mod.ID), head))
		moveKVs(ix, root, r)
		if r.DryRun {
			d.AddNext("archo pin " + a.Pos[0] + " --move-to-library")
		}
		return d, nil
	}
	f := c.App.PinFor
	if a.Bool("update") {
		f = c.App.PinUpdateFor
	}
	res, err := f(a.Pos[0], a.Str("name"), a.Bool("git"), a.Bool("dry-run"), a.Str("reason"))
	if err != nil {
		return nil, err
	}
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	from := modLabel(ix, res.From.ID)
	head := fmt.Sprintf("%s locked: copied to %s", from, res.Copy.Dir)
	if res.Old != nil {
		head = fmt.Sprintf("%s: new locked version copied to %s", from, res.Copy.Dir)
	}
	if res.DryRun {
		head = fmt.Sprintf("dry run, nothing changed: would copy %s to %s", from, res.Copy.Dir)
	}
	d, root := render.New(head)
	if res.Moved != nil {
		moveKVs(ix, root.KV("old copy", "in Mods/, made before the lock library"), res.Moved)
	}
	if res.Old != nil {
		n := root.KV("upstream changes", "compared with "+ix.Ref(res.Old.ID)+": "+render.Plural(len(res.Diff), "semantic file"))
		for _, s := range res.Diff {
			n.Add(s)
		}
		n.Truncate(10, "archo mod "+ix.Ref(res.Old.ID)+" --diff "+ix.Ref(res.From.ID))
	}
	if res.CopyMod != nil {
		root.KV("copy", ix.Ref(res.CopyMod.ID)+"  (same lineage, origin marker .rimarcho/origin.json)")
	}
	root.KV("files", fmt.Sprintf("%d (%.1f MB)", res.Copy.Files, float64(res.Copy.Bytes)/(1<<20)))
	switch {
	case res.Copy.Git:
		root.KV("git", "repository with a baseline commit of every file")
	case res.Copy.GitError != "":
		root.KV("git", "not created: "+res.Copy.GitError)
	case res.DryRun && a.Bool("git"):
		root.KV("git", "would make a repository with a baseline commit")
	}
	if len(res.Copy.Skipped) > 0 {
		n := root.KV("skipped", fmt.Sprintf("%d links that point outside the mod folder", len(res.Copy.Skipped)))
		for _, s := range res.Copy.Skipped {
			n.Add(s)
		}
		n.Truncate(5, "")
	}
	if res.Old != nil {
		root.KV("old copy", ix.Ref(res.Old.ID)+" stays in the lock library")
	}
	root.KV("plan", pinPlanText(ix, res))
	switch {
	case res.DryRun:
		next := "archo pin " + a.Pos[0]
		if a.Bool("update") {
			next = "archo pin --update " + a.Pos[0]
		}
		d.AddNext(next)
	case res.Swap != nil && len(res.Swap.Changes) > 0:
		d.AddNext("archo save")
	}
	return d, nil
}

func pinPlanText(ix *app.Index, res *app.PinResult) string {
	switch {
	case res.Replaced == nil && res.Old != nil:
		return fmt.Sprintf("unchanged: neither %s nor %s is enabled", ix.Ref(res.Old.ID), ix.Ref(res.From.ID))
	case res.Replaced == nil:
		return fmt.Sprintf("unchanged: %s is not enabled", ix.Ref(res.From.ID))
	case res.DryRun:
		return fmt.Sprintf("the copy would replace %s in the current plan", ix.Ref(res.Replaced.ID))
	case res.CopyMod == nil:
		return "unchanged: the copy was not found after the rescan"
	case res.Blocked != "":
		return fmt.Sprintf("unchanged: the game would not load %s (%s)", ix.Ref(res.CopyMod.ID), res.Blocked)
	}
	return fmt.Sprintf("%s replaces %s in the current plan (archo undo reverts it); the game gets it through an _Archo_ link on save or launch", ix.Ref(res.CopyMod.ID), ix.Ref(res.Replaced.ID))
}

// moveKVs describes moving a copy out of Mods/ into the lock library.
func moveKVs(ix *app.Index, n *render.Node, r *app.MoveResult) {
	for _, line := range r.Links.Lines() {
		n.KV("affected link", line)
	}
	n.KV("from", r.From)
	n.KV("to", r.To)
	how := "renamed (same volume)"
	if r.Copied {
		how = "copied to the other volume and checked, then the original goes to the recycle bin"
	}
	n.KV("how", how)
	if r.After != nil {
		n.KV("now", ix.Ref(r.After.ID))
	}
	if r.Swap != nil && len(r.Swap.Changes) > 0 {
		n.KV("plan", "the current plan follows the moved copy")
	}
}
