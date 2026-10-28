package app

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/content"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/identity"
)

// ParentPart identifies content that cannot inherit its missing template. The claim
// describes the patched document, unlike the cached Def facts (03 §5.1).
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
	Problems        []*Problem
	Parts           []ParentPart
	Fixes           []ParentFix
	Quiet           bool
	CacheIncomplete bool
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
			// Compare conditional-content keys without enumerating any directory.
			resolved := content.ResolveFS("", mod.Decl().LoadFolders, st.GameMM(), isActive, func(string) ([]fs.DirEntry, error) { return nil, nil })
			if a.Store.ContentKey(mod.ID) != resolved.Key(mod.SnapHash, st.GameMM())+fmt.Sprintf("|a%d", analyze.Version) {
				h.CacheIncomplete = true
				continue
			}
			afterEnable := func(pkg string) bool {
				return dependencyPackage(pkg) == dependencyPackage(mod.PackageID) || isActive(pkg)
			}
			enabledContent := content.ResolveFS("", mod.Decl().LoadFolders, st.GameMM(), afterEnable, func(string) ([]fs.DirEntry, error) { return nil, nil })
			if enabledContent.Conds != resolved.Conds {
				continue
			}
			if !d.Loaded(afterEnable) {
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
			candidates[pkg] = scanDep{pkg, name, url}
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
	return h, nil
}

func (h *ParentHelp) Meaning(name string) string {
	return fmt.Sprintf("%s: %d added things cannot load because their base content is missing", name, len(h.Parts))
}
