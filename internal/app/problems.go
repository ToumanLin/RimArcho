package app

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/order"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Problem is a finding as shown to the player: with a stable ref and its ignore state.
type Problem struct {
	*finding.Finding
	Ref           string // p:<hex>
	Ignored       bool
	VersionMuted  bool    // global package + game minor preference; survives updates
	IgnoreExpired bool    // ignored once, but the subject changed since (05 §4.2)
	Log           *LogRun // the log of a log.* problem
}

// ProblemSet is every current problem of the environment.
type ProblemSet struct {
	GameLinks []*GameLink // refreshed independently of analysis and mod caches
	State     *LoadoutState
	List      []*Problem // neither ignored nor version-muted, sorted by severity then subject
	Ignored   []*Problem
	Muted     []*Problem // version warnings muted by the player, never counted
	// Stale holds the problems of stale logs (LogRun.Stale): shown apart, never counted.
	Stale    []*Problem
	Logs     []*LogRun // the logs whose clusters joined the problems
	Analysis *AnalysisState
	// AnalysisErr is set when the Def/Patch layers could not be brought up to date; the
	// declaration layer is still complete.
	AnalysisErr error
}

// ProblemRef is the stable reference of a finding.
func ProblemRef(f *finding.Finding, lineage string) string {
	h := sha1.Sum([]byte(f.Code + "|" + f.Key + "|" + lineage))
	return fmt.Sprintf("p:%x", h[:3])
}

// Problems collects the declaration layer (computed now) and the stored Def/Patch layers.
// With ensure, a stale or missing analysis is rebuilt first. A pack's links left in Mods/
// (10 §3) are looked at every time: they change on disk, outside what the cache knows.
func (a *App) Problems(ensure bool, progress func(string)) (*ProblemSet, error) {
	ps, err := a.problems(ensure, progress)
	if err != nil || ps.State == nil {
		return ps, err
	}
	// What Mods/ holds now (09 §3, 10 §3): a pack's links left there, and two folders of one
	// packageId, which the game reports as an error.
	ix := ps.State.Ix
	dups := ModsDuplicates(a.ModsCopies(ix))
	var extra []*finding.Finding
	ps.GameLinks, err = a.InspectGameLinks()
	if err != nil {
		return nil, err
	}
	for _, l := range ps.GameLinks {
		if l.Code != "" {
			extra = append(extra, l.Finding())
		}
	}
	if l := a.LeftPackLinks(ix); l != nil {
		for _, d := range dups {
			if d.onlyPacks() {
				l.Duplicates = append(l.Duplicates, d)
			}
		}
		extra = append(extra, l.Finding())
	}
	extra = append(extra, a.duplicateFindings(ps.State, dups)...)
	if len(extra) > 0 {
		c := *ps
		c.List = append([]*Problem{}, ps.List...)
		for _, f := range extra {
			lin := ""
			if m := ix.ByID[f.SubjectInstance()]; m != nil {
				lin = m.LineageID
			}
			p := &Problem{Finding: f, Ref: ProblemRef(f, lin)}
			i := 0
			for i < len(c.List) && sevRank(c.List[i].Severity) <= sevRank(f.Severity) {
				i++
			}
			c.List = append(c.List[:i], append([]*Problem{p}, c.List[i:]...)...)
		}
		ps = &c
	}
	return ps, nil
}

func sevRank(s string) int {
	switch s {
	case finding.Error:
		return 0
	case finding.Warning:
		return 1
	}
	return 2
}

