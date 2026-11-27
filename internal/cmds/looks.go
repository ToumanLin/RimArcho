package cmds

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/looks"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Looks (21): which Mod's textures the game shows when several Mods of the plan ship a
// texture at the same path, and the choices that decide it without changing the load order.
func init() {
	dry := Flag{Name: "dry-run", Help: "show what would be written (choices, files, link or copy, the folder)"}
	scope := Flag{Name: "scope", Value: "plan|all", Help: "the current plan (default) or all plans"}
	register(&Command{Name: "looks", Summary: "the looks of the current plan: textures that several of its Mods ship at the same path, and which Mod the game shows", MaxArgs: 0, Run: looksCmd,
		Flags: []Flag{
			{Name: "mod", Value: "<ref>", Help: "only the looks this Mod has a texture of"},
			{Name: "filter", Value: "overridden|overriding|mixed|chosen|nodef", Help: "only the looks where a Mod's texture is overridden, overrides another, the game mixes Mods, a choice exists, or no Def uses it"},
			{Name: "search", Value: "<text>", Help: "only the looks whose id or Def contains the text"},
			{Name: "all", Help: "every look (default: the first 30)"},
		}})
	register(&Command{Name: "looks show", Args: "<look-id>", Summary: "one look: its Defs, its slots with every Mod's texture in load order, what the game shows, and the choices", MinArgs: 1, MaxArgs: 1, Run: looksShowCmd})
	register(&Command{Name: "looks apply", Args: "<look-id>", Summary: "show a look, or some of its slots, from a Mod of the plan: writes the choice and rebuilds the generated Mod RimArcho Looks Preference; the load order stays", MinArgs: 1, MaxArgs: 1, Run: looksApplyCmd,
		Flags: []Flag{
			{Name: "from", Value: "<ref>", Help: "the Mod whose textures the game should show"},
			{Name: "slot", Value: "<slot>", Multi: true, Help: "only this slot (repeatable; default: the entire look)"},
			scope, dry}})
	register(&Command{Name: "looks undo", Args: "<look-id>", Summary: "remove a look's choices in one scope; the game shows the load-order result again", MinArgs: 1, MaxArgs: 1, Run: looksUndoCmd,
		Flags: []Flag{scope, dry}})
	register(&Command{Name: "looks rebuild", Summary: "rebuild the generated Mods RimArcho Looks Preference and RimArcho Styles Preference for the current plan (also done before every write to the game)", MaxArgs: 0, Run: looksRebuildCmd,
		Flags: []Flag{dry}})
	registerStyles(dry)
}

func looksState(c *Ctx) (*app.LooksState, error) {
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	return c.App.LooksState(c.Context, st, nil)
}

// lookMod names the plan's Mod at a provider position.
func lookMod(ls *app.LooksState, pos int) string {
	m := ls.Mods[pos]
	if m == nil {
		return fmt.Sprintf("position %d", pos+1)
	}
	return ls.St.Ix.Ref(m.ID) + " " + m.DisplayName()
}

// lookFlags are a look's list states: overridden, mixed, and its choices' states.
func lookFlags(ls *app.LooksState, l *looks.Look) []string {
	var out []string
	for _, p := range l.Providers {
		if l.Overridden(p.Pos) {
			out = append(out, "overridden")
			break
		}
	}
	if l.Mixed(ls.ChoiceOf(l.ID)) {
		out = append(out, "mixed")
	}
	if s := ls.Looks[l.ID]; s != nil {
		seen := map[string]bool{}
		for _, ch := range s.Choices {
			if ch.State != "" && !seen[ch.State] {
				seen[ch.State] = true
				out = append(out, "chosen: "+ch.State)
			}
		}
	}
	if len(l.Defs) == 0 {
		out = append(out, "no Def")
	}
	return out
}

