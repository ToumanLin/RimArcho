package cmds

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Styles (21 §5): entries that keep a Mod's version of a look beside the one the game
// shows, as a style in a style category or as a random variant, through the generated Mod
// RimArcho Styles Preference. An entry applies to all plans.
func registerStyles(dry Flag) {
	from := Flag{Name: "from", Value: "<ref>", Help: "the Mod of the current plan whose version of the look is kept"}
	register(&Command{Name: "looks style list", Summary: "the style and random variant entries with their state in the current plan (active, skipped, broken, asleep)", MaxArgs: 0, Run: stylesListCmd})
	register(&Command{Name: "looks style choices", Args: "<look-id>", Summary: "what can be added for a look from a Mod: the style categories and the random variant targets, or why not", MinArgs: 1, MaxArgs: 1, Run: stylesChoicesCmd,
		Flags: []Flag{from}})
	register(&Command{Name: "looks style add", Args: "<look-id>", Summary: "keep a Mod's version of a look as a new style in a style category, or as a random variant of the default look or of a style; for all plans", MinArgs: 1, MaxArgs: 1, Run: stylesAddCmd,
		Flags: []Flag{from,
			{Name: "category", Value: "<StyleCategoryDef>", Help: "add it as a style in this style category"},
			{Name: "random", Help: "with --category: new items may also receive the style at random"},
			{Name: "variant", Value: "default|<ThingStyleDef>", Help: "add it as a random variant of the thing's default look, or of this style"},
			dry}})
	register(&Command{Name: "looks style remove", Args: "<entry>", Summary: "remove one style or random variant entry (its number in looks style list)", MinArgs: 1, MaxArgs: 1, Run: stylesRemoveCmd,
		Flags: []Flag{dry}})
}

func stylesState(c *Ctx) (*app.StylesState, error) {
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	return c.App.StylesState(c.Context, st, nil)
}

// styleKind names an entry's kind and target.
func styleKind(kind, target string, random bool) string {
	switch {
	case kind == app.StyleKindStyle && random:
		return "style in " + target + " (also at random)"
	case kind == app.StyleKindStyle:
		return "style in " + target
	case target == app.StyleTargetDefault:
		return "random variant of the default look"
	}
	return "random variant of the style " + target
}

// styleLine is one entry: its number, look, kind, Mod and state.
func styleLine(ss *app.StylesState, e *app.StyleEntryState) string {
	ix := ss.Ls.St.Ix
	who := ix.LRef(e.Lineage)
	if e.Mod != nil {
		who = ix.Ref(e.Mod.ID) + " " + e.Mod.DisplayName()
	}
	line := fmt.Sprintf("%d  %s  %s from %s  %s", e.ID, e.Look, styleKind(e.Kind, e.Target, e.Random), who, e.State)
	if e.Why.Code != "" {
		line += ": " + e.Why.Text()
	}
	if len(e.Things) > 0 {
		line += "  (" + strings.Join(e.Things, ", ") + ")"
	}
	return line
}

// stylesSummary adds the entry counts and where RimArcho Styles Preference is in the plan.
func stylesSummary(root *render.Node, ss *app.StylesState) {
	root.KV("entries", fmt.Sprintf("%d active, %d skipped, %d broken, %d asleep", ss.Active, ss.Skipped, ss.Broken, ss.Asleep))
	n := len(ss.Ls.St.List)
	switch {
	case ss.Entry == 0 && ss.Active > 0:
		root.KV("plan", app.StylesName+" is not in the plan: the entries do not reach the game (archo looks rebuild adds it)")
	case ss.Entry == 0:
	case ss.Entry >= n-1:
		root.KV("plan", fmt.Sprintf("%s at position %d of %d", app.StylesName, ss.Entry, n))
	default:
		root.KV("plan", fmt.Sprintf("%s at position %d of %d (archo looks rebuild moves it to the end)", app.StylesName, ss.Entry, n))
	}
}

func stylesListCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ss, err := stylesState(c)
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("%s: %s (all plans), states in plan:%s", app.StylesName, render.Plural(len(ss.Entries), "entry"), ss.Ls.St.Plan.Name))
	if len(ss.Entries) == 0 {
		d.AddNext("archo looks style choices <look-id> --from <mod>")
		return d, nil
	}
	stylesSummary(root, ss)
	list := root.Section("entries")
	for i := range ss.Entries {
		list.Add(styleLine(ss, &ss.Entries[i]))
	}
	d.AddNext("archo looks style remove <entry>", "archo looks style add <look-id> --from <mod> --category <StyleCategoryDef> --dry-run")
	return d, nil
}

func stylesChoicesCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ss, err := stylesState(c)
	if err != nil {
		return nil, err
	}
	st := ss.Ls.St
	l := ss.Ls.Index.ByID[strings.ToLower(a.Pos[0])]
	if l == nil {
		return nil, fmt.Errorf("no look %q in plan:%s (archo looks --search <text>)", a.Pos[0], st.Plan.Name)
	}
	if a.Str("from") == "" {
		return nil, &UsageError{Msg: "name the Mod with --from <ref> (" + commentHint + ")"}
	}
	m, err := st.Ix.ResolveOne(a.Str("from"))
	if err != nil {
		return nil, err
	}
	pos := st.List.Index(m.ID)
	if pos < 0 {
		return nil, fmt.Errorf("%s %s is not in plan:%s", st.Ix.Ref(m.ID), m.DisplayName(), st.Plan.Name)
	}
	d, root := render.New(fmt.Sprintf("what can be added for look %s from %s %s", l.ID, st.Ix.Ref(m.ID), m.DisplayName()))
	for _, ch := range ss.StyleChoices(l.ID, pos) {
		line := styleKind(ch.Kind, ch.Target, false)
		switch {
		case ch.Why.Code != "":
			line += "  no: " + ch.Why.Text()
		case ch.Shared:
			line += "  (" + strings.Join(ch.Things, ", ") + "; the category already has a style for it: the game uses one of them per ideoligion, not per item)"
		default:
			line += "  (" + strings.Join(ch.Things, ", ") + ")"
		}
		root.Add(line)
	}
	d.AddNext("archo looks style add "+l.ID+" --from <mod> --category <StyleCategoryDef> [--random] --dry-run", "archo looks style add "+l.ID+" --from <mod> --variant default --dry-run")
	return d, nil
}