func (a *App) problems(ensure bool, progress func(string)) (*ProblemSet, error) {
	ps := &ProblemSet{}
	if ensure {
		ps.Analysis, ps.AnalysisErr = a.EnsureAnalysis(progress)
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	// Cached until the database or ModsConfig.xml changes (the TUI asks often).
	key := ""
	if a.stCache != nil && a.stCache.Ix == st.Ix {
		key = a.stKey + "|" + a.GameVersion + "|" + a.corpusStamp()
		if p := a.ProblemLog(st); p != "" {
			key += "|" + p + "|" + logStamp(p) + fmt.Sprint(a.LogCacheOnly)
		}
	}
	if key != "" && a.prCache != nil && a.prKey == key && ps.AnalysisErr == nil {
		c := *a.prCache
		c.State = st
		return &c, nil
	}
	defer func() {
		if key != "" && ps.AnalysisErr == nil && ps.State != nil {
			a.prCache, a.prKey = ps, key
		}
	}()
	ps.State = st
	if ps.Analysis == nil {
		if ps.Analysis, err = a.Analysis(st); err != nil {
			return nil, err
		}
	}
	if err := a.collectProblems(ps); err != nil {
		return nil, err
	}
	return ps, nil
}

type planProblems struct {
	key string
	ps  *ProblemSet
	// summary is PlanSummary's result for ps, made once per set.
	summary    any
	summarized bool
}

// PlanSummary is f of plan p's problem set (PlanProblems), made once for as long as that
// set is kept: the Launch page summarizes every other plan on each read (16 §2.3), and
// summarizing (Can it run's items) cost more than the cached set itself (18 §2).
func (a *App) PlanSummary(p *store.Plan, f func(*ProblemSet) any) (any, error) {
	ps, err := a.PlanProblems(p)
	if err != nil {
		return nil, err
	}
	c := a.planPS[p.ID]
	if c == nil || c.ps != ps {
		return f(ps), nil
	}
	if !c.summarized {
		c.summary, c.summarized = f(ps), true
	}
	return c.summary, nil
}

// PlanSummaryKnown is PlanSummary's result when it is already made for p's present
// problem set; known is false when making it would collect the set first. The Launch
// page's first read shows only these (16 §2.3).
func (a *App) PlanSummaryKnown(p *store.Plan) (summary any, known bool, err error) {
	_, st, key, err := a.planKey(p)
	if err != nil {
		return nil, false, err
	}
	if c := a.planPS[st.Plan.ID]; key != "" && c != nil && c.key == key && c.summarized {
		return c.summary, true, nil
	}
	return nil, false, nil
}

// planKey is plan p's state, the current state, and the key p's problem set is kept
// under ("" when it cannot be kept).
func (a *App) planKey(p *store.Plan) (cur, st *LoadoutState, key string, err error) {
	if cur, err = a.Loadout(); err != nil {
		return nil, nil, "", err
	}
	if st, err = a.PlanState(cur, p); err != nil {
		return nil, nil, "", err
	}
	if a.stCache != nil && a.stCache.Ix == cur.Ix {
		key = a.stKey + "|" + a.GameVersion + "|" + ListFingerprint(st.List)
		if lp := a.ProblemLog(st); lp != "" {
			key += "|" + lp + "|" + logStamp(lp)
		}
	}
	return cur, st, key, nil
}

// PlanProblems is plan p's problem set from stored results only, never computing an
// analysis or parsing a log: the declaration layer (computed, cheap), the stored run of
// p's analysis slot (05 §3) and the log's stored report judged against p (06 §4). The
// home page counts every plan's problems with it (16 §2.3). The result is kept until the
// database, ModsConfig.xml or the log changes.
func (a *App) PlanProblems(p *store.Plan) (*ProblemSet, error) {
	cur, st, key, err := a.planKey(p)
	if err != nil {
		return nil, err
	}
	if c := a.planPS[st.Plan.ID]; key != "" && c != nil && c.key == key {
		return c.ps, nil
	}
	ps := &ProblemSet{State: st}
	if ps.Analysis, err = a.analysisOf(st, st == cur); err != nil {
		return nil, err
	}
	only := a.LogCacheOnly
	a.LogCacheOnly = true
	err = a.collectProblems(ps)
	a.LogCacheOnly = only
	if err != nil {
		return nil, err
	}
	if key != "" {
		if a.planPS == nil {
			a.planPS = map[int64]*planProblems{}
		}
		a.planPS[st.Plan.ID] = &planProblems{key: key, ps: ps}
	}
	return ps, nil
}

// collectProblems fills ps for ps.State with ps.Analysis set: the declaration layer, the
// stored Def/Patch findings, the problem log's clusters, and the ignores.
func (a *App) collectProblems(ps *ProblemSet) error {
	st := ps.State
	all, err := a.DeclFindings(st)
	if err != nil {
		return err
	}
	if r := ps.Analysis.Run; r != nil {
		fs, err := a.Store.Findings(r.ID)
		if err != nil {
			return err
		}
		for _, f := range fs {
			// A stale run may name instances that are gone.
			if id := f.SubjectInstance(); id != "" && st.Ix.ByID[id] == nil {
				continue
			}
			all = append(all, f)
		}
		all = a.quickPatchTargets(st, ps.Analysis, all) // a stale run: follow the list (05 §4.4)
	}
	lfs, hot, run := a.logFindings(st)
	if run != nil {
		ps.Logs = append(ps.Logs, run)
		if run.Stale() {
			hot = nil // an old log does not raise current findings
		}
	}
	all = escalateSkips(all, hot)
	fromLog := map[*finding.Finding]bool{}
	for _, f := range lfs {
		fromLog[f] = true
	}
	all = append(all, lfs...)
	return a.finishProblems(ps, all, fromLog, run)
}

func (a *App) finishProblems(ps *ProblemSet, all []*finding.Finding, fromLog map[*finding.Finding]bool, run *LogRun) error {
	st := ps.State
	mutes, err := a.Store.VersionMutes(st.GameMM())
	if err != nil {
		return err
	}
	ignores, err := a.Store.Ignores(st.Ix.Env.ID)
	if err != nil {
		return err
	}
	type igKey struct{ lin, code, key string }
	ig := map[igKey]*store.Ignore{}
	for _, g := range ignores {
		ig[igKey{g.LineageID, g.Code, g.Key}] = g
	}
	for _, f := range all {
		p := &Problem{Finding: f}
		if fromLog[f] {
			p.Log = run
		}
		lin, snap := "", int64(0)
		if m := st.Ix.ByID[f.SubjectInstance()]; m != nil {
			lin, snap = m.LineageID, m.SnapID
		}
		p.Ref = ProblemRef(f, lin)
		if f.Code == "version.unsupported" {
			if m := st.Ix.ByID[f.SubjectInstance()]; m != nil {
				p.VersionMuted = mutes[store.VersionMuteKey(m.PackageID)]
			}
		}
		if g := ig[igKey{lin, f.Code, f.Key}]; g != nil {
			if g.SnapshotID == snap {
				p.Ignored = true
			} else {
				p.IgnoreExpired = true
			}
		}
		switch {
		case p.VersionMuted:
			ps.Muted = append(ps.Muted, p)
		case p.Ignored:
			ps.Ignored = append(ps.Ignored, p)
		case p.Log != nil && p.Log.Stale():
			ps.Stale = append(ps.Stale, p)
		default:
			ps.List = append(ps.List, p)
		}
	}
	positions := make(map[string]int, len(st.List))
	for i, e := range st.List {
		if _, exists := positions[e.Instance]; !exists {
			positions[e.Instance] = i + 1
		}
	}
	orders := make(map[string]string)
	order := func(f *finding.Finding) string {
		if m := st.Ix.ByID[f.SubjectInstance()]; m != nil {
			if key, ok := orders[m.ID]; ok {
				return key
			}
			key := fmt.Sprintf("%06d %s", positions[m.ID], strings.ToLower(m.DisplayName()))
			orders[m.ID] = key
			return key
		}
		return "~" + f.Key
	}
	sortProblems(ps.List, order)
	sortProblems(ps.Ignored, order)
	sortProblems(ps.Muted, order)
	sortProblems(ps.Stale, order)
	return nil
}

func sortProblems(ps []*Problem, order func(*finding.Finding) string) {
	fs := make([]*finding.Finding, len(ps))
	by := map[*finding.Finding]*Problem{}
	for i, p := range ps {
		fs[i] = p.Finding
		by[p.Finding] = p
	}
	finding.Sort(fs, order)
	for i, f := range fs {
		ps[i] = by[f]
	}
}

// All is every problem: current, ignored, version-muted and from stale logs.
func (ps *ProblemSet) All() []*Problem {
	return append(append(append(append([]*Problem{}, ps.List...), ps.Ignored...), ps.Stale...), ps.Muted...)
}

// IgnoreProblem records an ignore on the subject's lineage at its current snapshot.
func (a *App) IgnoreProblem(ref string, undo bool) (*Problem, error) {
	ps, err := a.Problems(false, nil)
	if err != nil {
		return nil, err
	}
	for _, p := range ps.All() {
		if p.Ref != ref {
			continue
		}
		lin, snap := "", int64(0)
		if m := ps.State.Ix.ByID[p.SubjectInstance()]; m != nil {
			lin, snap = m.LineageID, m.SnapID
		}
		if undo {
			_, err := a.Store.RemoveIgnore(ps.State.Ix.Env.ID, lin, p.Code, p.Key)
			p.Ignored = false
			return p, err
		}
		err := a.Store.SetIgnore(ps.State.Ix.Env.ID, &store.Ignore{LineageID: lin, Code: p.Code, Key: p.Key, SnapshotID: snap, CreatedAt: a.Now()})
		p.Ignored = true
		return p, err
	}
	return nil, &RefError{Ref: ref, Hint: "archo problems lists the current problems with their p: refs"}
}

// DeclFindings computes the declaration layer (05 §4.3) for the workspace.
func (a *App) DeclFindings(st *LoadoutState) ([]*finding.Finding, error) {
	d := &declRun{a: a, st: st, mm: st.GameMM(), activePos: map[string]int{}, activeByPkg: map[string][]*store.Mod{}}
	d.listFindings()
	if err := a.Store.LoadDecls(d.active); err != nil {
		return nil, err
	}
	d.locked = st.Locked()
	d.lockedCopy = map[string]string{} // copy ID → upstream ID
	for _, l := range d.locked {
		d.lockedCopy[l.Copy.ID] = l.Upstream.ID
	}
	d.deps = a.newDependencyIndex(st, d.activeByPkg)
	d.installed = d.deps.installed

	for _, m := range d.active {
		d.modFindings(m)
	}
	// What the log corpus observed (undeclared prerequisites, known-bad combinations).
	a.knownFindings(st.Ix.Known(), d.active, d.activeByPkg, d.installed, d.add)
	d.duplicateFindings()
	d.hiddenFindings()
	d.lockedFindings()
	d.installedFindings()
	if err := d.orderFindings(); err != nil {
		return nil, err
	}
	return d.out, nil
}

// declRun is one computation of the declaration layer: the enabled Mods and the findings so far.
type declRun struct {
	a           *App
	st          *LoadoutState
	mm          string
	out         []*finding.Finding
	activePos   map[string]int // enabled copy → 1-based position
	activeByPkg map[string][]*store.Mod
	active      []*store.Mod
	locked      []*Locked
	lockedCopy  map[string]string // copy ID → upstream ID
	deps        *dependencyIndex
	installed   map[string][]*store.Mod
	// the sort of the list, for the order findings
	in order.Input
	r  order.Result
}

// add records a declaration finding, certain unless it says otherwise.
func (d *declRun) add(f *finding.Finding) {
	f.Layer = "decl"
	if f.Confidence == "" {
		f.Confidence = finding.Certain
	}
	d.out = append(d.out, f)
}

// listFindings reports the list's entries that match no installed Mod and collects the
// enabled Mods.
func (d *declRun) listFindings() {
	st := d.st
	missingInfo := map[string]SortMod{}
	for _, m := range d.a.MissingMods(st) {
		missingInfo[m.PackageID] = m.SortMod
	}
	for i, e := range st.List {
		if e.Instance == "" {
			d.add(&finding.Finding{Code: "entry.missing", Severity: finding.Warning, Key: e.Missing,
				Subjects:    []string{"pkg:" + e.Missing},
				Claim:       fmt.Sprintf("pkg:%s in the Loadout matches no installed mod (kept in ModsConfig.xml)", e.Missing),
				Suggestions: []string{"archo find " + e.Missing, "archo disable pkg:" + e.Missing}})
			if w := missingInfo[dependencyPackage(e.Missing)]; w.WorkshopID != "" {
				f := d.out[len(d.out)-1]
				f.Suggestions = append(f.Suggestions, "subscribe: https://steamcommunity.com/sharedfiles/filedetails/?id="+w.WorkshopID)
			}
			continue
		}
		m := st.Ix.ByID[e.Instance]
		if m == nil {
			continue
		}
		if _, dup := d.activePos[m.ID]; !dup {
			d.activePos[m.ID] = i + 1
			d.active = append(d.active, m)
			p := strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")
			d.activeByPkg[p] = append(d.activeByPkg[p], m)
		}
	}
	d.coreFinding()
}

// CodeCoreMissing is the list without Core (05 §4.3): the game resets ModsConfig.xml when it
// starts and loads Core alone (VM 2026-11-23, "Resetting mods config").
const CodeCoreMissing = "official.core-missing"

// coreFinding reports a list with no Core entry. A Core entry that is not installed is
// entry.missing instead. Sorting adds the installed Core (08 §1).
func (d *declRun) coreFinding() {
	if len(d.activeByPkg["ludeon.rimworld"]) > 0 {
		return
	}
	for _, e := range d.st.List {
		if e.Instance == "" && dependencyPackage(e.Missing) == "ludeon.rimworld" {
			return
		}
	}
	const why = " is not in the list: the game resets its mod list when it starts and loads Core alone"
	f := &finding.Finding{Code: CodeCoreMissing, Severity: finding.Error, Key: "ludeon.rimworld", Subjects: []string{"pkg:ludeon.rimworld"},
		Claim: "Core (pkg:ludeon.rimworld)" + why + "; Core is not installed either", Suggestions: []string{"archo sort"}}
	if core := d.st.Reg.Resolve("ludeon.rimworld").Instance; core != "" && d.st.Reg.Loadable(core) {
		f.Subjects = []string{finding.Inst(core)}
		f.Claim = finding.Inst(core) + why
		f.Suggestions = append(f.Suggestions, "archo enable "+finding.Inst(core)+" --at 1")
	}
	d.add(f)
}

// modFindings reports what an enabled Mod declares: a replacement, unloadability, missing
// dependencies, incompatibilities and unsupported versions.
func (d *declRun) modFindings(m *store.Mod) {
	st, mm, inst := d.st, d.mm, finding.Inst
	// Use This Instead (08 §2.3): the community knows a maintained replacement for this game version.
	if r := d.a.replacementFor(st, m); r != nil {
		f := &finding.Finding{Code: "mod.replaced", Severity: finding.Warning, Key: r.NewWorkshopID, Subjects: []string{inst(m.ID)},
			Claim:    fmt.Sprintf("%s has a replacement for %s: %s by %s", inst(m.ID), mm, r.NewName, r.NewAuthor),
			Evidence: []finding.Evidence{{Source: finding.Inferred, Ref: "replacements.json", Text: fmt.Sprintf("Use This Instead: ws:%s → ws:%s", r.OldWorkshopID, r.NewWorkshopID)}}}
		if c := replacementCandidate(st, m, r); c != nil && swapBlocker(st, c, m) == "" {
			f.Subjects = append(f.Subjects, inst(c.ID))
			f.Suggestions = []string{"archo disable " + inst(m.ID), "archo enable " + inst(c.ID)}
		} else if r.NewWorkshopID != "" {
			f.Suggestions = []string{"subscribe: https://steamcommunity.com/sharedfiles/filedetails/?id=" + r.NewWorkshopID}
		}
		d.add(f)
	}
	if why := whyNotLoadable(st, m); why != "" {
		d.add(&finding.Finding{Code: "mod.unloadable", Severity: finding.Error, Key: "active", Subjects: []string{inst(m.ID)},
			Claim:       fmt.Sprintf("%s is enabled but the game will not load it: %s", inst(m.ID), why),
			Suggestions: []string{"archo disable " + inst(m.ID)}})
	}
	d.depFindings(m)
	d.incompatFindings(m)
	d.versionFinding(m)
}

// depFindings reports m's declared dependencies that are not enabled: installed but
// disabled, or not installed.
func (d *declRun) depFindings(m *store.Mod) {
	inst := finding.Inst
	for _, dep := range d.deps.missing(m) {
		p := dep.pkg
		ev := finding.Evidence{Source: finding.Declared, Ref: inst(m.ID), Text: "About.xml modDependencies: " + p}
		if cands := d.deps.candidates(p); len(cands) > 0 {
			f := &finding.Finding{Code: "dep.inactive", Severity: finding.Error, Key: p, Subjects: []string{inst(m.ID), inst(cands[0].ID)},
				Claim:    fmt.Sprintf("%s requires pkg:%s; %s is installed but not enabled", inst(m.ID), p, inst(cands[0].ID)),
				Evidence: []finding.Evidence{ev}}
			for _, c := range cands {
				f.Suggestions = append(f.Suggestions, "archo enable "+inst(c.ID))
			}
			d.add(f)
			continue
		}
		name := dep.name
		if name == "" {
			name = p
		}
		f := &finding.Finding{Code: "dep.missing", Severity: finding.Error, Key: p, Subjects: []string{inst(m.ID), "pkg:" + p},
			Claim: fmt.Sprintf("%s requires %s (pkg:%s), which is not installed", inst(m.ID), name, p), Evidence: []finding.Evidence{ev}}
		if p == "ludeon.rimworld" || strings.HasPrefix(p, "ludeon.rimworld.") {
			f.Claim = fmt.Sprintf("%s requires the %s DLC (pkg:%s), which is not installed", inst(m.ID), name, p)
		} else {
			ws, _ := d.a.dependencyWorkshop(dep)
			if dep.url != "" {
				f.Suggestions = append(f.Suggestions, "subscribe: "+dep.url)
			} else if ws != "" {
				f.Suggestions = append(f.Suggestions, "subscribe: https://steamcommunity.com/sharedfiles/filedetails/?id="+ws)
			}
			if ws != "" {
				f.Suggestions = append(f.Suggestions, "steam://url/CommunityFilePage/"+ws)
			}
		}
		d.add(f)
	}
}

// incompatFindings reports the enabled Mods m declares itself incompatible with, once per pair.
func (d *declRun) incompatFindings(m *store.Mod) {
	inst, decl := finding.Inst, m.Decl()
	inc := append(append([]string{}, decl.IncompatibleWith...), decl.IncompatibleByVersion["v"+d.mm]...)
	for _, x := range inc {
		p := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(x)), "_steam")
		for _, o := range d.activeByPkg[p] {
			if o.ID == m.ID {
				continue
			}
			a1, b1 := m, o
			if d.activePos[b1.ID] < d.activePos[a1.ID] {
				a1, b1 = b1, a1
			}
			key := a1.LineageID + "|" + b1.LineageID
			dup := false
			for _, f := range d.out {
				if f.Code == "incompat.active" && f.Key == key {
					f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Declared, Ref: inst(m.ID), Text: "About.xml incompatibleWith: " + p})
					dup = true
				}
			}
			if dup {
				continue
			}
			d.add(&finding.Finding{Code: "incompat.active", Severity: finding.Error, Key: key, Subjects: []string{inst(m.ID), inst(o.ID)},
				Claim:       fmt.Sprintf("%s is incompatible with %s, and both are enabled", inst(m.ID), inst(o.ID)),
				Evidence:    []finding.Evidence{{Source: finding.Declared, Ref: inst(m.ID), Text: "About.xml incompatibleWith: " + p}},
				Suggestions: []string{"archo disable " + inst(o.ID), "archo disable " + inst(m.ID), "archo why " + inst(m.ID) + " " + inst(o.ID)}})
		}
	}
}

