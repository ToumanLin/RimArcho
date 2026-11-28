package analyze

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/content"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/harmony"
)

// AssemblyFacts is the Harmony layer of one effective DLL of an instance.
type AssemblyFacts struct {
	Instance string
	File     string // relative to the mod root
	Skipped  bool   // known library: Harmony facts not collected
	Err      string
	*harmony.Assembly
}

// AssemblyKey identifies the DLL input independently of XML folder selection. Skipped
// libraries depend on the owning package and the caller's skip policy as well as files.
func AssemblyKey(m *Mod, folders []string, skip []string) string {
	var files []string
	for _, a := range content.CodeFiles(m.Dir, folders) {
		files = append(files, fmt.Sprintf("%s:%t", a.Rel, harmony.SkippedFor(filepath.Base(a.Path), m.Pkg, skip)))
	}
	data, _ := json.Marshal(files)
	return fmt.Sprintf("%x|dll%x|a%d", m.SnapHash, sha256.Sum256(data), Version)
}

// harmonyLayer reads every effective DLL of the active mods (05 §5.3). It is independent
// of the Def/Patch layers: a failure only affects its own facts.
func harmonyLayer(active []*Mod, folders map[string][]string, skip []string, workers int, cache AssemblyCache) ([]*AssemblyFacts, []DLLTime) {
	type job struct {
		m *Mod
		a content.Asset
	}
	var jobs []job
	for _, m := range active {
		for _, a := range content.CodeFiles(m.Dir, folders[m.Instance]) {
			jobs = append(jobs, job{m, a})
		}
	}
	out := make([]*AssemblyFacts, len(jobs))
	times := make([]DLLTime, len(jobs))
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				j := jobs[i]
				f := &AssemblyFacts{Instance: j.m.Instance, File: j.a.Rel, Skipped: harmony.SkippedFor(filepath.Base(j.a.Path), j.m.Pkg, skip)}
				t0 := time.Now()
				r, cached, err := readAssembly(cache, j.a.Path, f.Skipped)
				times[i] = DLLTime{Instance: j.m.Instance, Pkg: j.m.Pkg, File: j.a.Rel, Cached: cached, Dur: time.Since(t0)}
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
	return out, times
}

type hpatch struct {
	mod *Mod
	asm *AssemblyFacts
	p   harmony.Patch
}

// manualStat sums a mod's manual Harmony.Patch calls over its assemblies.
type manualStat struct {
	calls, resolved int
	why             map[string]int
}

// harmonyIndex groups the static patches of the active mods by target and collects the
// mods that patch from code.
type harmonyIndex struct {
	byTarget   map[string][]hpatch
	targets    []string // sorted
	manual     map[*Mod][]string
	mstats     map[*Mod]*manualStat
	manualMods []*Mod
}

// harmonyFindings: shared targets, skipping prefixes, several transpilers, manual patching.
func harmonyFindings(active []*Mod, asms []*AssemblyFacts) []*Finding {
	ix := indexHarmony(active, asms)
	var out []*Finding
	for _, t := range ix.targets {
		out = targetFindings(out, t, ix.byTarget[t])
	}
	for _, m := range ix.manualMods {
		out = append(out, manualFinding(m, ix.manual[m], ix.mstats[m]))
	}
	return out
}

// indexHarmony reads the assemblies of the active mods, skipped libraries excluded.
func indexHarmony(active []*Mod, asms []*AssemblyFacts) *harmonyIndex {
	byInst := map[string]*Mod{}
	for _, m := range active {
		byInst[m.Instance] = m
	}
	ix := &harmonyIndex{byTarget: map[string][]hpatch{}, manual: map[*Mod][]string{}, mstats: map[*Mod]*manualStat{}}
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
			if ix.byTarget[t] == nil {
				ix.targets = append(ix.targets, t)
			}
			ix.byTarget[t] = append(ix.byTarget[t], hpatch{m, a, p})
		}
		if a.Manual {
			ix.addManual(m, a)
		}
	}
	sort.Strings(ix.targets)
	return ix
}

// addManual records an assembly of m that calls Harmony.Patch from code.
func (ix *harmonyIndex) addManual(m *Mod, a *AssemblyFacts) {
	if ix.manual[m] == nil {
		ix.manualMods = append(ix.manualMods, m)
		ix.mstats[m] = &manualStat{why: map[string]int{}}
	}
	ix.manual[m] = append(ix.manual[m], a.File)
	ms := ix.mstats[m]
	ms.calls += a.ManualCalls
	ms.resolved += a.ManualResolved
	for r, n := range a.Unresolved {
		ms.why[r] += n
	}
}

// patchEvidence describes one patch as declared evidence.
func patchEvidence(h hpatch) finding.Evidence {
	t := fmt.Sprintf("%s %s.%s (%s)", h.p.Kind, h.p.PatchType, h.p.PatchMethod, h.asm.File)
	if h.p.Skips {
		t += " returns bool: may skip the original"
	}
	return finding.Evidence{Source: finding.Declared, Ref: finding.Inst(h.mod.Instance), Text: t}
}

// patchingMods lists the mods behind the patches of one target, in load order.
func patchingMods(hs []hpatch) []*Mod {
	mods := map[*Mod]bool{}
	var order []*Mod
	for _, h := range hs {
		if !mods[h.mod] {
			mods[h.mod] = true
			order = append(order, h.mod)
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i].Pos < order[j].Pos })
	return order
}

