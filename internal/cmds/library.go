package cmds

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Mod 库存 (06 §2.9): every mod on this machine, one line per Lineage; --lineage shows each
// copy. The GUI page, the TUI list and MCP list_library filter the same app.Library.
func init() {
	flags := []Flag{
		{Name: "search", Value: "<words>", Help: "all words in a name, package id, author, alias, tag, note, Workshop ID or folder"},
		{Name: "include", Value: "<key:value>", Multi: true, Help: "keep copies with this value; keys tag, source, location (source folder path), version, state, used, updated (week, month, half-year, older); one key's values: either, different keys: all"},
		{Name: "exclude", Value: "<key:value>", Multi: true, Help: "drop copies with this value"},
		{Name: "sort", Value: "name|size|state|source|used|updated|version|author|tags|changed|played", Help: "default name; a leading - reverses"},
		{Name: "lineage", Value: "<ref>", Help: "one mod: every copy with its path, states, plans and last launch"},
		{Name: "facets", Help: "list each filter's values with how many mods they show"},
		{Name: "all", Help: "do not truncate"},
	}
	register(&Command{Name: "library", Summary: "the mod library: every mod on this machine, its copies, sizes, states and the plans and packs using it",
		MaxArgs: 0, Flags: flags, Run: libraryCmd})
	register(&Command{Name: "library remove-from-plans", Args: "<ref…>", Summary: "take mods out of every plan that has them (one undo step per plan)",
		MinArgs: 1, MaxArgs: -1, Run: libraryRemoveCmd})
	register(&Command{Name: "library move-to-locked", Args: "<ref>", Summary: "move a spare copy out of Mods/ into the lock library; plans keep using it",
		MinArgs: 1, MaxArgs: 1, Flags: []Flag{{Name: "reason", Value: "<text>", Help: "why: kept in the copy's note"}, {Name: "dry-run", Help: "show what would move"}}, Run: libraryMoveCmd})
}

// libStateText is a state in the fixed English of the text output (05 §3).
func libStateText(s, mm string) string {
	switch s {
	case app.LibUnloadable:
		return "unloadable"
	case app.LibMissing:
		return "subscribed, not on disk"
	case app.LibIgnored:
		return "ignored by the game (another copy in Mods/ wins)"
	case app.LibResidue:
		return "unsubscribed leftover"
	case app.LibHidden:
		return "hidden by another manager"
	case app.LibUpdated:
		return "upstream updated"
	case app.LibSourceUpdate:
		return "source update available"
	}
	return s
}

func libKind(c *Ctx, i *app.LibInstance) string {
	if i.Source == app.LibGit {
		return "Git"
	}
	if i.Source == app.LibPage {
		return "download page"
	}
	if i.Source == app.LibLocked && !i.Created.IsZero() {
		return "locked " + c.App.FormatTime(i.Created)
	}
	if i.Source == app.LibLocal && len(i.DirectoryValues) > 0 { // local-<game>: which game's Mods/
		return i.DirectoryValues[0]
	}
	return i.Source
}

func libSize(n int64, ok bool) string {
	if !ok {
		return "—"
	}
	return render.Size(n)
}

func libUses(us []app.LibUse) string {
	switch len(us) {
	case 0:
		return ""
	case 1:
		return us[0].Value()
	}
	return fmt.Sprintf("%s +%d", us[0].Value(), len(us)-1)
}

func libraryQuery(a *Args) app.LibQuery {
	return app.LibQuery{Search: a.Str("search"), Include: a.List("include"), Exclude: a.List("exclude"), Sort: a.Str("sort")}
}

// libraryArgs repeats the query for the "… more" and next: commands.
func libraryArgs(q app.LibQuery) string {
	var s []string
	if q.Search != "" {
		s = append(s, "--search "+quote(q.Search))
	}
	for _, f := range q.Include {
		s = append(s, "--include "+quote(f))
	}
	for _, f := range q.Exclude {
		s = append(s, "--exclude "+quote(f))
	}
	if q.Sort != "" {
		s = append(s, "--sort "+q.Sort)
	}
	return strings.Join(s, " ")
}

