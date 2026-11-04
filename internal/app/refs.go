package app

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/ids"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Index is an environment's instances with reference helpers (05 §2).
type Index struct {
	store     *store.Store // lazy foreign-list lookups follow the owning session on adoption
	Env       *env.Environment
	Mods      []*store.Mod // sorted by display name, then ID
	ByID      map[string]*store.Mod
	ByLineage map[string][]*store.Mod
	Lineages  map[string]*store.Lineage
	Sources   []*source.Source
	inst, lin *ids.Shortener
	own       map[string]*store.Mod // packageId → this game's own Mods/ copy (OwnCopy)
}

// OwnCopy is this game's own Mods/ copy of a packageId (not a RimArcho link), or nil. A list
// naming another game's Mods/ copy uses it instead (02 §0 item 14, 04 §1.7).
func (ix *Index) OwnCopy(pkg string) *store.Mod { return ix.own[strings.ToLower(pkg)] }

// Elsewhere is m when it is another game's Mods/ copy that this game has no copy of its
// own for — the one a list links in — else the copy to use instead.
func (ix *Index) Elsewhere(m *store.Mod) *store.Mod {
	if m == nil || m.Kind != source.Elsewhere {
		return m
	}
	if o := ix.OwnCopy(m.PackageID); o != nil {
		return o
	}
	return m
}

// Index loads the environment's instances. The result is cached until the tables it is
// built from may have changed (Store.IndexKey: writes to index tables on any
// connection), so neither a TUI issuing many read commands nor a list edit
// reloads 1000+ rows each time.
func (a *App) Index() (*Index, error) {
	e, err := a.Env()
	if err != nil {
		return nil, err
	}
	key := a.Store.IndexKey() + "|" + e.ID
	if a.ixCache != nil && a.ixKey == key {
		return a.ixCache, nil
	}
	ix, err := a.loadIndex(e)
	if err != nil {
		return nil, err
	}
	a.ixCache, a.ixKey = ix, key
	return ix, nil
}

// IndexCached reports whether Index would return its cached index without loading.
func (a *App) IndexCached() bool {
	if a.ixCache == nil {
		return false
	}
	e, err := a.Env()
	return err == nil && a.ixKey == a.Store.IndexKey()+"|"+e.ID
}

func (a *App) loadIndex(e *env.Environment) (*Index, error) {
	return a.readIndex(e, true)
}

func (a *App) readIndex(e *env.Environment, repair bool) (*Index, error) {
	mods, err := a.Store.Mods(e.ID)
	if err != nil {
		return nil, err
	}
	srcs, err := a.Store.Sources(e.ID)
	if err != nil {
		return nil, err
	}
	if repair {
		if repaired, err := a.repairSharedLinkInstances(mods, srcs); err != nil {
			return nil, err
		} else if repaired {
			if mods, err = a.Store.Mods(e.ID); err != nil {
				return nil, err
			}
		}
	}
	lins, err := a.Store.Lineages(e.ID)
	if err != nil {
		return nil, err
	}
	pd, err := a.Store.PlayerDataAll(e.ID)
	if err != nil {
		return nil, err
	}
	for _, m := range mods {
		if p := pd[m.LineageID]; p != nil {
			m.Alias, m.Note, m.Tags = p.Alias, p.Note, slices.Clip(p.Tags)
		}
	}
	if mods, srcs, err = a.withElsewhere(e, mods, srcs); err != nil {
		return nil, err
	}
	ix := &Index{store: a.Store, Env: e, Mods: mods, ByID: map[string]*store.Mod{}, ByLineage: map[string][]*store.Mod{}, Lineages: lins, Sources: srcs}
	var iids, lids []string
	ix.own = map[string]*store.Mod{}
	for _, m := range mods {
		ix.ByID[m.ID] = m
		ix.ByLineage[m.LineageID] = append(ix.ByLineage[m.LineageID], m)
		iids = append(iids, m.ID)
		if m.Kind == source.Local && m.Status == identity.Live && !m.Residue && m.Unloadable == "" && !managedLink(m.Rel) && m.PackageID != "" {
			if _, dup := ix.own[strings.ToLower(m.PackageID)]; !dup {
				ix.own[strings.ToLower(m.PackageID)] = m
			}
		}
	}
	for id := range lins {
		lids = append(lids, id)
	}
	ix.inst, ix.lin = ids.NewShortener(iids), ids.NewShortener(lids)
	// Read sessions share what is built here (ReadIndex): clipped slices make an append
	// by any of them copy instead of writing into the shared array.
	ix.Mods = slices.Clip(ix.Mods)
	SortMods(ix.Mods)
	for id, l := range ix.ByLineage {
		SortMods(l)
		ix.ByLineage[id] = slices.Clip(l)
	}
	ix.Sources = slices.Clip(ix.Sources)
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

// RefError is a reference that matched nothing. Find, when set, is what to search for
// instead: Error names the CLI command (search with archo find <Find>) and MCPError the
// MCP tool (search with find_mods query=<Find>), 05 §3.3.
type RefError struct{ Ref, Hint, Find string }

func (e *RefError) Error() string { return e.text("archo find " + quoteWords(e.Find)) }

// MCPError is Error as an MCP tool call shows it: the search names find_mods.
func (e *RefError) MCPError() string { return e.text("find_mods query=" + quoteWords(e.Find)) }

func (e *RefError) text(search string) string {
	hint := e.Hint
	if e.Find != "" {
		if hint != "" {
			hint += "; "
		}
		hint += "search with " + search
	}
	if hint != "" {
		return fmt.Sprintf("%s matches nothing (%s)", e.Ref, hint)
	}
	return e.Ref + " matches nothing"
}

func quoteWords(s string) string {
	if strings.ContainsAny(s, " \t") {
		return `"` + s + `"`
	}
	return s
}

// PackageRef returns the lower-case packageId a reference names: pkg:<packageId> or a bare
// packageId (BarePackageID), 05 §2.
func PackageRef(ref string) (string, bool) {
	r := strings.TrimSpace(ref)
	if p, ok := strings.CutPrefix(strings.ToLower(r), "pkg:"); ok {
		return p, true
	}
	if BarePackageID(r) {
		return strings.ToLower(r), true
	}
	return "", false
}

// BarePackageID reports whether a word is a packageId written without pkg: (05 §2): two or
// more dot-separated parts of ASCII letters, digits, "_" and "-", with at least one letter.
// No other reference has that shape: instance and lineage IDs have no dot, the prefixed
// forms have ":" or "#", Def references "/" or "@", paths "/" or "\", names a space, and a
// game version such as 1.6 has no letter.
func BarePackageID(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return false
	}
	letter := false
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, c := range part {
			switch {
			case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
				letter = true
			case c >= '0' && c <= '9' || c == '_' || c == '-':
			default:
				return false
			}
		}
	}
	return letter
}

