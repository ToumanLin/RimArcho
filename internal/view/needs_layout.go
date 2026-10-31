package view

import (
	"slices"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// How 能不能跑 lays its items out (06 §2.4, §5): what merges into the sort item, what the
// two columns say about each other, and what other pages handle.

// mergeIntoSort: weak order preferences and order cycles are what sorting settles anyway.
// With a counted item that sorts (S) they are lines of that item (Also), the cycle's
// list of requirements one of its actions, and they are shown inside it (Into). Without
// one they are hints of their own. Call after identifyNeed: Into is the host's ID.
func mergeIntoSort(items []Need) {
	host := slices.IndexFunc(items, func(it Need) bool {
		return it.Counted && it.Section == SectionBreak && slices.ContainsFunc(it.Actions, func(a Action) bool { return a.Kind == ActSort })
	})
	for i := range items {
		it := &items[i]
		if i == host || it.Group != GroupList {
			continue
		}
		weak := it.code == "order.violated" && !it.Counted
		cycle := it.code == "order.cycle"
		if !weak && !cycle {
			continue
		}
		if host < 0 {
			it.Section = SectionHint
			continue
		}
		h := &items[host]
		if weak {
			h.Also = append(h.Also, Tf("also meets %d weaker order preferences", len(it.Parts)))
		} else {
			h.Also = append(h.Also, Tf("the cycle of %d mods is settled by priority; %s", len(it.Mods), it.Why))
			for _, a := range it.Actions {
				if a.Kind == ActList {
					a.Key = "v"
					a.Label = T("see the cycle")
					h.Actions = append(h.Actions, a)
				}
			}
		}
		it.Into = h.ID
	}
	if host >= 0 && len(items[host].Also) > 0 {
		identifyNeed(&items[host]) // the actions it took over get their ids
	}
}

// relateNeeds links the last run's error groups with the left column's items about the
// same mod: a dependency it needs (dep.*'s RequiredBy) or a shared-log hint about it.
func relateNeeds(items []Need) {
	for i := range items {
		lg := &items[i]
		if lg.Group != GroupLastRun || len(lg.Mods) == 0 || lg.Dim {
			continue
		}
		subj := lg.Mods[0]
		for j := range items {
			o := &items[j]
			if o.Into != "" {
				continue
			}
			hit := false
			switch o.Section {
			case SectionBreak:
				hit = strings.HasPrefix(o.code, "dep.") && slices.ContainsFunc(o.RequiredBy, func(r ModOwner) bool { return r.ID == subj })
			case SectionHint:
				hit = slices.Contains(o.Mods, subj)
			}
			if !hit {
				continue
			}
			o.Related = append(o.Related, Related{ID: lg.ID, Text: Tf("in the last run: %s", needLine(*lg))})
			lg.Related = append(lg.Related, Related{ID: o.ID, Text: Tf("related: %s", needLine(*o))})
		}
	}
}

func needLine(n Need) string {
	if n.Title != "" {
		return n.Title
	}
	return n.Text
}

// elsewhere counts what another page handles (SectionElsewhere): hidden and locked mods
// in Mod 库存 (searched by their states), out-of-date converted textures in 贴图优化.
func elsewhere(items []Need) []Elsewhere {
	lib, tex := 0, 0
	for _, it := range items {
		if it.Section != SectionElsewhere {
			continue
		}
		if it.code == "texture.stale" {
			tex++
		} else {
			lib++
		}
	}
	var out []Elsewhere
	if lib > 0 {
		out = append(out, Elsewhere{Page: "library", Count: lib, Text: Tf("%d hidden or locked mods are in the Mod library", lib), Query: "state:hidden state:residue state:updated"})
	}
	if tex > 0 {
		out = append(out, Elsewhere{Page: "textures", Count: tex, Text: Tf("%d mods' converted textures are out of date", tex)})
	}
	return out
}

// currentLinksFirst puts the current game's link group first (06 §2.4: the others fold
// into one line); the repair still covers every game.
func currentLinksFirst(items []Need, gameRoot string) {
	key := env.PathKey(realViewPath(gameRoot))
	order := func(gs []LinkGroup) {
		for i := range gs {
			gs[i].Current = env.PathKey(gs[i].Path) == key
		}
		sort.SliceStable(gs, func(i, j int) bool { return gs[i].Current && !gs[j].Current })
	}
	for i := range items {
		order(items[i].LinkGroups)
		for j := range items[i].Actions {
			order(items[i].Actions[j].LinkGroups)
		}
	}
}

// NeedsLead is 能不能跑's lead sentence (06 §2.4): the open list's verdict from the
// counted items before the start (sections 1–3) and of the runs (the last section), and
// whether the files were never checked or only suggestions are left. GUI screens.js
// needsLead writes the same.
func NeedsLead(plan string, before, runs int, never, suggestions bool) string {
	if plan == "" {
		plan = T("the list")
	}
	switch {
	case before > 0:
		return Tf("%s is not safe to play yet: %d to handle before you start, %d error groups in the last run", plan, before, runs)
	case runs > 0:
		return Tf("%s: %d error groups in the last run", plan, runs)
	case never:
		return Tf("%s: the mods' files are not checked yet.", plan)
	case suggestions:
		return Tf("%s can run; suggestions below may help if you see errors.", plan)
	}
	return Tf("%s can run: nothing to handle.", plan)
}

// AutoFixCount is how many items 自动修复 (F) resolves: required mods that are installed but
// not enabled (CompleteDependencies enables them), and the sort it runs after (06 §5).
// 0: there is nothing it can fix, and the action is not offered.
func AutoFixCount(items []Need) int {
	enable, sorts := 0, 0
	for _, it := range items {
		if !it.Counted || it.Into != "" {
			continue
		}
		for _, a := range it.Actions {
			switch {
			case a.Kind == ActEnable && it.Target != "":
				enable++
			case a.Kind == ActSort:
				sorts++
			}
		}
	}
	if enable == 0 {
		return 0
	}
	return enable + sorts
}
