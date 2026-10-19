package cmds

import (
	"fmt"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/render"
)

// Staging (06 §5.4): a per-environment list RimArcho keeps and never writes to the game.

func init() {
	register(&Command{Name: "stage list", Summary: "staged mods, with the place each one came from", MaxArgs: 0, Run: stageList})
	register(&Command{Name: "stage add", Args: "<ref…>", Summary: "stage mods (enabled ones leave the Loadout and remember their place)", MinArgs: 1, MaxArgs: -1, Run: stageAdd})
	register(&Command{Name: "stage drop", Args: "<ref…>", Summary: "take mods out of staging without enabling them", MinArgs: 1, MaxArgs: -1, Run: stageDrop})
	register(&Command{Name: "stage restore", Args: "[<ref…>]", Summary: "enable staged mods again at their old place (all when no ref)", MaxArgs: -1,
		Flags: placeFlags, Run: stageRestore})
}

func stageList(c *Ctx, a *Args) (*render.Doc, error) {
	items, st, err := c.App.Staged()
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("staging: %s (%s)", render.Plural(len(items), "mod"), envRef(st.Ix.Env)))
	var rows [][]string
	for _, it := range items {
		m := st.Ix.ByID[it.Instance]
		from := "was not enabled"
		if it.Pos > 0 {
			from = fmt.Sprintf("was %d", it.Pos)
			if it.After != "" {
				from += ", after " + st.Ix.Ref(it.After)
			}
		}
		rows = append(rows, []string{st.Ix.Ref(m.ID), m.DisplayName(), from})
	}
	root.AddColumns(rows)
	if len(items) > 0 {
		d.AddNext("archo stage restore")
	}
	return d, nil
}

func stageAdd(c *Ctx, a *Args) (*render.Doc, error) {
	r, err := c.App.Stage(a.Pos)
	if err != nil {
		return nil, err
	}
	d := editDoc(c, r)
	d.Nodes[0].Children = append([]*render.Node{{Line: "staged: " + render.Plural(len(a.Pos), "mod")}}, d.Nodes[0].Children...)
	return d, nil
}

func stageDrop(c *Ctx, a *Args) (*render.Doc, error) {
	gone, err := c.App.Unstage(a.Pos)
	if err != nil {
		return nil, err
	}
	d, root := render.New("unstaged: " + render.Plural(len(gone), "mod"))
	if len(gone) == 0 {
		root.Add("nothing was staged")
	}
	return d, nil
}

func stageRestore(c *Ctx, a *Args) (*render.Doc, error) {
	p, err := placement(a, "at")
	if err != nil {
		return nil, err
	}
	r, err := c.App.Restore(a.Pos, p)
	if err != nil {
		return nil, err
	}
	return editDoc(c, r), nil
}

var _ = app.StageItem{}