// lookFilter builds the list's filter from --mod, --filter and --search.
func lookFilter(ls *app.LooksState, a *Args) (func(*looks.Look) bool, error) {
	pos := -1
	if ref := a.Str("mod"); ref != "" {
		m, err := ls.St.Ix.ResolveOne(ref)
		if err != nil {
			return nil, err
		}
		if pos = ls.St.List.Index(m.ID); pos < 0 {
			return nil, fmt.Errorf("%s %s is not in plan:%s", ls.St.Ix.Ref(m.ID), m.DisplayName(), ls.St.Plan.Name)
		}
	}
	filter := a.Str("filter")
	switch filter {
	case "", "overridden", "overriding", "mixed", "chosen", "nodef":
	default:
		return nil, &UsageError{Msg: fmt.Sprintf("filter %q: overridden, overriding, mixed, chosen or nodef", filter)}
	}
	search := strings.ToLower(a.Str("search"))
	return func(l *looks.Look) bool {
		if pos >= 0 && !lookHas(l, pos) || search != "" && !lookMatches(l, search) {
			return false
		}
		switch filter {
		case "overridden", "overriding":
			for _, p := range l.Providers {
				if pos >= 0 && p.Pos != pos {
					continue
				}
				if filter == "overridden" && l.Overridden(p.Pos) || filter == "overriding" && l.Overriding(p.Pos) {
					return true
				}
			}
			return false
		case "mixed":
			return l.Mixed(ls.ChoiceOf(l.ID))
		case "chosen":
			return ls.Looks[l.ID] != nil
		case "nodef":
			return len(l.Defs) == 0
		}
		return true
	}, nil
}

func lookHas(l *looks.Look, pos int) bool {
	for _, p := range l.Providers {
		if p.Pos == pos {
			return true
		}
	}
	return false
}

func lookMatches(l *looks.Look, text string) bool {
	if strings.Contains(l.ID, text) {
		return true
	}
	for _, d := range l.Defs {
		if strings.Contains(strings.ToLower(d.Name), text) || strings.Contains(strings.ToLower(d.Label), text) {
			return true
		}
	}
	return false
}

func looksCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ls, err := looksState(c)
	if err != nil {
		return nil, err
	}
	keep, err := lookFilter(ls, a)
	if err != nil {
		return nil, err
	}
	var rows [][]string
	overridden, mixed := 0, 0
	for _, l := range ls.Index.Looks {
		if !keep(l) {
			continue
		}
		flags := lookFlags(ls, l)
		for _, f := range flags {
			switch f {
			case "overridden":
				overridden++
			case "mixed":
				mixed++
			}
		}
		rows = append(rows, []string{l.ID, l.Kind, render.Plural(len(l.Providers), "Mod"), strings.Join(flags, ", ")})
	}
	d, root := render.New(fmt.Sprintf("looks of plan:%s: %s, %d overridden, %d mixed", ls.St.Plan.Name, render.Plural(len(rows), "look"), overridden, mixed))
	looksSummary(root, ls)
	list := root.Section("looks")
	list.AddColumns(rows)
	if !a.Bool("all") {
		list.Truncate(30, "archo looks --all")
	}
	if len(rows) == 0 {
		root.Children = root.Children[:len(root.Children)-1]
	}
	for _, w := range ls.Index.Warnings {
		root.KV("warning", w)
	}
	d.AddNext("archo looks show <look-id>", "archo looks apply <look-id> --from <mod> --dry-run")
	return d, nil
}

// looksSummary adds the choice counts and where RimArcho Looks Preference is in the plan.
func looksSummary(root *render.Node, ls *app.LooksState) {
	total := ls.Applied + ls.NotNeeded + ls.Asleep + ls.Broken
	if total == 0 && ls.Entry == 0 {
		return
	}
	root.KV("choices", fmt.Sprintf("%d applied, %d not needed, %d asleep, %d broken", ls.Applied, ls.NotNeeded, ls.Asleep, ls.Broken))
	switch {
	case ls.Entry == len(ls.St.List):
		root.KV(app.LooksName, fmt.Sprintf("position %d, the plan's last entry", ls.Entry))
	case ls.Entry > 0:
		root.KV(app.LooksName, fmt.Sprintf("position %d of %d: Mods after it override its textures (archo looks rebuild moves it last)", ls.Entry, len(ls.St.List)))
	case ls.Applied > 0:
		root.KV(app.LooksName, "not in the plan: the applied choices do not reach the game (archo looks rebuild adds it)")
	}
}

func looksShowCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ls, err := looksState(c)
	if err != nil {
		return nil, err
	}
	id := strings.ToLower(a.Pos[0])
	l := ls.Index.ByID[id]
	s := ls.Looks[id]
	if l == nil && s == nil {
		return nil, fmt.Errorf("no look %q in plan:%s (archo looks --search <text>)", a.Pos[0], ls.St.Plan.Name)
	}
	if l == nil {
		d, root := render.New("look " + id + "  no Mod of plan:" + ls.St.Plan.Name + " has its textures now")
		lookChoices(root, ls, s)
		return d, nil
	}
	head := "look " + l.ID + "  " + l.Kind
	if flags := lookFlags(ls, l); len(flags) > 0 {
		head += "  " + strings.Join(flags, ", ")
	}
	d, root := render.New(head)
	root.KV("category", l.Category)
	root.KV("tags", strings.Join(l.Tags, ", "))
	if len(l.Defs) > 0 {
		defs := root.Section("defs")
		for _, df := range l.Defs {
			line := df.Type + "/" + df.Name
			if df.Label != "" {
				line += " \"" + df.Label + "\""
			}
			defs.Add(line + "  " + df.Field + "  " + lookMod(ls, df.ModPos))
		}
	}
	for _, sty := range l.Styles {
		root.KV("style", fmt.Sprintf("%s (categories: %s; things: %s)", sty.StyleDef, strings.Join(sty.Categories, ", "), strings.Join(sty.ThingDefs, ", ")))
	}
	lookSlots(root, ls, l)
	if s != nil {
		lookChoices(root, ls, s)
	}
	if l.Kind == looks.KindFolder {
		root.KV("note", "a folder look: a choice replaces the files both Mods name alike; files only other Mods add stay")
	}
	d.AddNext("archo looks apply "+l.ID+" --from <mod> --dry-run", "archo looks undo "+l.ID)
	return d, nil
}

// lookSlots lists each slot's override chain in load order and what the game shows.
func lookSlots(root *render.Node, ls *app.LooksState, l *looks.Look) {
	wins := l.InGame(ls.ChoiceOf(l.ID))
	slots := root.Section("slots")
	for _, name := range l.SlotNames() {
		w := wins[name]
		how := "load order"
		if w.ByChoice {
			how = "by choice"
		}
		n := slots.Addf("%s: in game %s (%s)", name, lookMod(ls, w.Pos), how)
		for _, p := range l.Providers {
			f := p.Slots[name]
			if f == nil {
				continue
			}
			line := fmt.Sprintf("%s  %s  %s %d×%d %s", lookMod(ls, p.Pos), f.Rel, f.Format, f.W, f.H, render.Size(f.Size))
			if f.Alt != nil {
				line += " (+ " + f.Alt.Format + ")"
			}
			if f.SameAs >= 0 {
				line += "  identical to " + lookMod(ls, f.SameAs)
			}
			n.Add(line)
		}
	}
}

func lookChoices(root *render.Node, ls *app.LooksState, s *app.LookState) {
	sec := root.Section("choices")
	for _, ch := range s.Choices {
		scope := "all plans"
		if ch.Plan {
			scope = "plan:" + ls.St.Plan.Name
		}
		who := ls.St.Ix.LRef(ch.Lineage)
		if ch.Mod != nil {
			who = ls.St.Ix.Ref(ch.Mod.ID) + " " + ch.Mod.DisplayName()
		}
		state := ch.State
		switch {
		case ch.Overruled:
			state = "overruled by a choice that comes first"
		case ch.State == app.LookAsleep:
			state += ": that Mod is not in the plan"
		case ch.State == app.LookBroken:
			state += ": that Mod no longer has this texture (archo looks undo " + s.ID + ")"
		case ch.State == app.LookNotNeeded:
			state += ": the load order already shows it"
		}
		sec.Addf("%s  %s from %s  %s", scope, choiceSlot(ch.LookChoice), who, state)
	}
}

func choiceSlot(ch store.LookChoice) string {
	if ch.Slot == looks.All {
		return "entire look"
	}
	return "slot " + ch.Slot
}

