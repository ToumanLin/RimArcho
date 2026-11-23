package app

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"
)

// SaveReview is the playthrough review (12 §3): for every installed Mod some colony ran
// with, what it adds by category and how much of that the colony's saves show.
type SaveReview struct {
	Colonies    int // colonies looked at
	Saves       int // their saves counted
	Unread      int // their saves not counted: not read yet, or read by an older parser
	Done, Total int // saves processed, including unread saves
	Cached      int // per-save attributions merged without reading facts or attributing again
	Mods        []ReviewMod
}

// ReviewMod is one installed copy. Mods with something to see come first, the lowest share
// seen first; then the functional ones.
type ReviewMod struct {
	Name, PackageID, Library string
	Game                     bool // Core or a DLC
	Colonies, Saves          int  // counted colonies and saves that had it enabled
	// Functional: it adds nothing in a category a save can show (Detected), or the player
	// marked it so (Marked), and saves cannot tell whether it was used (12 §3.3).
	Functional bool
	Detected   bool
	// Marked is the player's mark: MarkFunctional, MarkNotFunctional or "" (none).
	Marked     string
	Adds, Seen int // over Cats
	Cats       []ReviewCat
	// Configured are the names of its content that no counted save has but some only sets
	// up (a bill, a quest part, research not begun; 12 §3.2).
	Configured []string
}

// The player's marks on a Mod (12 §3.3 rule 2).
const (
	MarkFunctional    = "functional"
	MarkNotFunctional = "not functional"
)

// The groups of 12 §3.1, by what the saves show of a Mod's content.
const (
	ReviewSeen       = "seen"       // some of it is in a counted save
	ReviewConfigured = "configured" // none is, but a save sets some of it up
	ReviewNeverSeen  = "never-seen" // neither
	ReviewFunctional = "functional" // nothing a save can show, or marked so
)

// Group is which group of 12 §3.1 the Mod is in.
func (m *ReviewMod) Group() string {
	switch {
	case m.Functional:
		return ReviewFunctional
	case m.Seen > 0:
		return ReviewSeen
	case len(m.Configured) > 0:
		return ReviewConfigured
	}
	return ReviewNeverSeen
}

// ReviewCat is one category of a Mod's content. Seen counts the names it adds that some
// counted save has; NotSeen lists the others.
type ReviewCat struct {
	Key        string
	Adds, Seen int
	Under      bool // a save undercounts this category (12 §2.3)
	NotSeen    []string
}

// reviewSkip are the categories the review does not count: nothing in a save shows a recipe
// or the original scenario, and "other" is mechanics, not content to play (12 §3.2).
var reviewSkip = map[string]bool{"recipes": true, "scenarios": true, "other": true}

// ReviewOpts are how SaveReview runs. Read: a save without a current full read is read now
// (a save read by an older parser lacks what the review counts, and the index keeps such a
// read: 12 §1.7); otherwise it is counted under Unread, as is one that cannot be read.
// Progress is called after each save. Stop, when it says true, ends the review with
// context.Canceled.
type ReviewOpts struct {
	Read     bool
	Progress func(done, total int)
	// Partial receives independent snapshots at most every 500 ms, after merging a save.
	Partial func(*SaveReview)
	Stop    func() bool
}

// SaveReview uses the cached reads, and reads saves only with ReviewOpts.Read.
func (a *App) SaveReview(cols []*Colony, o ReviewOpts) (*SaveReview, error) {
	out := &SaveReview{Colonies: len(cols)}
	catalog := map[string]map[string][]string{}
	src := &defSource{}
	catalogs := &reviewCatalogs{}
	mods := map[string]*reviewModState{}
	marks, err := a.Store.FunctionalByInstance()
	if err != nil {
		return nil, err
	}
	// Copy only the ordering slices, never the save facts. Callers may supply a filtered
	// listing; attribution still follows the Saves page's newest-first order.
	cols = slices.Clone(cols)
	sort.SliceStable(cols, func(i, j int) bool { return cols[i].Latest().ModTime.After(cols[j].Latest().ModTime) })
	for _, c := range cols {
		out.Total += len(c.Saves)
	}
	input, err := a.ReviewInputKey()
	if err != nil {
		return nil, err
	}
	// A review only reads: the rows of saves that are gone are tidied by the index (archo
	// saves index), and obsolete review rows only when this run writes one (reviewSave).
	run := &reviewRun{input: input, src: src, catalogs: catalogs, lists: map[int64]string{}}
	// Building a snapshot walks the accumulated catalog, not the saves. Facts and each
	// compact result fall out of scope before the next save.
	snapshot := func() *SaveReview { return reviewSnapshot(out, mods, catalog, marks) }
	sent := time.Now()
	for _, c := range cols {
		saves := slices.Clone(c.Saves)
		sort.SliceStable(saves, func(i, j int) bool { return saves[i].ModTime.After(saves[j].ModTime) })
		for _, sv := range saves {
			if o.Stop != nil && o.Stop() {
				return nil, context.Canceled
			}
			res, cached, err := a.reviewSave(sv, o.Read, run)
			if err != nil {
				return nil, err
			}
			out.Done++
			if cached {
				out.Cached++
			}
			if !res.Read {
				out.Unread++
			} else {
				out.Saves++
				for inst, cats := range res.Catalog {
					if catalog[inst] == nil {
						catalog[inst] = cats
					}
				}
				for _, m := range res.Mods {
					if m.Instance == "" || res.Catalog[m.Instance] == nil {
						continue // not installed, or not analysed: nothing to compare with
					}
					st := mods[m.Instance]
					if st == nil {
						st = &reviewModState{colonies: map[*Colony]bool{}, seen: map[string]map[string]bool{}, config: map[string]bool{}}
						st.Name, st.PackageID, st.Library = m.Name, m.PackageID, m.Library
						st.Game = strings.HasPrefix(normPkg(m.PackageID), "ludeon.rimworld")
						mods[m.Instance] = st
					}
					st.Saves++
					st.colonies[c] = true
					for _, n := range res.Config[m.Instance] {
						st.config[n] = true
					}
					for key, ns := range res.Names[m.Instance] {
						if st.seen[key] == nil {
							st.seen[key] = map[string]bool{}
						}
						for _, n := range ns {
							st.seen[key][n] = true
						}
					}
				}
			}
			if o.Progress != nil {
				o.Progress(out.Done, out.Total)
			}
			if o.Stop != nil && o.Stop() {
				return nil, context.Canceled
			}
			if o.Partial != nil && out.Done < out.Total && time.Since(sent) >= 500*time.Millisecond {
				o.Partial(snapshot())
				sent = time.Now()
			}
		}
	}
	return snapshot(), nil
}