// versionFinding reports a Mod whose supportedVersions leave out the game's version.
func (d *declRun) versionFinding(m *store.Mod) {
	inst, decl, mm := finding.Inst, m.Decl(), d.mm
	if m.Kind == "official" || len(decl.SupportedVersions) == 0 || mm == "" {
		return
	}
	for _, v := range decl.SupportedVersions {
		if strings.TrimSpace(v) == mm {
			return
		}
	}
	// Measured on 6,734 logs of 1.6 (decisions-corpus §2, G8; 05 §4.5): a mod that
	// stops at the previous minor version is an error source 2.6× as often as chance
	// (its code was built for the previous game): warning. A mod for older versions
	// that players still run is 0.3× chance (the ones that broke were dropped long
	// ago): info. The fold into one row keeps the count quiet.
	sev := finding.Info
	if prev := PrevMinor(mm); prev != "" {
		for _, v := range decl.SupportedVersions {
			if strings.TrimSpace(v) == prev {
				sev = finding.Warning
			}
		}
	}
	d.add(&finding.Finding{Code: "version.unsupported", Severity: sev, Key: mm, Subjects: []string{inst(m.ID)},
		Claim:    fmt.Sprintf("%s does not list %s in supportedVersions (%s)", inst(m.ID), mm, strings.Join(decl.SupportedVersions, " ")),
		Evidence: []finding.Evidence{{Source: finding.Declared, Ref: inst(m.ID), Text: "About.xml supportedVersions"}}})
}

