package cmds

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/store"
)

func init() {
	register(&Command{Name: "harmony", Summary: "Harmony patches: of a mod, on a target method, or shared between mods", MaxArgs: 0, Run: harmonyCmd,
		Flags: []Flag{{Name: "mod", Value: "<ref>"}, {Name: "target", Value: "<Type.Method | part>"}, {Name: "shared"}, {Name: "all"}, gameVersionFlag}})
}

func harmonyLine(ix *app.Index, h *store.HarmonyRow, withMod, withTarget bool) string {
	l := fmt.Sprintf("%-10s %s.%s", h.Kind, h.PatchType, h.PatchMethod)
	if withTarget {
		l = fmt.Sprintf("%-10s %s  ← %s.%s", h.Kind, h.Target, h.PatchType, h.PatchMethod)
	}
	if withMod {
		l = modLabel(ix, h.Instance) + "  " + l
	}
	if h.Skips {
		l += "  (bool prefix: may skip the original)"
	}
	switch {
	case h.Manual && h.Dynamic:
		// the target says why it was not resolved: <manual: …>
	case h.Manual:
		l += "  (Harmony.Patch call in code)"
	case h.Dynamic:
		l += "  (target chosen at runtime by TargetMethod())"
	case h.FromTargetMethod:
		l += "  (target read from TargetMethod())"
	}
	return l
}

func harmonyCmd(c *Ctx, a *Args) (*render.Doc, error) {
	st, s, err := analysed(c)
	if err != nil {
		return nil, err
	}
	ix := st.Ix
	limit := boolLimit(a, render.DefaultLimit)
	var inst string
	if r := a.Str("mod"); r != "" {
		m, err := ix.ResolveOne(r)
		if err != nil {
			return nil, err
		}
		inst = m.ID
		if a.Str("target") == "" && st.Position(m.ID) == 0 {
			return harmonyOutside(c, a, st, s, inst, limit)
		}
	}
	switch {
	case a.Str("target") != "":
		return harmonyTarget(c, st, s, a.Str("target"), inst, limit)
	case inst != "":
		return harmonyMod(c, a, st, s, inst, limit)
	default:
		d, root := render.New(fmt.Sprintf("harmony: %d patches in %d assemblies", s.Run.Stats.HarmonyPatches, s.Run.Stats.Assemblies))
		harmonyStale(s, root)
		addShared(c, st, root, "", limit)
		d.AddNext("archo harmony --target <Type.Method>", "archo problems --code harmony")
		return d, nil
	}
}

// harmonyStale notes a stale analysis.
func harmonyStale(s *app.AnalysisState, root *render.Node) {
	if !s.Fresh {
		root.KV("analysis", "stale (archo analyze)")
	}
}

// harmonyTarget is harmony --target: the patches on one target method (or on targets
// matching t), by kind, in load order.
func harmonyTarget(c *Ctx, st *app.LoadoutState, s *app.AnalysisState, t, inst string, limit int) (*render.Doc, error) {
	ix := st.Ix
	rows, err := c.App.Store.HarmonyPatches(store.HarmonyFilter{Target: t, Instance: inst})
	if err != nil {
		return nil, err
	}
	rows = harmonyInScope(st, rows, inst)
	like := false
	if len(rows) == 0 {
		rows, err = c.App.Store.HarmonyPatches(store.HarmonyFilter{Target: t, Instance: inst, Like: true})
		if err != nil {
			return nil, err
		}
		rows = harmonyInScope(st, rows, inst)
		like = true
	}
	mods := map[string]bool{}
	for _, h := range rows {
		mods[h.Instance] = true
	}
	head := fmt.Sprintf("harmony:%s  %d patches from %d mods", t, len(rows), len(mods))
	if like {
		head = fmt.Sprintf("harmony targets matching %q: %d patches from %d mods", t, len(rows), len(mods))
	}
	d, root := render.New(head)
	harmonyStale(s, root)
	harmonyByKind(st, root, rows, like, limit)
	if len(rows) == 0 && !dynamicOnTarget(c, st, root, t, inst) {
		root.Add("no declared patches (mods that patch from code are listed by archo problems --code harmony.manual)")
	}
	seenT := map[string]bool{}
	for _, h := range rows {
		if len(d.Next) >= 3 || seenT[h.PatchType] {
			continue
		}
		seenT[h.PatchType] = true
		typ := h.PatchType
		if i := strings.IndexByte(typ, '+'); i >= 0 {
			typ = typ[:i] // ilspycmd decompiles the outer type with its nested ones
		}
		d.AddNext(fmt.Sprintf("archo source %s --type %s", ix.Ref(h.Instance), typ))
	}
	return d, nil
}