type reviewModState struct {
	ReviewMod
	colonies map[*Colony]bool
	seen     map[string]map[string]bool
	config   map[string]bool // names some save only sets up
}

func reviewSnapshot(counts *SaveReview, mods map[string]*reviewModState, catalog map[string]map[string][]string, marks map[string]bool) *SaveReview {
	out := *counts
	out.Mods = nil
	for inst, st := range mods {
		m := st.ReviewMod
		m.Colonies = len(st.colonies)
		for _, key := range ContentCategoryOrder {
			names := catalog[inst][key]
			if reviewSkip[key] || len(names) == 0 {
				continue
			}
			cat := ReviewCat{Key: key, Adds: len(names), Under: contentUnder(key)}
			for _, n := range names {
				if st.seen[key][n] {
					cat.Seen++
				} else {
					cat.NotSeen = append(cat.NotSeen, n)
					if st.config[n] {
						m.Configured = append(m.Configured, n)
					}
				}
			}
			m.Adds += cat.Adds
			m.Seen += cat.Seen
			m.Cats = append(m.Cats, cat)
		}
		sort.Strings(m.Configured)
		m.Configured = slices.Compact(m.Configured)
		m.Detected = m.Adds == 0
		m.Functional = m.Detected
		if mark, ok := marks[inst]; ok {
			m.Functional = mark
			m.Marked = MarkNotFunctional
			if mark {
				m.Marked = MarkFunctional
			}
		}
		out.Mods = append(out.Mods, m)
	}
	sort.Slice(out.Mods, func(i, j int) bool {
		x, y := out.Mods[i], out.Mods[j]
		if x.Functional != y.Functional {
			return y.Functional
		}
		// x.Seen/x.Adds < y.Seen/y.Adds, without dividing
		if l, r := x.Seen*y.Adds, y.Seen*x.Adds; l != r {
			return l < r
		}
		if x.Adds != y.Adds {
			return x.Adds > y.Adds
		}
		if x.Name != y.Name {
			return x.Name < y.Name
		}
		return x.Library < y.Library
	})
	return &out
}

// contentCatalog fills catalog for the analysed copies among insts it lacks: by category,
// the names of the Defs each provides when loaded with the save's Mods (active), where of
// several copies defining a Def the last in the save's order (pos) has it, as in the
// attribution. A pawn kind is its race, not content of its own (12 §2.2).
func (a *App) contentCatalog(catalog map[string]map[string][]string, src *defSource, insts []string, ready map[string]bool, pos map[string]int, active map[string]bool) error {
	need := map[string]bool{}
	var list []string
	for _, inst := range insts {
		if ready[inst] && catalog[inst] == nil {
			need[inst] = true
			list = append(list, inst)
		}
	}
	if len(list) == 0 {
		return nil
	}
	if src == nil {
		src = &defSource{}
	}
	if err := src.ensure(a, insts); err != nil {
		return err
	}
	lookup := map[string]bool{"Normal": true, "Mechanoid": true}
	for _, inst := range list {
		for _, n := range src.defName[inst] {
			lookup[n] = true
		}
	}
	r, err := a.contentResolver(src, insts, pos, active, lookup)
	if err != nil {
		return err
	}
	for _, inst := range list {
		catalog[inst] = map[string][]string{}
	}
	had := map[string]bool{}
	for name := range r.defs {
		for _, n := range r.winners(name, "") {
			inst := n.fact.Instance
			if !need[inst] || n.fact.Type == "PawnKindDef" {
				continue
			}
			fields, ok := r.resolve(n, 0)
			if isMinified(fields) {
				continue
			}
			key, _, _ := r.category(n, fields, ok)
			if id := inst + "|" + key + "|" + name; !had[id] {
				had[id] = true
				catalog[inst][key] = append(catalog[inst][key], name)
			}
		}
	}
	for _, inst := range list {
		for _, ns := range catalog[inst] {
			sort.Strings(ns)
		}
	}
	return nil
}