// duplicateFindings reports packages with more than one enabled copy, or more than one
// installed copy.
func (d *declRun) duplicateFindings() {
	var pkgs []string
	for p := range d.installed {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	for _, p := range pkgs {
		if as := d.activeByPkg[p]; len(as) > 1 {
			d.duplicateActive(p, as)
		} else if ins := d.installed[p]; p != MonitorPackage && len(ins) > 1 && !onlyLocks(ins, d.lockedCopy) {
			d.duplicateInstalled(p, as, ins)
		}
	}
}

// duplicateActive reports the enabled copies as of package p.
func (d *declRun) duplicateActive(p string, as []*store.Mod) {
	inst := finding.Inst
	f := &finding.Finding{Code: "pkg.duplicate-active", Severity: finding.Error, Key: p,
		Claim: fmt.Sprintf("%d copies of pkg:%s are enabled; the game loads only one", len(as), p)}
	for _, m := range as {
		f.Subjects = append(f.Subjects, inst(m.ID))
		f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Inferred, Ref: inst(m.ID), Text: fmt.Sprintf("enabled at %d (src:%s)", d.activePos[m.ID], m.SourceLabel)})
	}
	for _, m := range as[1:] {
		f.Suggestions = append(f.Suggestions, "archo disable "+inst(m.ID))
	}
	d.add(f)
}

