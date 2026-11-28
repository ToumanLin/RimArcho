// Package analyze builds the Def and Patch layers (05 §2, §5.1, §5.2): Def facts from each
// mod's effective Defs, and a load-order simulation of every PatchOperation against the
// merged Defs document. It runs in the `archo analyze` child process; it knows nothing of
// the store — the caller supplies the mods and persists the Output.
package analyze

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/content"
	"github.com/ToumanLin/RimArcho/internal/harmony"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/xdoc"
)

// Version of the analyzers; facts written by another version are rebuilt (05 §2).
const Version = 12 // 12: unresolved content classification fields (12 §2).

// PatchVersion versions the patch facts alone: it is in the run key (app.AnalysisKey), so
// lists are simulated again, but not in the Def and assembly fact keys, which stay. 1:
// succeeded operations reached only through a guard are marked (ReasonGuarded, Stats.Guards).
// 2: touches under <success>Always</success> count as guarded (they cannot fail). 3: a
// target only in a conditional load folder that does not load is folder-inactive.
const PatchVersion = 3

// Mod is one installed instance.
type Mod struct {
	Instance string
	Lineage  string
	SnapID   int64
	SnapHash []byte
	Dir      string
	Pkg      string // lower case packageId without _steam
	Name     string // display name (FindMod matches it; DLCs use their folder name)
	Decl     scan.Decl
	Pos      int // 1-based load position; 0 = inactive
}

// DefFact is one top-level Def (05 §2).
type DefFact struct {
	Instance string `json:"-"`
	Type     string `json:"t"`
	DefName  string `json:"d,omitempty"`
	Name     string `json:"n,omitempty"`
	Parent   string `json:"p,omitempty"`
	Abstract bool   `json:"a,omitempty"`
	File     string `json:"f"` // relative to the mod root
	Line     int    `json:"l"`
	// MayRequire / MayRequireAnyOf on the def: the game skips it unless they are active.
	MayRequire    string      `json:"mr,omitempty"`
	MayRequireAny string      `json:"mra,omitempty"`
	Content       *DefContent `json:"content,omitempty"`
}

// Loaded reports whether the game loads the def with the given packages active.
func (f *DefFact) Loaded(active func(string) bool) bool {
	return mayRequireOK(f.MayRequire, f.MayRequireAny, active)
}

func mayRequireOK(all, any string, active func(string) bool) bool {
	for _, p := range splitPkgs(all) {
		if !active(p) {
			return false
		}
	}
	if anyOf := splitPkgs(any); len(anyOf) > 0 {
		for _, p := range anyOf {
			if active(p) {
				return true
			}
		}
		return false
	}
	return true
}

// Input to Run.
type Input struct {
	Mods []*Mod // any order; Pos gives the load order
	MM   string // game major.minor
	// Cached returns stored Def facts of an instance for an effective-content key, if any.
	Cached func(instance, key string) ([]DefFact, bool)
	// Progress receives one line per phase step (may be nil).
	Progress func(string)
	Workers  int
	// HarmonySkip lists library DLLs (globs) whose Harmony facts are skipped; nil = default.
	HarmonySkip []string
	// Assemblies keeps Harmony facts by DLL content between checks; nil reads every DLL.
	Assemblies AssemblyCache
	// AfterSimulation is called with the merged document still live (diagnostics).
	AfterSimulation func()
}

// ContentState is the effective content of one instance.
type ContentState struct {
	Key         string
	AssemblyKey string // snapshot, effective DLL files, analyzer and skip policy
	Folders     []string
	DefFiles    int
	PatchFiles  int
	ParseErrors []string
}

// Output of Run.
type Output struct {
	Content    map[string]*ContentState // by instance
	NewDefs    map[string][]DefFact     // instances whose facts were (re)built
	Patches    []*PatchFact
	Assemblies []*AssemblyFacts
	Findings   []*Finding
	Stats      Stats
}

