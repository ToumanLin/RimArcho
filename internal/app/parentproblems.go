package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/content"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/modsearch"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// ParentPart identifies content that cannot inherit its missing template. The claim
// describes the patched document, unlike the cached Def facts (05 §5.1).
type ParentPart struct{ Type, Name, Parent string }

var parentClaim = regexp.MustCompile(`^(\S+) (.*?) inherits ParentName="([^"]+)"`)

func MissingParent(p *Problem) (ParentPart, bool) {
	m := parentClaim.FindStringSubmatch(p.Claim)
	if m == nil {
		return ParentPart{}, false
	}
	return ParentPart{m[1], m[2], m[3]}, true
}

// ParentFix is either a verified cached provider, a declared candidate, or the
// affected mod's requirements page. Candidates are never claimed to define a Name.
type ParentFix struct{ Kind, Ref, Name, URL, Basis, Parent string }
type ParentHelp struct {
	Problems         []*Problem
	Parts            []ParentPart
	Fixes            []ParentFix
	Quiet            bool
	CacheIncomplete  bool
	SearchIncomplete bool
}

// ParentProblems groups the visible missing-parent findings of one mod. An ignored
// finding stays individually inspectable and never hides its unignored siblings.
func ParentProblems(ps *ProblemSet, p *Problem) []*Problem {
	if p.Ignored {
		return []*Problem{p}
	}
	var out []*Problem
	for _, q := range ps.List {
		if q.Code == p.Code && q.SubjectInstance() == p.SubjectInstance() {
			out = append(out, q)
		}
	}
	if len(out) == 0 {
		out = append(out, p)
	}
	return out
}