// duplicateInstalled reports the installed copies ins of package p, of which as (at most
// one) are enabled.
func (d *declRun) duplicateInstalled(p string, as, ins []*store.Mod) {
	st, inst := d.st, finding.Inst
	subj := ins[0]
	if len(as) == 1 {
		subj = as[0]
	}
	f := &finding.Finding{Code: "pkg.duplicate-installed", Severity: finding.Info, Key: p, Subjects: []string{inst(subj.ID)},
		Claim: fmt.Sprintf("%d copies of pkg:%s are installed", len(ins), p)}
	anyLoadable := false
	for _, m := range ins {
		if st.Reg.Loadable(m.ID) {
			anyLoadable = true
		}
	}
	for _, m := range ins {
		if m != subj {
			f.Subjects = append(f.Subjects, inst(m.ID))
		}
		note := "installed"
		if d.activePos[m.ID] > 0 {
			note = fmt.Sprintf("enabled at %d", d.activePos[m.ID])
		} else if !st.Reg.Loadable(m.ID) && anyLoadable {
			note = "ignored by the game (another copy wins)"
		}
		f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Inferred, Ref: inst(m.ID), Text: fmt.Sprintf("src:%s %s", m.SourceLabel, note)})
	}
	// swap needs an enabled copy (host round 2026-11-05: `archo swap` on a copy that is not
	// in the list fails with "not enabled").
	if len(as) == 1 {
		f.Suggestions = []string{"archo swap " + inst(subj.ID)}
	}
	d.add(f)
}

