package app

// List modifiers (06 §2): for a "Collection was modified" error, or a tick crash that a
// changed comp list produces, the code that changed the list ran inside the loop body and is
// not on the stack. Every enabled mod's assemblies are scanned (IL, no decompilation) for
// methods that change a thing's comp list (dotnet.CompEdits); comp tick overrides rank first,
// since ThingWithComps.Tick calls them inside its loop. Results are cached per DLL hash in
// <data>/cache/ilscan/ (MCP eval h7: the culprit comp removed itself in CompTick and had no
// frame in the stack; the default source search covered only the two mods in it).

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/dotnet"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Attribution roles of the list-modifier and caller-patch rules (06 §3).
const (
	// RoleListModifier: the mod's code changes a thing's comp list (not on the stack).
	RoleListModifier = "list-modifier"
	// RoleListed: only a patch on a caller of the throwing frame, no frame of its own; shown,
	// not a candidate (kept out of ImplicatedMods and source searches).
	RoleListed = "listed"
)

// ListScan is what the list-modifier rule found for a cluster.
type ListScan struct {
	// Enumerated: the list the throwing method enumerates with foreach, when its IL says so
	// ("Verse.ThingWithComps.AllComps"); EnumeratedBy is that method's mod.
	Enumerated, EnumeratedBy string `json:",omitempty"`
	// Mods and Assemblies scanned (every enabled mod's effective DLLs, libraries left out).
	Mods, Assemblies int
	Modifiers        []*ListModifier `json:",omitempty"`
}

// ListModifier is one method of an enabled mod that changes a comp list.
type ListModifier struct {
	Instance string
	File     string // the DLL, relative to the mod folder
	Type     string
	Method   string
	Ops      []string
	Tick     string   `json:",omitempty"` // the ThingComp tick method it overrides
	Attached []string `json:",omitempty"` // "ThingDef/Human by #ref Patches/X.xml:41"
}

// ilScanVersion invalidates cached scans when dotnet.CompEdits changes.
const ilScanVersion = 1

var (
	collModRe = regexp.MustCompile(`Collection was modified`)
	// listExcRe: the exceptions a changed comp list throws in a tick loop: InvalidOperationException
	// from a foreach, ArgumentOutOfRangeException from the game's cached-count index loop.
	listExcRe = regexp.MustCompile(`InvalidOperationException|ArgumentOutOfRangeException`)
)

// wantsListScan: a "Collection was modified" cluster, or a tick crash (hot path) with an
// exception a changed comp list can throw.
func wantsListScan(lc *LogCluster, hot bool) bool {
	m := lc.First.Message
	return collModRe.MatchString(m) || hot && listExcRe.MatchString(m)
}

// throwingFrame is the index of the topmost frame that is not .NET's own (List`1+Enumerator,
// ThrowHelper …): the method whose code threw. -1 when the stack is not an exception's.
func throwingFrame(e *logs.Entry) int {
	for i, fr := range e.Frames {
		if fr.Caller {
			return -1
		}
		if strings.HasPrefix(fr.Type, "System.") || strings.HasPrefix(fr.Type, "Mono.") || fr.Type == "" {
			continue
		}
		return i
	}
	return -1
}

