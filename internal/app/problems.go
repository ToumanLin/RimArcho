package app

import (
	"crypto/sha1"
	"fmt"
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
	IgnoreExpired bool    // ignored once, but the subject changed since (03 §4.2)
	Log           *LogRun // the log of a log.* problem
}

// ProblemSet is every current problem of the environment.
type ProblemSet struct {
	State   *LoadoutState
	List    []*Problem // not ignored, sorted by severity then subject
	Ignored []*Problem
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
// With ensure, a stale or missing analysis is rebuilt first.
func (a *App) Problems(ensure bool, progress func(string)) (*ProblemSet, error) {
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
		key = a.stKey + "|" + a.GameVersion
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
	all, err := a.DeclFindings(st)
	if err != nil {
		return nil, err
	}
	if r := ps.Analysis.Run; r != nil {
		fs, err := a.Store.Findings(r.ID)
		if err != nil {
			return nil, err
		}
		for _, f := range fs {
			// A stale run may name instances that are gone.
			if id := f.SubjectInstance(); id != "" && st.Ix.ByID[id] == nil {
				continue
			}
			all = append(all, f)
		}
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
	ignores, err := a.Store.Ignores(st.Ix.Env.ID)
	if err != nil {
		return nil, err
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
		if g := ig[igKey{lin, f.Code, f.Key}]; g != nil {
			if g.SnapshotID == snap {
				p.Ignored = true
			} else {
				p.IgnoreExpired = true
			}
		}
		switch {
		case p.Ignored:
			ps.Ignored = append(ps.Ignored, p)
		case p.Log != nil && p.Log.Stale():
			ps.Stale = append(ps.Stale, p)
		default:
			ps.List = append(ps.List, p)
		}
	}
	order := func(f *finding.Finding) string {
		if m := st.Ix.ByID[f.SubjectInstance()]; m != nil {
			return fmt.Sprintf("%06d %s", st.Position(m.ID), strings.ToLower(m.DisplayName()))
		}
		return "~" + f.Key
	}
	sortProblems(ps.List, order)
	sortProblems(ps.Ignored, order)
	sortProblems(ps.Stale, order)
	return ps, nil
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

// All is every problem: current, ignored and from stale logs.
func (ps *ProblemSet) All() []*Problem {
	return append(append(append([]*Problem{}, ps.List...), ps.Ignored...), ps.Stale...)
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

// DeclFindings computes the declaration layer (03 §4.3) for the workspace.
func (a *App) DeclFindings(st *LoadoutState) ([]*finding.Finding, error) {
	mm := st.GameMM()
	var out []*finding.Finding
	add := func(f *finding.Finding) {
		f.Layer = "decl"
		if f.Confidence == "" {
			f.Confidence = finding.Certain
		}
		out = append(out, f)
	}
	inst := finding.Inst
	activePos := map[string]int{}
	activeByPkg := map[string][]*store.Mod{}
	var active []*store.Mod
	for i, e := range st.List {
		if e.Instance == "" {
			add(&finding.Finding{Code: "entry.missing", Severity: finding.Warning, Key: e.Missing,
				Subjects:    []string{"pkg:" + e.Missing},
				Claim:       fmt.Sprintf("pkg:%s in the Loadout matches no installed mod (kept in ModsConfig.xml)", e.Missing),
				Suggestions: []string{"archo find " + e.Missing, "archo disable pkg:" + e.Missing}})
			continue
		}
		m := st.Ix.ByID[e.Instance]
		if m == nil {
			continue
		}
		if _, dup := activePos[m.ID]; !dup {
			activePos[m.ID] = i + 1
			active = append(active, m)
			p := strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")
			activeByPkg[p] = append(activeByPkg[p], m)
		}
	}
	if err := a.Store.LoadDecls(active); err != nil {
		return nil, err
	}
	installed := map[string][]*store.Mod{} // live, loadable instances by packageId
	for _, m := range st.Ix.Mods {
		if m.TombstoneAt.IsZero() && !m.Residue && m.Unloadable == "" && st.Reg.Known(m.ID) {
			p := strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")
			installed[p] = append(installed[p], m)
		}
	}

	for _, m := range active {
		d := m.Decl()
		if why := whyNotLoadable(st, m); why != "" {
			add(&finding.Finding{Code: "mod.unloadable", Severity: finding.Error, Key: "active", Subjects: []string{inst(m.ID)},
				Claim:       fmt.Sprintf("%s is enabled but the game will not load it: %s", inst(m.ID), why),
				Suggestions: []string{"archo disable " + inst(m.ID)}})
		}
		// Dependencies.
		deps := append([]scanDep{}, toDeps(d.Dependencies)...)
		deps = append(deps, toDeps(d.DependenciesByVersion["v"+mm])...)
		seen := map[string]bool{}
		for _, dep := range deps {
			p := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(dep.pkg)), "_steam")
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			if len(activeByPkg[p]) > 0 {
				continue
			}
			ev := finding.Evidence{Source: finding.Declared, Ref: inst(m.ID), Text: "About.xml modDependencies: " + p}
			if cands := installed[p]; len(cands) > 0 {
				f := &finding.Finding{Code: "dep.inactive", Severity: finding.Error, Key: p, Subjects: []string{inst(m.ID), inst(cands[0].ID)},
					Claim:    fmt.Sprintf("%s requires pkg:%s; %s is installed but not enabled", inst(m.ID), p, inst(cands[0].ID)),
					Evidence: []finding.Evidence{ev}}
				for _, c := range cands {
					f.Suggestions = append(f.Suggestions, "archo enable "+inst(c.ID))
				}
				add(f)
				continue
			}
			name := dep.name
			if name == "" {
				name = p
			}
			f := &finding.Finding{Code: "dep.missing", Severity: finding.Error, Key: p, Subjects: []string{inst(m.ID), "pkg:" + p},
				Claim: fmt.Sprintf("%s requires %s (pkg:%s), which is not installed", inst(m.ID), name, p), Evidence: []finding.Evidence{ev}}
			switch {
			case strings.HasPrefix(p, "ludeon.rimworld."):
				f.Claim = fmt.Sprintf("%s requires the %s DLC (pkg:%s), which is not installed", inst(m.ID), name, p)
			case dep.url != "":
				f.Suggestions = append(f.Suggestions, "subscribe: "+dep.url)
			}
			if ws := workshopID(dep.url); ws != "" {
				f.Suggestions = append(f.Suggestions, "steam://url/CommunityFilePage/"+ws)
			}
			add(f)
		}
		// Incompatibilities (reported once per pair).
		inc := append(append([]string{}, d.IncompatibleWith...), d.IncompatibleByVersion["v"+mm]...)
		for _, x := range inc {
			p := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(x)), "_steam")
			for _, o := range activeByPkg[p] {
				if o.ID == m.ID {
					continue
				}
				a1, b1 := m, o
				if activePos[b1.ID] < activePos[a1.ID] {
					a1, b1 = b1, a1
				}
				key := a1.LineageID + "|" + b1.LineageID
				dup := false
				for _, f := range out {
					if f.Code == "incompat.active" && f.Key == key {
						f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Declared, Ref: inst(m.ID), Text: "About.xml incompatibleWith: " + p})
						dup = true
					}
				}
				if dup {
					continue
				}
				add(&finding.Finding{Code: "incompat.active", Severity: finding.Error, Key: key, Subjects: []string{inst(m.ID), inst(o.ID)},
					Claim:       fmt.Sprintf("%s is incompatible with %s, and both are enabled", inst(m.ID), inst(o.ID)),
					Evidence:    []finding.Evidence{{Source: finding.Declared, Ref: inst(m.ID), Text: "About.xml incompatibleWith: " + p}},
					Suggestions: []string{"archo disable " + inst(o.ID), "archo disable " + inst(m.ID), "archo why " + inst(m.ID) + " " + inst(o.ID)}})
			}
		}
		// Supported versions.
		if m.Kind != "official" && len(d.SupportedVersions) > 0 && mm != "" {
			ok := false
			for _, v := range d.SupportedVersions {
				if strings.TrimSpace(v) == mm {
					ok = true
				}
			}
			if !ok {
				// Listing the previous minor version only is the normal state right after a game
				// update: info. Anything older: warning (decisions-m3 §12.1).
				sev := finding.Warning
				if prev := PrevMinor(mm); prev != "" {
					for _, v := range d.SupportedVersions {
						if strings.TrimSpace(v) == prev {
							sev = finding.Info
						}
					}
				}
				add(&finding.Finding{Code: "version.unsupported", Severity: sev, Key: mm, Subjects: []string{inst(m.ID)},
					Claim:    fmt.Sprintf("%s does not list %s in supportedVersions (%s)", inst(m.ID), mm, strings.Join(d.SupportedVersions, " ")),
					Evidence: []finding.Evidence{{Source: finding.Declared, Ref: inst(m.ID), Text: "About.xml supportedVersions"}}})
			}
		}
	}
	// Duplicated packages.
	var pkgs []string
	for p := range installed {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	for _, p := range pkgs {
		if as := activeByPkg[p]; len(as) > 1 {
			f := &finding.Finding{Code: "pkg.duplicate-active", Severity: finding.Error, Key: p,
				Claim: fmt.Sprintf("%d copies of pkg:%s are enabled; the game loads only one", len(as), p)}
			for _, m := range as {
				f.Subjects = append(f.Subjects, inst(m.ID))
				f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Inferred, Ref: inst(m.ID), Text: fmt.Sprintf("enabled at %d (src:%s)", activePos[m.ID], m.SourceLabel)})
			}
			for _, m := range as[1:] {
				f.Suggestions = append(f.Suggestions, "archo disable "+inst(m.ID))
			}
			add(f)
		} else if ins := installed[p]; len(ins) > 1 {
			subj := ins[0]
			if len(as) == 1 {
				subj = as[0]
			}
			f := &finding.Finding{Code: "pkg.duplicate-installed", Severity: finding.Info, Key: p, Subjects: []string{inst(subj.ID)},
				Claim: fmt.Sprintf("%d copies of pkg:%s are installed", len(ins), p)}
			for _, m := range ins {
				if m != subj {
					f.Subjects = append(f.Subjects, inst(m.ID))
				}
				note := "installed"
				if activePos[m.ID] > 0 {
					note = fmt.Sprintf("enabled at %d", activePos[m.ID])
				} else if !st.Reg.Loadable(m.ID) {
					note = "ignored by the game (another copy wins)"
				}
				f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Inferred, Ref: inst(m.ID), Text: fmt.Sprintf("src:%s %s", m.SourceLabel, note)})
			}
			f.Suggestions = []string{"archo swap " + inst(subj.ID)}
			add(f)
		}
	}
	// About.xml renamed away (about-disabled) on installed mods that are not enabled.
	for _, m := range st.Ix.Mods {
		if m.TombstoneAt.IsZero() && m.Unloadable != "" && activePos[m.ID] == 0 {
			add(&finding.Finding{Code: "mod.unloadable", Severity: finding.Info, Key: m.Unloadable, Subjects: []string{inst(m.ID)},
				Claim: fmt.Sprintf("%s is installed but cannot be loaded: %s", inst(m.ID), m.Unloadable)})
		}
	}
	// Order: violations and cycles from the sort constraints.
	in, err := a.SortInput(st, st.List)
	if err != nil {
		return nil, err
	}
	r := order.Sort(in)
	inCycle := map[int]bool{}
	for _, cy := range r.Cycles {
		for _, m := range cy.Members {
			inCycle[m] = true
		}
	}
	nodeTok := func(i int) string {
		if nd := in.Nodes[i]; nd.Instance != "" {
			return inst(nd.Instance)
		}
		return "pkg:" + in.Nodes[i].PackageID
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
				officialSubject = nodeTok(e.To)
			}
			official = append(official, finding.Evidence{Source: finding.Inferred, Text: ev})
			continue
		}
		src := finding.Declared
		switch e.C.Kind {
		case order.KindCommunity:
			src = finding.Community
		case order.KindUser:
			src = finding.User
		}
		key := in.Nodes[e.From].PackageID + ">" + in.Nodes[e.To].PackageID
		f := &finding.Finding{Code: "order.violated", Severity: finding.Warning, Key: key, Subjects: []string{nodeTok(e.To), nodeTok(e.From)},
			Claim:       fmt.Sprintf("%s should load after %s but is at %d, before %d", nodeTok(e.To), nodeTok(e.From), e.To+1, e.From+1),
			Evidence:    []finding.Evidence{{Source: src, Text: ev}},
			Suggestions: []string{"archo sort --dry-run", "archo move " + nodeTok(e.To) + " --after " + nodeTok(e.From)}}
		if !strings.HasPrefix(ig, "archo ") {
			f.Suggestions = append(f.Suggestions, "archo rule add ignore "+ig)
		}
		add(f)
	}
	if len(official) > 0 {
		add(&finding.Finding{Code: "official.order", Severity: finding.Error, Key: "official", Subjects: []string{officialSubject},
			Claim:    "Core and the DLCs are not first in the list, in release order",
			Evidence: official, Suggestions: []string{"archo sort --dry-run"}})
	}
	for _, cy := range r.Cycles {
		var refs []string
		for _, mi := range cy.Members {
			refs = append(refs, nodeTok(mi))
		}
		f := &finding.Finding{Code: "order.cycle", Severity: finding.Warning, Key: strings.Join(refs, " "), Subjects: refs,
			Claim: fmt.Sprintf("load-order constraints form a cycle among %s (kept in the current order)", strings.Join(refs, " "))}
		for i, e := range cy.Edges {
			ev, ig := edgeText(st, in, e)
			t := ev
			if i == cy.Suggest {
				t += "  [suggested to ignore]"
				f.Suggestions = append([]string{"archo rule add ignore " + ig}, f.Suggestions...)
			}
			f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Declared, Text: t})
		}
		add(f)
	}
	return out, nil
}

type scanDep struct{ pkg, name, url string }

func toDeps(l []scan.Dependency) []scanDep {
	out := make([]scanDep, 0, len(l))
	for _, d := range l {
		out = append(out, scanDep{d.PackageID, d.DisplayName, d.SteamWorkshopURL})
	}
	return out
}

// workshopID extracts the item ID from a Steam workshop URL.
func workshopID(u string) string {
	i := strings.Index(u, "id=")
	if i < 0 {
		return ""
	}
	id := u[i+3:]
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

// NoisyCodes are hidden from default problem lists (decisions-m3 §12.2); they show with
// --all, an explicit --code or --subject, severity info, and in def / patches output.
var NoisyCodes = map[string]bool{"def.override": true, "patch.contested": true}