// harmonyByKind adds one section per patch kind, its rows in load order.
func harmonyByKind(st *app.LoadoutState, root *render.Node, rows []*store.HarmonyRow, like bool, limit int) {
	byKind := map[string][]*store.HarmonyRow{}
	for _, h := range rows {
		byKind[h.Kind] = append(byKind[h.Kind], h)
	}
	for _, k := range []string{"prefix", "postfix", "transpiler", "finalizer"} {
		if len(byKind[k]) == 0 {
			continue
		}
		sec := root.Section(map[string]string{"prefix": "prefixes", "postfix": "postfixes", "transpiler": "transpilers", "finalizer": "finalizers"}[k])
		sort.SliceStable(byKind[k], func(i, j int) bool {
			return st.Position(byKind[k][i].Instance) < st.Position(byKind[k][j].Instance)
		})
		for _, h := range byKind[k] {
			line := modLabel(st.Ix, h.Instance) + "  " + h.PatchType + "." + h.PatchMethod
			if like {
				line += "  on " + h.Target
			}
			if h.Skips {
				line += "  (returns bool: may skip the original)"
			}
			sec.Add(line)
		}
		sec.Truncate(limit, "add --all")
	}
}

// harmonyInScope keeps the patches of the current list's Mods and of inst. The stored
// facts cover every copy analysed for this game version, in any list or Game Config
// (content_state); the others printed as #? or as a long ID no reference resolves.
func harmonyInScope(st *app.LoadoutState, rows []*store.HarmonyRow, inst string) []*store.HarmonyRow {
	out := rows[:0:0]
	for _, h := range rows {
		if h.Instance == inst || st.Position(h.Instance) > 0 {
			out = append(out, h)
		}
	}
	return out
}

// harmonyScanBudget bounds the on-demand read of a Mod outside the list (14 §1).
var harmonyScanBudget = 10 * time.Second

// harmonyOutside is harmony --mod for a Mod outside the current list: the stored analysis
// of that copy for this game version when one exists (it was in another list), otherwise its
// assemblies read now, not stored (05 §5.3).
func harmonyOutside(c *Ctx, a *Args, st *app.LoadoutState, s *app.AnalysisState, inst string, limit int) (*render.Doc, error) {
	asms, err := c.App.Store.Assemblies(inst)
	if err != nil {
		return nil, err
	}
	if len(asms) > 0 {
		rows, err := c.App.Store.HarmonyPatches(store.HarmonyFilter{Instance: inst})
		if err != nil {
			return nil, err
		}
		return harmonyModRows(c, a, st, s, inst, asms, rows, "not in the current list: from an earlier analysis of this copy for "+st.GameMM(), limit), nil
	}
	m := st.Ix.ByID[inst]
	asms, rows, pending, err := c.App.ScanModHarmony(st, m, harmonyScanBudget)
	if err != nil {
		return nil, err
	}
	scope := "not in the current list: its assemblies read now for " + st.GameMM() + ", not stored"
	if pending > 0 {
		scope += fmt.Sprintf("; %d assemblies not read within %s: call again", pending, harmonyScanBudget)
	}
	return harmonyModRows(c, a, st, nil, inst, asms, rows, scope, limit), nil
}

// harmonyMod is harmony --mod: one mod's assemblies and patches.
func harmonyMod(c *Ctx, a *Args, st *app.LoadoutState, s *app.AnalysisState, inst string, limit int) (*render.Doc, error) {
	asms, err := c.App.Store.Assemblies(inst)
	if err != nil {
		return nil, err
	}
	rows, err := c.App.Store.HarmonyPatches(store.HarmonyFilter{Instance: inst})
	if err != nil {
		return nil, err
	}
	return harmonyModRows(c, a, st, s, inst, asms, rows, "", limit), nil
}