func looksApplyCmd(c *Ctx, a *Args) (*render.Doc, error) {
	from := a.Str("from")
	if from == "" {
		return nil, &UsageError{Msg: "name the Mod with --from <ref> (" + commentHint + ")"}
	}
	r, err := c.App.LooksApply(c.Context, a.Pos[0], from, a.List("slot"), a.Str("scope"), a.Bool("dry-run"))
	if err != nil && r == nil {
		return nil, err
	}
	verb := "applied"
	if r.DryRun {
		verb = "would apply"
	}
	d, root := render.New(fmt.Sprintf("%s look %s from %s %s (%s)", verb, r.Look, r.State.St.Ix.Ref(r.From.ID), r.From.DisplayName(), looksScopeText(r)))
	sec := root.Section("choices")
	for _, ch := range r.Set {
		sec.Add(choiceSlot(ch))
	}
	if r.Mixed {
		root.KV("warning", "the look is mixed: the game takes its slots from more than one Mod (archo looks show "+r.Look+")")
	}
	looksResult(root, r)
	return d, err
}

func looksUndoCmd(c *Ctx, a *Args) (*render.Doc, error) {
	r, err := c.App.LooksUndo(c.Context, a.Pos[0], a.Str("scope"), a.Bool("dry-run"))
	if err != nil && r == nil {
		return nil, err
	}
	verb := "removed"
	if r.DryRun {
		verb = "would remove"
	}
	d, root := render.New(fmt.Sprintf("%s %s of look %s (%s)", verb, render.Plural(r.Removed, "choice"), r.Look, looksScopeText(r)))
	looksResult(root, r)
	return d, err
}

func looksRebuildCmd(c *Ctx, a *Args) (*render.Doc, error) {
	r, err := c.App.LooksRebuild(c.Context, a.Bool("dry-run"))
	if err != nil && r == nil {
		return nil, err
	}
	d, root := render.New(app.LooksName + " for plan:" + r.Plan)
	if r.State != nil {
		looksSummary(root, r.State)
	}
	looksResult(root, r)
	if err != nil {
		return d, err
	}
	// The same pass rebuilds RimArcho Styles Preference (21 §5).
	sr, err := c.App.StylesRebuild(c.Context, a.Bool("dry-run"))
	if err != nil && sr == nil {
		return d, err
	}
	if sr.State != nil && (len(sr.State.Entries) > 0 || sr.State.Entry > 0) {
		sec := root.Section(app.StylesName)
		stylesSummary(sec, sr.State)
		stylesResult(sec, sr)
	}
	return d, err
}

func looksScopeText(r *app.LooksResult) string {
	if r.Scope == app.LooksAll {
		return "all plans"
	}
	return "plan:" + r.Plan
}

// looksResult adds what an apply, undo or rebuild wrote, or would write.
func looksResult(root *render.Node, r *app.LooksResult) {
	would := func(done, dry string) string {
		if r.DryRun {
			return dry
		}
		return done
	}
	if r.Added {
		root.KV("plan", would("added ", "would add ")+app.LooksName+" as the last entry of plan:"+r.Plan+" (one step: archo undo)")
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
		root.KV("files", fmt.Sprintf("%s in %s, unchanged: nothing written", render.Plural(len(b.Files), "file"), b.Dir))
		return
	case r.BuildErr != "":
		root.KV("files", fmt.Sprintf("%s due in %s", render.Plural(len(b.Files), "file"), b.Dir))
	default:
		root.KV("files", fmt.Sprintf("%s%s in %s: %s, %s", would("wrote ", "would write "), render.Plural(len(b.Files), "file"), b.Dir, render.Plural(b.Links, "hard link"), render.Plural(b.Copies, "copy")))
	}
	files := root.Children[len(root.Children)-1]
	sort.SliceStable(b.Files, func(i, j int) bool { return b.Files[i].Rel < b.Files[j].Rel })
	for _, f := range b.Files {
		how := "copy of"
		if f.Linked {
			how = "hard link to"
		}
		files.Addf("%s  %s %s", f.Rel, how, f.Source)
	}
	files.Truncate(30, "")
}
