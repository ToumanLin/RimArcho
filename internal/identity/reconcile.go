// Package identity reconciles scan observations against known Instances (02 §5): rules
// R1–R7, lineage membership, change events and identity questions. It is pure logic; the
// caller loads the known state and persists the result.
package identity

import (
	"fmt"
	"slices"
	"strings"

	"github.com/rimarcho/rimarcho/internal/scan"
	"github.com/rimarcho/rimarcho/internal/source"
)

// Instance status values.
const (
	Live      = "live"
	Tombstone = "tombstone"
)

// Lineage membership basis (02 §5.2, §6).
const (
	BasisRoot      = ""          // founded the lineage
	BasisInferred  = "inferred"  // R4: similar content + same declaration
	BasisConfirmed = "confirmed" // R5 origin marker, or the player said "copy"
	BasisManual    = "manual"    // lineage split/merge
)

// Thresholds (02 §5.2).
const (
	MoveSimilarity     = 0.85 // R3
	CopySimilarity     = 0.6  // R4
	QuestionSimilarity = 0.6  // R6
)

// Event kinds (00 §6.8, 02 §5.3).
const (
	EvDiscovered       = "discovered"
	EvUpdated          = "updated"
	EvMoved            = "moved"
	EvCopied           = "copied"
	EvDisappeared      = "disappeared"
	EvReappeared       = "reappeared"
	EvPackageIDChanged = "package-id-changed"
	EvRenamed          = "renamed"
	EvAuthorChanged    = "author-changed"
	EvVersionSupport   = "version-support-changed"
	EvDependencies     = "dependencies-changed"
	EvLoadRules        = "load-rules-changed"
)

// Snap is the part of a stored snapshot reconciliation needs.
type Snap struct {
	ID          int64
	Hash        scan.Hash
	Decl        scan.Decl
	WorkshopID  string
	ACFManifest string
	GitHead     string
	Origin      *scan.Origin
}

// Known is an existing Instance (live or tombstone) with its latest snapshot.
type Known struct {
	ID        string
	LineageID string
	SourceID  string
	Kind      source.Kind
	Label     string
	Rel       string
	Status    string
	Snap      *Snap

	seen bool
}

// Input to Reconcile.
type Input struct {
	Obs   []*scan.Observation // in reconciliation order (scan.Collect order)
	Known []*Known
	// Skip lists sources that were not scanned (missing or awaiting re-binding): their
	// instances are neither matched nor tombstoned.
	Skip map[string]bool
	// Lineages that exist (to validate origin markers).
	Lineages map[string]bool
	NewID    func() string
	// Semantic loads the semantic set of a known instance's latest snapshot.
	Semantic func(k *Known) scan.SemanticSet
}

// Assignment maps one observation to an Instance.
type Assignment struct {
	Obs          *scan.Observation
	Known        *Known // nil when a new Instance was created
	InstanceID   string
	LineageID    string
	NewLineage   bool
	LineageBasis string
	Rule         string // R1 … R7
	Prev         *Snap  // previous snapshot (nil for new instances)
	NeedSnapshot bool   // content or external state changed: write a new snapshot
}

// Event is a change event; After is the assignment whose new snapshot is the "after" side.
type Event struct {
	InstanceID string
	Kind       string
	Detail     string
	Before     int64 // snapshot id, 0 if none
	After      *Assignment
}

// Question is an identity question raised by R6.
type Question struct {
	New       *Assignment
	Candidate *Known
	Score     float64
}

// Result of a reconciliation.
type Result struct {
	Assignments []*Assignment
	Events      []*Event
	Questions   []*Question
	Disappeared []*Known
}

type reconciler struct {
	in       Input
	res      *Result
	semCache map[*Known]scan.SemanticSet
	// instances present in this scan: matched known ones and newly created ones
	present []*Assignment
}

