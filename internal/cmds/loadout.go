package cmds

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/order"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/store"
)

var placeFlags = []Flag{
	{Name: "at", Value: "<n>", Help: "1-based position"},
	{Name: "before", Value: "<ref>"},
	{Name: "after", Value: "<ref>"},
}

func init() {
	register(&Command{Name: "enable", Args: "<ref…>", Summary: "add mods to the workspace Loadout", MinArgs: 1, MaxArgs: -1, Flags: placeFlags, Run: enableCmd})
	register(&Command{Name: "disable", Args: "<ref…>", Summary: "remove mods from the workspace Loadout", MinArgs: 1, MaxArgs: -1, Run: disableCmd})
	register(&Command{Name: "move", Args: "<ref>", Summary: "move an enabled mod", MinArgs: 1, MaxArgs: 1,
		Flags: []Flag{{Name: "to", Value: "<n>"}, {Name: "before", Value: "<ref>"}, {Name: "after", Value: "<ref>"}, {Name: "up", Value: "", Help: "one up"}, {Name: "down", Help: "one down"}}, Run: moveCmd})
	register(&Command{Name: "swap", Args: "<ref> [<ref>]", Summary: "replace an enabled mod by another instance of its lineage", MinArgs: 1, MaxArgs: 2, Run: swapCmd})
	register(&Command{Name: "sort", Summary: "sort the workspace by load-order constraints", MaxArgs: 0,
		Flags: []Flag{{Name: "dry-run", Help: "show the result without changing the workspace"}}, Run: sortCmd})
	register(&Command{Name: "undo", Summary: "undo the last workspace edit", MaxArgs: 0, Run: func(c *Ctx, a *Args) (*render.Doc, error) { return undoRedo(c, false) }})
	register(&Command{Name: "redo", Summary: "redo an undone workspace edit", MaxArgs: 0, Run: func(c *Ctx, a *Args) (*render.Doc, error) { return undoRedo(c, true) }})
	register(&Command{Name: "save", Summary: "write the workspace to ModsConfig.xml (backup + atomic replace)", MaxArgs: 0,
		Flags: []Flag{{Name: "dry-run", Help: "show what would be written"}}, Run: saveCmd})
	register(&Command{Name: "loadout log", Summary: "saved and imported Loadout versions", MaxArgs: 0, Flags: []Flag{{Name: "all", Help: "no truncation"}}, Run: loadoutLog})
	register(&Command{Name: "loadout diff", Args: "[<v>] [<v>]", Summary: "differences between versions (default: newest version → workspace)", MaxArgs: 2,
		Flags: []Flag{{Name: "all", Help: "no truncation"}}, Run: loadoutDiff})
	register(&Command{Name: "loadout checkout", Args: "<v>", Summary: "load a version into the workspace (save to apply)", MinArgs: 1, MaxArgs: 1, Run: loadoutCheckout})
	register(&Command{Name: "loadout tag", Args: "<v> <name>", Summary: "name a version", MinArgs: 2, MaxArgs: 2, Run: loadoutTag})
	register(&Command{Name: "rule add", Args: "after|before|incompatible|attach <A> <B> | ignore <constraint>", Summary: "add a user rule",
		MinArgs: 2, MaxArgs: 3, Flags: []Flag{{Name: "note", Value: "<text>"}}, Run: ruleAdd})
	register(&Command{Name: "rule list", Summary: "list user rules", MaxArgs: 0, Run: ruleList})
	register(&Command{Name: "rule remove", Args: "<r:n>", Summary: "delete a user rule", MinArgs: 1, MaxArgs: 1, Run: ruleRemove})
	register(&Command{Name: "launch", Summary: "start the game", MaxArgs: 0,
		Flags: []Flag{{Name: "dry-run", Help: "show the command only"}, {Name: "force", Help: "launch with the saved list despite unsaved edits"}}, Run: launchCmd})
}