// hiddenFindings reports About.xml renamed away (about-disabled) on installed mods that are
// not enabled: pinned by a copy, unsubscribed, or hidden on their own (09 §5).
func (d *declRun) hiddenFindings() {
	inst := finding.Inst
	for _, h := range d.st.Hidden() {
		m := h.Mod
		if d.activePos[m.ID] != 0 {
			continue
		}
		switch h.Kind {
		case HiddenPinned:
			f := &finding.Finding{Code: "mod.pinned", Severity: finding.Info, Key: "pinned", Subjects: []string{inst(m.ID), inst(h.Copy.ID)},
				Claim:       fmt.Sprintf("%s is pinned: the game uses %s, this workshop copy is hidden (About.xml renamed)", inst(m.ID), inst(h.Copy.ID)),
				Suggestions: []string{"archo unpin " + inst(m.ID)}}
			if h.Upstream {
				f.Severity, f.Key = finding.Warning, "upstream"
				f.Claim = fmt.Sprintf("%s is pinned to %s, and the workshop copy has been updated since", inst(m.ID), inst(h.Copy.ID))
				f.Suggestions = []string{"archo mod " + inst(h.Copy.ID) + " --diff " + inst(m.ID), "archo unpin " + inst(m.ID)}
			}
			d.add(f)
		case HiddenResidue:
			d.add(&finding.Finding{Code: "mod.hidden-residue", Severity: finding.Warning, Key: "residue", Subjects: []string{inst(m.ID)},
				Claim:       fmt.Sprintf("%s is hidden (About.xml renamed) and no longer subscribed; its folder is left over", inst(m.ID)),
				Suggestions: []string{"archo trash " + inst(m.ID)}})
		default:
			d.add(&finding.Finding{Code: "mod.hidden", Severity: finding.Warning, Key: "hidden", Subjects: []string{inst(m.ID)},
				Claim:       fmt.Sprintf("%s is hidden from the game (About.xml renamed, probably by another mod manager)", inst(m.ID)),
				Suggestions: []string{"archo unhide " + inst(m.ID), "archo trash " + inst(m.ID)}})
		}
	}
}

// lockedFindings reports the locked versions RimArcho made (07 §5): the upstream stays
// visible; the copy is what the plan uses. Reported while the upstream itself is not enabled.
func (d *declRun) lockedFindings() {
	inst := finding.Inst
	for _, l := range d.locked {
		up, cp := l.Upstream, l.Copy
		if d.activePos[up.ID] != 0 {
			continue
		}
		f := &finding.Finding{Code: "mod.pinned", Severity: finding.Info, Key: "copy:" + cp.ID, Subjects: []string{inst(up.ID), inst(cp.ID)},
			Claim:       fmt.Sprintf("%s is locked: %s is a copy of it made by archo pin (.rimarcho/origin.json)", inst(up.ID), inst(cp.ID)),
			Evidence:    []finding.Evidence{{Source: finding.Inferred, Ref: inst(cp.ID), Text: "origin_snapshot " + shortHash(l.Origin.SnapshotHash)}},
			Suggestions: []string{"archo unpin " + inst(cp.ID)}}
		if l.Updated {
			f.Severity, f.Key = finding.Warning, "upstream:"+cp.ID
			f.Claim = fmt.Sprintf("%s is locked to %s, and %s has been updated since the copy was made", inst(up.ID), inst(cp.ID), inst(up.ID))
			f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Inferred, Ref: inst(up.ID), Text: "current snapshot " + shortHash(hex.EncodeToString(up.SnapHash))})
			f.Suggestions = []string{"archo mod " + inst(cp.ID) + " --diff " + inst(up.ID), "archo pin --update " + inst(cp.ID), "archo unpin " + inst(cp.ID)}
		}
		d.add(f)
	}
}

// installedFindings reports stale converted textures and installed Mods that cannot load.
func (d *declRun) installedFindings() {
	inst := finding.Inst
	// Texture optimisation: a mod updated since RimArcho wrote its DDS may show its old textures (a DDS
	// shadows the PNG of the same name).
	for _, m := range d.a.StaleTextures(d.st.Ix) {
		d.add(&finding.Finding{Code: "texture.stale", Severity: finding.Warning, Key: "textures", Subjects: []string{inst(m.ID)},
			Claim:       fmt.Sprintf("%s changed since its textures were converted to DDS; the game may show its old textures", inst(m.ID)),
			Suggestions: []string{"archo textures optimize " + inst(m.ID), "archo textures clean " + inst(m.ID)}})
	}
	for _, m := range d.st.Ix.Mods {
		if m.TombstoneAt.IsZero() && m.Unloadable != "" && m.Unloadable != scan.UnloadableAboutDisabled && d.activePos[m.ID] == 0 {
			d.add(&finding.Finding{Code: "mod.unloadable", Severity: finding.Info, Key: m.Unloadable, Subjects: []string{inst(m.ID)},
				Claim: fmt.Sprintf("%s is installed but cannot be loaded: %s", inst(m.ID), m.Unloadable)})
		}
	}
}

// nodeTok is how a finding names node i of the sort: its copy, or its package.
func (d *declRun) nodeTok(i int) string {
	if nd := d.in.Nodes[i]; nd.Instance != "" {
		return finding.Inst(nd.Instance)
	}
	return "pkg:" + d.in.Nodes[i].PackageID
}

