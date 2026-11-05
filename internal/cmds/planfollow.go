package cmds

import (
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Following Steam Workshop collections (07 §7.6): link, unlink, check and catch up.

func init() {
	register(&Command{Name: "plan follow", Args: "<name> <collection link|ID>", Summary: "make a plan follow a Steam Workshop collection: RimArcho tells you when it changes (uses the network)",
		MinArgs: 2, MaxArgs: 2, Run: planFollow,
		Flags: []Flag{{Name: "mode", Value: "add|follow", Help: "catching up enables the new mods (add, default), or also disables the ones the collection dropped (follow)"}}})
	register(&Command{Name: "plan unfollow", Args: "<name> <collection link|ID>", Summary: "stop following a collection; the plan stays as it is", MinArgs: 2, MaxArgs: 2, Run: planUnfollow})
	register(&Command{Name: "plan sync", Args: "<name> [<collection>]", Summary: "catch a plan up with a collection it follows (one undoable edit; uses the network)",
		MinArgs: 1, MaxArgs: 2, Run: planSync,
		Flags: []Flag{{Name: "check", Help: "only read the collections and list what changed"}, {Name: "dry-run", Help: "show what catching up would enable and disable"}}})
}

func wsLink(id string) string { return "https://steamcommunity.com/sharedfiles/filedetails/?id=" + id }

func itemName(it store.CollectionItem) string {
	if it.Title != "" {
		return it.Title
	}
	return "ws:" + it.WorkshopID
}

// followedNode describes one followed collection under n.
func followedNode(c *Ctx, n *render.Node, f *app.Followed) {
	line := fmt.Sprintf("%s (ws:%s): %s, %s", f.Title, f.WorkshopID, render.Plural(len(f.Items()), "item"), map[string]string{app.FollowAdd: "catching up enables new mods", app.FollowFollow: "catching up also disables dropped mods"}[f.Mode])
	if f.Checked.IsZero() {
		line += ", not checked yet"
	} else {
		line += ", checked " + c.App.FormatTime(f.Checked)
	}
	k := n.Add(line)
	if !f.Changed() && !f.Checked.IsZero() {
		k.Add("no change since the last catch-up")
	}
	if len(f.Added) > 0 {
		a := k.KV("new", fmt.Sprint(len(f.Added)))
		for _, it := range f.Added {
			a.Add("+ " + itemName(it))
		}
		a.Truncate(render.DefaultLimit, "")
	}
	if len(f.Removed) > 0 {
		r := k.KV("dropped", fmt.Sprint(len(f.Removed)))
		for _, it := range f.Removed {
			r.Add("- " + itemName(it))
		}
		r.Truncate(render.DefaultLimit, "")
	}
	if len(f.NotInstalled) > 0 {
		m := k.KV("not installed", fmt.Sprintf("%d (subscribe in the Workshop)", len(f.NotInstalled)))
		for _, it := range f.NotInstalled {
			m.Add(itemName(it) + "  " + wsLink(it.WorkshopID))
		}
		m.Truncate(render.DefaultLimit, "")
	}
}

func planFollow(c *Ctx, a *Args) (*render.Doc, error) {
	f, err := c.App.FollowCollection(a.Pos[0], a.Pos[1], a.Str("mode"))
	if err != nil {
		return nil, err
	}
	p, err := c.App.FindPlan(a.Pos[0])
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("%s follows %s", app.PlanRef(p), f.Title))
	followedNode(c, root, f)
	root.KV("plan", "unchanged; archo plan sync tells and applies what changes")
	d.AddNext("archo plan sync " + quoteArg(p.Name) + " --check")
	return d, nil
}

func planUnfollow(c *Ctx, a *Args) (*render.Doc, error) {
	col, err := c.App.UnfollowCollection(a.Pos[0], a.Pos[1])
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("plan:%s no longer follows %s", a.Pos[0], col.Title))
	root.KV("plan", "unchanged")
	return d, nil
}

func planSync(c *Ctx, a *Args) (*render.Doc, error) {
	ref := ""
	if len(a.Pos) > 1 {
		ref = a.Pos[1]
	}
	if a.Bool("check") {
		fs, err := c.App.CheckFollowed(a.Pos[0], ref, true)
		if err != nil && len(fs) == 0 {
			return nil, err
		}
		p, perr := c.App.FindPlan(a.Pos[0])
		if perr != nil {
			return nil, perr
		}
		changed := 0
		for _, f := range fs {
			if f.Changed() {
				changed++
			}
		}
		d, root := render.New(fmt.Sprintf("%s follows %s; %d changed", app.PlanRef(p), render.Plural(len(fs), "collection"), changed))
		for _, f := range fs {
			followedNode(c, root, f)
		}
		if err != nil {
			root.KV("could not read", err.Error())
		}
		if changed > 0 {
			d.AddNext("archo plan sync " + quoteArg(p.Name) + " --dry-run")
		}
		return d, nil
	}
	r, err := c.App.SyncCollection(a.Pos[0], ref, a.Bool("dry-run"))
	if err != nil {
		return nil, err
	}
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	verb := "caught up with"
	if r.DryRun {
		verb = "would catch up with"
	}
	d, root := render.New(fmt.Sprintf("%s %s %s: %d enabled, %d disabled", app.PlanRef(r.Plan), verb, r.Collection.Title, len(r.Enabled), len(r.Disabled)))
	if len(r.Enabled) > 0 {
		n := root.KV("enabled", fmt.Sprint(len(r.Enabled)))
		for _, m := range r.Enabled {
			n.Add(fmt.Sprintf("%s at #%d", modLabel(ix, m.ID), r.After.Index(m.ID)+1))
		}
	}
	if len(r.Disabled) > 0 {
		n := root.KV("disabled", fmt.Sprint(len(r.Disabled)))
		for _, m := range r.Disabled {
			n.Add(modLabel(ix, m.ID))
		}
	}
	if kept := len(r.Collection.Removed) - len(r.Disabled); kept > 0 && r.Collection.Mode == app.FollowAdd {
		root.KV("kept", fmt.Sprintf("%s the collection dropped stay (--mode follow disables them)", render.Plural(kept, "mod")))
	}
	if len(r.NotInstalled) > 0 {
		n := root.KV("not installed", fmt.Sprintf("%d (subscribe in the Workshop)", len(r.NotInstalled)))
		for _, it := range r.NotInstalled {
			n.Add(itemName(it) + "  " + wsLink(it.WorkshopID))
		}
	}
	if r.DryRun {
		d.AddNext("archo plan sync " + quoteArg(r.Plan.Name) + " " + r.Collection.WorkshopID)
	} else if !r.After.Equal(r.Before) {
		if st := c.Loadout(); st != nil && st.Plan.ID == r.Plan.ID {
			root.KV("undo", "archo undo")
		} else {
			root.KV("undo", "archo plan use "+quoteArg(r.Plan.Name)+", then archo undo")
		}
	}
	return d, nil
}
