package app

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rimarcho/rimarcho/internal/env"
	"github.com/rimarcho/rimarcho/internal/identity"
	"github.com/rimarcho/rimarcho/internal/ids"
	"github.com/rimarcho/rimarcho/internal/source"
	"github.com/rimarcho/rimarcho/internal/store"
)

// Index is an environment's instances with reference helpers (02 §7).
type Index struct {
	Env       *env.Environment
	Mods      []*store.Mod // sorted by display name, then ID
	ByID      map[string]*store.Mod
	ByLineage map[string][]*store.Mod
	Lineages  map[string]*store.Lineage
	Sources   []*source.Source
	inst, lin *ids.Shortener
}

// Index loads the environment's instances.
func (a *App) Index() (*Index, error) {
	e, err := a.Env()
	if err != nil {
		return nil, err
	}
	mods, err := a.Store.Mods(e.ID)
	if err != nil {
		return nil, err
	}
	lins, err := a.Store.Lineages(e.ID)
	if err != nil {
		return nil, err
	}
	srcs, err := a.Store.Sources(e.ID)
	if err != nil {
		return nil, err
	}
	ix := &Index{Env: e, Mods: mods, ByID: map[string]*store.Mod{}, ByLineage: map[string][]*store.Mod{}, Lineages: lins, Sources: srcs}
	var iids, lids []string
	for _, m := range mods {
		ix.ByID[m.ID] = m
		ix.ByLineage[m.LineageID] = append(ix.ByLineage[m.LineageID], m)
		iids = append(iids, m.ID)
	}
	for id := range lins {
		lids = append(lids, id)
	}
	ix.inst, ix.lin = ids.NewShortener(iids), ids.NewShortener(lids)
	SortMods(ix.Mods)
	for _, l := range ix.ByLineage {
		SortMods(l)
	}
	return ix, nil
}

// SortMods orders by display name (case-insensitive), then source label, then ID.
func SortMods(ms []*store.Mod) {
	sort.SliceStable(ms, func(i, j int) bool {
		a, b := ms[i], ms[j]
		if x, y := strings.ToLower(a.DisplayName()), strings.ToLower(b.DisplayName()); x != y {
			return x < y
		}
		if a.SourceLabel != b.SourceLabel {
			return a.SourceLabel < b.SourceLabel
		}
		return a.ID < b.ID
	})
}

// Ref is an instance's reference: "#" + shortest unique prefix.
func (ix *Index) Ref(id string) string { return "#" + ix.inst.Short(id) }

// LRef is a lineage's reference.
func (ix *Index) LRef(id string) string { return "L#" + ix.lin.Short(id) }

// Source returns a source by ID.
func (ix *Index) Source(id string) *source.Source {
	for _, s := range ix.Sources {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// RefError is a reference that matched nothing.
type RefError struct{ Ref, Hint string }

func (e *RefError) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("%s matches nothing (%s)", e.Ref, e.Hint)
	}
	return e.Ref + " matches nothing"
}

// Resolve maps a reference to instances: #id, L#lineage, pkg:, ws:, or a bare ID prefix.
// pkg:, ws: and L# may return several instances; callers list them instead of choosing (05 I3).
func (ix *Index) Resolve(ref string) ([]*store.Mod, error) {
	r := strings.TrimSpace(ref)
	lower := strings.ToLower(r)
	var out []*store.Mod
	switch {
	case strings.HasPrefix(lower, "l#"):
		p := lower[2:]
		if !ids.Valid(p) {
			return nil, fmt.Errorf("%s: lineage references need at least %d id characters", ref, ids.MinPrefix)
		}
		lids := ix.lin.Lookup(p)
		if len(lids) > 1 {
			return nil, fmt.Errorf("%s is ambiguous (%d lineages)", ref, len(lids))
		}
		for _, l := range lids {
			out = append(out, ix.ByLineage[l]...)
		}
	case strings.HasPrefix(lower, "pkg:"):
		p := lower[4:]
		for _, m := range ix.Mods {
			if m.PackageID == p {
				out = append(out, m)
			}
		}
	case strings.HasPrefix(lower, "ws:"):
		p := r[3:]
		for _, m := range ix.Mods {
			if m.WorkshopID() == p {
				out = append(out, m)
			}
		}
	default:
		p := strings.TrimPrefix(lower, "#")
		if !ids.Valid(p) {
			return nil, &RefError{Ref: ref, Hint: "use #id, L#id, pkg:<packageId> or ws:<workshopId>; search with archo find"}
		}
		for _, id := range ix.inst.Lookup(p) {
			out = append(out, ix.ByID[id])
		}
	}
	if len(out) == 0 {
		return nil, &RefError{Ref: ref, Hint: "archo find <words>"}
	}
	return out, nil
}