func stylesAddCmd(c *Ctx, a *Args) (*render.Doc, error) {
	in := app.StyleAddInput{Look: a.Pos[0], Mod: a.Str("from"), Random: a.Bool("random")}
	cat, variant := a.Str("category"), a.Str("variant")
	switch {
	case in.Mod == "":
		return nil, &UsageError{Msg: "name the Mod with --from <ref> (" + commentHint + ")"}
	case (cat == "") == (variant == ""):
		return nil, &UsageError{Msg: "give --category <StyleCategoryDef> for a style, or --variant default|<ThingStyleDef> for a random variant"}
	case cat != "":
		in.Kind, in.Target = app.StyleKindStyle, cat
	default:
		if in.Random {
			return nil, &UsageError{Msg: "--random goes with --category"}
		}
		in.Kind, in.Target = app.StyleKindVariant, variant
	}
	r, err := c.App.StyleAdd(c.Context, in, a.Bool("dry-run"))
	if err != nil && r == nil {
		return nil, err
	}
	verb := "added"
	if r.DryRun {
		verb = "would add"
	}
	e := r.Entry
	d, root := render.New(fmt.Sprintf("%s look %s as %s (all plans)", verb, e.Look, styleKind(e.Kind, e.Target, e.Random)))
	if e.ID > 0 {
		root.KV("entry", strconv.FormatInt(e.ID, 10))
	}
	root.KV("things", strings.Join(e.Things, ", "))
	switch {
	case e.Kind == app.StyleKindStyle && e.Shared:
		root.KV("note", "the category already has a style for this thing: the game uses one of them per ideoligion, not per item")
	case e.Kind == app.StyleKindVariant:
		root.KV("note", "items already in a save may change look; which variant an item shows follows its ID")
	}
	stylesResult(root, r)
	return d, err
}

func stylesRemoveCmd(c *Ctx, a *Args) (*render.Doc, error) {
	id, perr := strconv.ParseInt(a.Pos[0], 10, 64)
	if perr != nil {
		return nil, &UsageError{Msg: fmt.Sprintf("entry %q: the number archo looks style list shows", a.Pos[0])}
	}
	r, err := c.App.StyleRemove(c.Context, id, a.Bool("dry-run"))
	if err != nil && r == nil {
		return nil, err
	}
	verb := "removed"
	if r.DryRun {
		verb = "would remove"
	}
	d, root := render.New(fmt.Sprintf("%s entry %d: look %s as %s", verb, r.Entry.ID, r.Entry.Look, styleKind(r.Entry.Kind, r.Entry.Target, r.Entry.Random)))
	root.KV("note", "a save that has items with a removed style logs a missing Def when it loads and shows them in their default look")
	stylesResult(root, r)
	return d, err
}

// stylesResult adds what an add, remove or rebuild wrote, or would write.
func stylesResult(root *render.Node, r *app.StylesResult) {
	would := func(done, dry string) string {
		if r.DryRun {
			return dry
		}
		return done
	}
	if r.Added {
		root.KV("plan", would("put ", "would put ")+app.StylesName+" at the end of plan:"+r.Plan+", before "+app.LooksName+" (one step: archo undo)")
	}
	if r.Dir != "" {
		dir := r.Dir
		if r.DirChosen {
			dir += would(" (chosen now: looks.dir)", " (would be chosen: looks.dir)")
		}
		root.KV("folder", dir)
	}
	if r.BuildErr != "" {
		root.KV("not rebuilt", r.BuildErr)
	}
	b := r.Build
	if b == nil {
		return
	}
	switch {
	case b.Unchanged:
		root.KV("files", fmt.Sprintf("%s and %s in %s, unchanged: nothing written", render.Plural(len(b.Files), "texture"), render.Plural(len(b.Texts), "Def file"), b.Dir))
		return
	case r.BuildErr != "":
		root.KV("files", fmt.Sprintf("%s and %s due in %s", render.Plural(len(b.Files), "texture"), render.Plural(len(b.Texts), "Def file"), b.Dir))
	default:
		root.KV("files", fmt.Sprintf("%s%s and %s in %s: %s, %s", would("wrote ", "would write "), render.Plural(len(b.Files), "texture"),
			render.Plural(len(b.Texts), "Def file"), b.Dir, render.Plural(b.Links, "hard link"), render.Plural(b.Copies, "copy")))
	}
	files := root.Children[len(root.Children)-1]
	for _, t := range b.Texts {
		files.Add(t.Rel + "  generated")
	}
	for _, f := range b.Files {
		how := "copy of"
		if f.Linked {
			how = "hard link to"
		}
		files.Addf("%s  %s %s", f.Rel, how, f.Source)
	}
	files.Truncate(30, "")
}
