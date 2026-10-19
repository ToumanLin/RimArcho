package cmds

import (
	"fmt"
	"sort"
	"strings"

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
	if h.Dynamic {
		l += "  (target chosen at runtime by TargetMethod())"
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
	}
	stale := func(root *render.Node) {
		if !s.Fresh {
			root.KV("analysis", "stale (archo analyze)")
		}
	}
	switch {
	case a.Str("target") != "":
		t := a.Str("target")
		rows, err := c.App.Store.HarmonyPatches(store.HarmonyFilter{Target: t, Instance: inst})
		if err != nil {
			return nil, err
		}
		like := false
		if len(rows) == 0 {
			rows, err = c.App.Store.HarmonyPatches(store.HarmonyFilter{Target: t, Instance: inst, Like: true})
			if err != nil {
				return nil, err
			}
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
		stale(root)
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
				line := modLabel(ix, h.Instance) + "  " + h.PatchType + "." + h.PatchMethod
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
		if len(rows) == 0 {
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
	case inst != "":
		m := ix.ByID[inst]
		asms, err := c.App.Store.Assemblies(inst)
		if err != nil {
			return nil, err
		}
		rows, err := c.App.Store.HarmonyPatches(store.HarmonyFilter{Instance: inst})
		if err != nil {
			return nil, err
		}
		d, root := render.New(fmt.Sprintf("%s  harmony: %d patches", modLabel(ix, m.ID), len(rows)))
		stale(root)
		if len(asms) == 0 {
			root.Add("no assemblies in the effective content")
		}
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
		if len(rows) > 0 {
			ps := root.Section("patches")
			for _, h := range rows {
				ps.Add(harmonyLine(ix, h, false, true))
			}
			ps.Truncate(limit, "archo harmony --mod "+ix.Ref(m.ID)+" --all")
		}
		if manual {
			root.KV("manual", "targets patched from code are not statically known (archo source search \".Patch(\" --in "+ix.Ref(m.ID)+")")
		} else if len(rows) == 0 && len(asms) > 0 {
			root.Add("no Harmony patches declared")
		}
		if a.Bool("shared") {
			addShared(c, ix, root, inst, limit)
		}
		return d, nil
	default:
		d, root := render.New(fmt.Sprintf("harmony: %d patches in %d assemblies", s.Run.Stats.HarmonyPatches, s.Run.Stats.Assemblies))
		stale(root)
		addShared(c, ix, root, "", limit)
		d.AddNext("archo harmony --target <Type.Method>", "archo problems --code harmony")
		return d, nil
	}
}

// addShared lists targets patched by several mods (only those involving inst, if set).
func addShared(c *Ctx, ix *app.Index, root *render.Node, inst string, limit int) {
	rows, err := c.App.Store.HarmonyPatches(store.HarmonyFilter{})
	if err != nil {
		return
	}
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
		var refs []string
		for id := range ms {
			refs = append(refs, ix.Ref(id))
		}
		sort.Strings(refs)
		sec.Addf("%s  %d mods: %s", t, len(ms), strings.Join(refs, " "))
	}
	if len(sec.Children) == 0 {
		sec.Add("(none)")
	}
	sec.Truncate(limit, "add --all")
}

// shortMethod: "Verse.Pawn.Tick" → "Tick" (decompiled code rarely spells the full name).
func shortMethod(t string) string {
	if i := strings.IndexByte(t, '('); i >= 0 {
		t = t[:i]
	}
	if i := strings.LastIndexByte(t, '.'); i >= 0 {
		return t[i+1:]
	}
	return t
}
