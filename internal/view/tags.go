package view

import (
	"sort"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
)

// Tag candidates (16 §3.8): the tags the selected mods have come first (all of them, then
// some), then the rest; within each, the pinned tags (config tags.pinned), then the tags in
// use, most used first. Each says how many of the selected mods have it.

// TagCand is one candidate tag.
type TagCand struct {
	Tag  string `json:"tag"`
	N    int    `json:"n"`    // mods of the environment that have it
	Have int    `json:"have"` // of the selected mods
}

// TagCandidates lists the candidates for a selection, given each selected mod's tags.
func TagCandidates(a *app.App, envID string, selected [][]string) []TagCand {
	have := map[string]int{}
	for _, tags := range selected {
		for _, t := range tags {
			have[strings.ToLower(t)]++
		}
	}
	var out []TagCand
	seen := map[string]bool{}
	add := func(tag string, n int) {
		k := strings.ToLower(tag)
		if tag == "" || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, TagCand{Tag: tag, N: n, Have: min(have[k], len(selected))})
	}
	counts := map[string]int{}
	var used []TagCand
	if ts, err := a.Store.TagCounts(envID); err == nil {
		for _, t := range ts {
			n, _ := strconv.Atoi(t[1])
			counts[strings.ToLower(t[0])] = n
			used = append(used, TagCand{Tag: t[0], N: n})
		}
	}
	for _, t := range PinnedTags(a) {
		add(t, counts[strings.ToLower(t)])
	}
	sort.SliceStable(used, func(i, j int) bool { return used[i].N > used[j].N })
	for _, t := range used {
		add(t.Tag, t.N)
	}
	rank := func(c TagCand) int {
		switch {
		case c.Have > 0 && c.Have == len(selected):
			return 0
		case c.Have > 0:
			return 1
		}
		return 2
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	return out
}

// PinnedTags: config tags.pinned, split on ASCII and fullwidth commas.
func PinnedTags(a *app.App) []string { return cmds.PinnedTagList(a) }
