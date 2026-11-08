package cmds

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// dynamicOnTarget answers `harmony --target X` when no row names X: patch classes whose target
// is chosen at run time (TargetMethod() the analysis could not read) may still patch it. Those
// the player's newest log shows under a frame X are listed; otherwise their count is given,
// so the answer never claims there are no declared patches (MCP eval h4).
func dynamicOnTarget(c *Ctx, st *app.LoadoutState, root *render.Node, t, inst string) bool {
	rows, err := c.App.Store.HarmonyPatches(store.HarmonyFilter{Target: "<dynamic ", Like: true, Instance: inst})
	if err != nil {
		return false
	}
	var dyn []*store.HarmonyRow
	for _, h := range rows {
		if h.Dynamic && !h.Manual && st.Position(h.Instance) > 0 {
			dyn = append(dyn, h)
		}
	}
	if len(dyn) == 0 {
		return false
	}
	seen := logPatchTypesOn(c, st, t)
	var hit []*store.HarmonyRow
	mods := map[string]bool{}
	for _, h := range dyn {
		mods[h.Instance] = true
		if seen[h.PatchType] {
			hit = append(hit, h)
		}
	}
	ix := st.Ix
	if len(hit) > 0 {
		sort.SliceStable(hit, func(i, j int) bool { return st.Position(hit[i].Instance) < st.Position(hit[j].Instance) })
		sec := root.Section("dynamic patches the newest log shows on this method (target chosen by TargetMethod())")
		for _, h := range hit {
			sec.Add(fmt.Sprintf("%s  %-10s %s.%s", modLabel(ix, h.Instance), h.Kind, h.PatchType, h.PatchMethod))
		}
		return true
	}
	classes := fmt.Sprintf("%d patch classes in %s choose their", len(dyn), app.EnabledMods(len(mods)))
	if len(dyn) == 1 {
		classes = "1 patch class in 1 enabled mod chooses its"
	}
	root.Add("no patch names this method statically; " + classes + " target at run time (TargetMethod()) and may include it: archo harmony --mod <ref> lists them")
	return true
}

// logPatchTypesOn: the patch classes the problem log's stored report lists under a frame whose
// method is t ("Verse.Pawn.Tick"; a nested type's '+' read as '.'). The log is not parsed here.
func logPatchTypesOn(c *Ctx, st *app.LoadoutState, t string) map[string]bool {
	out := map[string]bool{}
	p := c.App.ProblemLog(st)
	if p == "" {
		return out
	}
	r, err := c.App.CachedLog(p)
	if err != nil {
		return out
	}
	for _, lc := range r.Clusters {
		for _, fr := range lc.First.Frames {
			m := strings.ReplaceAll(fr.Type, "+", ".") + "." + fr.Method
			if m != t && !strings.HasSuffix(m, "."+t) {
				continue
			}
			for _, h := range fr.Harmony {
				out[h.PatchType] = true
			}
		}
	}
	return out
}
