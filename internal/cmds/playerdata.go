package cmds

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Player data on lineages (07 §6.1, 13 §4.2): every instance of a lineage shares it.

func init() {
	register(&Command{Name: "alias", Args: "<ref> [<name>]", Summary: "set the name RimArcho shows for a mod (all its copies)", MinArgs: 1, MaxArgs: 2, Run: aliasCmd,
		Flags: []Flag{{Name: "clear"}}})
	register(&Command{Name: "note", Args: "<ref> [<text>]", Summary: "attach a plain-text note to a mod (all its copies), or with --copy to this copy only", MinArgs: 1, MaxArgs: 2, Run: noteCmd,
		Flags: []Flag{{Name: "clear"}, {Name: "copy", Help: "this copy's own note (Mod library), not the mod's"}}})
	register(&Command{Name: "tag", Args: "add|remove <ref> <tag…> | list | rename <tag> <new> | merge <tag…> --into <tag> | delete <tag…> | note <tag> [<text>] | color <tag> <colour> | order <ref> <tag…>", Summary: "tags for filtering (list --filter tag:<tag>); rename, merge, delete, note and color apply to the tag on every mod; color marks its mods in the lists (blue, cyan, teal, grass, orange, pink, plum, violet, #rrggbb or none); order sets a mod's tag order (its first coloured tag marks it)", MinArgs: 1, MaxArgs: -1, Run: tagCmd,
		Flags: []Flag{{Name: "mod", Value: "<ref>", Multi: true, Help: "more mods to tag (add/remove)"},
			{Name: "into", Value: "<tag>", Help: "merge: the tag the others become (new or existing)"},
			{Name: "dry-run", Help: "rename, merge, delete: show what would change"},
			{Name: "clear", Help: "note, color: clear the tag's note or colour"}}})
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
	if a.Bool("copy") {
		return copyNoteCmd(c, a)
	}
	return setText(c, a, "note", c.App.Store.SetNote)
}

