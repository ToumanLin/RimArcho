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
		{Name: "update", Help: "remake the locked version from the upstream's current content, in a new folder"}, {Name: "dry-run", Help: "show what would be copied"}}
	register(&Command{Name: "pin", Args: "<ref>", Summary: "lock a mod: copy it into the local Mods folder (origin marker) and use the copy in the current plan", MinArgs: 1, MaxArgs: 1, Run: pinCmd, Flags: flags})
	register(&Command{Name: "copy-local", Args: "<ref>", Summary: "alias of pin", MinArgs: 1, MaxArgs: 1, Run: pinCmd, Flags: flags})
}

func pinCmd(c *Ctx, a *Args) (*render.Doc, error) {
	f := c.App.Pin
	if a.Bool("update") {
		f = c.App.PinUpdate
	}
	res, err := f(a.Pos[0], a.Str("name"), a.Bool("git"), a.Bool("dry-run"))
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
		root.KV("old copy", ix.Ref(res.Old.ID)+" stays where it is")
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
	case res.Blocked != "" && res.Old != nil:
		return fmt.Sprintf("unchanged: the game ignores %s while %s (same packageId, earlier in Mods/) is there; move %s's folder out of Mods/, then archo swap %s %s",
			ix.Ref(res.CopyMod.ID), ix.Ref(res.Old.ID), ix.Ref(res.Old.ID), ix.Ref(res.Replaced.ID), ix.Ref(res.CopyMod.ID))
	case res.Blocked != "":
		return fmt.Sprintf("unchanged: the game would not load %s (%s)", ix.Ref(res.CopyMod.ID), res.Blocked)
	}
	return fmt.Sprintf("%s replaces %s in the current plan (archo undo reverts it)", ix.Ref(res.CopyMod.ID), ix.Ref(res.Replaced.ID))
}