// ParentHelp reads stored facts/declarations only: no XML scans, analysis or network.
// Called when a player opens a problem, rather than while loading its page.
func (a *App) ParentHelp(ps *ProblemSet, p *Problem) (*ParentHelp, error) {
	st := ps.State
	h := &ParentHelp{Problems: ParentProblems(ps, p)}
	for _, q := range h.Problems {
		if part, ok := MissingParent(q); ok {
			h.Parts = append(h.Parts, part)
		}
	}
	m := st.Ix.ByID[p.SubjectInstance()]
	if m == nil {
		return h, nil
	}
	prevMM := a.Store.MM
	a.Store.MM = st.GameMM()
	defer func() { a.Store.MM = prevMM }()
	active := map[string]bool{}
	installed := map[string]bool{}
	for _, mod := range st.Ix.Mods {
		if mod.Status == identity.Live && !mod.Residue {
			installed[dependencyPackage(mod.PackageID)] = true
		}
		if st.Position(mod.ID) > 0 {
			active[dependencyPackage(mod.PackageID)] = true
		}
	}
	isActive := func(pkg string) bool { return active[dependencyPackage(pkg)] }
	// Absence of an error only counts for a fully attributed log of this list.
	for _, run := range ps.Logs {
		if !run.Stale() && !run.Pending {
			h.Quiet = true
			break
		}
	}
	for _, q := range ps.All() {
		if q.Log == nil || q.Log.Stale() {
			continue
		}
		if q.Severity != finding.Error && q.Severity != finding.Warning {
			continue
		}
		for _, s := range q.Subjects {
			if finding.InstID(s) == m.ID {
				h.Quiet = false
			}
		}
	}
	seen := map[string]bool{}
	add := func(f ParentFix) {
		key := f.Kind + "|" + f.Ref + "|" + f.URL
		if f.Kind == "enable" {
			key += "|" + f.Parent
		}
		if !seen[key] {
			seen[key] = true
			h.Fixes = append(h.Fixes, f)
		}
	}
	names := map[string]bool{}
	fresh := ps.Analysis != nil && ps.Analysis.Fresh
	h.CacheIncomplete = !fresh
	for _, part := range h.Parts {
		if names[part.Parent] {
			continue
		}
		names[part.Parent] = true
		defs, err := a.Store.DefsNamed(part.Parent, true)
		if err != nil {
			return nil, err
		}
		for _, d := range defs {
			mod := st.Ix.ByID[d.Instance]
			if !fresh || mod == nil || mod.Status != identity.Live || mod.Residue || st.Position(mod.ID) > 0 || whyNotLoadable(st, mod) != "" {
				continue
			}
			current, sameWhenEnabled, afterEnable := parentIndex(st, mod, a.Store.ContentKey(mod.ID), isActive)
			if !current {
				h.CacheIncomplete = true
				continue
			}
			if !sameWhenEnabled || !d.Loaded(afterEnable) {
				continue
			}
			add(ParentFix{Kind: "enable", Ref: st.Ix.Ref(mod.ID), Name: mod.DisplayName(), Parent: part.Parent, Basis: "provider"})
		}
	}
	decl := m.Decl()
	candidates := map[string]scanDep{}
	basis := map[string]string{}
	put := func(pkg, name, url, why string) {
		pkg = dependencyPackage(pkg)
		if pkg == "" || active[pkg] || installed[pkg] {
			return
		}
		old, exists := candidates[pkg]
		if !exists || old.url == "" && url != "" {
			candidates[pkg] = scanDep{pkg: pkg, name: name, url: url}
			basis[pkg] = why
		}
	}
	deps := append(toDeps(decl.Dependencies), toDeps(decl.DependenciesByVersion["v"+st.GameMM()])...)
	for _, d := range deps {
		put(d.pkg, d.name, d.url, "dependency")
	}
	after := append(append([]string{}, decl.LoadAfter...), decl.LoadAfterByVersion["v"+st.GameMM()]...)
	after = append(after, decl.ForceLoadAfter...)
	for _, pkg := range after {
		put(pkg, pkg, "", "loadAfter")
	}
	// These are references, not proof that the package supplies the missing template.
	defs, err := a.Store.DefsOf(m.ID)
	if err != nil {
		return nil, err
	}
	for _, d := range defs {
		for _, pkg := range strings.FieldsFunc(d.MayRequire+","+d.MayRequireAny, func(r rune) bool { return r == ',' || r == ';' }) {
			put(pkg, strings.TrimSpace(pkg), "", "MayRequire")
		}
	}
	keys := make([]string, 0, len(candidates))
	for pkg := range candidates {
		keys = append(keys, pkg)
	}
	sort.Strings(keys)
	for _, pkg := range keys {
		d := candidates[pkg]
		ws, name := a.dependencyWorkshop(d)
		if name == "" {
			name = d.name
		}
		if name == "" {
			name = pkg
		}
		if ws != "" {
			add(ParentFix{Kind: "subscribe", Ref: ws, Name: name, URL: "https://steamcommunity.com/sharedfiles/filedetails/?id=" + ws, Basis: basis[pkg]})
		} else {
			add(ParentFix{Kind: "search", Ref: pkg, Name: name, Basis: basis[pkg]})
		}
	}
	ws := m.WorkshopID()
	if ws == "" {
		if w, ok := a.WorkshopFor(m.PackageID); ok {
			ws = w.WorkshopID
		}
	}
	u := ""
	if ws != "" {
		u = "https://steamcommunity.com/sharedfiles/filedetails/?id=" + ws
	}
	add(ParentFix{Kind: "requirements", Name: m.DisplayName(), URL: u})
	h.dropCandidates()
	h.sortFixes()
	return h, nil
}

// parentIndex compares a mod's stored Name facts with what it would load: current means
// the analysis indexed its present content; sameWhenEnabled means enabling it changes no
// load-folder condition. Both hold for a mod whose facts already answer "who defines this
// Name", so a file search cannot find more there. No directory is enumerated.
func parentIndex(st *LoadoutState, mod *store.Mod, key string, isActive func(string) bool) (current, sameWhenEnabled bool, afterEnable func(string) bool) {
	noDirs := func(string) ([]fs.DirEntry, error) { return nil, nil }
	resolved := content.ResolveFS(st.Ix.ModDir(mod), mod.Decl().LoadFolders, st.GameMM(), isActive, noDirs)
	afterEnable = func(pkg string) bool {
		return dependencyPackage(pkg) == dependencyPackage(mod.PackageID) || isActive(pkg)
	}
	matches := key == resolved.Key(mod.SnapHash, st.GameMM())+analyze.DefKeySuffix()
	// Without LoadFolders there are no plan-dependent directories. The snapshot guards
	// the version-directory layout, which this fast path deliberately does not enumerate.
	if mod.Decl().LoadFolders == "" {
		matches = strings.HasPrefix(key, fmt.Sprintf("%x|f", mod.SnapHash)) && strings.HasSuffix(key, analyze.DefKeySuffix())
	}
	if !matches {
		return false, false, afterEnable
	}
	enabled := content.ResolveFS(st.Ix.ModDir(mod), mod.Decl().LoadFolders, st.GameMM(), afterEnable, noDirs)
	return true, enabled.Conds == resolved.Conds, afterEnable
}