// copyNoteCmd sets one copy's own note (16 §2.9); the mod's note stays as it is.
func copyNoteCmd(c *Ctx, a *Args) (*render.Doc, error) {
	v := ""
	switch {
	case a.Bool("clear"):
	case len(a.Pos) == 2:
		v = a.Pos[1]
	default:
		return nil, &UsageError{Msg: "give the note text, or --clear"}
	}
	m, err := c.App.SetCopyNote(a.Pos[0], v)
	if err != nil {
		return nil, err
	}
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	if v == "" {
		d, _ := render.New("copy note cleared for " + modLabel(ix, m.ID))
		return d, nil
	}
	d, root := render.New("copy note set for " + modLabel(ix, m.ID))
	root.KV("note", v)
	root.KV("copy", "this instance only (src:"+m.SourceLabel+" "+m.Rel+"); the mod's note is archo note "+ix.Ref(m.ID))
	return d, nil
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
		all, err := TagList(c.App)
		if err != nil {
			return nil, err
		}
		var ts []TagInfo
		for _, t := range all {
			if t.N > 0 {
				ts = append(ts, t)
			}
		}
		slices.SortStableFunc(ts, func(x, y TagInfo) int { return strings.Compare(strings.ToLower(x.Tag), strings.ToLower(y.Tag)) })
		d, root := render.New(fmt.Sprintf("tags: %d", len(ts)))
		var rows [][]string
		for _, t := range ts {
			r := []string{t.Tag, render.Plural(t.N, "mod")}
			var more []string
			if t.Pinned {
				more = append(more, "pinned")
			}
			if t.Color != "" {
				more = append(more, "colour: "+t.Color)
			}
			if t.Note != "" {
				more = append(more, "note: "+strings.ReplaceAll(t.Note, "\n", " / "))
			}
			rows = append(rows, append(r, strings.Join(more, "  ")))
		}
		root.AddColumns(rows)
		if len(ts) == 0 {
			root.Add("(none)")
		}
		root.Truncate(boolLimit(a, render.DefaultLimit), "")
		if len(ts) > 0 {
			d.AddNext("archo list --filter tag:" + quoteArg(ts[0].Tag))
		}
		return d, nil
	case "rename":
		if len(a.Pos) != 3 {
			return nil, &UsageError{Msg: "tag rename <tag> <new name> [--dry-run]"}
		}
		e, err := RenameTag(c.App, a.Pos[1], a.Pos[2], a.Bool("dry-run"))
		if err != nil {
			return nil, err
		}
		return tagEditDoc(e, a.Bool("dry-run"), "renamed", fmt.Sprintf("%s → %s", e.From[0], e.Into)), nil
	case "merge":
		if len(a.Pos) < 2 || a.Str("into") == "" {
			return nil, &UsageError{Msg: "tag merge <tag…> --into <tag> [--dry-run]"}
		}
		e, err := MergeTags(c.App, a.Pos[1:], a.Str("into"), a.Bool("dry-run"))
		if err != nil {
			return nil, err
		}
		return tagEditDoc(e, a.Bool("dry-run"), "merged", fmt.Sprintf("%s → %s", strings.Join(e.From, ", "), e.Into)), nil
	case "delete":
		if len(a.Pos) < 2 {
			return nil, &UsageError{Msg: "tag delete <tag…> [--dry-run]"}
		}
		e, err := DeleteTags(c.App, a.Pos[1:], a.Bool("dry-run"))
		if err != nil {
			return nil, err
		}
		return tagEditDoc(e, a.Bool("dry-run"), "deleted", strings.Join(e.From, ", ")), nil
	case "note":
		v := ""
		switch {
		case len(a.Pos) == 2 && a.Bool("clear"):
		case len(a.Pos) == 3:
			v = a.Pos[2]
		default:
			return nil, &UsageError{Msg: "tag note <tag> <text> | tag note <tag> --clear"}
		}
		t, err := SetTagNote(c.App, a.Pos[1], v)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(v) == "" {
			d, _ := render.New("note cleared for tag " + t)
			return d, nil
		}
		d, root := render.New("note set for tag " + t)
		root.KV("note", strings.TrimSpace(v))
		return d, nil
	case "color":
		v := ""
		switch {
		case len(a.Pos) == 2 && a.Bool("clear"):
		case len(a.Pos) == 3:
			v = a.Pos[2]
		default:
			return nil, &UsageError{Msg: "tag color <tag> <colour> | tag color <tag> --clear (colours: " + strings.Join(TagColorNames, ", ") + ", #rrggbb, none)"}
		}
		t, col, err := SetTagColor(c.App, a.Pos[1], v)
		if err != nil {
			return nil, err
		}
		if col == "" {
			d, _ := render.New("colour cleared for tag " + t)
			return d, nil
		}
		d, root := render.New("colour set for tag " + t)
		root.KV("colour", col)
		root.KV("shows", "a band at the left of its mods' rows, where it is the mod's first coloured tag")
		return d, nil
	case "order":
		if len(a.Pos) < 3 {
			return nil, &UsageError{Msg: "tag order <ref> <tag…> (the tags first, in this order)"}
		}
		m, err := ix.ResolveOne(a.Pos[1])
		if err != nil {
			return nil, err
		}
		tags, err := SetModTagOrder(c.App, a.Pos[1], a.Pos[2:])
		if err != nil {
			return nil, err
		}
		d, root := render.New("tag order set for " + modLabel(ix, m.ID))
		root.KV("tags", strings.Join(tags, ", "))
		return d, nil
	case "add", "remove":
		if len(a.Pos) < 3 {
			return nil, &UsageError{Msg: "tag " + a.Pos[0] + " <ref> <tag…> [--mod <ref>]…"}
		}
		var mods []*store.Mod
		for _, r := range append([]string{a.Pos[1]}, a.List("mod")...) {
			m, err := ix.ResolveOne(r)
			if err != nil {
				return nil, err
			}
			mods = append(mods, m)
		}
		var tags []string
		for _, t := range a.Pos[2:] {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
		verb := map[string]string{"add": "tagged", "remove": "untagged"}[a.Pos[0]]
		var d *render.Doc
		var root *render.Node
		if len(mods) == 1 {
			d, root = render.New(fmt.Sprintf("%s %s: %s", verb, modLabel(ix, mods[0].ID), strings.Join(tags, ", ")))
		} else {
			d, root = render.New(fmt.Sprintf("%s %s: %s", verb, render.Plural(len(mods), "mod"), strings.Join(tags, ", ")))
		}
		seen := map[string]bool{} // copies of one lineage share tags
		for _, m := range mods {
			if seen[m.LineageID] {
				continue
			}
			seen[m.LineageID] = true
			var same []string
			for _, t := range tags {
				var ok bool
				var err error
				if a.Pos[0] == "add" {
					ok, err = c.App.Store.AddTag(ix.Env.ID, m.LineageID, t)
				} else {
					ok, err = c.App.Store.RemoveTag(ix.Env.ID, m.LineageID, t)
				}
				if err != nil {
					return nil, err
				}
				if !ok {
					same = append(same, t)
				}
			}
			if len(same) > 0 {
				k := "unchanged"
				if len(mods) > 1 {
					k = "unchanged for " + modLabel(ix, m.ID)
				}
				root.KV(k, strings.Join(same, ", "))
			}
		}
		return d, nil
	}
	return nil, &UsageError{Msg: "tag add|remove <ref> <tag…> | list | rename <tag> <new> | merge <tag…> --into <tag> | delete <tag…> | note <tag> [<text>]"}
}

// tagEditDoc reports a rename, merge or delete (or with dryRun what it would do).
func tagEditDoc(e TagEdit, dryRun bool, verb, what string) *render.Doc {
	head := fmt.Sprintf("tag %s: %s", verb, what)
	if dryRun {
		head = fmt.Sprintf("dry run, tag %s: %s", verb, what)
	}
	d, root := render.New(head)
	mods, change, lose := render.Plural(e.Lineages, "mod"), "change", "lose the tag; their"
	if e.Lineages == 1 {
		change, lose = "changes", "loses the tag; its"
	}
	switch {
	case e.Into == "":
		root.KV("mods", fmt.Sprintf("%s %s files and lists are not touched", mods, lose))
	case e.Both > 0:
		root.KV("mods", fmt.Sprintf("%s %s to %s (%d already had it)", mods, change, e.Into, e.Both))
	default:
		root.KV("mods", fmt.Sprintf("%s %s to %s", mods, change, e.Into))
	}
	if dryRun {
		root.KV("undo", "tag changes are not in the undo history")
	}
	return d
}