// Stats summarise a run.
type Stats struct {
	ActiveMods, InstalledMods int
	DefFiles, PatchFiles      int
	Defs, TopOps, UnknownOps  int
	// SlowOps: operations not simulated because an XPath evaluation went past the step
	// budget (patch.too-slow, 05 §5.2); SlowestOp and SlowestMs: the operation that took
	// longest ("file:line", for the feedback report's Mod check line)
	SlowOps                    int    `json:",omitempty"`
	SlowestOp                  string `json:",omitempty"`
	SlowestMs                  int64  `json:",omitempty"`
	Failed                     int
	ParseErrors                int
	Nodes, TextMB              int
	IndexHits, IndexRebuilds   int
	Evaluations                int
	DefsMs, PatchMs, TotalMs   int64
	CachedMods                 int
	PeakHeapMB, LiveHeapMB     int
	Assemblies, AssemblyErrors int
	HarmonyPatches, Types      int
	HarmonyMs                  int64
	// Guards: the run marks guarded operations (PatchVersion ≥ 1); the app's quick patch
	// check between runs trusts only such runs (05 §4.4 "Between checks")
	Guards bool `json:",omitempty"`

	// For the slow-check report (16 §2.14 "Slow checks"), not stored: the patch layer's
	// reading and simulation (the rest of PatchMs builds facts and findings), its slowest
	// operations and the Harmony layer's slowest DLLs (slowest first), how many DLLs came
	// from the assembly cache, and how many methods the static patches target.
	PatchParseMs, PatchSimMs int64     `json:"-"`
	SlowestOps               []OpTime  `json:"-"`
	SlowestDLLs              []DLLTime `json:"-"`
	DLLsCached               int       `json:"-"`
	HarmonyTargets           int       `json:"-"`
}

// SlowestOpsKept and SlowestDLLsKept: how many of each Stats keeps.
const (
	SlowestOpsKept  = 10
	SlowestDLLsKept = 5
)

// OpTime is one top-level patch operation and how long the simulation spent on it.
type OpTime struct {
	Instance, Pkg string
	File          string // the patch file, relative to the Mod's folder
	Class         string // PatchOperationAdd, or a Mod's own class
	XPath         string // the operation's (a sequence's first); the one past the step budget when TooSlow
	TooSlow       bool   // patch.too-slow (05 §5.2)
	Dur           time.Duration
}

// DLLTime is one DLL the Harmony layer read and how long it took.
type DLLTime struct {
	Instance, Pkg string
	File          string // relative to the Mod's folder
	Cached        bool   // its facts came from the assembly cache
	Dur           time.Duration
}

// ProgressEvery is the longest the Patch simulation goes without a progress line while it
// is still moving (05 §3).
var ProgressEvery = 2 * time.Second

func (in *Input) progress(f string, a ...any) {
	if in.Progress != nil {
		in.Progress(fmt.Sprintf(f, a...))
	}
}

// fileJob is one content file of a Mod to parse.
type fileJob struct {
	mod   *Mod
	asset content.Asset
}

// run is the state Run's steps share: the input, the output being filled, the active Mods
// in load order and the merged Defs document.
type run struct {
	in       *Input
	out      *Output
	active   []*Mod
	isActive func(string) bool
	folders  map[string][]string
	requires map[string]map[string][]string
	skipped  map[string][]content.Skipped // conditional folders that do not load, by instance
	names    *xdoc.Names
	doc      *xdoc.Doc
	root     int32
	allDefs  map[string][]DefFact
	origin   map[int32]defLoc // top-level node -> where the def came from
}