// harmonyModRows prints one Mod's assemblies and patches; scope, when set, says where they
// come from (a Mod outside the list), and s is nil for facts read now.
func harmonyModRows(c *Ctx, a *Args, st *app.LoadoutState, s *app.AnalysisState, inst string, asms []*store.AssemblyRow, rows []*store.HarmonyRow, scope string, limit int) *render.Doc {
	ix := st.Ix
	m := ix.ByID[inst]
	d, root := render.New(fmt.Sprintf("%s  harmony: %d patches", modLabel(ix, m.ID), len(rows)))
	if scope != "" {
		root.KV("scope", scope)
	}
	if s != nil {
		harmonyStale(s, root)
	}
	if len(asms) == 0 {
		root.Add("no assemblies in the effective content")
	}
	manual := harmonyAssemblies(root, asms, limit)
	if len(rows) > 0 {
		ps := root.Section("patches")
		for _, h := range rows {
			ps.Add(harmonyLine(ix, h, false, true))
		}
		ps.Truncate(limit, "archo harmony --mod "+ix.Ref(m.ID)+" --all")
	}
	if manual {
		harmonyManual(ix, root, m.ID, rows, limit)
	} else if len(rows) == 0 && len(asms) > 0 {
		root.Add("no Harmony patches declared")
	}
	if a.Bool("shared") {
		addShared(c, st, root, inst, limit)
	}
	return d
}

// harmonyAssemblies adds a mod's assemblies and reports whether one calls Harmony.Patch
// from code.
func harmonyAssemblies(root *render.Node, asms []*store.AssemblyRow, limit int) bool {
	sec := root.Section("assemblies")
	manual := false
	for _, as := range asms {
		l := fmt.Sprintf("%s  types %d, patches %d", as.File, as.Types, as.Patches)
		switch {
		case as.Err != "":
			l += "  (unreadable: " + as.Err + ")"
		case as.Skipped:
			l += "  (known library: patches not collected)"
		case as.Manual:
			l += "  (calls Harmony.Patch from code)"
			manual = true
		}
		sec.Add(l)
	}
	sec.Truncate(limit, "")
	return manual
}

// harmonyManual adds G9 (05 §5.3): the targets of Harmony.Patch calls read from IL, and
// why the others could not be resolved.
func harmonyManual(ix *app.Index, root *render.Node, id string, rows []*store.HarmonyRow, limit int) {
	found := 0
	var open []*store.HarmonyRow
	for _, h := range rows {
		switch {
		case h.Manual && h.Dynamic:
			open = append(open, h)
		case h.Manual:
			found++
		}
	}
	mn := root.Add(fmt.Sprintf("manual: %d patch targets found in Harmony.Patch calls, %d not resolved (archo source search \".Patch(\" --in %s)", found, len(open), ix.Ref(id)))
	for _, h := range open {
		why := strings.TrimSuffix(strings.TrimPrefix(h.Target, "<manual: "), ">")
		mn.Addf("%-10s %s.%s  not resolved: %s", h.Kind, h.PatchType, h.PatchMethod, why)
	}
	mn.Truncate(limit, "archo harmony --mod "+ix.Ref(id)+" --all")
}

// addShared lists targets patched by several mods of the list (only those involving inst,
// if set; inst may be outside the list), each Mod by reference and name.
func addShared(c *Ctx, st *app.LoadoutState, root *render.Node, inst string, limit int) {
	ix := st.Ix
	rows, err := c.App.Store.HarmonyPatches(store.HarmonyFilter{})
	if err != nil {
		return
	}
	rows = harmonyInScope(st, rows, inst)
	by := map[string]map[string]bool{}
	var order []string
	for _, h := range rows {
		if by[h.Target] == nil {
			by[h.Target] = map[string]bool{}
			order = append(order, h.Target)
		}
		by[h.Target][h.Instance] = true
	}
	sort.SliceStable(order, func(i, j int) bool {
		if len(by[order[i]]) != len(by[order[j]]) {
			return len(by[order[i]]) > len(by[order[j]])
		}
		return order[i] < order[j]
	})
	sec := root.Section("shared-targets")
	for _, t := range order {
		ms := by[t]
		if len(ms) < 2 || (inst != "" && !ms[inst]) {
			continue
		}
		var ids []string
		for id := range ms {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return st.Position(ids[i]) < st.Position(ids[j]) })
		var names []string
		for _, id := range ids {
			names = append(names, modLabel(ix, id))
		}
		sec.Addf("%s  %d mods: %s", t, len(ms), strings.Join(names, ", "))
	}
	if len(sec.Children) == 0 {
		sec.Add("(none)")
	}
	sec.Truncate(limit, "add --all")
}