// packageSearch is the word to search for when a packageId matches nothing: its longest
// part, usually the mod's name (khamenman.outfitstandsplus → outfitstandsplus), which
// find also matches in names, folders and other packageIds.
func packageSearch(pkg string) string {
	best := ""
	for _, part := range strings.Split(pkg, ".") {
		if len(part) > len(best) {
			best = part
		}
	}
	return best
}

// Resolve maps a reference to instances: #id, L#lineage, pkg:<packageId> or a bare
// packageId, ws:, or a bare ID prefix. A packageId, ws: and L# may return several
// instances; callers list them instead of choosing (05 I3).
func (ix *Index) Resolve(ref string) ([]*store.Mod, error) {
	r := strings.TrimSpace(ref)
	lower := strings.ToLower(r)
	var out []*store.Mod
	if p, ok := PackageRef(r); ok {
		if p == "" {
			return nil, &RefError{Ref: ref, Hint: "pkg: needs a packageId", Find: "<words>"}
		}
		for _, m := range ix.Mods {
			if m.PackageID == p {
				out = append(out, m)
			}
		}
		if len(out) == 0 {
			return nil, &RefError{Ref: ref, Hint: "no mod has packageId " + p, Find: packageSearch(p)}
		}
		return out, nil
	}
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
	case strings.HasPrefix(lower, "ws:"):
		p := r[3:]
		for _, m := range ix.Mods {
			if m.WorkshopID() == p {
				out = append(out, m)
			}
		}
		if len(out) == 0 && p != "" {
			return nil, &RefError{Ref: ref, Hint: "no mod has Workshop ID " + p, Find: p}
		}
	default:
		p := strings.TrimPrefix(lower, "#")
		if !ids.Valid(p) {
			return nil, &RefError{Ref: ref, Hint: "not a reference (#id, L#id, pkg:<packageId> or a bare packageId, ws:<workshopId>)", Find: r}
		}
		for _, id := range ix.inst.Lookup(p) {
			out = append(out, ix.ByID[id])
		}
		if len(out) == 0 && !strings.HasPrefix(r, "#") {
			return nil, &RefError{Ref: ref, Find: r} // a bare word may be a name: archo mod Alpha
		}
	}
	if len(out) == 0 {
		return nil, &RefError{Ref: ref, Find: "<words>"}
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

// IdentityQuestion looks up a question visible to the current environment.
func (a *App) IdentityQuestion(qid int64) (*store.Question, error) {
	e, err := a.Env()
	if err != nil {
		return nil, err
	}
	q, err := a.Store.QuestionByID(qid)
	if err != nil {
		return nil, err
	}
	if q == nil || (q.EnvID != e.ID && q.EnvID != store.Shared) {
		return nil, fmt.Errorf("no question q:%d in env:%s (archo identity list)", qid, e.Name)
	}
	return q, nil
}

// ResolveQuestion answers an identity question: same | copy | unrelated (02 §5.4).
func (a *App) ResolveQuestion(qid int64, answer string) (*store.Question, error) {
	q, err := a.IdentityQuestion(qid)
	if err != nil {
		return nil, err
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

// CanSeparate reports whether the detail can list this live copy as an independent mod.
func (ix *Index) CanSeparate(m *store.Mod) bool {
	if m == nil || m.Status != identity.Live || m.Kind == source.Official {
		return false
	}
	for _, other := range ix.ByLineage[m.LineageID] {
		if other.ID != m.ID && other.Status == identity.Live {
			return true
		}
	}
	return false
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
