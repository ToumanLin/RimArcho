package app

import (
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/savefile"
)

// What a save needs of each of its Mods (12 §4.7): what removing that one Mod would cost,
// whatever list the save is checked against. The verdicts follow Can it load?'s levels
// (12 §4.3): breaks is what makes a missing Mod's row an error there, loses a warning.
const (
	NeedBreaks    = "breaks"    // pawns, factions, classes or a map's weather: the save may not load
	NeedLoses     = "loses"     // things in the world: the save loads without them
	NeedCare      = "care"      // only marks: research, recipes, appearance and the like reset
	NeedSafe      = "safe"      // nothing of it is in the save
	NeedUnchecked = "unchecked" // not installed or not analysed: nothing can be said
)

// NeedOrder lists the verdicts, heaviest first.
var NeedOrder = []string{NeedBreaks, NeedLoses, NeedCare, NeedSafe, NeedUnchecked}

// Why a Mod is NeedUnchecked.
const (
	NeedWhyGone   = "not installed"
	NeedWhyNoDefs = "not analysed for this game version"
	NeedWhyNoCode = "its code is not analysed"
	// required by a Mod that is itself not checked
	NeedWhyDependent = "a Mod that needs it could not be checked"
)

// SaveNeed is one Mod of the save.
type SaveNeed struct {
	Name, PackageID, Library string
	Game                     bool // Core or a DLC: part of the game, never removed
	Verdict                  string
	Why                      string // NeedUnchecked: one of the NeedWhy texts

	Colonists, Dead int      // the player's humanlike pawns of its races, alive and dead
	Factions        []string // factions of its FactionDefs: name, else the Def
	Classes         []string // Class="…" values only its assemblies define among the save's Mods
	Weather         []string // its WeatherDefs a map has now or had last
	Defs            int      // distinct Defs of it the save references
	// NeededBy are the save's other Mods whose About.xml requires this one, by name: its
	// verdict is at least the heaviest of theirs, since they do not run without it.
	NeededBy []string
	Cats     []SaveContentCategory
}

// needWorld are the content categories whose Defs are objects in the world, plus culture:
// a precept whose Def is gone leaves the ideology broken rather than reset.
var needWorld = map[string]bool{
	"factions": true, "races": true, "animals": true, "mechanoids": true, "creatures": true,
	"buildings": true, "weapons": true, "apparel": true, "plants": true, "items": true,
	"terrain": true, "culture": true,
}

// Severe: what the game cannot load the save without.
func (n *SaveNeed) Severe() bool {
	return n.Colonists+n.Dead > 0 || len(n.Factions) > 0 || len(n.Classes) > 0 || len(n.Weather) > 0
}

func (n *SaveNeed) verdict() string {
	if n.Severe() {
		return NeedBreaks
	}
	care := false
	for _, c := range n.Cats {
		if c.Key == "factions" { // a FactionDef the save names with no faction of it left
			return NeedBreaks
		}
		if needWorld[c.Key] {
			return NeedLoses
		}
		care = true
	}
	if care {
		return NeedCare
	}
	return NeedSafe
}

// needOwners is what the content attribution found besides the categories: which copy
// provides each race, faction Def and weather the save names.
type needOwners struct {
	things   map[string]string // ThingDef name → instance
	factions map[string]string // FactionDef name → instance
	weather  map[string][]string
}

// saveNeeds gives every Mod of the save its verdict. cats are the content categories by
// instance; ready the instances with content facts.
func (a *App) saveNeeds(f *savefile.Facts, mods []SaveMod, ready map[string]bool, cats map[string][]SaveContentCategory, own needOwners, deps map[string][]scanDep) ([]SaveNeed, error) {
	var insts []string
	for _, m := range mods {
		if m.Instance != "" {
			insts = append(insts, m.Instance)
		}
	}
	classes, code, err := a.saveClassOwners(f, insts)
	if err != nil {
		return nil, err
	}
	type pawns struct{ alive, dead int }
	colonists := map[string]*pawns{}
	for _, p := range f.Colonists {
		inst := own.things[p.Race]
		if inst == "" {
			continue
		}
		c := colonists[inst]
		if c == nil {
			c = &pawns{}
			colonists[inst] = c
		}
		if p.Dead {
			c.dead++
		} else {
			c.alive++
		}
	}
	factions := map[string][]string{}
	for _, fa := range f.Factions {
		if inst := own.factions[fa.Def]; inst != "" {
			name := fa.Name
			if name == "" {
				name = fa.Def
			}
			factions[inst] = append(factions[inst], name)
		}
	}
	out := make([]SaveNeed, 0, len(mods))
	for _, m := range mods {
		n := SaveNeed{Name: m.Name, PackageID: m.PackageID, Library: m.Library, Game: strings.HasPrefix(normPkg(m.PackageID), "ludeon.rimworld")}
		switch {
		case m.Instance == "":
			n.Verdict, n.Why = NeedUnchecked, NeedWhyGone
		case !ready[m.Instance]:
			n.Verdict, n.Why = NeedUnchecked, NeedWhyNoDefs
		default:
			n.Cats = cats[m.Instance]
			for _, c := range n.Cats {
				n.Defs += c.Count
			}
			if c := colonists[m.Instance]; c != nil {
				n.Colonists, n.Dead = c.alive, c.dead
			}
			n.Factions = factions[m.Instance]
			n.Classes = classes[m.Instance]
			n.Weather = own.weather[m.Instance]
			sort.Strings(n.Weather)
			n.Verdict = n.verdict()
			// Without its type facts a class of its own would go unseen: only what is
			// already known to break the save can be said.
			if n.Verdict != NeedBreaks && !code[m.Instance] && !n.Game {
				n.Verdict, n.Why = NeedUnchecked, NeedWhyNoCode
			}
		}
		out = append(out, n)
	}
	needDependents(out, mods, deps)
	return out, nil
}