// Run performs the analysis.
func Run(in Input) (*Output, error) {
	t0 := time.Now()
	if in.Workers <= 0 {
		in.Workers = min(max(2, runtime.NumCPU()), 8)
	}
	r, inactive := newRun(&in)
	r.resolveContent()

	// Def layer: active mods into the merged document, in load order.
	r.activeDefs()
	// Inactive mods: facts only (cached by effective-content key).
	r.inactiveDefs(inactive)
	r.out.Stats.DefsMs = time.Since(t0).Milliseconds()

	// Patch layer.
	t1 := time.Now()
	sim := newSim(r.doc, r.root, r.active, in.MM, r.folders)
	sim.skippedOf = r.skipped
	tops := r.parsePatches()
	t1s := time.Now()
	r.out.Stats.PatchParseMs = t1s.Sub(t1).Milliseconds()
	r.simulate(sim, tops)
	r.out.Stats.PatchSimMs = time.Since(t1s).Milliseconds()
	if in.AfterSimulation != nil {
		in.AfterSimulation()
	}

	// Facts and findings.
	r.patchFindings(sim, tops)
	r.out.Stats.PatchMs = time.Since(t1).Milliseconds()
	// The merged document (most of the heap) is not needed by the Harmony layer: freed
	// before it, the child's soft memory limit (app.AnalyzeMemoryLimit) does not make the
	// collector run without pause while the DLLs are read (18 §2).
	r.doc = nil

	// Harmony layer (independent of the others).
	t2 := time.Now()
	r.readAssemblies()
	out := r.out
	out.Stats.HarmonyMs = time.Since(t2).Milliseconds()
	out.Stats.TotalMs = time.Since(t0).Milliseconds()
	in.progress("done: %d failed operations, %d findings, %v", out.Stats.Failed, len(out.Findings), time.Since(t0).Round(time.Millisecond))
	return out, nil
}

// newRun splits the Mods into active (in load order) and inactive ones and starts the output.
func newRun(in *Input) (*run, []*Mod) {
	var active, inactive []*Mod
	for _, m := range in.Mods {
		if m.Pos > 0 {
			active = append(active, m)
		} else {
			inactive = append(inactive, m)
		}
	}
	sort.SliceStable(active, func(i, j int) bool { return active[i].Pos < active[j].Pos })
	activePkg := map[string]bool{}
	for _, m := range active {
		activePkg[m.Pkg] = true
	}
	r := &run{in: in, active: active,
		isActive: func(p string) bool { return activePkg[strings.TrimSuffix(strings.ToLower(p), "_steam")] }}
	r.out = &Output{Content: map[string]*ContentState{}, NewDefs: map[string][]DefFact{}}
	r.out.Stats.ActiveMods, r.out.Stats.InstalledMods = len(active), len(in.Mods)
	r.out.Stats.Guards = true
	return r, inactive
}

// resolveContent resolves every Mod's load folders and records its content state.
func (r *run) resolveContent() {
	r.folders = map[string][]string{}
	r.requires = map[string]map[string][]string{}
	r.skipped = map[string][]content.Skipped{}
	for _, m := range r.in.Mods {
		res := content.Resolve(m.Dir, m.Decl.LoadFolders, r.in.MM, r.isActive)
		r.folders[m.Instance] = res.Folders
		r.requires[m.Instance] = res.Requires
		r.skipped[m.Instance] = res.Skipped
		r.out.Content[m.Instance] = &ContentState{Key: res.Key(m.SnapHash, r.in.MM) + DefKeySuffix(), Folders: res.Folders}
		if m.Pos > 0 {
			r.out.Content[m.Instance].AssemblyKey = AssemblyKey(m, res.Folders, r.in.HarmonySkip)
		}
	}
}

// assetJobs lists the files of kind (Defs, Patches) in the Mods' load folders, and their paths.
func (r *run) assetJobs(mods []*Mod, kind string) ([]fileJob, []string) {
	var jobs []fileJob
	for _, m := range mods {
		for _, a := range content.Assets(m.Dir, r.folders[m.Instance], kind) {
			jobs = append(jobs, fileJob{m, a})
		}
	}
	paths := make([]string, len(jobs))
	for i, j := range jobs {
		paths[i] = j.asset.Path
	}
	return jobs, paths
}

// parseFailed records a file that could not be parsed; count adds it to the run's total.
func (r *run) parseFailed(j fileJob, err error, count bool) {
	cs := r.out.Content[j.mod.Instance]
	cs.ParseErrors = append(cs.ParseErrors, j.asset.Rel+": "+err.Error())
	if count {
		r.out.Stats.ParseErrors++
	}
}

