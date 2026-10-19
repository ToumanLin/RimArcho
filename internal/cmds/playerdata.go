package cmds

import (
	"fmt"
	"strings"

	"github.com/rimarcho/rimarcho/internal/render"
	"github.com/rimarcho/rimarcho/internal/store"
)

// Player data on lineages (04 §7.1, 05 §3): every instance of a lineage shares it.

func init() {
	register(&Command{Name: "alias", Args: "<ref> [<name>]", Summary: "set the name RimArcho shows for a mod (all its copies)", MinArgs: 1, MaxArgs: 2, Run: aliasCmd,
		Flags: []Flag{{Name: "clear"}}})
	register(&Command{Name: "note", Args: "<ref> [<text>]", Summary: "attach a plain-text note to a mod (all its copies)", MinArgs: 1, MaxArgs: 2, Run: noteCmd,
		Flags: []Flag{{Name: "clear"}}})
	register(&Command{Name: "tag", Args: "add|remove <ref> <tag…> | list", Summary: "tags for filtering (list --filter tag:<tag>)", MinArgs: 1, MaxArgs: -1, Run: tagCmd})
}

func playerTarget(c *Ctx, ref string) (*store.Mod, string, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, "", err
	}
	m, err := ix.ResolveOne(ref)
	if err != nil {
		return nil, "", err
	}
	return m, ix.Env.ID, nil
}

func aliasCmd(c *Ctx, a *Args) (*render.Doc, error) {
	return setText(c, a, "alias", c.App.Store.SetAlias)
}

func noteCmd(c *Ctx, a *Args) (*render.Doc, error) {
	return setText(c, a, "note", c.App.Store.SetNote)
}

func setText(c *Ctx, a *Args, what string, set func(env, lin, v string) error) (*render.Doc, error) {
	m, envID, err := playerTarget(c, a.Pos[0])
	if err != nil {
		return nil, err
	}
	v := ""
	switch {
	case a.Bool("clear"):
	case len(a.Pos) == 2:
		v = a.Pos[1]
	default:
		return nil, &UsageError{Msg: fmt.Sprintf("give the %s text, or --clear", what)}
	}
	if err := set(envID, m.LineageID, v); err != nil {
		return nil, err
	}
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	if v == "" {
		d, _ := render.New(fmt.Sprintf("%s cleared for %s", what, modLabel(ix, m.ID)))
		return d, nil
	}
	d, root := render.New(fmt.Sprintf("%s set for %s", what, modLabel(ix, m.ID)))
	root.KV(what, v)
	if n := len(ix.ByLineage[m.LineageID]); n > 1 {
		root.KV("lineage", fmt.Sprintf("%s: shared by its %d instances", ix.LRef(m.LineageID), n))
	}
	return d, nil
}

func tagCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	switch a.Pos[0] {
	case "list":
		ts, err := c.App.Store.TagCounts(ix.Env.ID)
		if err != nil {
			return nil, err
		}
		d, root := render.New(fmt.Sprintf("tags: %d", len(ts)))
		var rows [][]string
		for _, t := range ts {
			rows = append(rows, []string{t[0], t[1] + " mods"})
		}
		root.AddColumns(rows)
		if len(ts) == 0 {
			root.Add("(none)")
		}
		root.Truncate(boolLimit(a, render.DefaultLimit), "")
		if len(ts) > 0 {
			d.AddNext("archo list --filter tag:" + ts[0][0])
		}
		return d, nil
	case "add", "remove":
		if len(a.Pos) < 3 {
			return nil, &UsageError{Msg: "tag " + a.Pos[0] + " <ref> <tag…>"}
		}
		m, err := ix.ResolveOne(a.Pos[1])
		if err != nil {
			return nil, err
		}
		var changed, same []string
		for _, t := range a.Pos[2:] {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			var ok bool
			if a.Pos[0] == "add" {
				ok, err = c.App.Store.AddTag(ix.Env.ID, m.LineageID, t)
			} else {
				ok, err = c.App.Store.RemoveTag(ix.Env.ID, m.LineageID, t)
			}
			if err != nil {
				return nil, err
			}
			if ok {
				changed = append(changed, t)
			} else {
				same = append(same, t)
			}
		}
		verb := map[string]string{"add": "tagged", "remove": "untagged"}[a.Pos[0]]
		d, root := render.New(fmt.Sprintf("%s %s: %s", verb, modLabel(ix, m.ID), strings.Join(changed, ", ")))
		if len(same) > 0 {
			root.KV("unchanged", strings.Join(same, ", "))
		}
		return d, nil
	}
	return nil, &UsageError{Msg: "tag add|remove <ref> <tag…> | tag list"}
}