// orderFindings reports the violations and cycles of the sort constraints.
func (d *declRun) orderFindings() error {
	in, err := d.a.SortInput(d.st, d.st.List)
	if err != nil {
		return err
	}
	d.in, d.r = in, order.Sort(in)
	d.violationFindings()
	d.cycleFindings()
	return nil
}

// violationFindings reports the broken constraints: one order.violated finding each, and
// official content out of place (mostly a DLC enabled later, appended to the end of the
// list) as one official.order finding with the cheapest moves. Constraints from an official
// package to a mod are part of it rather than findings of their own.
func (d *declRun) violationFindings() {
	st, in, r := d.st, d.in, d.r
	inCycle := map[int]bool{}
	for _, cy := range r.Cycles {
		for _, m := range cy.Members {
			inCycle[m] = true
		}
	}
	officialBroken := false
	for _, e := range r.Edges {
		if e.C.Kind == order.KindOfficial && e.From > e.To {
			officialBroken = true
		}
	}
	var official []finding.Evidence
	officialSubject := ""
	for _, e := range r.Edges {
		if e.From < e.To || inCycle[e.From] && inCycle[e.To] {
			continue
		}
		ev, ig := edgeText(st, in, e)
		if e.C.Kind == order.KindOfficial {
			if officialSubject == "" {
				officialSubject = d.nodeTok(e.To)
			}
			official = append(official, finding.Evidence{Source: finding.Inferred, Text: ev})
			continue
		}
		as := order.Assess(in, e)
		if officialBroken && order.IsOfficial(in.Nodes[e.From].PackageID) && !order.IsOfficial(in.Nodes[e.To].PackageID) {
			if as.Strength == order.Strong {
				official = append(official, finding.Evidence{Source: finding.Declared, Text: ev + " (needs it loaded first)"})
			}
			continue
		}
		d.add(d.violation(e, as, ev, ig))
	}
	if len(official) > 0 {
		d.officialFinding(official, officialSubject)
	}
}

// violation is the order.violated finding of edge e, assessed as as; ev and ig are its
// evidence text and how to ignore it.
func (d *declRun) violation(e order.Edge, as order.Assessment, ev, ig string) *finding.Finding {
	in := d.in
	src := finding.Declared
	switch e.C.Kind {
	case order.KindCommunity:
		src = finding.Community
	case order.KindUser:
		src = finding.User
	}
	// Severity by constraint strength (08 §1.3): dependencies, loadBefore, forced orders
	// and user rules are warnings; loadAfter and community preferences, and rules the
	// log corpus saw broken without effect, are information.
	sev := finding.Warning
	claim := fmt.Sprintf("%s should load after %s but is at %d, before %d", d.nodeTok(e.To), d.nodeTok(e.From), e.To+1, e.From+1)
	switch as.Strength {
	case order.Weak:
		sev = finding.Info
	case order.Harmless:
		sev = finding.Info
		claim += " (declared, but breaking it is not known to cause errors)"
	}
	key := in.Nodes[e.From].PackageID + ">" + in.Nodes[e.To].PackageID
	f := &finding.Finding{Code: "order.violated", Severity: sev, Key: key, Subjects: []string{d.nodeTok(e.To), d.nodeTok(e.From)},
		Claim:       claim,
		Evidence:    []finding.Evidence{{Source: src, Text: ev}},
		Suggestions: []string{"archo sort --dry-run", "archo move " + d.nodeTok(e.To) + " --after " + d.nodeTok(e.From)}}
	if as.Why != "" {
		f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Inferred, Text: as.Why})
	}
	f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Inferred, Text: StrengthPrefix + as.Strength.String()})
	if !strings.HasPrefix(ig, "archo ") {
		f.Suggestions = append(f.Suggestions, "archo rule add ignore "+ig)
		if as.Strength == order.Harmless {
			f.Suggestions = []string{"archo rule add ignore " + ig}
		}
	}
	return f
}

// officialFinding reports Core and the DLCs out of place, with the cheapest moves.
func (d *declRun) officialFinding(official []finding.Evidence, officialSubject string) {
	in := d.in
	// Warning, not error (G8): a mod ahead of a DLC is an error source about twice as often
	// as chance (G4 2.1×, G2 pairs 1.8×), like a broken dependency order, not like a
	// missing dependency.
	f := &finding.Finding{Code: "official.order", Severity: finding.Warning, Key: "official", Subjects: []string{officialSubject},
		Claim:    "Core and the DLCs are not first in the list, in release order",
		Evidence: official, Suggestions: []string{"archo sort --dry-run"}}
	if moves := order.OfficialMoves(in, d.r.Edges, 3); len(moves) > 0 {
		var cmds []string
		dlc := true
		for _, m := range moves {
			cmds = append(cmds, "archo move "+d.nodeTok(m.Node)+" --after "+d.nodeTok(m.After))
			dlc = dlc && order.IsOfficial(in.Nodes[m.Node].PackageID)
		}
		f.Subjects = []string{d.nodeTok(moves[0].Node)}
		if dlc {
			f.Claim = fmt.Sprintf("%s loads after mods (a DLC enabled later goes to the end of the list); move it back after %s",
				d.nodeTok(moves[0].Node), d.nodeTok(moves[0].After))
			if len(moves) > 1 {
				f.Claim = fmt.Sprintf("%d DLCs load after mods (a DLC enabled later goes to the end of the list); move them back to the official position", len(moves))
			}
		}
		f.Suggestions = append(cmds, f.Suggestions...)
	}
	d.add(f)
}