// changeRows renders Loadout changes as aligned rows.
func changeRows(c *Ctx, st *app.LoadoutState, chs []loadout.Change) [][]string {
	var rows [][]string
	name := func(e loadout.Entry) (string, string) {
		if e.Instance == "" {
			return "pkg:" + e.Missing, "(missing)"
		}
		ref := st.Ix.Ref(e.Instance)
		if m := st.Ix.ByID[e.Instance]; m != nil {
			return ref, m.DisplayName()
		}
		return ref, ""
	}
	for _, ch := range chs {
		ref, n := name(ch.Entry)
		var where string
		switch ch.Kind {
		case loadout.Added:
			where = fmt.Sprintf("at %d", ch.To)
		case loadout.Removed:
			where = fmt.Sprintf("was %d", ch.From)
		case loadout.Moved:
			where = fmt.Sprintf("%d → %d", ch.From, ch.To)
		case loadout.Replaced:
			old, _ := name(ch.Old)
			where = fmt.Sprintf("replaces %s at %d", old, ch.To)
		}
		rows = append(rows, []string{ch.Kind, ref, n, where})
	}
	return rows
}

func addChanges(c *Ctx, n *render.Node, st *app.LoadoutState, chs []loadout.Change, limit int, expand string) {
	rows := changeRows(c, st, chs)
	shown := rows
	if limit > 0 && len(rows) > limit {
		shown = rows[:limit]
	}
	n.AddColumns(shown)
	if len(shown) < len(rows) {
		n.Addf("… %d more (%s)", len(rows)-len(shown), expand)
	}
}

func activeCount(st *app.LoadoutState) int { return len(st.List) }

// workspaceLine summarises the workspace: size and unsaved changes.
func workspaceLine(st *app.LoadoutState) string {
	s := fmt.Sprintf("workspace: %d active", activeCount(st))
	if n := len(st.Changes()); n > 0 {
		s += fmt.Sprintf(", %s unsaved", render.Plural(n, "change"))
	} else {
		s += ", saved"
	}
	return s + " (" + envRef(st.Ix.Env) + ")"
}

func editDoc(c *Ctx, r *app.EditResult) *render.Doc {
	d, root := render.New(workspaceLine(r.State))
	if len(r.Changes) == 0 {
		root.Add("no change")
	}
	addChanges(c, root, r.State, r.Changes, render.DefaultLimit, "archo loadout diff --all")
	for _, n := range r.Notes {
		root.KV("note", n)
	}
	if len(r.State.Changes()) > 0 {
		d.AddNext("archo save")
	}
	return d
}

func placement(a *Args, toFlag string) (app.Placement, error) {
	var p app.Placement
	if v := a.Str(toFlag); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return p, &UsageError{Msg: "--" + toFlag + " needs a position ≥ 1"}
		}
		p.At = n
	}
	p.Before, p.After = a.Str("before"), a.Str("after")
	if a.Bool("up") {
		p.Delta = -1
	}
	if a.Bool("down") {
		p.Delta = 1
	}
	return p, nil
}

func enableCmd(c *Ctx, a *Args) (*render.Doc, error) {
	p, err := placement(a, "at")
	if err != nil {
		return nil, err
	}
	r, err := c.App.Enable(a.Pos, p)
	if err != nil {
		return nil, err
	}
	return editDoc(c, r), nil
}

func disableCmd(c *Ctx, a *Args) (*render.Doc, error) {
	r, err := c.App.Disable(a.Pos)
	if err != nil {
		return nil, err
	}
	return editDoc(c, r), nil
}

func moveCmd(c *Ctx, a *Args) (*render.Doc, error) {
	p, err := placement(a, "to")
	if err != nil {
		return nil, err
	}
	r, err := c.App.Move(a.Pos[0], p)
	if err != nil {
		return nil, err
	}
	return editDoc(c, r), nil
}

func swapCmd(c *Ctx, a *Args) (*render.Doc, error) {
	with := ""
	if len(a.Pos) == 2 {
		with = a.Pos[1]
	}
	r, err := c.App.Swap(a.Pos[0], with)
	if err != nil {
		return nil, err
	}
	return editDoc(c, r), nil
}

func undoRedo(c *Ctx, redo bool) (*render.Doc, error) {
	r, op, err := c.App.UndoRedo(redo)
	if err != nil {
		return nil, err
	}
	d := editDoc(c, r)
	verb := "undid"
	if redo {
		verb = "redid"
	}
	d.Nodes[0].Children = append([]*render.Node{{Line: verb + ": " + op}}, d.Nodes[0].Children...)
	return d, nil
}

