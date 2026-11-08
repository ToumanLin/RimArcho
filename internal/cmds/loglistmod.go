package cmds

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/dotnet"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// listScanLines renders the list-modifier rule of a cluster (06 §2): the list the throwing loop
// enumerates and the enabled mods' code that changes a comp list.
func listScanLines(root *render.Node, ix *app.Index, lc *app.LogCluster) {
	ls := lc.ListScan
	if ls == nil {
		return
	}
	if ls.Enumerated != "" {
		v := strings.TrimPrefix(ls.Enumerated, "Verse.") + " (foreach in " + modLabel(ix, ls.EnumeratedBy)
		if dotnet.IsCompList(ls.Enumerated) {
			v += "; the game's ThingWithComps.Tick indexes it"
		}
		root.KV("enumerated", v+"): the frame above List.Enumerator.MoveNext is the loop that threw; code in its body changed the list and is not on the stack")
	}
	if ls.Enumerated != "" && !dotnet.IsCompList(ls.Enumerated) {
		return
	}
	head := fmt.Sprintf("comp list changed by (candidates; %s, %d assemblies scanned)", app.EnabledMods(ls.Mods), ls.Assemblies)
	sec := root.Section(head)
	if len(ls.Modifiers) == 0 {
		sec.Add("(no enabled mod's code changes AllComps or comps; reflection and a mod's own reference to the list are not seen)")
		return
	}
	for _, lm := range ls.Modifiers {
		line := fmt.Sprintf("%s  %s.%s  %s", modLabel(ix, lm.Instance), strings.ReplaceAll(lm.Type, "+", "."), lm.Method, strings.Join(lm.Ops, ", "))
		if lm.Tick != "" {
			line += "  ← overrides ThingComp." + lm.Tick + ": runs inside ThingWithComps.Tick's loop"
		} else {
			line += "  (not in a tick method)"
		}
		n := sec.Add(line)
		for _, at := range lm.Attached {
			n.Add("attached to: " + at)
		}
	}
	sec.Truncate(8, "")
}