func libraryCmd(c *Ctx, a *Args) (*render.Doc, error) {
	lib, err := c.App.Library()
	if err != nil {
		return nil, err
	}
	if ref := a.Str("lineage"); ref != "" {
		l, err := lib.Lineage(ref)
		if err != nil {
			return nil, err
		}
		return libraryLineageDoc(c, lib, l), nil
	}
	q := libraryQuery(a)
	res, err := lib.Filter(q)
	if err != nil {
		return nil, err
	}
	copies, total := 0, int64(0)
	for _, l := range lib.Lineages {
		copies += len(l.Instances)
		n, _ := l.Bytes()
		total += n
	}
	head := fmt.Sprintf("library: %s, %s, %s (%s · %s)", render.Plural(len(lib.Lineages), "mod"), render.Plural(copies, "copy"), render.Size(total), envRef(lib.Env), lib.GameMM)
	var conds []string
	if q.Search != "" {
		conds = append(conds, "search "+quote(q.Search))
	}
	conds = append(conds, q.Include...)
	for _, f := range q.Exclude {
		conds = append(conds, "-"+f)
	}
	if len(conds) > 0 {
		head = fmt.Sprintf("library: %d of %s match (%s)", len(res.Matches), render.Plural(len(lib.Lineages), "mod"), strings.Join(conds, ", "))
	}
	d, root := render.New(head)
	if a.Bool("facets") {
		fs := root.Section("filters")
		for _, f := range res.Facets {
			var vs []string
			for _, v := range f.Values {
				mark := ""
				switch v.On {
				case 1:
					mark = "+"
				case -1:
					mark = "-"
				}
				vs = append(vs, fmt.Sprintf("%s%s %d", mark, v.Value, v.Count))
			}
			if len(vs) == 0 {
				vs = []string{"—"}
			}
			fs.KV(f.Key, strings.Join(vs, ", "))
		}
		return d, nil
	}
	var rows [][]string
	for _, m := range res.Matches {
		var parts []string
		switch {
		case len(m.Instances) == 1:
			parts = append(parts, libKind(c, m.Instances[0]))
		case len(m.Shown) < len(m.Instances):
			parts = append(parts, fmt.Sprintf("%d of %d copies", len(m.Shown), len(m.Instances)))
		default:
			parts = append(parts, render.Plural(len(m.Instances), "copy"))
		}
		parts = append(parts, libSize(m.Bytes()))
		if s := m.State(); s != "" {
			parts = append(parts, libStateText(s, lib.GameMM))
		}
		if u := libUses(m.UsedIn()); u != "" {
			parts = append(parts, u)
		}
		rows = append(rows, []string{m.Ref, m.Name, "[" + strings.Join(parts, ", ") + "]"})
	}
	if len(rows) > render.DefaultLimit && !a.Bool("all") {
		more := len(rows) - render.DefaultLimit
		root.AddColumns(rows[:render.DefaultLimit])
		expand := strings.TrimSpace("archo library " + libraryArgs(q) + " --all")
		root.Addf("… %d more (%s)", more, expand)
	} else {
		root.AddColumns(rows)
	}
	switch {
	case len(lib.Lineages) == 0:
		d.AddNext("archo scan")
	case len(res.Matches) == 1:
		d.AddNext("archo library --lineage " + res.Matches[0].Ref)
	case len(res.Matches) == 0 && len(conds) > 0:
		d.AddNext("archo library --facets " + libraryArgs(q))
	}
	return d, nil
}

func libraryLineageDoc(c *Ctx, lib *app.Library, l *app.LibLineage) *render.Doc {
	n, ok := l.Bytes()
	d, root := render.New(fmt.Sprintf("%s %s  [%s, %s]", l.Ref, l.Name, render.Plural(len(l.Instances), "copy"), libSize(n, ok)))
	if len(l.Tags) > 0 {
		root.KV("tags", strings.Join(l.Tags, ", "))
	}
	if l.Alias != "" {
		root.KV("alias", l.Alias)
	}
	if l.Note != "" {
		root.KV("note", l.Note)
	}
	for _, i := range l.Instances {
		line := i.Ref + " " + libKind(c, i) + "  " + libSize(i.Bytes, i.Sized)
		if i.InPlan {
			line += "  (current plan)"
		}
		x := root.Add(line)
		subscriptionLines(c, x, i.Subscription)
		if i.Path != "" {
			x.KV("path", i.Path)
		}
		if i.WorkshopID != "" && isDigits(i.WorkshopID) {
			x.KV("workshop", "https://steamcommunity.com/sharedfiles/filedetails/?id="+i.WorkshopID)
		}
		if len(i.Versions) > 0 {
			x.KV("supports", strings.Join(i.Versions, " "))
		}
		if !i.Updated.IsZero() {
			x.KV("updated", c.App.FormatTime(i.Updated))
		}
		if n := copyNoteText(c, i); n != "" {
			x.KV("note", n)
		}
		if i.Locked != nil {
			x.KV("upstream", modLabel(lib.St.Ix, i.Locked.Upstream.ID))
		}
		var ss []string
		for _, s := range i.States {
			ss = append(ss, libStateText(s, lib.GameMM))
		}
		if len(ss) > 0 {
			x.KV("states", strings.Join(ss, ", "))
		}
		if len(i.UsedIn) > 0 {
			var us []string
			for _, u := range i.UsedIn {
				us = append(us, u.Value())
			}
			x.KV("used in", strings.Join(us, ", "))
		}
		if !i.Played.IsZero() {
			x.KV("played", c.App.FormatTime(i.Played))
		}
		for _, cmd := range libNext(lib.St.Ix, i) {
			d.AddNext(cmd)
		}
	}
	return d
}

