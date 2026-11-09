package view

import (
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// A required mod that is missing is often outdated, with a continuation under another
// packageId already enabled. Can it run offers the enabled mods that look like one, so a
// replaces rule (08 §2.1) can say so in one step.

var (
	nameTags  = regexp.MustCompile(`[\[(\x{FF08}\x{3010}{][^\])\x{FF09}\x{3011}}]*[\])\x{FF09}\x{3011}}]`) // ASCII and full-width brackets
	nameVers  = regexp.MustCompile(`\bv?\d+(\.\d+)+\b`)
	nameWords = map[string]bool{"continued": true, "unofficial": true, "updated": true, "update": true, "fixed": true, "fork": true,
		"redux": true, "reborn": true, "revived": true, "legacy": true, "version": true, "ver": true}
	pkgGeneric = map[string]bool{"patches": true, "framework": true, "continued": true, "unofficial": true, "updated": true, "rimworld": true}
)

// nameCore is a mod name without tags in brackets, version numbers, words such as
// Continued, spaces and punctuation, in lower case.
func nameCore(s string) string {
	s = nameVers.ReplaceAllString(nameTags.ReplaceAllString(strings.ToLower(s), " "), " ")
	var b strings.Builder
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if !nameWords[w] {
			b.WriteString(w)
		}
	}
	return b.String()
}

// pkgParts are the distinctive parts of a packageId after its author.
func pkgParts(pkg string) []string {
	var out []string
	parts := strings.Split(strings.ToLower(pkg), ".")
	for _, p := range parts[min(1, len(parts)-1):] {
		if len(p) >= 6 && !pkgGeneric[p] {
			out = append(out, p)
		}
	}
	return out
}

// standInCandidates: enabled mods that may replace the package pkg (named name), best first,
// at most 3. Same name once tags and versions are dropped, then one name inside the other,
// then a shared packageId part. Never official content, a copy of pkg, or a mod that itself
// requires pkg (another translation of it).
func standInCandidates(st *app.LoadoutState, pkg, name string) []*store.Mod {
	want := nameCore(name)
	parts := pkgParts(pkg)
	type hit struct {
		m     *store.Mod
		score int
	}
	var hits []hit
	seen := map[string]bool{}
	for _, e := range st.List {
		m := st.Ix.ByID[e.Instance]
		if m == nil || seen[m.LineageID] {
			continue
		}
		seen[m.LineageID] = true
		mp := strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")
		if mp == pkg || strings.HasPrefix(mp, "ludeon.rimworld") {
			continue
		}
		score := 0
		got := nameCore(m.DisplayName())
		short, long := []rune(want), []rune(got)
		if len(short) > len(long) {
			short, long = long, short
		}
		switch {
		case want != "" && got == want:
			score = 3
		case len(short) >= 5 && strings.Contains(string(long), string(short)):
			score = 2
		case slices.ContainsFunc(pkgParts(mp), func(p string) bool { return slices.Contains(parts, p) }):
			score = 1
		}
		if score == 0 || requires(st, m, pkg) {
			continue
		}
		hits = append(hits, hit{m, score})
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return b.score - a.score })
	var out []*store.Mod
	for _, h := range hits[:min(3, len(hits))] {
		out = append(out, h.m)
	}
	return out
}

// requires: m declares a dependency on pkg (generally or for this game version).
func requires(st *app.LoadoutState, m *store.Mod, pkg string) bool {
	d := m.Decl()
	for _, x := range append(slices.Clone(d.Dependencies), d.DependenciesByVersion["v"+st.GameMM()]...) {
		if strings.TrimSuffix(strings.ToLower(strings.TrimSpace(x.PackageID)), "_steam") == pkg {
			return true
		}
	}
	return false
}

// standInAction offers the candidates as a replaces rule each; ok is false when none looks
// like a replacement.
func standInAction(st *app.LoadoutState, pkg, name string) (Action, bool) {
	cs := standInCandidates(st, pkg, name)
	if len(cs) == 0 {
		return Action{}, false
	}
	act := Action{Kind: ActStandIn, Key: "R", Label: T("an enabled mod replaces it"), Title: Tf("which enabled mod replaces %s?", name)}
	for _, c := range cs {
		act.Options = append(act.Options, Option{Label: Tf("%s replaces it", c.DisplayName()), Refs: []string{st.Ix.LRef(c.LineageID), "pkg:" + pkg},
			Done: Tf("%s now stands in for %s (a rule: remove it under load-order rules)", c.DisplayName(), name)})
	}
	return act, true
}

// depActions: a missing dependency's own action first, then an enabled mod in its place
// when one looks like a replacement, then ignore.
func depActions(st *app.LoadoutState, p *app.Problem, first Action, pkg, name string) []Action {
	acts := []Action{first}
	if s, ok := standInAction(st, pkg, name); ok {
		acts = append(acts, s)
	}
	return append(acts, ignoreDep(p))
}

// ignoreDep hides a missing dependency until the mod that needs it changes (05 §4.2).
func ignoreDep(p *app.Problem) Action {
	return Action{Kind: ActIgnore, Key: "I", Label: T("ignore it"), Refs: []string{p.Ref}, Problem: p.Ref,
		Done: T("ignored; shown again when the mod that needs it changes")}
}