// activeDefs builds the merged Defs document from the active Mods' Def files, in load order.
func (r *run) activeDefs() {
	r.names = xdoc.NewNames()
	r.doc = xdoc.New(r.names)
	r.root = r.doc.NewElement("Defs")
	r.doc.AppendChild(r.doc.Root(), r.root)
	r.allDefs = map[string][]DefFact{}
	r.origin = map[int32]defLoc{}
	jobs, paths := r.assetJobs(r.active, "Defs")
	done := 0
	parseOrdered(paths, r.in.Workers, func(i int, evs []xdoc.Event, err error) {
		j := jobs[i]
		r.out.Content[j.mod.Instance].DefFiles++
		if err != nil {
			r.parseFailed(j, err, true)
			return
		}
		tops, lines := r.doc.Build(r.root, evs, true, false)
		for k, t := range tops {
			f := defFact(r.doc, t, j.asset.Rel, int(lines[k]))
			r.allDefs[j.mod.Instance] = append(r.allDefs[j.mod.Instance], f)
			r.origin[t] = defLoc{mod: j.mod, fact: f}
		}
		done++
		if done%500 == 0 {
			r.in.progress("defs: %d/%d files", done, len(jobs))
		}
	})
	r.out.Stats.DefFiles = len(jobs)
	for _, m := range r.active {
		r.out.NewDefs[m.Instance] = r.allDefs[m.Instance]
		r.out.Stats.Defs += len(r.allDefs[m.Instance])
	}
	r.in.progress("defs: %d files, %d defs from %d active mods", len(jobs), r.out.Stats.Defs, len(r.active))
}

// inactiveDefs takes the inactive Mods' Def facts from the cache, or indexes their files.
func (r *run) inactiveDefs(inactive []*Mod) {
	var todo []*Mod
	for _, m := range inactive {
		if r.in.Cached != nil {
			if fs, ok := r.in.Cached(m.Instance, r.out.Content[m.Instance].Key); ok {
				r.allDefs[m.Instance] = fs
				r.out.Stats.CachedMods++
				continue
			}
		}
		todo = append(todo, m)
	}
	if len(todo) == 0 {
		return
	}
	for _, m := range todo {
		r.out.NewDefs[m.Instance] = []DefFact{} // record the key even with no defs
	}
	jobs, paths := r.assetJobs(todo, "Defs")
	parseOrdered(paths, r.in.Workers, func(i int, evs []xdoc.Event, err error) {
		j := jobs[i]
		r.out.Content[j.mod.Instance].DefFiles++
		if err != nil {
			r.parseFailed(j, err, false)
			return
		}
		scratch := xdoc.New(r.names)
		sroot := scratch.NewElement("Defs")
		tops, lines := scratch.Build(sroot, evs, true, false)
		for k, t := range tops {
			f := defFact(scratch, t, j.asset.Rel, int(lines[k]))
			r.allDefs[j.mod.Instance] = append(r.allDefs[j.mod.Instance], f)
		}
	})
	for _, m := range todo {
		r.out.NewDefs[m.Instance] = r.allDefs[m.Instance]
	}
	r.in.progress("defs: %d inactive mods indexed (%d cached)", len(todo), r.out.Stats.CachedMods)
}

// parsePatches reads the active Mods' top-level patch operations, in load order.
func (r *run) parsePatches() []*topOp {
	jobs, paths := r.assetJobs(r.active, "Patches")
	var tops []*topOp
	parseOrdered(paths, r.in.Workers, func(i int, evs []xdoc.Event, err error) {
		j := jobs[i]
		r.out.Content[j.mod.Instance].PatchFiles++
		if err != nil {
			r.parseFailed(j, err, true)
			return
		}
		pd := xdoc.New(r.names)
		pd.Build(pd.Root(), evs, false, true)
		de := pd.DocElement()
		if de == xdoc.None || pd.Name(de) != "Patch" {
			return
		}
		for _, c := range pd.Children(de) {
			if pd.Name(c) == "Operation" {
				tops = append(tops, &topOp{mod: j.mod, file: j.asset.Rel, op: parseOp(pd, c), folderReq: folderRequires(r.requires[j.mod.Instance], j.asset.Path)})
			}
		}
	})
	r.out.Stats.PatchFiles = len(jobs)
	r.in.progress("patches: %d files, %d operations", len(jobs), len(tops))
	return tops
}