// copyNoteText is a copy's note: the player's words, else what RimArcho knows about how it
// made the copy (the window shows the same in the interface language, view.CopyNote).
func copyNoteText(c *Ctx, i *app.LibInstance) string {
	if i.Note != nil && i.Note.Text != "" {
		return i.Note.Text
	}
	var parts []string
	var made *store.Made
	if i.Note != nil {
		made = i.Note.Made
	}
	switch {
	case made != nil:
		what := map[string]string{"pin": "locked version made", "pin-update": "new locked version made", "move": "moved into the lock library"}[made.Kind]
		s := what + " " + c.App.FormatDate(made.At)
		if len(made.Plans) > 0 {
			s += " for plan:" + strings.Join(made.Plans, ", plan:")
		}
		parts = append(parts, s)
	case i.Locked != nil && !i.Created.IsZero():
		parts = append(parts, "locked version made "+c.App.FormatDate(i.Created))
	}
	if i.Locked != nil && i.UpstreamUpdates > 0 {
		parts = append(parts, fmt.Sprintf("the upstream updated %s since", render.Plural(i.UpstreamUpdates, "time")))
	}
	if made != nil && made.Reason != "" {
		parts = append(parts, "reason: "+made.Reason)
	}
	if len(parts) > 0 {
		return "(by RimArcho) " + strings.Join(parts, "; ")
	}
	return ""
}

// libNext is what each state's action is as a command (06 §2.9's table).
func libNext(ix *app.Index, i *app.LibInstance) []string {
	if i.Mod == nil {
		return nil
	}
	var out []string
	if i.Source == app.LibWorkshop && i.Locked == nil && ix.CanResubscribe(i.Mod) && !i.Mod.Residue {
		out = append(out, "archo workshop resubscribe "+i.Ref+" --dry-run")
	}
	for _, s := range i.States {
		switch s {
		case app.LibHidden:
			out = append(out, "archo unhide "+i.Ref+" --dry-run")
		case app.LibResidue:
			out = append(out, "archo workshop subscribe "+i.Ref, "archo trash "+i.Ref+" --dry-run")
		case app.LibUpdated:
			out = append(out, "archo pin --update "+i.Ref+" --dry-run")
		case app.LibSourceUpdate:
			from := ""
			if i.Subscription != nil && i.Subscription.Kind == "page" {
				from = " --from <zip|folder>"
			}
			out = append(out, "archo subscribe update "+i.Ref+from+" --dry-run")
		}
	}
	if i.Spare && i.Source == app.LibLocal {
		out = append(out, "archo library move-to-locked "+i.Ref+" --dry-run")
	}
	return out
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func libraryRemoveCmd(c *Ctx, a *Args) (*render.Doc, error) {
	rs, err := c.App.RemoveFromPlans(a.Pos)
	if err != nil {
		return nil, err
	}
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	what := strings.Join(a.Pos, " ")
	if len(a.Pos) == 1 {
		if m, err := ix.ResolveOne(a.Pos[0]); err == nil {
			what = modLabel(ix, m.ID)
		}
	}
	if len(rs) == 0 {
		d, _ := render.New("nothing changed: no plan uses " + what)
		return d, nil
	}
	d, root := render.New(fmt.Sprintf("removed %s from %s", what, render.Plural(len(rs), "plan")))
	cur := false
	for _, r := range rs {
		how := "undo it in that plan: archo plan use " + quote(r.Plan) + ", then archo undo"
		if strings.Contains(r.Plan, "/") {
			how = "another environment's plan; undo it there"
		}
		if r.Current {
			how, cur = "archo undo reverts it", true
		}
		root.KV("plan:"+r.Plan, fmt.Sprintf("%s (%s)", render.Plural(r.Removed, "entry"), how))
	}
	if cur {
		d.AddNext("archo save")
	}
	return d, nil
}

func libraryMoveCmd(c *Ctx, a *Args) (*render.Doc, error) {
	r, err := c.App.MoveToLockedFor(a.Pos[0], a.Bool("dry-run"), a.Str("reason"))
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
	d, root := render.New(fmt.Sprintf("%s %s %s", ix.Ref(r.Mod.ID), r.Mod.DisplayName(), head))
	moveKVs(ix, root, r)
	if r.Origin {
		w := "written (the folder had none)"
		if r.DryRun {
			w = "would be written (the folder has none)"
		}
		root.KV("marker", ".rimarcho/origin.json "+w)
	}
	plans := "none"
	if len(r.Plans) > 0 {
		plans = strings.Join(r.Plans, ", ") + " keep it: the same instance; affected game links are restored during the move"
	}
	root.KV("plans", plans)
	switch {
	case r.DryRun:
		d.AddNext("archo library move-to-locked " + a.Pos[0])
	case len(r.Plans) > 0:
		d.AddNext("archo save")
	}
	return d, nil
}