// dropCandidates removes unconfirmed candidates once installed providers cover every
// missing Name: a loadAfter or MayRequire mention is not a reason to subscribe then.
func (h *ParentHelp) dropCandidates() {
	if len(h.Parts) == 0 || len(h.UnresolvedNames()) > 0 {
		return
	}
	h.Fixes = slices.DeleteFunc(h.Fixes, func(f ParentFix) bool { return f.Kind == "subscribe" || f.Kind == "search" })
}

func (h *ParentHelp) Meaning(name string) string {
	return fmt.Sprintf("%s: %d added things cannot load because their base content is missing", name, len(h.Parts))
}

// UnresolvedNames excludes names with an already verified installed provider.
func (h *ParentHelp) UnresolvedNames() map[string]bool {
	names := map[string]bool{}
	for _, part := range h.Parts {
		names[part.Parent] = true
	}
	for _, fix := range h.Fixes {
		if fix.Kind == "enable" {
			delete(names, fix.Parent)
		}
	}
	return names
}

// FindParentHelp supplements the cache only on an explicit detail request.
func (a *App) FindParentHelp(ctx context.Context, ps *ProblemSet, p *Problem) (*ParentHelp, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h, err := a.ParentHelp(ps, p)
	if err != nil {
		return nil, err
	}
	names := h.UnresolvedNames()
	if len(names) == 0 {
		return h, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	st := ps.State
	active := map[string]bool{}
	for _, m := range st.Ix.Mods {
		if st.Position(m.ID) > 0 {
			active[dependencyPackage(m.PackageID)] = true
		}
	}
	// A fresh analysis indexed every installed mod's Names; search only the mods whose
	// facts may differ from what they would load once enabled (05 §4.3).
	var keys map[string]string
	if ps.Analysis != nil && ps.Analysis.Fresh {
		prevMM := a.Store.MM
		a.Store.MM = st.GameMM()
		keys, err = a.Store.ContentKeys()
		a.Store.MM = prevMM
		if err != nil {
			return nil, err
		}
	}
	isActive := func(pkg string) bool { return active[dependencyPackage(pkg)] }
	var mods []modsearch.Mod
	for _, m := range st.Ix.Mods {
		if m.Status != identity.Live || m.Residue || st.Position(m.ID) > 0 || whyNotLoadable(st, m) != "" {
			continue
		}
		if keys != nil {
			if current, same, _ := parentIndex(st, m, keys[m.ID], isActive); current && same {
				continue
			}
		}
		mods = append(mods, modsearch.Mod{Ref: st.Ix.Ref(m.ID), PackageID: m.PackageID, Name: m.DisplayName(), Root: m.RealPath, LoadFoldersXML: m.Decl().LoadFolders})
		if len(mods) > 1000 {
			break
		}
	}
	if len(mods) == 0 {
		h.CacheIncomplete, h.SearchIncomplete = false, false
		return h, nil
	}
	matches, incomplete, err := modsearch.FindNames(ctx, mods, modsearch.Options{MM: st.GameMM(), Active: isActive}, names)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	h.CacheIncomplete = false
	h.SearchIncomplete = incomplete || err != nil
	for _, match := range matches {
		h.Fixes = append(h.Fixes, ParentFix{Kind: "enable", Ref: match.Mod.Ref, Name: match.Mod.Name, Parent: match.Name, Basis: "scan"})
	}
	h.dropCandidates()
	h.sortFixes()
	return h, nil
}

var parentFileLine = regexp.MustCompile(`(?i)^.+\.xml:[0-9]+$`)

// ParentLocations separates bare source locations from explanatory evidence.
func ParentLocations(p *Problem) (locations, other []string) {
	for _, e := range p.Evidence {
		if parentFileLine.MatchString(e.Text) {
			locations = append(locations, e.Text)
		} else {
			other = append(other, e.Text)
		}
	}
	return
}

func (h *ParentHelp) sortFixes() {
	priority := func(kind string) int {
		switch kind {
		case "enable":
			return 0
		case "subscribe":
			return 1
		case "search":
			return 2
		}
		return 3
	}
	sort.SliceStable(h.Fixes, func(i, j int) bool { return priority(h.Fixes[i].Kind) < priority(h.Fixes[j].Kind) })
}