// Reconcile applies 02 §5.2. R1, R2, R5 and R3 match against the known state rule by rule;
// the remaining observations then go through R4, R6 and R7 one at a time, so an Instance
// created earlier in the same scan can be the original of a later copy (first scan of a
// workshop item and its Mods/ copy, or Steam + SteamCMD).
func Reconcile(in Input) *Result {
	r := &reconciler{in: in, res: &Result{}, semCache: map[*Known]scan.SemanticSet{}}
	assigned := make([]*Assignment, len(in.Obs))

	byLoc := map[string]*Known{}
	for _, k := range in.Known {
		byLoc[scan.LocKey(k.SourceID, k.Rel)] = k
	}
	// R1: same source, same relative path.
	for i, ob := range in.Obs {
		if k := byLoc[ob.Loc.Key()]; k != nil && !k.seen {
			assigned[i] = r.match(ob, k, "R1")
		}
	}
	// R2: workshop-like folder name (= workshop ID) seen before under a workshop-like
	// source, and that Instance did not appear this scan.
	for i, ob := range in.Obs {
		if assigned[i] != nil || !ob.Loc.Kind.WorkshopLike() {
			continue
		}
		var best *Known
		for _, k := range in.Known {
			if k.seen || in.Skip[k.SourceID] || !k.Kind.WorkshopLike() || !strings.EqualFold(k.Rel, ob.Loc.Rel) {
				continue
			}
			if best == nil || (best.Status == Tombstone && k.Status == Live) {
				best = k
			}
		}
		if best != nil {
			assigned[i] = r.match(ob, best, "R2")
		}
	}
	// R5: origin marker (before R3/R4). A marker identical to that of a vanished instance
	// is that instance renamed, not a new copy.
	for i, ob := range in.Obs {
		if assigned[i] != nil || ob.Origin == nil {
			continue
		}
		if k, _ := r.best(ob, r.vanishing(), MoveSimilarity, func(k *Known) bool {
			return k.Snap != nil && k.Snap.Origin != nil && k.Snap.Origin.InstanceID == ob.Origin.InstanceID
		}); k != nil {
			assigned[i] = r.match(ob, k, "R3")
			continue
		}
		lineage := ""
		if in.Lineages[ob.Origin.LineageID] {
			lineage = ob.Origin.LineageID
		} else {
			for _, k := range in.Known {
				if k.ID == ob.Origin.InstanceID {
					lineage = k.LineageID
				}
			}
		}
		a := r.create(ob, "R5", lineage, BasisConfirmed)
		r.event(a, EvCopied, fmt.Sprintf("origin #%s (.rimarcho/origin.json)", ob.Origin.InstanceID), 0)
		assigned[i] = a
	}
	// R3: moved — a vanished Instance with similar content and the same declaration.
	for i, ob := range in.Obs {
		if assigned[i] != nil {
			continue
		}
		if k, _ := r.best(ob, r.vanishing(), MoveSimilarity, declMatch(ob)); k != nil {
			assigned[i] = r.match(ob, k, "R3")
		}
	}
	// R4 / R6 / R7, observation by observation.
	for i, ob := range in.Obs {
		if assigned[i] != nil {
			continue
		}
		if orig, _ := r.bestPresent(ob); orig != nil {
			a := r.create(ob, "R4", orig.LineageID, BasisInferred)
			r.event(a, EvCopied, "inferred copy of #"+orig.InstanceID, 0)
			assigned[i] = a
			continue
		}
		var gone []*Known
		for _, k := range in.Known {
			if !k.seen && !in.Skip[k.SourceID] {
				gone = append(gone, k)
			}
		}
		k, score := r.best(ob, gone, QuestionSimilarity, nil)
		rule := "R7"
		if k != nil {
			rule = "R6"
		}
		a := r.create(ob, rule, "", BasisRoot)
		r.event(a, EvDiscovered, "src:"+ob.Loc.Label+" "+ob.Loc.Rel, 0)
		if k != nil {
			r.res.Questions = append(r.res.Questions, &Question{New: a, Candidate: k, Score: score})
		}
		assigned[i] = a
	}
	for _, k := range in.Known {
		if !k.seen && k.Status == Live && !in.Skip[k.SourceID] {
			r.res.Disappeared = append(r.res.Disappeared, k)
			ev := &Event{InstanceID: k.ID, Kind: EvDisappeared, Detail: "src:" + k.Label + " " + k.Rel}
			if k.Snap != nil {
				ev.Before = k.Snap.ID
			}
			r.res.Events = append(r.res.Events, ev)
		}
	}
	for _, a := range assigned {
		r.res.Assignments = append(r.res.Assignments, a)
	}
	return r.res
}

// vanishing returns live Instances that have not appeared (yet) in this scan.
func (r *reconciler) vanishing() []*Known {
	var out []*Known
	for _, k := range r.in.Known {
		if !k.seen && k.Status == Live && !r.in.Skip[k.SourceID] {
			out = append(out, k)
		}
	}
	return out
}

func (r *reconciler) sem(k *Known) scan.SemanticSet {
	if s, ok := r.semCache[k]; ok {
		return s
	}
	var s scan.SemanticSet
	if r.in.Semantic != nil {
		s = r.in.Semantic(k)
	}
	r.semCache[k] = s
	return s
}

func declMatch(ob *scan.Observation) func(k *Known) bool {
	return func(k *Known) bool {
		if k.Snap == nil {
			return false
		}
		return SameDecl(ob.Decl.PackageID, ob.WorkshopID, k.Snap.Decl.PackageID, k.Snap.WorkshopID)
	}
}

// SameDecl reports whether two mods declare the same packageId or the same workshop ID.
func SameDecl(pkgA, wsA, pkgB, wsB string) bool {
	return pkgA != "" && strings.EqualFold(pkgA, pkgB) || wsA != "" && wsA == wsB
}

