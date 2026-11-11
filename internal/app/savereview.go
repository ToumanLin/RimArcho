package app

import (
	"sort"
	"strings"
)

// SaveReview is the playthrough review (12 §3): for every installed Mod some colony ran
// with, what it adds by category and how much of that the colony's saves show.
type SaveReview struct {
	Colonies int // colonies looked at
	Saves    int // their saves counted
	Unread   int // their saves not counted: not read yet, or read by an older parser
	Mods     []ReviewMod
}

// ReviewMod is one installed copy. Mods with something to see come first, the lowest share
// seen first; then the functional ones.
type ReviewMod struct {
	Name, PackageID, Library string
	Game                     bool // Core or a DLC
	Colonies, Saves          int  // counted colonies and saves that had it enabled
	// Functional: it adds nothing in a category a save can show, so saves cannot tell
	// whether it was used (12 §3.3).
	Functional bool
	Adds, Seen int // over Cats
	Cats       []ReviewCat
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

// SaveReview uses the cached reads. A save without a current one is read now when read is
// set (a save read by an older parser lacks what the review counts, and the index keeps
// such a read: 12 §1.7), and is otherwise counted under Unread, as is one that cannot be
// read. progress, when not nil, is called after each save.
func (a *App) SaveReview(cols []*Colony, read bool, progress func(done, total int)) (*SaveReview, error) {
	type modState struct {
		ReviewMod
		colonies map[*Colony]bool
		seen     map[string]map[string]bool // category → name
	}
	out := &SaveReview{Colonies: len(cols)}
	catalog := map[string]map[string][]string{}
	src := &defSource{}
	mods := map[string]*modState{}
	total, done := 0, 0
	for _, c := range cols {
		total += len(c.Saves)
	}
	for _, c := range cols {
		for _, sv := range c.Saves {
			if read {
				a.SaveFacts(sv, true) // a failure leaves it unread
			}
			res, err := a.saveContent(sv, contentOpts{seen: true, catalog: catalog, src: src})
			if err != nil {
				return nil, err
			}
			done++
			if progress != nil {
				progress(done, total)
			}
			if !res.Read || sv.Head == nil {
				out.Unread++
				continue
			}
			out.Saves++
			for _, m := range res.mods {
				if m.Instance == "" || catalog[m.Instance] == nil {
					continue // not installed, or not analysed: nothing to compare with
				}
				st := mods[m.Instance]
				if st == nil {
					st = &modState{colonies: map[*Colony]bool{}, seen: map[string]map[string]bool{}}
					st.Name, st.PackageID, st.Library = m.Name, m.PackageID, m.Library
					st.Game = strings.HasPrefix(normPkg(m.PackageID), "ludeon.rimworld")
					mods[m.Instance] = st
				}
				st.Saves++
				st.colonies[c] = true
				for key, ns := range res.names[m.Instance] {
					if st.seen[key] == nil {
						st.seen[key] = map[string]bool{}
					}
					for _, n := range ns {
						st.seen[key][n.Name] = true
					}
				}
			}
		}
	}
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
				}
			}
			m.Adds += cat.Adds
			m.Seen += cat.Seen
			m.Cats = append(m.Cats, cat)
		}
		m.Functional = m.Adds == 0
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
	return out, nil
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