// simulate runs the operations against the merged document and records the run's size.
func (r *run) simulate(sim *sim, tops []*topOp) {
	seq := map[string]int{}
	st := &r.out.Stats
	last := time.Now()
	slow := slowest[*topOp]{n: SlowestOpsKept}
	for k, t := range tops {
		seq[t.mod.Instance]++
		t.seq = seq[t.mod.Instance]
		t0 := time.Now()
		sim.run(t)
		now := time.Now()
		slow.add(t, now.Sub(t0))
		if d := now.Sub(t0).Milliseconds(); d > st.SlowestMs {
			st.SlowestMs, st.SlowestOp = d, fmt.Sprintf("%s %s:%d", t.mod.Name, t.file, t.op.Line)
		}
		// every 1000 operations, and at least every ProgressEvery while operations are
		// slow: the window's elapsed time moves only with these lines (05 §3)
		if (k+1)%1000 == 0 || k+1 == len(tops) || now.Sub(last) >= ProgressEvery {
			r.in.progress("patches: %d/%d operations", k+1, len(tops))
			last = now
		}
	}
	st.TopOps = len(tops)
	for i, t := range slow.items {
		st.SlowestOps = append(st.SlowestOps, OpTime{Instance: t.mod.Instance, Pkg: t.mod.Pkg, File: t.file,
			Class: t.op.Class, XPath: t.reportXPath(), TooSlow: t.slow, Dur: slow.durs[i]})
	}
	st.IndexHits, st.IndexRebuilds, st.Evaluations = sim.ix.hits, sim.ix.rebuilds, sim.evals
	st.Nodes, st.TextMB = r.doc.Len(), r.doc.TextBytes()>>20
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	st.PeakHeapMB = int(ms.HeapInuse >> 20)
	runtime.GC()
	runtime.ReadMemStats(&ms)
	st.LiveHeapMB = int(ms.HeapInuse >> 20)
}

// patchFindings builds the patch facts and the Def, parent and patch findings.
func (r *run) patchFindings(sim *sim, tops []*topOp) {
	out := r.out
	kb := newKnowledge(r.in.Mods, r.allDefs)
	out.Patches = sim.facts(tops, kb)
	for i, p := range out.Patches {
		if p.Result == ResultFailed {
			out.Stats.Failed++
		}
		if p.Result == ResultUnknown {
			out.Stats.UnknownOps++
		}
		if tops[i].slow {
			out.Stats.SlowOps++
		}
	}
	out.Findings = append(out.Findings, defFindings(r.active, r.allDefs, r.isActive)...)
	out.Findings = append(out.Findings, sim.parentFindings(r.origin, tops, r.isActive)...)
	r.origin = nil // not needed by the Harmony layer; free it as the unsplit Run did
	out.Findings = append(out.Findings, sim.findings(tops, out.Patches)...)
}

// readAssemblies reads the active Mods' assemblies and adds the Harmony findings.
func (r *run) readAssemblies() {
	out := r.out
	skip := r.in.HarmonySkip
	if skip == nil {
		skip = harmony.DefaultSkip
	}
	var times []DLLTime
	out.Assemblies, times = harmonyLayer(r.active, r.folders, skip, r.in.Workers, r.in.Assemblies)
	targets := map[string]bool{}
	for _, a := range out.Assemblies {
		out.Stats.Assemblies++
		out.Stats.HarmonyPatches += len(a.Patches)
		out.Stats.Types += len(a.Types)
		if a.Err != "" {
			out.Stats.AssemblyErrors++
		}
		for _, p := range a.Patches {
			if p.TargetType != "" && p.TargetMethod != "" {
				targets[p.TargetType+"."+p.TargetMethod+p.Args] = true
			}
		}
	}
	out.Stats.HarmonyTargets = len(targets)
	slow := slowest[DLLTime]{n: SlowestDLLsKept}
	for _, t := range times {
		if t.Cached {
			out.Stats.DLLsCached++
		}
		slow.add(t, t.Dur)
	}
	out.Stats.SlowestDLLs = slow.items
	out.Findings = append(out.Findings, harmonyFindings(r.active, out.Assemblies)...)
	out.Findings = append(out.Findings, asmDuplicates(r.active, out.Assemblies)...)
}