// constraintText describes one sort edge for evidence lines.
func constraintText(st *app.LoadoutState, in order.Input, e order.Edge) (evidence, ignore string) {
	ref := func(i int) string {
		if i < 0 || i >= len(in.Nodes) {
			return "?"
		}
		nd := in.Nodes[i]
		if nd.Instance == "" {
			return "pkg:" + nd.PackageID
		}
		return st.Ix.Ref(nd.Instance)
	}
	owner := ref(e.C.Owner)
	switch e.C.Kind {
	case order.KindOfficial:
		return fmt.Sprintf("builtin %s official content first (pkg:%s)", owner, e.C.Target), "official:" + owner
	case order.KindCommunity:
		return fmt.Sprintf("community pkg:%s %s pkg:%s", in.Nodes[e.C.Owner].PackageID, e.C.Field, e.C.Target), e.C.Ref
	case order.KindUser:
		return fmt.Sprintf("rule r:%d %s %s pkg:%s", e.C.RuleID, owner, e.C.Field, e.C.Target), fmt.Sprintf("archo rule remove r:%d", e.C.RuleID)
	default:
		// The stored reference is keyed by lineage; print it with the declaring instance,
		// which `rule add ignore` resolves back to the same lineage.
		ig := e.C.Ref
		if parts := strings.Split(e.C.Ref, ":"); len(parts) == 4 {
			ig = "decl:" + owner + ":" + parts[2] + ":" + parts[3]
		}
		return fmt.Sprintf("decl %s About.xml %s pkg:%s", owner, e.C.Field, e.C.Target), ig
	}
}

func sortCmd(c *Ctx, a *Args) (*render.Doc, error) {
	dry := a.Bool("dry-run")
	r, err := c.App.Sort(dry)
	if err != nil {
		return nil, err
	}
	st := r.State
	moved := 0
	for _, ch := range r.Changes {
		if ch.Kind == loadout.Moved {
			moved++
		}
	}
	head := fmt.Sprintf("sort: %d moved", moved)
	switch {
	case dry:
		head += " (dry run; the workspace is unchanged)"
	case moved > 0:
		head += " (applied to the workspace; not saved)"
	}
	d, root := render.New(head)
	addChanges(c, root, st, r.Changes, render.DefaultLimit, "archo loadout diff --all")
	if len(r.Result.Cycles) > 0 {
		pr := root.Section("problems")
		for _, cy := range r.Result.Cycles {
			var refs []string
			for _, m := range cy.Members {
				nd := r.Input.Nodes[m]
				if nd.Instance != "" {
					refs = append(refs, st.Ix.Ref(nd.Instance))
				}
			}
			p := pr.Addf("warning order.cycle  %s  (kept in current order)", strings.Join(refs, " "))
			seen := map[string]bool{}
			for i, e := range cy.Edges {
				ev, ig := constraintText(st, r.Input, e)
				if seen[ev] {
					continue
				}
				seen[ev] = true
				line := "evidence: " + ev
				if strings.HasPrefix(ig, "archo ") {
					line += "  (" + ig + ")"
				} else {
					line += "  (ignore: " + ig + ")"
				}
				if i == cy.Suggest {
					line += "  [suggested]"
				}
				p.Add(line)
			}
			p.Truncate(12, "")
		}
		if cy := r.Result.Cycles[0]; cy.Suggest >= 0 {
			_, ig := constraintText(st, r.Input, cy.Edges[cy.Suggest])
			d.AddNext("archo rule add ignore " + ig)
		}
	}
	root.KV("constraints", fmt.Sprintf("%d edges, %d ignored by rules", len(r.Result.Edges), r.Result.Ignored))
	if dry && moved > 0 {
		d.AddNext("archo sort")
	} else if !dry && len(st.Changes()) > 0 {
		d.AddNext("archo save")
	}
	return d, nil
}

