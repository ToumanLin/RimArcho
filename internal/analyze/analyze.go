// Package analyze builds the Def and Patch layers (03 §2, §5.1, §5.2): Def facts from each
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

	"github.com/rimarcho/rimarcho/internal/content"
	"github.com/rimarcho/rimarcho/internal/scan"
	"github.com/rimarcho/rimarcho/internal/xdoc"
)

// Version of the analyzers; facts written by another version are rebuilt (03 §2).
const Version = 1

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

// DefFact is one top-level Def (03 §2).
type DefFact struct {
	Instance string `json:"-"`
	Type     string `json:"t"`
	DefName  string `json:"d,omitempty"`
	Name     string `json:"n,omitempty"`
	Parent   string `json:"p,omitempty"`
	Abstract bool   `json:"a,omitempty"`
	File     string `json:"f"` // relative to the mod root
	Line     int    `json:"l"`
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
}

// ContentState is the effective content of one instance.
type ContentState struct {
	Key         string
	Folders     []string
	DefFiles    int
	PatchFiles  int
	ParseErrors []string
}

// Output of Run.
type Output struct {
	Content  map[string]*ContentState // by instance
	NewDefs  map[string][]DefFact     // instances whose facts were (re)built
	Patches  []*PatchFact
	Findings []*Finding
	Stats    Stats
}

// Stats summarise a run.
type Stats struct {
	ActiveMods, InstalledMods int
	DefFiles, PatchFiles      int
	Defs, TopOps, UnknownOps  int
	Failed                    int
	ParseErrors               int
	Nodes, TextMB             int
	IndexHits, IndexRebuilds  int
	Evaluations               int
	DefsMs, PatchMs, TotalMs  int64
	CachedMods                int
	PeakHeapMB                int
}

func (in *Input) progress(f string, a ...any) {
	if in.Progress != nil {
		in.Progress(fmt.Sprintf(f, a...))
	}
}