// needRank orders the verdicts for what a Mod takes from the Mods that require it: an
// unchecked dependent outweighs safe, since nothing can be said of it.
var needRank = map[string]int{NeedBreaks: 5, NeedLoses: 4, NeedCare: 3, NeedUnchecked: 2, NeedSafe: 1}

// needDependents raises a Mod's verdict to the heaviest of the save's Mods that require it
// (About.xml modDependencies for the save's game version, as 08 reads them), through chains
// of requirements: without it they do not run, so what they hold in the save goes with it.
// A requirement several of the save's Mods can meet (alternatives) binds none of them. A
// Mod required only by Mods safe to remove is NeedCare, not safe: removing it alone leaves
// them broken. The game's own Mods are left as they are.
func needDependents(out []SaveNeed, mods []SaveMod, deps map[string][]scanDep) {
	at := map[string]int{}
	for i, m := range mods {
		if p := dependencyPackage(m.PackageID); p != "" {
			if _, ok := at[p]; !ok {
				at[p] = i
			}
		}
	}
	by := make([][]int, len(mods)) // the Mods requiring each one
	for j, m := range mods {
		seen := map[int]bool{}
		for _, d := range deps[m.Instance] {
			met := -1
			for _, p := range append([]string{d.pkg}, d.alts...) {
				i, ok := at[dependencyPackage(p)]
				if !ok || i == met {
					continue
				}
				if met >= 0 { // two of the save's Mods meet it
					met = -2
					break
				}
				met = i
			}
			if met < 0 || met == j || seen[met] || out[met].Game {
				continue
			}
			seen[met] = true
			by[met] = append(by[met], j)
			out[met].NeededBy = append(out[met].NeededBy, out[j].Name)
		}
	}
	for changed := true; changed; {
		changed = false
		for i := range out {
			for _, j := range by[i] {
				v := out[j].Verdict
				if v == NeedSafe {
					v = NeedCare
				}
				if needRank[v] > needRank[out[i].Verdict] {
					out[i].Verdict, changed = v, true
					if v == NeedUnchecked {
						out[i].Why = NeedWhyDependent
					} else {
						out[i].Why = ""
					}
				}
			}
		}
	}
}

// saveClassOwners gives, by instance, the save's Class="…" values that only that copy
// defines among insts: a class the game, or another of the save's Mods, also defines is not
// lost with it. code says which instances have type facts (05 §3).
func (a *App) saveClassOwners(f *savefile.Facts, insts []string) (map[string][]string, map[string]bool, error) {
	m := a.ownerMemo()
	defer m.mu.Unlock()
	if err := m.loadViews(a.Store, insts); err != nil {
		return nil, nil, err
	}
	if m.game == 0 {
		ok, err := a.Store.GameTypesIndexed()
		if err != nil {
			return nil, nil, err
		}
		m.game = map[bool]int8{true: 1, false: -1}[ok]
	}
	classes := make([]string, 0, len(f.Classes))
	for c := range f.Classes {
		if c != "" {
			classes = append(classes, c)
		}
	}
	sort.Strings(classes)
	if err := m.loadClasses(a.Store, classes, m.game > 0); err != nil {
		return nil, nil, err
	}
	in, code := map[string]bool{}, map[string]bool{}
	for _, i := range insts {
		in[i] = true
		code[i] = m.views[i].DLLs
	}
	owned := map[string][]string{}
	for _, c := range classes {
		for _, p := range classParts(c) {
			ci := m.classes[p]
			if ci.builtin {
				continue
			}
			only := ""
			for _, o := range ci.owners {
				if !in[o] {
					continue
				}
				if only != "" && only != o {
					only = ""
					break
				}
				only = o
			}
			if only != "" {
				owned[only] = append(owned[only], c)
			}
		}
	}
	for i, cs := range owned {
		owned[i] = dedupe(cs)
	}
	return owned, code, nil
}