// best returns the candidate with the highest similarity ≥ min that passes ok.
func (r *reconciler) best(ob *scan.Observation, cands []*Known, min float64, ok func(*Known) bool) (*Known, float64) {
	var best *Known
	bestS := -1.0
	for _, k := range cands {
		if ok != nil && !ok(k) {
			continue
		}
		s := scan.Similarity(ob.FP.Semantic, r.sem(k))
		if s >= min && s > bestS {
			best, bestS = k, s
		}
	}
	return best, bestS
}

// bestPresent finds an Instance present in this scan that ob is an inferred copy of (R4).
func (r *reconciler) bestPresent(ob *scan.Observation) (*Assignment, float64) {
	var best *Assignment
	bestS := -1.0
	for _, a := range r.present {
		if !SameDecl(ob.Decl.PackageID, ob.WorkshopID, a.Obs.Decl.PackageID, a.Obs.WorkshopID) {
			continue
		}
		s := scan.Similarity(ob.FP.Semantic, a.Obs.FP.Semantic)
		if s >= CopySimilarity && s > bestS {
			best, bestS = a, s
		}
	}
	return best, bestS
}

func (r *reconciler) match(ob *scan.Observation, k *Known, rule string) *Assignment {
	k.seen = true
	a := &Assignment{Obs: ob, Known: k, InstanceID: k.ID, LineageID: k.LineageID, Rule: rule, Prev: k.Snap}
	a.NeedSnapshot = k.Snap == nil || k.Snap.Hash != ob.FP.SnapshotHash ||
		k.Snap.GitHead != ob.GitHead || k.Snap.ACFManifest != acfManifest(ob) || !sameOrigin(k.Snap.Origin, ob.Origin)
	r.present = append(r.present, a)
	before := int64(0)
	if k.Snap != nil {
		before = k.Snap.ID
	}
	if k.Status == Tombstone {
		r.event(a, EvReappeared, "src:"+ob.Loc.Label+" "+ob.Loc.Rel, before)
	}
	if rule != "R1" {
		r.event(a, EvMoved, fmt.Sprintf("src:%s %s → src:%s %s", k.Label, k.Rel, ob.Loc.Label, ob.Loc.Rel), before)
	}
	if k.Snap != nil && k.Snap.Hash != ob.FP.SnapshotHash {
		r.event(a, EvUpdated, "", before)
		r.fieldEvents(a, &k.Snap.Decl, &ob.Decl, before)
	}
	return a
}

func (r *reconciler) create(ob *scan.Observation, rule, lineage, basis string) *Assignment {
	a := &Assignment{Obs: ob, InstanceID: r.in.NewID(), Rule: rule, NeedSnapshot: true, LineageBasis: basis}
	if lineage == "" {
		a.LineageID, a.NewLineage, a.LineageBasis = r.in.NewID(), true, BasisRoot
	} else {
		a.LineageID = lineage
	}
	r.present = append(r.present, a)
	return a
}

func (r *reconciler) event(a *Assignment, kind, detail string, before int64) {
	r.res.Events = append(r.res.Events, &Event{InstanceID: a.InstanceID, Kind: kind, Detail: detail, Before: before, After: a})
}

// fieldEvents emits the per-field change events of 02 §5.3.
func (r *reconciler) fieldEvents(a *Assignment, old, cur *scan.Decl, before int64) {
	if old.PackageID != cur.PackageID {
		r.event(a, EvPackageIDChanged, fmt.Sprintf("pkg:%s → pkg:%s", old.PackageID, cur.PackageID), before)
	}
	if old.Name != cur.Name {
		r.event(a, EvRenamed, fmt.Sprintf("%q → %q", old.Name, cur.Name), before)
	}
	if !slices.Equal(old.Authors, cur.Authors) {
		r.event(a, EvAuthorChanged, fmt.Sprintf("%s → %s", strings.Join(old.Authors, ", "), strings.Join(cur.Authors, ", ")), before)
	}
	if !slices.Equal(old.SupportedVersions, cur.SupportedVersions) {
		r.event(a, EvVersionSupport, fmt.Sprintf("%s → %s", strings.Join(old.SupportedVersions, " "), strings.Join(cur.SupportedVersions, " ")), before)
	}
	if od, cd := old.AllDeps(), cur.AllDeps(); !slices.Equal(od, cd) {
		r.event(a, EvDependencies, diffList(od, cd), before)
	}
	if old.LoadRulesKey() != cur.LoadRulesKey() {
		r.event(a, EvLoadRules, "", before)
	}
}

func diffList(old, cur []string) string {
	var parts []string
	for _, c := range cur {
		if !slices.Contains(old, c) {
			parts = append(parts, "+pkg:"+c)
		}
	}
	for _, o := range old {
		if !slices.Contains(cur, o) {
			parts = append(parts, "-pkg:"+o)
		}
	}
	return strings.Join(parts, " ")
}

func acfManifest(ob *scan.Observation) string {
	if ob.ACF == nil {
		return ""
	}
	return ob.ACF.Manifest
}

func sameOrigin(a, b *scan.Origin) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
