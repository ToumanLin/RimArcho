package analyze

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/rimarcho/rimarcho/internal/content"
	"github.com/rimarcho/rimarcho/internal/finding"
	"github.com/rimarcho/rimarcho/internal/harmony"
)

// AssemblyFacts is the Harmony layer of one effective DLL of an instance.
type AssemblyFacts struct {
	Instance string
	File     string // relative to the mod root
	Skipped  bool   // known library: Harmony facts not collected
	Err      string
	*harmony.Assembly
}

// harmonyLayer reads every effective DLL of the active mods (03 §5.3). It is independent
// of the Def/Patch layers: a failure only affects its own facts.
func harmonyLayer(active []*Mod, folders map[string][]string, skip []string, workers int) []*AssemblyFacts {
	type job struct {
		m *Mod
		a content.Asset
	}
	var jobs []job
	for _, m := range active {
		for _, a := range content.Files(m.Dir, folders[m.Instance], "Assemblies", ".dll") {
			jobs = append(jobs, job{m, a})
		}
	}
	out := make([]*AssemblyFacts, len(jobs))
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				j := jobs[i]
				f := &AssemblyFacts{Instance: j.m.Instance, File: j.a.Rel, Skipped: harmony.Skipped(filepath.Base(j.a.Path), skip)}
				r, err := harmony.Read(j.a.Path, f.Skipped)
				if err != nil {
					f.Err = err.Error()
					f.Assembly = &harmony.Assembly{}
				} else {
					f.Assembly = r
				}
				out[i] = f
			}
		}()
	}
	for i := range jobs {
		next <- i
	}
	close(next)
	wg.Wait()
	return out
}

type hpatch struct {
	mod *Mod
	asm *AssemblyFacts
	p   harmony.Patch
}

// harmonyFindings: shared targets, skipping prefixes, several transpilers, manual patching.
func harmonyFindings(active []*Mod, asms []*AssemblyFacts) []*Finding {
	byInst := map[string]*Mod{}
	for _, m := range active {
		byInst[m.Instance] = m
	}
	byTarget := map[string][]hpatch{}
	var targets []string
	manual := map[*Mod][]string{}
	var manualMods []*Mod
	for _, a := range asms {
		m := byInst[a.Instance]
		if m == nil || a.Skipped {
			continue
		}
		for _, p := range a.Patches {
			if p.Dynamic {
				continue
			}
			t := p.Target()
			if byTarget[t] == nil {
				targets = append(targets, t)
			}
			byTarget[t] = append(byTarget[t], hpatch{m, a, p})
		}
		if a.Manual {
			if manual[m] == nil {
				manualMods = append(manualMods, m)
			}
			manual[m] = append(manual[m], a.File)
		}
	}
	sort.Strings(targets)
	var out []*Finding
	ev := func(h hpatch) finding.Evidence {
		t := fmt.Sprintf("%s %s.%s (%s)", h.p.Kind, h.p.PatchType, h.p.PatchMethod, h.asm.File)
		if h.p.Skips {
			t += " returns bool: may skip the original"
		}
		return finding.Evidence{Source: finding.Declared, Ref: finding.Inst(h.mod.Instance), Text: t}
	}
	for _, t := range targets {
		hs := byTarget[t]
		mods := map[*Mod]bool{}
		var order []*Mod
		for _, h := range hs {
			if !mods[h.mod] {
				mods[h.mod] = true
				order = append(order, h.mod)
			}
		}
		sort.SliceStable(order, func(i, j int) bool { return order[i].Pos < order[j].Pos })
		if len(order) < 2 {
			continue
		}
		var subj []string
		for _, m := range order {
			subj = append(subj, finding.Inst(m.Instance))
		}
		var evs []finding.Evidence
		for _, h := range hs {
			evs = append(evs, ev(h))
		}
		f := &Finding{Code: "harmony.shared-target", Severity: finding.Info, Layer: "harmony", Confidence: finding.Certain, Key: t,
			Subjects: subj, Claim: fmt.Sprintf("%d mods patch %s", len(order), t), Evidence: evs,
			Suggestions: []string{"archo harmony --target " + t}}
		out = append(out, f)
		// Transpilers from several mods.
		trans := map[*Mod]bool{}
		for _, h := range hs {
			if h.p.Kind == harmony.Transpiler {
				trans[h.mod] = true
			}
		}
		if len(trans) > 1 {
			var ts []string
			var tev []finding.Evidence
			for _, m := range order {
				if trans[m] {
					ts = append(ts, finding.Inst(m.Instance))
				}
			}
			for _, h := range hs {
				if h.p.Kind == harmony.Transpiler {
					tev = append(tev, ev(h))
				}
			}
			out = append(out, &Finding{Code: "harmony.multi-transpiler", Severity: finding.Warning, Layer: "harmony", Confidence: finding.Likely,
				Key: t, Subjects: ts, Evidence: tev,
				Claim:       fmt.Sprintf("%d mods rewrite %s with transpilers; their IL edits can conflict", len(trans), t),
				Suggestions: []string{"archo harmony --target " + t, "archo source search \"" + shortMethod(t) + "\" --in " + strings.Join(ts, " --in ")}})
		}
		// A bool prefix that can skip the original while other mods patch it too.
		for _, h := range hs {
			if !h.p.Skips {
				continue
			}
			others := 0
			for _, m := range order {
				if m != h.mod {
					others++
				}
			}
			f := &Finding{Code: "harmony.skip-prefix", Severity: finding.Warning, Layer: "harmony", Confidence: finding.Possible,
				Key:      t + "|" + h.p.PatchType + "." + h.p.PatchMethod,
				Subjects: []string{finding.Inst(h.mod.Instance)},
				Claim:    fmt.Sprintf("%s has a bool prefix on %s that can skip the original; %d other mods patch it", finding.Inst(h.mod.Instance), t, others),
				Evidence: []finding.Evidence{ev(h)}, Suggestions: []string{"archo harmony --target " + t,
					fmt.Sprintf("archo source %s --type %s", finding.Inst(h.mod.Instance), h.p.PatchType)}}
			for _, m := range order {
				if m != h.mod {
					f.Subjects = append(f.Subjects, finding.Inst(m.Instance))
				}
			}
			out = append(out, f)
		}
	}
	for _, m := range manualMods {
		out = append(out, &Finding{Code: "harmony.manual", Severity: finding.Info, Layer: "harmony", Confidence: finding.Certain,
			Key: "manual", Subjects: []string{finding.Inst(m.Instance)},
			Claim:       fmt.Sprintf("%s calls Harmony.Patch from code; those targets cannot be determined statically", finding.Inst(m.Instance)),
			Evidence:    []finding.Evidence{{Source: finding.Declared, Ref: finding.Inst(m.Instance), Text: "MemberRef HarmonyLib.Harmony::Patch in " + strings.Join(manual[m], ", ")}},
			Suggestions: []string{fmt.Sprintf("archo source search \".Patch(\" --in %s", finding.Inst(m.Instance))}})
	}
	return out
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