// slowest keeps the n items that took longest, slowest first. Adding costs one comparison
// unless the item is among them (the slow-check report's cheap collection, 16 §2.14).
type slowest[T any] struct {
	n     int
	items []T
	durs  []time.Duration
}

func (s *slowest[T]) add(x T, d time.Duration) {
	if len(s.items) == s.n && d <= s.durs[len(s.durs)-1] {
		return
	}
	i := len(s.items)
	if i == s.n {
		i--
	} else {
		var zero T
		s.items, s.durs = append(s.items, zero), append(s.durs, 0)
	}
	for i > 0 && s.durs[i-1] < d {
		s.items[i], s.durs[i] = s.items[i-1], s.durs[i-1]
		i--
	}
	s.items[i], s.durs[i] = x, d
}

// reportXPath is the expression the report names: the one past the step budget, the
// operation's own, or for a sequence or a conditional the first inside it.
func (t *topOp) reportXPath() string {
	if t.slowXPath != "" {
		return t.slowXPath
	}
	return firstXPath(t.op)
}

func firstXPath(o *Op) string {
	if o == nil {
		return ""
	}
	if o.XPath != "" {
		return o.XPath
	}
	for _, c := range o.ops {
		if x := firstXPath(c); x != "" {
			return x
		}
	}
	if x := firstXPath(o.match); x != "" {
		return x
	}
	return firstXPath(o.nomatch)
}

func defFact(d *xdoc.Doc, t int32, file string, line int) DefFact {
	f := DefFact{Type: d.Name(t), File: file, Line: line}
	f.Content = extractDefContent(d, t)
	if c := d.Child(t, "defName"); c != xdoc.None {
		f.DefName = strings.Clone(strings.TrimSpace(d.InnerText(c))) // detach from the doc's text pages
	}
	f.Name, _ = d.Attr(t, "Name")
	f.Parent, _ = d.Attr(t, "ParentName")
	f.MayRequire, _ = d.Attr(t, "MayRequire")
	f.MayRequireAny, _ = d.Attr(t, "MayRequireAnyOf")
	if a, ok := d.Attr(t, "Abstract"); ok && strings.EqualFold(strings.TrimSpace(a), "true") {
		f.Abstract = true
	}
	return f
}

// folderRequires returns the packages a conditional load folder containing path requires.
func folderRequires(req map[string][]string, path string) []string {
	var out []string
	lp := strings.ToLower(path)
	for f, pkgs := range req {
		if strings.HasPrefix(lp, strings.ToLower(f)+string(os.PathSeparator)) {
			out = append(out, pkgs...)
		}
	}
	return out
}

// parseOrdered reads and tokenises files on several goroutines and calls fn for each in
// order. At most 2×workers files are held in memory at once.
func parseOrdered(paths []string, workers int, fn func(i int, evs []xdoc.Event, err error)) {
	type res struct {
		evs []xdoc.Event
		err error
	}
	slots := make([]chan res, len(paths))
	for i := range slots {
		slots[i] = make(chan res, 1)
	}
	sem := make(chan struct{}, workers*2)
	next := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				b, err := os.ReadFile(paths[i])
				var evs []xdoc.Event
				if err == nil {
					evs, err = xdoc.Parse(b)
				}
				slots[i] <- res{evs, err}
			}
		}()
	}
	go func() {
		for i := range paths {
			sem <- struct{}{}
			next <- i
		}
		close(next)
	}()
	for i := range paths {
		r := <-slots[i]
		fn(i, r.evs, r.err)
		<-sem
	}
	wg.Wait()
}
