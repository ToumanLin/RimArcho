package analyze

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/finding"
)

// asmDuplicates: two or more enabled mods ship an assembly with the same file name in their
// effective folders (05 §4.3, asm.duplicate-name). RimWorld loads each mod's DLLs in load
// order with Assembly.LoadFrom, and a later DLL whose name is already loaded is not loaded
// again: the first copy in load order is the one every mod gets (MCP eval h2, real 1.6: a
// stale OutfitStandsPlus.dll in a patch mod listed first hid four types of Outfit Stands
// Plus; listing Outfit Stands Plus first, or removing the stale copy, fixed it).
//
// Identical type sets are a shared library (LunarFramework.dll in every Lunar mod): no
// finding. Known libraries (harmony.DefaultSkip: 0Harmony.dll, HugsLib.dll …) are left out:
// many mods ship their own copy of another version, and those libraries manage it. A later copy that defines types the loaded copy lacks is a warning (those types
// are missing in game); a later copy with fewer or other types only is info.
func asmDuplicates(active []*Mod, asms []*AssemblyFacts) []*Finding {
	pos := map[string]int{}
	for _, m := range active {
		pos[m.Instance] = m.Pos
	}
	byName := map[string][]*AssemblyFacts{}
	var names []string
	for _, a := range asms {
		if a.Err != "" || a.Assembly == nil || a.Skipped || pos[a.Instance] == 0 {
			continue
		}
		n := strings.ToLower(path.Base(a.File))
		if byName[n] == nil {
			names = append(names, n)
		}
		byName[n] = append(byName[n], a)
	}
	sort.Strings(names)
	var out []*Finding
	for _, n := range names {
		copies := byName[n]
		// One copy per mod (the first of its folders), in load order.
		seen := map[string]bool{}
		var cs []*AssemblyFacts
		for _, a := range copies {
			if !seen[a.Instance] {
				seen[a.Instance] = true
				cs = append(cs, a)
			}
		}
		if len(cs) < 2 {
			continue
		}
		sort.SliceStable(cs, func(i, j int) bool { return pos[cs[i].Instance] < pos[cs[j].Instance] })
		first := cs[0]
		loaded := map[string]bool{}
		for _, t := range first.Types {
			loaded[t] = true
		}
		var lost []string // types of later copies the loaded copy lacks
		var lostBy []string
		differs := false
		for _, a := range cs[1:] {
			var mine []string
			for _, t := range a.Types {
				if !loaded[t] && !strings.Contains(t, "<") && !compilerType(t) {
					mine = append(mine, t)
				}
			}
			if len(mine) > 0 {
				lostBy = append(lostBy, finding.Inst(a.Instance))
				lost = append(lost, mine...)
			}
			if len(mine) > 0 || len(a.Types) != len(first.Types) {
				differs = true
			}
		}
		if !differs {
			continue
		}
		sort.Strings(lost)
		lost = uniq(lost)
		subjects := []string{finding.Inst(first.Instance)}
		var evs []finding.Evidence
		for _, a := range cs {
			if a != first {
				subjects = append(subjects, finding.Inst(a.Instance))
			}
			role := "not loaded: the name is taken"
			if a == first {
				role = "loaded (first in load order)"
			}
			evs = append(evs, finding.Evidence{Source: finding.Declared, Ref: finding.Inst(a.Instance),
				Text: fmt.Sprintf("%s: %s, list position %d, %s", a.File, nTypes(len(a.Types)), pos[a.Instance], role)})
		}
		file := path.Base(first.File)
		f := &Finding{Code: "asm.duplicate-name", Layer: "harmony", Key: n, Subjects: subjects, Evidence: evs, Confidence: finding.Likely,
			Suggestions: []string{"archo mod " + finding.Inst(first.Instance), "archo mod " + subjects[1]}}
		if len(lost) > 0 {
			f.Severity = finding.Warning
			f.Claim = fmt.Sprintf("%s loads its %s first; the copy in %s defines %s it lacks (%s), which are missing in game",
				finding.Inst(first.Instance), file, strings.Join(lostBy, ", "), nTypes(len(lost)), sample(lost, 4))
			f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Declared, Text: "missing in game: " + sample(lost, 20)})
		} else {
			f.Severity = finding.Info
			f.Claim = fmt.Sprintf("%d mods ship different copies of %s; %s's loads (first in load order) and has every type of the others",
				len(cs), file, finding.Inst(first.Instance))
		}
		out = append(out, f)
	}
	return out
}

// compilerType: a type the compiler or a polyfill package adds to an assembly
// (Microsoft.CodeAnalysis.EmbeddedAttribute, System.Runtime.CompilerServices.RefSafetyRulesAttribute).
// No XML or patch names these, so a copy without them loses nothing (VM round 2026-11-05: the h2
// warning stayed after Outfit Stands Plus was moved first, on these two types alone).
func compilerType(t string) bool {
	return strings.HasPrefix(t, "System.") || strings.HasPrefix(t, "Microsoft.")
}

func nTypes(n int) string {
	if n == 1 {
		return "1 type"
	}
	return fmt.Sprintf("%d types", n)
}

// sample is up to n items joined, with "+k more".
func sample(ss []string, n int) string {
	if len(ss) <= n {
		return strings.Join(ss, ", ")
	}
	return strings.Join(ss[:n], ", ") + fmt.Sprintf(" +%d more", len(ss)-n)
}