func saveCmd(c *Ctx, a *Args) (*render.Doc, error) {
	r, err := c.App.Save(a.Bool("dry-run"))
	if err != nil {
		return nil, err
	}
	var d *render.Doc
	var root *render.Node
	if r.DryRun {
		d, root = render.New("save: dry run, nothing written")
	} else {
		d, root = render.New(fmt.Sprintf("saved v%d to ModsConfig.xml", r.Version))
	}
	root.KV("path", r.Path)
	if r.Backup != "" {
		root.KV("backup", r.Backup)
	}
	switch {
	case r.OldVersion == "":
		root.KV("version", r.NewVersion)
	case env.MajorMinor(r.OldVersion) != env.MajorMinor(r.NewVersion):
		root.KV("version", fmt.Sprintf("%s → %s (rewritten from %s to %s; the game would otherwise reset the list)",
			r.OldVersion, r.NewVersion, env.MajorMinor(r.OldVersion), env.MajorMinor(r.NewVersion)))
	case r.OldVersion != r.NewVersion:
		root.KV("version", r.OldVersion+" → "+r.NewVersion)
	default:
		root.KV("version", r.NewVersion)
	}
	entries := fmt.Sprintf("%d", len(r.Tokens))
	if r.Missing > 0 {
		entries += fmt.Sprintf(" (%d missing entries kept)", r.Missing)
	}
	root.KV("entries", entries)
	if r.Source == "undo" {
		root.KV("source", fmt.Sprintf("undo (restores v%d)", r.Restores))
	}
	base := "workspace"
	if r.State.Latest != nil {
		base = fmt.Sprintf("v%d", r.State.Latest.N)
	}
	ch := root.KV("changes", fmt.Sprintf("%d since %s", len(r.Changes), base))
	if ch != nil {
		addChanges(c, ch, r.State, r.Changes, render.DefaultLimit/2, "archo loadout diff --all")
	}
	if r.DryRun {
		d.AddNext("archo save")
	} else {
		d.AddNext("archo launch")
	}
	return d, nil
}

func loadoutLog(c *Ctx, a *Args) (*render.Doc, error) {
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	vs, err := c.App.Store.Versions(st.Ix.Env.ID)
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("versions: %d (%s)", len(vs), envRef(st.Ix.Env)))
	root.Add(workspaceLine(st))
	if st.WS.Conflict != 0 {
		root.KV("pending", fmt.Sprintf("v%d came from outside RimArcho and was not loaded (archo loadout checkout v%d)", st.WS.Conflict, st.WS.Conflict))
	}
	var rows [][]string
	for _, v := range vs {
		tag := ""
		if len(v.Tags) > 0 {
			tag = "[" + strings.Join(v.Tags, ", ") + "]"
		}
		note := v.Note
		if tag != "" {
			note = strings.TrimSpace(tag + " " + note)
		}
		gv := v.GameVersion
		if gv == "" {
			gv = "-"
		}
		rows = append(rows, []string{fmt.Sprintf("v%d", v.N), c.App.FormatTime(v.Created), v.Source,
			render.Plural(len(loadout.Decode(v.Entries)), "entry"), gv, note})
	}
	limit := render.DefaultLimit
	if a.Bool("all") {
		limit = 0
	}
	shown := rows
	if limit > 0 && len(rows) > limit {
		shown = rows[:limit]
	}
	root.AddColumns(shown)
	if len(shown) < len(rows) {
		root.Addf("… %d more (archo loadout log --all)", len(rows)-len(shown))
	}
	return d, nil
}

func loadoutDiff(c *Ctx, a *Args) (*render.Doc, error) {
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	envID := st.Ix.Env.ID
	from, to := 0, 0
	if st.Latest != nil {
		from = st.Latest.N
	}
	name := func(n int) string {
		if n == 0 {
			return "workspace"
		}
		return fmt.Sprintf("v%d", n)
	}
	switch len(a.Pos) {
	case 1:
		if from, err = c.App.ParseVersionRef(envID, a.Pos[0]); err != nil {
			return nil, err
		}
	case 2:
		if from, err = c.App.ParseVersionRef(envID, a.Pos[0]); err != nil {
			return nil, err
		}
		if to, err = c.App.ParseVersionRef(envID, a.Pos[1]); err != nil {
			return nil, err
		}
	}
	la, err := c.App.ListAt(st, from)
	if err != nil {
		return nil, err
	}
	lb, err := c.App.ListAt(st, to)
	if err != nil {
		return nil, err
	}
	chs := loadout.Diff(la, lb, st.LineageOf)
	d, root := render.New(fmt.Sprintf("diff %s → %s: %s", name(from), name(to), render.Plural(len(chs), "change")))
	limit := render.DefaultLimit
	if a.Bool("all") {
		limit = 0
	}
	addChanges(c, root, st, chs, limit, "archo loadout diff "+strings.Join(a.Pos, " ")+" --all")
	return d, nil
}

func loadoutCheckout(c *Ctx, a *Args) (*render.Doc, error) {
	r, n, err := c.App.Checkout(a.Pos[0])
	if err != nil {
		return nil, err
	}
	d := editDoc(c, r)
	d.Nodes[0].Children = append([]*render.Node{{Line: fmt.Sprintf("checked out v%d into the workspace (archo save writes it to the game)", n)}}, d.Nodes[0].Children...)
	return d, nil
}