// listModifiers runs the list-modifier rule on a cluster.
func (cx *attrCtx) listModifiers(lc *LogCluster) {
	ls := &ListScan{}
	lc.ListScan = ls
	collMod := collModRe.MatchString(lc.First.Message)
	if i := throwingFrame(lc.First); i >= 0 && collMod {
		fr := lc.First.Frames[i]
		if !logs.GameNamespace(fr.Type) {
			for _, id := range cx.types(fr.Type) {
				m := cx.st.Ix.ByID[id]
				if m == nil || cx.st.Position(id) == 0 {
					continue
				}
				for _, p := range cx.assemblyPaths(m) {
					a, err := dotnet.Open(p)
					if err != nil {
						continue
					}
					if en := a.Enumerated(fr.Type, fr.Method); len(en) > 0 {
						ls.Enumerated, ls.EnumeratedBy = en[0], id
						for _, x := range en {
							if dotnet.IsCompList(x) {
								ls.Enumerated = x
							}
						}
						break
					}
				}
				if ls.Enumerated != "" {
					break
				}
			}
		}
	}
	if collMod && ls.Enumerated == "" {
		lc.Details = append(lc.Details, "the frame above List.Enumerator.MoveNext is the loop that threw; the list was changed by code running in that loop body, which is not on the stack")
	}
	if ls.Enumerated != "" && !dotnet.IsCompList(ls.Enumerated) {
		return // another list: the comp-list scan says nothing about it
	}
	type res struct {
		m     *store.Mod
		file  string
		edits []dotnet.CompEdit
		tm    map[string]string
	}
	mods := cx.enabledMods()
	var mu sync.Mutex
	var found []res
	var wg sync.WaitGroup
	next := make(chan [2]string)
	byID := map[string]*store.Mod{}
	for _, m := range mods {
		byID[m.ID] = m
	}
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range next {
				c := cx.a.compScan(j[1])
				if c == nil || len(c.Edits) == 0 {
					continue
				}
				m := byID[j[0]]
				rel, _ := filepath.Rel(cx.st.Ix.ModDir(m), j[1])
				mu.Lock()
				found = append(found, res{m, filepath.ToSlash(rel), c.Edits, c.CompClasses})
				mu.Unlock()
			}
		}()
	}
	for _, m := range mods {
		ps := cx.assemblyPaths(m)
		if len(ps) > 0 {
			ls.Mods++
		}
		for _, p := range ps {
			ls.Assemblies++
			next <- [2]string{m.ID, p}
		}
	}
	close(next)
	wg.Wait()
	compClasses := map[string]map[string]string{} // instance → properties class → comp class
	for _, r := range found {
		if compClasses[r.m.ID] == nil {
			compClasses[r.m.ID] = map[string]string{}
		}
		for k, v := range r.tm {
			compClasses[r.m.ID][k] = v
		}
		for _, e := range r.edits {
			ls.Modifiers = append(ls.Modifiers, &ListModifier{Instance: r.m.ID, File: r.file, Type: e.Type, Method: e.Method, Ops: e.Ops, Tick: e.Tick})
		}
	}
	sort.SliceStable(ls.Modifiers, func(i, j int) bool {
		a, b := ls.Modifiers[i], ls.Modifiers[j]
		if (a.Tick != "") != (b.Tick != "") {
			return a.Tick != ""
		}
		if pa, pb := cx.st.Position(a.Instance), cx.st.Position(b.Instance); pa != pb {
			return pa < pb
		}
		return a.Type+"."+a.Method < b.Type+"."+b.Method
	})
	ticks := 0
	for _, lm := range ls.Modifiers {
		if lm.Tick != "" {
			ticks++
			cx.attachedTo(lm, compClasses[lm.Instance])
		}
	}
	// Candidates: the tick overrides; the others too while they are few (a list of every mod
	// that adds a comp somewhere would bury the one that matters).
	for _, lm := range ls.Modifiers {
		where := fmt.Sprintf("%s.%s (%s)", lm.Type, lm.Method, strings.Join(lm.Ops, ", "))
		switch {
		case lm.Tick != "":
			lc.add(lm.Instance, RoleListModifier, "changes the comp list in "+where+": a ThingComp."+lm.Tick+" override, which ThingWithComps ticks inside its loop over the comps (not on the stack)")
		case len(ls.Modifiers)-ticks <= 3 && collMod:
			lc.add(lm.Instance, RoleListModifier, "changes the comp list in "+where+" (not in a tick method)")
		}
	}
	for _, lm := range ls.Modifiers {
		if lm.Tick != "" {
			lc.next(fmt.Sprintf("archo source %s --type %s", cx.st.Ix.Ref(lm.Instance), outerType(lm.Type)))
			break
		}
	}
}

// attachedTo fills where XML attaches a candidate comp class: the effective Defs and Patches
// that name it as <compClass> text (or name a CompProperties class whose constructor sets it),
// with the def or the patch's touched defs from the analysis (05 §5.2).
func (cx *attrCtx) attachedTo(lm *ListModifier, props map[string]string) {
	names := []string{lm.Type}
	for p, c := range props {
		if c == lm.Type {
			names = append(names, p)
		}
	}
	sort.Strings(names[1:])
	x, _ := cx.a.RefIndex(cx.st)
	if x == nil {
		return
	}
	want := map[string]int{}
	for i, n := range names {
		want[strings.ReplaceAll(n, "+", ".")] = i
	}
	files := map[int]bool{}
	for n := range want {
		for _, f := range x.filesWith(n) {
			files[f] = true
		}
	}
	var idx []int
	for f := range files {
		idx = append(idx, f)
	}
	sort.Ints(idx)
	patchFacts := map[string][]*analyze.PatchFact{}
	for _, f := range idx {
		rf := x.files[f]
		if cx.st.Position(rf.Inst) == 0 {
			continue
		}
		b, err := os.ReadFile(rf.Path)
		if err != nil {
			continue
		}
		refScan(b, want, func(i, line int) {
			if len(lm.Attached) >= 3 {
				return
			}
			what := ""
			rel := filepath.ToSlash(rf.Rel)
			if cx.run != nil && strings.Contains(strings.ToLower(rel), "patches/") {
				pf, ok := patchFacts[rf.Inst]
				if !ok {
					pf, _ = cx.a.Store.PatchFacts(cx.run.ID, store.PatchFilter{Instance: rf.Inst})
					patchFacts[rf.Inst] = pf
				}
				var best *analyze.PatchFact
				for _, p := range pf {
					if filepath.ToSlash(p.File) == rel && p.Line <= line && (best == nil || p.Line > best.Line) {
						best = p
					}
				}
				switch {
				case best != nil && len(best.Targets) > 0:
					what = strings.Join(best.Targets, ", ")
				case best != nil:
					what = "nothing (the patch matches no def now)"
				}
			} else if ds, err := cx.a.Store.DefsInFile(filepath.Base(rel), map[string]bool{rf.Inst: true}); err == nil {
				var best *analyze.DefFact
				for k := range ds {
					d := &ds[k]
					if filepath.ToSlash(d.File) == rel && d.Line <= line && (best == nil || d.Line > best.Line) {
						best = d
					}
				}
				if best != nil {
					what = best.Type + "/" + best.DefName
					if best.DefName == "" {
						what = best.Type + "@" + best.Name
					}
				}
			}
			via := ""
			if i > 0 {
				via = " (as " + names[i] + ")"
			}
			if what == "" {
				lm.Attached = append(lm.Attached, fmt.Sprintf("named%s by %s %s:%d", via, cx.st.Ix.Ref(rf.Inst), rel, line))
				return
			}
			lm.Attached = append(lm.Attached, fmt.Sprintf("%s%s by %s %s:%d", what, via, cx.st.Ix.Ref(rf.Inst), rel, line))
		})
	}
}

