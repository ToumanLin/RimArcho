package cmds

import (
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/render"
)

func init() {
	register(&Command{Name: "collection", Args: "<link|ID>", Summary: "read a Steam Workshop collection and compare it with the installed mods (uses the network)", MinArgs: 1, MaxArgs: 1, Run: collectionCmd,
		Flags: []Flag{{Name: "import", Help: "replace the workspace with the collection's installed mods, in its order (undoable)"}}})
}

func collectionCmd(c *Ctx, a *Args) (*render.Doc, error) {
	col, err := c.App.ReadCollection(a.Pos[0])
	if err != nil {
		return nil, err
	}
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	title := col.Title
	if title == "" {
		title = "ws:" + col.ID
	}
	var have, missing, subs []string
	for _, it := range col.Items {
		name := it.Title
		if name == "" {
			name = "ws:" + it.WorkshopID
		}
		switch {
		case it.Sub:
			subs = append(subs, fmt.Sprintf("%s  https://steamcommunity.com/sharedfiles/filedetails/?id=%s", name, it.WorkshopID))
		case it.Mod != nil:
			have = append(have, modLabel(ix, it.Mod.ID))
		default:
			missing = append(missing, fmt.Sprintf("%s  https://steamcommunity.com/sharedfiles/filedetails/?id=%s", name, it.WorkshopID))
		}
	}
	if a.Bool("import") {
		r, il, err := c.App.ImportCollection(col)
		if err != nil {
			return nil, err
		}
		d := editDoc(c, r)
		head := []*render.Node{{Line: fmt.Sprintf("imported collection %s: %d of %d mods are installed", title, len(have), len(have)+len(missing))}}
		if len(il.Unresolved) > 0 {
			n := &render.Node{Line: fmt.Sprintf("not installed (subscribe in the Workshop): %d", len(il.Unresolved))}
			for _, m := range missing {
				n.Children = append(n.Children, &render.Node{Line: m})
			}
			head = append(head, n)
		}
		d.Nodes[0].Children = append(head, d.Nodes[0].Children...)
		return d, nil
	}
	d, root := render.New(fmt.Sprintf("collection %s: %d items", title, len(col.Items)))
	for _, g := range []struct {
		key   string
		lines []string
	}{{"installed", have}, {"not installed", missing}, {"nested collections (not expanded)", subs}} {
		if len(g.lines) == 0 {
			continue
		}
		n := root.KV(g.key, fmt.Sprint(len(g.lines)))
		for _, l := range g.lines {
			n.Add(l)
		}
		n.Truncate(render.DefaultLimit, "")
	}
	d.AddNext("archo collection " + col.ID + " --import")
	return d, nil
}