// targetFindings appends the findings of target t, patched by hs, when several mods patch it.
func targetFindings(out []*Finding, t string, hs []hpatch) []*Finding {
	order := patchingMods(hs)
	if len(order) < 2 {
		return out
	}
	var subj []string
	for _, m := range order {
		subj = append(subj, finding.Inst(m.Instance))
	}
	var evs []finding.Evidence
	for _, h := range hs {
		evs = append(evs, patchEvidence(h))
	}
	f := &Finding{Code: "harmony.shared-target", Severity: finding.Info, Layer: "harmony", Confidence: finding.Certain, Key: t,
		Subjects: subj, Claim: fmt.Sprintf("%d mods patch %s", len(order), t), Evidence: evs,
		Suggestions: []string{"archo harmony --target " + t}}
	out = append(out, f)
	if f := multiTranspiler(t, hs, order); f != nil {
		out = append(out, f)
	}
	// A bool prefix that can skip the original while other mods patch it too.
	for _, h := range hs {
		if h.p.Skips {
			out = append(out, skipPrefix(t, h, order))
		}
	}
	return out
}

// multiTranspiler is the finding for transpilers from several mods on t, or nil.
func multiTranspiler(t string, hs []hpatch, order []*Mod) *Finding {
	trans := map[*Mod]bool{}
	for _, h := range hs {
		if h.p.Kind == harmony.Transpiler {
			trans[h.mod] = true
		}
	}
	if len(trans) <= 1 {
		return nil
	}
	var ts []string
	var tev []finding.Evidence
	for _, m := range order {
		if trans[m] {
			ts = append(ts, finding.Inst(m.Instance))
		}
	}
	for _, h := range hs {
		if h.p.Kind == harmony.Transpiler {
			tev = append(tev, patchEvidence(h))
		}
	}
	// Info (05 §4.3.1): after the manual targets of G9, the subjects of this finding are
	// error sources 2.3x as often as chance, less than harmony.shared-target (2.6x, info),
	// and the rewritten method is in an error's stack no more often than a shared target
	// (0.5% / 0.6%). It is raised to warning when a current log's stack passes through it.
	return &Finding{Code: "harmony.multi-transpiler", Severity: finding.Info, Layer: "harmony", Confidence: finding.Likely,
		Key: t, Subjects: ts, Evidence: tev,
		Claim:       fmt.Sprintf("%d mods rewrite %s with transpilers; their IL edits can conflict", len(trans), t),
		Suggestions: []string{"archo harmony --target " + t, "archo source search \"" + shortMethod(t) + "\" --in " + strings.Join(ts, " --in ")}}
}

// skipPrefix is the finding for h, a bool prefix on t, which the other mods in order also patch.
func skipPrefix(t string, h hpatch, order []*Mod) *Finding {
	others := 0
	for _, m := range order {
		if m != h.mod {
			others++
		}
	}
	f := &Finding{Code: "harmony.skip-prefix", Severity: finding.Info, Layer: "harmony", Confidence: finding.Possible,
		Key:      t + "|" + h.p.PatchType + "." + h.p.PatchMethod,
		Subjects: []string{finding.Inst(h.mod.Instance)},
		Claim:    fmt.Sprintf("%s has a bool prefix on %s that can skip the original; %d other mods patch it", finding.Inst(h.mod.Instance), t, others),
		Evidence: []finding.Evidence{patchEvidence(h)}, Suggestions: []string{"archo harmony --target " + t,
			fmt.Sprintf("archo source %s --type %s", finding.Inst(h.mod.Instance), h.p.PatchType)}}
	for _, m := range order {
		if m != h.mod {
			f.Subjects = append(f.Subjects, finding.Inst(m.Instance))
		}
	}
	return f
}

// manualFinding is the finding for m, which calls Harmony.Patch from the files listed.
func manualFinding(m *Mod, files []string, ms *manualStat) *Finding {
	claim := fmt.Sprintf("%s calls Harmony.Patch from code; those targets cannot be determined statically", finding.Inst(m.Instance))
	if ms.calls > 0 {
		claim = fmt.Sprintf("%s calls Harmony.Patch from code; %d of %d calls have a target found statically", finding.Inst(m.Instance), ms.resolved, ms.calls)
	}
	evs := []finding.Evidence{{Source: finding.Declared, Ref: finding.Inst(m.Instance), Text: "Harmony.Patch calls in " + strings.Join(files, ", ")}}
	if len(ms.why) > 0 {
		var ws []string
		for r, n := range ms.why {
			ws = append(ws, fmt.Sprintf("%s %d", r, n))
		}
		sort.Strings(ws)
		evs = append(evs, finding.Evidence{Source: finding.Declared, Ref: finding.Inst(m.Instance), Text: "not resolved: " + strings.Join(ws, ", ")})
	}
	return &Finding{Code: "harmony.manual", Severity: finding.Info, Layer: "harmony", Confidence: finding.Certain,
		Key: "manual", Subjects: []string{finding.Inst(m.Instance)}, Claim: claim, Evidence: evs,
		Suggestions: []string{fmt.Sprintf("archo source search \".Patch(\" --in %s", finding.Inst(m.Instance))}}
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