// ResolveOne requires a single instance; several matches are reported as candidates.
type Ambiguous struct {
	Ref  string
	Mods []*store.Mod
}

func (e *Ambiguous) Error() string { return fmt.Sprintf("%s matches %d instances", e.Ref, len(e.Mods)) }

// ResolveOne resolves a reference that must name exactly one instance.
func (ix *Index) ResolveOne(ref string) (*store.Mod, error) {
	ms, err := ix.Resolve(ref)
	if err != nil {
		return nil, err
	}
	if len(ms) > 1 {
		return nil, &Ambiguous{Ref: ref, Mods: ms}
	}
	return ms[0], nil
}

// ResolveLineage maps L#…, or any single-instance reference, to a lineage ID.
func (ix *Index) ResolveLineage(ref string) (string, error) {
	ms, err := ix.Resolve(ref)
	if err != nil {
		return "", err
	}
	lin := ms[0].LineageID
	for _, m := range ms {
		if m.LineageID != lin {
			return "", &Ambiguous{Ref: ref, Mods: ms}
		}
	}
	return lin, nil
}

// ParseQuestionRef parses "q:12" (or "12").
func ParseQuestionRef(ref string) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimPrefix(strings.ToLower(ref), "q:"), 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not a question reference (q:<n>, see archo identity list)", ref)
	}
	return n, nil
}

// ResolveQuestion answers an identity question: same | copy | unrelated (02 §5.4).
func (a *App) ResolveQuestion(qid int64, answer string) (*store.Question, error) {
	e, err := a.Env()
	if err != nil {
		return nil, err
	}
	q, err := a.Store.QuestionByID(qid)
	if err != nil {
		return nil, err
	}
	if q == nil || q.EnvID != e.ID {
		return nil, fmt.Errorf("no question q:%d in env:%s (archo identity list)", qid, e.Name)
	}
	if q.Status != "open" {
		return nil, fmt.Errorf("q:%d is already resolved (%s)", qid, q.Resolution)
	}
	now := a.Now()
	switch answer {
	case "same":
		err = a.Store.ResolveSame(q, now)
	case "copy":
		err = a.Store.ResolveCopy(q, now)
	case "unrelated":
		err = a.Store.ResolveUnrelated(q, now)
	default:
		return nil, fmt.Errorf("answer must be same, copy or unrelated, not %q", answer)
	}
	if err != nil {
		return nil, err
	}
	q.Status, q.Resolution = "resolved", answer
	return q, nil
}

// SplitLineage gives an instance a lineage of its own; returns the new lineage ID.
func (a *App) SplitLineage(ix *Index, m *store.Mod) (string, error) {
	if len(ix.ByLineage[m.LineageID]) == 1 {
		return "", fmt.Errorf("%s is already alone in %s", ix.Ref(m.ID), ix.LRef(m.LineageID))
	}
	id := a.NewID()
	return id, a.Store.SplitLineage(ix.Env.ID, m.ID, id, a.Now())
}

// MergeLineages folds lineage `from` into `into`.
func (a *App) MergeLineages(ix *Index, into, from string) error {
	if into == from {
		return fmt.Errorf("both references are %s", ix.LRef(into))
	}
	return a.Store.MergeLineages(ix.Env.ID, into, from)
}

// GC removes expired tombstones (02 §5.5).
func (a *App) GC(dryRun bool) (*store.GCResult, time.Time, error) {
	e, err := a.Env()
	if err != nil {
		return nil, time.Time{}, err
	}
	cutoff := a.Now().Add(-time.Duration(a.Config.TombstoneDays) * 24 * time.Hour)
	r, err := a.Store.GC(e.ID, cutoff, dryRun)
	return r, cutoff, err
}

// Basis renders a lineage membership basis.
func Basis(b string) string {
	switch b {
	case identity.BasisInferred:
		return "inferred"
	case identity.BasisConfirmed:
		return "confirmed"
	case identity.BasisManual:
		return "manual"
	}
	return ""
}