func loadoutTag(c *Ctx, a *Args) (*render.Doc, error) {
	e, err := c.App.Env()
	if err != nil {
		return nil, err
	}
	n, err := c.App.ParseVersionRef(e.ID, a.Pos[0])
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errors.New("only saved versions can be tagged")
	}
	name := a.Pos[1]
	if _, err := strconv.Atoi(strings.TrimPrefix(name, "v")); err == nil || strings.ContainsAny(name, " :#") || name == "workspace" {
		return nil, fmt.Errorf("tag %q would be confused with a version reference", name)
	}
	if err := c.App.Store.TagVersion(e.ID, name, n); err != nil {
		return nil, err
	}
	d, _ := render.New(fmt.Sprintf("v%d tagged %s", n, name))
	d.AddNext("archo loadout checkout " + name)
	return d, nil
}

func ruleLine(c *Ctx, ix *app.Index, r *store.UserRule) []string {
	lin := func(id string) string {
		l := ix.LRef(id)
		if ms := ix.ByLineage[id]; len(ms) > 0 {
			if p := ix.Lineages[id]; p != nil && ix.ByID[p.PrimaryID] != nil {
				return l + " " + ix.ByID[p.PrimaryID].DisplayName()
			}
			return l + " " + ms[0].DisplayName()
		}
		return l + " (gone)"
	}
	if r.Kind == order.RuleIgnore {
		return []string{fmt.Sprintf("r:%d", r.ID), r.Kind, ix.DisplayConstraintRef(r.Target, primaryRef(ix)), "", r.Note}
	}
	return []string{fmt.Sprintf("r:%d", r.ID), r.Kind, lin(r.A), lin(r.B), r.Note}
}

func primaryRef(ix *app.Index) func(string) string {
	return func(lin string) string {
		if l := ix.Lineages[lin]; l != nil && l.PrimaryID != "" {
			return ix.Ref(l.PrimaryID)
		}
		return ix.LRef(lin)
	}
}

func ruleAdd(c *Ctx, a *Args) (*render.Doc, error) {
	spec := app.RuleSpec{Kind: strings.ToLower(a.Pos[0]), Note: a.Str("note")}
	switch {
	case spec.Kind == order.RuleIgnore && len(a.Pos) == 2:
		spec.Target = a.Pos[1]
	case spec.Kind != order.RuleIgnore && len(a.Pos) == 3:
		spec.A, spec.B = a.Pos[1], a.Pos[2]
	default:
		return nil, &UsageError{Msg: "after|before|incompatible|attach take two references; ignore takes one constraint reference"}
	}
	r, err := c.App.AddRule(spec)
	if err != nil {
		return nil, err
	}
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("r:%d added", r.ID))
	root.AddColumns([][]string{ruleLine(c, ix, r)})
	switch r.Kind {
	case order.RuleIncompatible:
		root.KV("note", "incompatible rules are reported by the problem analysis (M3); sorting ignores them")
	default:
		d.AddNext("archo sort --dry-run")
	}
	return d, nil
}

func ruleList(c *Ctx, a *Args) (*render.Doc, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	rs, err := c.App.Store.Rules(ix.Env.ID)
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("rules: %d (%s)", len(rs), envRef(ix.Env)))
	var rows [][]string
	for _, r := range rs {
		rows = append(rows, ruleLine(c, ix, r))
	}
	root.AddColumns(rows)
	return d, nil
}

func ruleRemove(c *Ctx, a *Args) (*render.Doc, error) {
	e, err := c.App.Env()
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(strings.ToLower(a.Pos[0]), "r:"), 10, 64)
	if err != nil {
		return nil, &UsageError{Msg: "rule references look like r:<n> (archo rule list)"}
	}
	ok, err := c.App.Store.RemoveRule(e.ID, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no rule r:%d (archo rule list)", id)
	}
	d, _ := render.New(fmt.Sprintf("r:%d removed", id))
	return d, nil
}

func launchCmd(c *Ctx, a *Args) (*render.Doc, error) {
	dry := a.Bool("dry-run")
	p, _, err := c.App.Launch(dry, a.Bool("force"))
	if err != nil {
		return nil, err
	}
	head := "launched RimWorld via " + p.Method
	if dry {
		head = "launch: dry run (" + p.Method + ")"
	}
	d, root := render.New(head)
	root.KV("command", p.Command())
	return d, nil
}