// cycleFindings reports the constraint cycles and the rules sorting set aside to keep the
// official order.
func (d *declRun) cycleFindings() {
	st, in, r := d.st, d.in, d.r
	for _, cy := range r.Cycles {
		var refs []string
		for _, mi := range cy.Members {
			refs = append(refs, d.nodeTok(mi))
		}
		// Info (G8): mods on a cycle were error sources no more often than chance (6.8% vs
		// 6.1% in 804 logs). Sorting gives up the weakest constraints (08 §1.2).
		f := &finding.Finding{Code: "order.cycle", Severity: finding.Info, Key: strings.Join(refs, " "), Subjects: refs,
			Claim: fmt.Sprintf("load-order constraints form a cycle among %s (sorting gives up the weakest)", strings.Join(refs, " "))}
		for i, e := range cy.Edges {
			ev, ig := edgeText(st, in, e)
			t := ev
			if slices.Contains(cy.Broken, i) {
				t += "  [not used by sorting]"
			}
			if i == cy.Suggest {
				t += "  [suggested to ignore]"
				f.Suggestions = append([]string{"archo rule add ignore " + ig}, f.Suggestions...)
			}
			f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Declared, Text: t})
		}
		d.add(f)
	}
	for _, dr := range r.Dropped {
		if dr.Reason != order.ReasonOfficialFirst {
			continue
		}
		ev, _ := edgeText(st, in, dr.Edge)
		var refs []string
		for _, i := range dr.Members {
			refs = append(refs, d.nodeTok(i))
		}
		d.add(&finding.Finding{Code: "order.cycle", Severity: finding.Info, Key: "official:" + ev,
			Subjects: refs,
			Claim:    "sorting keeps the official order and sets aside a contradictory rule",
			Evidence: []finding.Evidence{{Source: finding.Declared, Text: ev + "  [not used by sorting]"}}})
	}
}

// scanDep is one declared dependency; alts are its alternativePackageIds.
type scanDep struct {
	pkg, name, url string
	alts           []string
}

func toDeps(l []scan.Dependency) []scanDep {
	out := make([]scanDep, 0, len(l))
	for _, d := range l {
		out = append(out, scanDep{pkg: d.PackageID, name: d.DisplayName, url: d.SteamWorkshopURL, alts: d.Alternatives})
	}
	return out
}

// workshopID extracts the item ID from a Steam workshop URL.
func workshopID(u string) string {
	u = strings.TrimSpace(u)
	id := ""
	if rest, ok := strings.CutPrefix(u, "steam://url/CommunityFilePage/"); ok {
		id = rest
	} else if i := strings.Index(u, "id="); i >= 0 {
		id = u[i+3:]
	}
	for j, c := range id {
		if c < '0' || c > '9' {
			id = id[:j]
			break
		}
	}
	return id
}

// edgeText describes a sort constraint with instance tokens, and returns its ignore ref
// (or the command that removes a user rule).
func edgeText(st *LoadoutState, in order.Input, e order.Edge) (evidence, ignore string) {
	tok := func(i int) string {
		if i < 0 || i >= len(in.Nodes) {
			return "?"
		}
		if nd := in.Nodes[i]; nd.Instance != "" {
			return finding.Inst(nd.Instance)
		}
		return "pkg:" + in.Nodes[i].PackageID
	}
	owner := tok(e.C.Owner)
	switch e.C.Kind {
	case order.KindOfficial:
		return fmt.Sprintf("builtin %s official content first (pkg:%s)", owner, e.C.Target), "official:" + owner
	case order.KindCommunity:
		return fmt.Sprintf("community pkg:%s %s pkg:%s", in.Nodes[e.C.Owner].PackageID, e.C.Field, e.C.Target), e.C.Ref
	case order.KindUser:
		return fmt.Sprintf("rule r:%d %s %s pkg:%s", e.C.RuleID, owner, e.C.Field, e.C.Target), fmt.Sprintf("archo rule remove r:%d", e.C.RuleID)
	}
	ig := e.C.Ref
	if parts := strings.Split(e.C.Ref, ":"); len(parts) == 4 {
		ig = "decl:" + owner + ":" + parts[2] + ":" + parts[3]
	}
	return fmt.Sprintf("decl %s About.xml %s pkg:%s", owner, e.C.Field, e.C.Target), ig
}

// PrevMinor returns the previous minor version ("1.7" → "1.6"; "" for X.0 or bad input).
func PrevMinor(mm string) string {
	var a, b int
	if n, _ := fmt.Sscanf(mm, "%d.%d", &a, &b); n != 2 || b == 0 {
		return ""
	}
	return fmt.Sprintf("%d.%d", a, b-1)
}

// NoisyCodes are hidden from default problem lists (05 §4.5); they show with
// --all, an explicit --code or --subject, severity info, and in def / patches output.
var NoisyCodes = map[string]bool{"def.override": true, "patch.contested": true}

// onlyLocks reports whether ins is one instance plus locked copies of it (07 §5): those are
// reported as mod.pinned, not as duplicates.
func onlyLocks(ins []*store.Mod, lockedCopy map[string]string) bool {
	var rest []*store.Mod
	for _, m := range ins {
		if _, ok := lockedCopy[m.ID]; !ok {
			rest = append(rest, m)
		}
	}
	if len(rest) != 1 {
		return false
	}
	for _, m := range ins {
		if up, ok := lockedCopy[m.ID]; ok && up != rest[0].ID {
			return false
		}
	}
	return true
}

// shortHash is the first 12 characters of a hex hash.
func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
