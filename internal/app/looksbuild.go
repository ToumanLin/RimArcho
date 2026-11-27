package app

import (
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/looks"
	"github.com/ToumanLin/RimArcho/internal/oplog"
)

// The generated Mod RimArcho Looks Preference (21 §4): About/About.xml and, for every slot whose choice
// is applied, the chosen file under its key's path. How it is built, swapped in and left out
// of a write to the game is shared with RimArcho Styles Preference (generated.go).

// looksPlan lists the files the generated Mod holds for ls: for every slot whose choice is
// applied, the chosen file, its second format and its mask (21 §4 "Contents").
func looksPlan(ls *LooksState) []LooksFile {
	ids := make([]string, 0, len(ls.Looks))
	for id := range ls.Looks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []LooksFile
	seen := map[string]bool{}
	add := func(f *looks.File, look, slot string) {
		for ; f != nil; f = f.Alt {
			rel := "Textures/" + f.Tex
			if f.Tex == "" || seen[strings.ToLower(rel)] {
				continue
			}
			seen[strings.ToLower(rel)] = true
			out = append(out, LooksFile{Rel: rel, Source: f.Path, Size: f.Size, MTime: f.MTime, Look: look, Slot: slot})
		}
	}
	for _, id := range ids {
		s := ls.Looks[id]
		if s.Look == nil {
			continue
		}
		wins := s.Look.InGame(s.Choice)
		for _, slot := range textureSlots(s.Look) {
			if s.Slots[slot] != LookApplied {
				continue
			}
			w := wins[slot]
			add(w.File, id, slot)
			if m := wins[looks.MaskOf(slot)]; m.File != nil && m.Pos == w.Pos {
				add(m.File, id, looks.MaskOf(slot))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out
}

// looksRebuild rebuilds RimArcho Looks Preference in the looks directory dir from ls (21 §4
// "Rebuild").
func (a *App) looksRebuild(ls *LooksState, dir string, dryRun bool, h *oplog.Handle) (*LooksBuild, error) {
	return a.genRebuild(looksGen, ls.St, dir, looksPlan(ls), nil, dryRun, h)
}