// compScanResult is the cached scan of one DLL.
type compScanResult struct {
	V           int               `json:"v"`
	Edits       []dotnet.CompEdit `json:"edits,omitempty"`
	CompClasses map[string]string `json:"compClasses,omitempty"`
}

var compScanMemo sync.Map // path|size|mtime → *compScanResult (this process)

// compScan reads the comp-list edits of a DLL: from memory, from <data>/cache/ilscan/<sha256>.json,
// or by scanning the IL now (and storing it). nil when the file cannot be read.
func (a *App) compScan(path string) *compScanResult {
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	mk := fmt.Sprintf("%s|%d|%d", path, fi.Size(), fi.ModTime().UnixNano())
	if v, ok := compScanMemo.Load(mk); ok {
		return v.(*compScanResult)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	f.Close()
	if err != nil {
		return nil
	}
	cache := filepath.Join(a.cacheDir("ilscan"), hex.EncodeToString(h.Sum(nil))+".json")
	if b, err := os.ReadFile(cache); err == nil {
		var r compScanResult
		if json.Unmarshal(b, &r) == nil && r.V == ilScanVersion {
			compScanMemo.Store(mk, &r)
			return &r
		}
	}
	r := &compScanResult{V: ilScanVersion}
	if asm, err := dotnet.Open(path); err == nil {
		r.Edits = asm.CompEdits()
		if len(r.Edits) > 0 {
			r.CompClasses = asm.CompClassesSet()
		}
	}
	if b, err := json.Marshal(r); err == nil && a.Home != "" {
		if os.MkdirAll(filepath.Dir(cache), 0o755) == nil {
			tmp := cache + ".tmp"
			if os.WriteFile(tmp, b, 0o644) == nil {
				os.Rename(tmp, cache)
			}
		}
	}
	compScanMemo.Store(mk, r)
	return r
}

// callerPatch reports a Harmony patch line that only sits on a caller of the throwing frame:
// at a frame below it, not a transpiler (a transpiler rewrites the caller's own code), by a
// mod with no frame of its own in the stack (06 §3). Such a mod is listed, not a candidate
// (MCP eval h7: a freeze prefix on Pawn.Tick, two frames below a profiler's foreach).
func callerPatch(frame, throwing int, kind string, ownFrame bool) bool {
	return throwing >= 0 && frame > throwing && !strings.EqualFold(kind, "transpiler") && !ownFrame
}

// throwingModFrame is throwingFrame when that frame is an enabled mod's own code, else -1. The
// caller-patch rule applies only then: the error has a source with a frame, and a prefix two
// frames below adds nothing. When game code threw, a patch on a caller can be all there is
// (not measured on the log corpus, so the rule stays this narrow; 06 §3).
func (cx *attrCtx) throwingModFrame(e *logs.Entry) int {
	i := throwingFrame(e)
	if i < 0 || logs.GameNamespace(e.Frames[i].Type) || len(cx.types(e.Frames[i].Type)) == 0 {
		return -1
	}
	return i
}

// ownFrames: the enabled mods whose types appear as frames of the stack.
func (cx *attrCtx) ownFrames(e *logs.Entry) map[string]bool {
	out := map[string]bool{}
	for _, fr := range e.Frames {
		if logs.GameNamespace(fr.Type) {
			continue
		}
		for _, id := range cx.types(fr.Type) {
			out[id] = true
		}
	}
	return out
}

// dynamicPatch reports whether a mod's patch class picks its target at run time
// (TargetMethod() that the analysis could not read): `archo harmony --target <frame>` finds
// no row for it (MCP eval h4).
func (cx *attrCtx) dynamicPatch(inst, patchType string) bool {
	if cx.dynMemo == nil {
		cx.dynMemo = map[string]map[string]bool{}
	}
	rows, ok := cx.dynMemo[inst]
	if !ok {
		rows = map[string]bool{}
		hs, _ := cx.a.Store.HarmonyPatches(store.HarmonyFilter{Instance: inst})
		for _, h := range hs {
			if h.Dynamic && !h.Manual {
				rows[h.PatchType] = true
			}
		}
		cx.dynMemo[inst] = rows
	}
	return rows[patchType]
}