// Run performs the analysis.
func Run(in Input) (*Output, error) {
	t0 := time.Now()
	if in.Workers <= 0 {
		in.Workers = max(2, runtime.NumCPU())
	}
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
	isActive := func(p string) bool { return activePkg[strings.TrimSuffix(strings.ToLower(p), "_steam")] }

	out := &Output{Content: map[string]*ContentState{}, NewDefs: map[string][]DefFact{}}
	out.Stats.ActiveMods, out.Stats.InstalledMods = len(active), len(in.Mods)
	folders := map[string][]string{}
	for _, m := range in.Mods {
		r := content.Resolve(m.Dir, m.Decl.LoadFolders, in.MM, isActive)
		folders[m.Instance] = r.Folders
		out.Content[m.Instance] = &ContentState{Key: r.Key(m.SnapHash, in.MM) + fmt.Sprintf("|a%d", Version), Folders: r.Folders}
	}

	// Def layer: active mods into the merged document, in load order.
	names := xdoc.NewNames()
	doc := xdoc.New(names)
	root := doc.NewElement("Defs")
	doc.AppendChild(doc.Root(), root)
	allDefs := map[string][]DefFact{}
	type fileJob struct {
		mod   *Mod
		asset content.Asset
	}
	var jobs []fileJob
	for _, m := range active {
		for _, a := range content.Assets(m.Dir, folders[m.Instance], "Defs") {
			jobs = append(jobs, fileJob{m, a})
		}
	}
	paths := make([]string, len(jobs))
	for i, j := range jobs {
		paths[i] = j.asset.Path
	}
	done := 0
	parseOrdered(paths, in.Workers, func(i int, evs []xdoc.Event, err error) {
		j := jobs[i]
		cs := out.Content[j.mod.Instance]
		cs.DefFiles++
		if err != nil {
			cs.ParseErrors = append(cs.ParseErrors, j.asset.Rel+": "+err.Error())
			out.Stats.ParseErrors++
			return
		}
		tops, lines := doc.Build(root, evs, true, false)
		for k, t := range tops {
			allDefs[j.mod.Instance] = append(allDefs[j.mod.Instance], defFact(doc, t, j.asset.Rel, int(lines[k])))
		}
		done++
		if done%500 == 0 {
			in.progress("defs: %d/%d files", done, len(jobs))
		}
	})
	out.Stats.DefFiles = len(jobs)
	for _, m := range active {
		out.NewDefs[m.Instance] = allDefs[m.Instance]
		out.Stats.Defs += len(allDefs[m.Instance])
	}
	in.progress("defs: %d files, %d defs from %d active mods", len(jobs), out.Stats.Defs, len(active))

	// Inactive mods: facts only (cached by effective-content key).
	var todo []*Mod
	for _, m := range inactive {
		if in.Cached != nil {
			if fs, ok := in.Cached(m.Instance, out.Content[m.Instance].Key); ok {
				allDefs[m.Instance] = fs
				out.Stats.CachedMods++
				continue
			}
		}
		todo = append(todo, m)
	}
	if len(todo) > 0 {
		var ijobs []fileJob
		for _, m := range todo {
			for _, a := range content.Assets(m.Dir, folders[m.Instance], "Defs") {
				ijobs = append(ijobs, fileJob{m, a})
			}
			out.NewDefs[m.Instance] = []DefFact{} // record the key even with no defs
		}
		ipaths := make([]string, len(ijobs))
		for i, j := range ijobs {
			ipaths[i] = j.asset.Path
		}
		parseOrdered(ipaths, in.Workers, func(i int, evs []xdoc.Event, err error) {
			j := ijobs[i]
			cs := out.Content[j.mod.Instance]
			cs.DefFiles++
			if err != nil {
				cs.ParseErrors = append(cs.ParseErrors, j.asset.Rel+": "+err.Error())
				return
			}
			scratch := xdoc.New(names)
			sroot := scratch.NewElement("Defs")
			tops, lines := scratch.Build(sroot, evs, true, false)
			for k, t := range tops {
				f := defFact(scratch, t, j.asset.Rel, int(lines[k]))
				allDefs[j.mod.Instance] = append(allDefs[j.mod.Instance], f)
			}
		})
		for _, m := range todo {
			out.NewDefs[m.Instance] = allDefs[m.Instance]
		}
		in.progress("defs: %d inactive mods indexed (%d cached)", len(todo), out.Stats.CachedMods)
	}
	out.Stats.DefsMs = time.Since(t0).Milliseconds()

	// Patch layer.
	t1 := time.Now()
	sim := newSim(doc, root, active, in.MM, folders)
	var pjobs []fileJob
	for _, m := range active {
		for _, a := range content.Assets(m.Dir, folders[m.Instance], "Patches") {
			pjobs = append(pjobs, fileJob{m, a})
		}
	}
	ppaths := make([]string, len(pjobs))
	for i, j := range pjobs {
		ppaths[i] = j.asset.Path
	}
	var tops []*topOp
	parseOrdered(ppaths, in.Workers, func(i int, evs []xdoc.Event, err error) {
		j := pjobs[i]
		cs := out.Content[j.mod.Instance]
		cs.PatchFiles++
		if err != nil {
			cs.ParseErrors = append(cs.ParseErrors, j.asset.Rel+": "+err.Error())
			out.Stats.ParseErrors++
			return
		}
		pd := xdoc.New(names)
		pd.Build(pd.Root(), evs, false, true)
		de := pd.DocElement()
		if de == xdoc.None || pd.Name(de) != "Patch" {
			return
		}
		for _, c := range pd.Children(de) {
			if pd.Name(c) == "Operation" {
				tops = append(tops, &topOp{mod: j.mod, file: j.asset.Rel, op: parseOp(pd, c)})
			}
		}
	})
	out.Stats.PatchFiles = len(pjobs)
	in.progress("patches: %d files, %d operations", len(pjobs), len(tops))
	seq := map[string]int{}
	for k, t := range tops {
		seq[t.mod.Instance]++
		t.seq = seq[t.mod.Instance]
		sim.run(t)
		if (k+1)%1000 == 0 {
			in.progress("patches: %d/%d operations", k+1, len(tops))
		}
	}
	out.Stats.TopOps = len(tops)
	out.Stats.IndexHits, out.Stats.IndexRebuilds, out.Stats.Evaluations = sim.ix.hits, sim.ix.rebuilds, sim.evals
	out.Stats.Nodes, out.Stats.TextMB = doc.Len(), doc.TextBytes()>>20
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	out.Stats.PeakHeapMB = int(ms.HeapInuse >> 20)

	// Facts and findings.
	kb := newKnowledge(in.Mods, allDefs)
	out.Patches = sim.facts(tops, kb)
	for _, p := range out.Patches {
		if p.Result == ResultFailed {
			out.Stats.Failed++
		}
		if p.Result == ResultUnknown {
			out.Stats.UnknownOps++
		}
	}
	out.Findings = append(out.Findings, defFindings(active, allDefs)...)
	out.Findings = append(out.Findings, sim.findings(tops, out.Patches)...)
	out.Stats.PatchMs = time.Since(t1).Milliseconds()
	out.Stats.TotalMs = time.Since(t0).Milliseconds()
	in.progress("done: %d failed operations, %d findings, %v", out.Stats.Failed, len(out.Findings), time.Since(t0).Round(time.Millisecond))
	runtime.KeepAlive(doc)
	return out, nil
}

func defFact(d *xdoc.Doc, t int32, file string, line int) DefFact {
	f := DefFact{Type: d.Name(t), File: file, Line: line}
	if c := d.Child(t, "defName"); c != xdoc.None {
		f.DefName = strings.TrimSpace(d.InnerText(c))
	}
	f.Name, _ = d.Attr(t, "Name")
	f.Parent, _ = d.Attr(t, "ParentName")
	if a, ok := d.Attr(t, "Abstract"); ok && strings.EqualFold(strings.TrimSpace(a), "true") {
		f.Abstract = true
	}
	return f
}

// parseOrdered reads and tokenises files on several goroutines and calls fn for each in
// order. At most 4×workers files are held in memory at once.
func parseOrdered(paths []string, workers int, fn func(i int, evs []xdoc.Event, err error)) {
	type res struct {
		evs []xdoc.Event
		err error
	}
	slots := make([]chan res, len(paths))
	for i := range slots {
		slots[i] = make(chan res, 1)
	}
	sem := make(chan struct{}, workers*4)
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
