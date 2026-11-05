package app

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/ToumanLin/RimArcho/internal/savefile"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// What a Mod owns in a save (10 §4.6): the save's objects that need a Mod because nothing
// else the list loads provides them. Def references go to the Mods defining them (10 §1.5),
// Class="…" values to the Mods whose assemblies define the type (store.TypeDefiners) unless
// the game does (store.GameClassOwners), the player's pawns by their race Def and factions
// by their FactionDef.
//
// Which facts exist (03 §3): Def facts for every loadable installed Mod, once an analysis of
// this game version has run; type facts only for Mods enabled when an analysis ran (a
// disabled Mod keeps them while its content is unchanged); nothing for a Mod that is not
// installed. So what a Mod that is not installed owns is inferred by elimination: the
// Defs no installed Mod defines and the classes no installed assembly or the game defines.
// The same holds for a disabled Mod without type facts, for its classes.

// SaveOwned is what a Mod (or, when inferred, a group of Mods) owns in a save.
type SaveOwned struct {
	Colonists []OwnedRace `json:"colonists,omitempty"` // the player's humanlike pawns, by race
	Factions  []string    `json:"factions,omitempty"`  // factions of its FactionDefs: name, else the Def
	Classes   []string    `json:"classes,omitempty"`   // Class="…" values of its types, sorted
	Defs      []OwnedDefs `json:"defs,omitempty"`      // other Defs the save uses, by Def type, most used first
	// Inferred: some of this is the Mod's only by elimination (it has no facts here).
	// Among: that part may belong to this many other Mods without facts too.
	Inferred bool `json:"inferred,omitempty"`
	Among    int  `json:"among,omitempty"`
	// NoFacts: nothing is known of the Mod itself (never installed, or not analysed).
	NoFacts bool `json:"noFacts,omitempty"`
	// What could not be checked because the save's Mods or the game are not indexed for
	// this game version (03 §3, §5.3): Defs (and so pawns and factions), or classes.
	DefsUnchecked    bool `json:"defsUnchecked,omitempty"`
	ClassesUnchecked bool `json:"classesUnchecked,omitempty"`
}

// OwnedRace counts the player's pawns of one race.
type OwnedRace struct {
	Race  string `json:"race"` // the race's ThingDef
	Alive int    `json:"alive"`
	Dead  int    `json:"dead,omitempty"`
}

// OwnedDefs counts the Defs of one type.
type OwnedDefs struct {
	Type string `json:"type,omitempty"` // Def type, e.g. ThingDef; "" when inferred (no facts)
	Defs int    `json:"defs"`           // distinct defNames
	Uses int    `json:"uses"`           // <def> elements naming them: about one per object
	// Names are the defNames, most used first, at most OwnedDefNames of them.
	Names []string `json:"names,omitempty"`

	uses map[string]int // defName → uses while attributing; Names once finished
}

// OwnedDefNames bounds OwnedDefs.Names: a missing Mod can own thousands of Defs, and the
// interfaces show the most used ones (06 §10).
const OwnedDefNames = 100

func (d *OwnedDefs) note(name string, uses int) {
	if d.uses == nil {
		d.uses = map[string]int{}
	}
	d.uses[name] += uses
}

// finish turns the counted names into Names.
func (d *OwnedDefs) finish() {
	if d.uses == nil {
		return
	}
	names := make([]string, 0, len(d.uses))
	for n := range d.uses {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if a, b := d.uses[names[i]], d.uses[names[j]]; a != b {
			return a > b
		}
		return names[i] < names[j]
	})
	d.Names, d.uses = names[:min(len(names), OwnedDefNames)], nil
}

// Severe: it owns pawns, factions or classes, which the game cannot load without it (10
// §4.3: such a Mod's row is an error).
func (o *SaveOwned) Severe() bool {
	return o != nil && (len(o.Colonists) > 0 || len(o.Factions) > 0 || len(o.Classes) > 0)
}

// Empty: nothing in the save is known to need it.
func (o *SaveOwned) Empty() bool { return o.empty() }

func (o *SaveOwned) empty() bool { return !o.Severe() && len(o.Defs) == 0 }

// Parts lists what it owns in English, most important first: "3 colonists of race
// Kurin_Race (1 dead)", "1 faction (Kurin Clan)", "4 classes", "120 things".
func (o *SaveOwned) Parts() []string {
	if o == nil {
		return nil
	}
	var out []string
	for _, r := range o.Colonists {
		p := plural(r.Alive+r.Dead, "colonist") + " of race " + r.Race
		if r.Dead > 0 {
			p += fmt.Sprintf(" (%d dead)", r.Dead)
		}
		out = append(out, p)
	}
	if n := len(o.Factions); n > 0 {
		out = append(out, plural(n, "faction")+" ("+strings.Join(headList(o.Factions, 3), ", ")+")")
	}
	if n := len(o.Classes); n > 0 {
		out = append(out, plural(n, "class")+" ("+strings.Join(headList(o.Classes, 3), ", ")+")")
	}
	for i, d := range o.Defs {
		if i == 3 {
			rest := 0
			for _, e := range o.Defs[3:] {
				rest += e.Uses
			}
			out = append(out, fmt.Sprintf("%d more Def uses", rest))
			break
		}
		out = append(out, DefUsesText(d))
	}
	return out
}

// DefUsesText is one OwnedDefs in English: "120 things", "2 hediffs", "7 Def uses".
func DefUsesText(d OwnedDefs) string {
	if d.Type == "" {
		return plural(d.Uses, "Def use")
	}
	return plural(d.Uses, DefTypeNoun(d.Type))
}

// DefTypeNoun names a Def type's objects: ThingDef → "thing", WorldObjectDef → "world
// object".
func DefTypeNoun(t string) string {
	t = strings.TrimSuffix(t, "Def")
	var b strings.Builder
	for i, r := range t {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteByte(' ')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	if b.Len() == 0 {
		return "Def"
	}
	return b.String()
}

func plural(n int, w string) string {
	switch {
	case n == 1:
		return "1 " + w
	case strings.HasSuffix(w, "s"):
		return fmt.Sprintf("%d %ses", n, w)
	case strings.HasSuffix(w, "y") && !strings.HasSuffix(w, "ey"):
		return fmt.Sprintf("%d %sies", n, w[:len(w)-1])
	}
	return fmt.Sprintf("%d %ss", n, w)
}

func headList(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return append(append([]string{}, s[:n]...), fmt.Sprintf("%d more", len(s)-n))
}

// missingClass is a class no loaded Mod nor the game defines and no Mod with a row of its
// own explains: Mod is the installed Mod defining it ("" unknown).
type missingClass struct {
	Class string
	Mod   SaveMod
}

// saveOwnership is the attribution of one save.
type saveOwnership struct {
	owned   map[int]*SaveOwned // by index into the save's Mods
	missing []missingClass     // sorted by Mod, then class

	// For group: what each Mod with a row owns by its own facts, the pool of what nothing
	// installed provides, and the rows the pool's Defs and classes were shared out to.
	alone     map[int]*SaveOwned
	pool      SaveOwned
	defPool   []int
	classPool []int
}

// ownerQuery is what attributing a save needs from CheckSave.
type ownerQuery struct {
	facts  *savefile.Facts
	mods   []SaveMod       // the save's Mods in load order
	issues []int           // indexes into mods of the Mods with a not-installed or disabled row
	loaded map[string]bool // the instances the game would load with the list
	// cover are the loaded instances whose facts the check needs: those the save's own Mods
	// load from. A list's Mod the save lacks stored nothing in it.
	cover map[string]bool
	names func(inst string) (pkg, name string)
}

// saveOwnerMemo keeps lookups across saves while the database is unchanged: the saves of
// one colony name mostly the same Defs and classes, and `saves list` checks them all.
type saveOwnerMemo struct {
	mu      sync.Mutex
	key     string
	defs    map[string][]store.DefDefiner // defName → definers (empty: none, implied ones resolved)
	classes map[string]classInfo
	views   map[string]store.FactView // instance → views ({} cached as absent)
	pkgView map[string]store.FactView // packageId → views of any copy (Mods not installed here)
	pkgs    map[string][2]string      // instance → packageId, name
	game    int8                      // game types indexed: 0 unknown, 1 yes, -1 no
}

type classInfo struct {
	builtin bool     // the game or the runtime defines it
	owners  []string // instances whose assemblies define it
}

// impliedPrefixes and impliedSuffixes name the Defs the game generates from other Defs at
// startup (DefGenerator.GenerateImpliedDefs_PreResolve, 1.6): no Def file defines them, so
// they resolve by the Def they are made from. A save holds them as things: corpses, meat,
// blueprints and frames, techprints, trainers, and the stone terrains.
var (
	impliedPrefixes = []string{"Blueprint_Install_", "Blueprint_", "Frame_", "Corpse_", "Meat_", "Techprint_", "Psytrainer_", "Neurotrainer_", "Make_", "Administer_"}
	impliedSuffixes = []string{"_RoughHewn", "_Rough", "_Smooth"}
)

func impliedBase(name string) string {
	for _, p := range impliedPrefixes {
		if strings.HasPrefix(name, p) && len(name) > len(p) {
			return name[len(p):]
		}
	}
	for _, s := range impliedSuffixes {
		if strings.HasSuffix(name, s) && len(name) > len(s) {
			return name[:len(name)-len(s)]
		}
	}
	return ""
}

// saveOwnership attributes a save for CheckSave: rows are the indexes into mods of the Mods
// with a row; mm is the game version the list's facts are read for; packInst the installed
// copies of a pack's Mods.
func (a *App) saveOwnership(s *Save, st *LoadoutState, mm string, f *savefile.Facts, mods []SaveMod, rows []int, packInst []string) (*saveOwnership, error) {
	loaded := map[string]bool{}
	cover := loaded
	switch {
	case s.Pack != "": // a pack's Mods resolve by packageId: its copies are all needed
		for _, inst := range packInst {
			loaded[inst] = true
		}
	case s.Plan != nil:
		for _, e := range st.List {
			loaded[e.Instance] = true
		}
		cover = map[string]bool{}
		for _, m := range mods {
			if loaded[m.Instance] {
				cover[m.Instance] = true
			}
		}
	default: // no list: what the save's own Mods load from
		for _, m := range mods {
			loaded[m.Instance] = true
		}
	}
	delete(loaded, "")
	delete(cover, "")
	names := func(inst string) (string, string) {
		if mod := st.Ix.ByID[inst]; mod != nil {
			return mod.PackageID, mod.DisplayName()
		}
		return "", ""
	}
	old := a.Store.MM
	a.Store.MM = mm
	defer func() { a.Store.MM = old }()
	return a.attributeSave(ownerQuery{facts: f, mods: mods, issues: rows, loaded: loaded, cover: cover, names: names})
}

// of is what the Mod at index i owns; nil when nothing could be checked.
func (o *saveOwnership) of(i int) *SaveOwned {
	if o == nil {
		return nil
	}
	w := o.owned[i]
	if w == nil || w.DefsUnchecked && w.ClassesUnchecked && w.empty() {
		return nil
	}
	return w
}

// group is what the Mods at rows own together (the interfaces' one row for the Mods that are
// not installed): what each owns by its own facts, plus the part nothing installed provides
// once rather than once per Mod. nil when nothing could be checked.
func (o *saveOwnership) group(rows []int) *SaveOwned {
	if o == nil || len(rows) == 0 {
		return nil
	}
	g, in, any := &SaveOwned{}, map[int]bool{}, false
	for _, i := range rows {
		in[i] = true
		w := o.owned[i]
		if w == nil {
			continue
		}
		any = true
		g.add(o.alone[i])
		g.DefsUnchecked = g.DefsUnchecked || w.DefsUnchecked
		g.ClassesUnchecked = g.ClassesUnchecked || w.ClassesUnchecked
	}
	if !any {
		return nil
	}
	shared := func(pool []int) (got bool, others int) {
		for _, i := range pool {
			if in[i] {
				got = true
			} else {
				others++
			}
		}
		return got, others
	}
	p := o.pool
	if got, others := shared(o.defPool); got && (len(p.Colonists) > 0 || len(p.Factions) > 0 || len(p.Defs) > 0) {
		g.add(&SaveOwned{Colonists: p.Colonists, Factions: p.Factions, Defs: p.Defs})
		g.Inferred, g.Among = true, max(g.Among, others)
	}
	if got, others := shared(o.classPool); got && len(p.Classes) > 0 {
		g.add(&SaveOwned{Classes: p.Classes})
		g.Inferred, g.Among = true, max(g.Among, others)
	}
	if g.DefsUnchecked && g.ClassesUnchecked && g.empty() {
		return nil
	}
	return g
}

// clone copies o and its lists.
func (o *SaveOwned) clone() *SaveOwned {
	c := *o
	c.Colonists = append([]OwnedRace(nil), o.Colonists...)
	c.Factions = append([]string(nil), o.Factions...)
	c.Classes = append([]string(nil), o.Classes...)
	c.Defs = nil
	for _, d := range o.Defs {
		d.Names = append([]string(nil), d.Names...)
		c.Defs = append(c.Defs, d)
	}
	return &c
}

// add merges what b owns into o: races and Def types summed, factions and classes joined.
func (o *SaveOwned) add(b *SaveOwned) {
	if b == nil {
		return
	}
	races := map[string]*OwnedRace{}
	for _, r := range b.Colonists {
		r := r
		races[r.Race] = &r
	}
	o.Colonists = mergeRaces(o.Colonists, races)
	o.Factions = append(o.Factions, b.Factions...)
	o.Classes = append(o.Classes, b.Classes...)
	sort.Strings(o.Classes)
	o.Classes = dedupe(o.Classes)
	if len(b.Defs) == 0 {
		return
	}
	byType := map[string]*OwnedDefs{}
	for _, d := range append(append([]OwnedDefs(nil), o.Defs...), b.Defs...) {
		e := byType[d.Type]
		if e == nil {
			e = &OwnedDefs{Type: d.Type}
			byType[d.Type] = e
		}
		e.Defs += d.Defs
		e.Uses += d.Uses
		e.Names = append(e.Names, d.Names...)
	}
	for _, e := range byType {
		e.Names = dedupe(e.Names)
		e.Names = e.Names[:min(len(e.Names), OwnedDefNames)]
	}
	o.Defs = sortedDefs(byType)
}

// missingRows are the class-missing rows: one per Mod defining the classes, then one for
// the classes no installed Mod defines.
func (o *saveOwnership) missingRows() []SaveIssue {
	if o == nil {
		return nil
	}
	var out []SaveIssue
	for i := 0; i < len(o.missing); {
		j := i
		var classes []string
		for j < len(o.missing) && o.missing[j].Mod.PackageID == o.missing[i].Mod.PackageID {
			classes = append(classes, o.missing[j].Class)
			j++
		}
		is := SaveIssue{Kind: IssueClassMissing, Level: LevelError, Text: strings.Join(headList(classes, 3), ", "),
			Owns: &SaveOwned{Classes: classes}}
		if m := o.missing[i].Mod; m.PackageID != "" {
			is.Mods = []SaveMod{m}
		}
		out = append(out, is)
		i = j
	}
	// The unknown group sorts first (""); put the named Mods first instead.
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].Mods) > len(out[j].Mods) })
	return out
}

// ownerMemoLimit bounds the memo (06 §10): far above one library's distinct Defs and
// classes (a real 1.6 save names about 1,400 Defs, 07 §6).
const ownerMemoLimit = 200_000

// ownerMemo returns the lookup memo for the database as it is now, locked; the caller
// unlocks it.
func (a *App) ownerMemo() *saveOwnerMemo {
	m := &a.saveOwners
	m.mu.Lock()
	if key := a.Store.ChangeKey() + "|" + a.Store.MM; key != m.key || len(m.defs)+len(m.classes) > ownerMemoLimit {
		m.key = key
		m.defs, m.classes = map[string][]store.DefDefiner{}, map[string]classInfo{}
		m.views, m.pkgs, m.game = map[string]store.FactView{}, map[string][2]string{}, 0
		m.pkgView = map[string]store.FactView{}
	}
	return m
}

// attributeSave attributes a save's content to its Mods (10 §4.6). Only the Mods with a
// row (q.issues) get what they own; classes nothing explains come back as missing.
func (a *App) attributeSave(q ownerQuery) (*saveOwnership, error) {
	m := a.ownerMemo()
	defer m.mu.Unlock()
	f := q.facts
	out := &saveOwnership{owned: map[int]*SaveOwned{}, alone: map[int]*SaveOwned{}}

	// Are the save's Mods in the list (and the game) indexed for this game version?
	cover := q.cover
	if cover == nil {
		cover = q.loaded
	}
	var covered []string
	for inst := range cover {
		covered = append(covered, inst)
	}
	if err := m.loadViews(a.Store, covered); err != nil {
		return nil, err
	}
	defsOK, dllOK := len(covered) > 0, len(covered) > 0
	for _, inst := range covered {
		v := m.views[inst]
		defsOK = defsOK && v.Defs
		dllOK = dllOK && v.DLLs
	}
	if m.game == 0 {
		ok, err := a.Store.GameTypesIndexed()
		if err != nil {
			return nil, err
		}
		m.game = map[bool]int8{true: 1, false: -1}[ok]
	}
	gameOK := m.game > 0

	// The Mods with a row, by packageId, and which of them have no facts of a kind: the
	// pool of what nothing installed provides is theirs, inferred.
	issueAt := map[string]int{}
	var issueInst, issuePkg []string
	for _, i := range q.issues {
		issueAt[normPkg(q.mods[i].PackageID)] = i
		if q.mods[i].Instance != "" {
			issueInst = append(issueInst, q.mods[i].Instance)
		} else {
			issuePkg = append(issuePkg, normPkg(q.mods[i].PackageID))
		}
	}
	if err := m.loadViews(a.Store, issueInst); err != nil {
		return nil, err
	}
	if err := m.loadPkgViews(a.Store, issuePkg); err != nil {
		return nil, err
	}
	// facts is what is known of a Mod with a row: its own instance's views, or for one not
	// installed here, those of any copy (another game's, or a tombstone's).
	facts := func(i int) store.FactView {
		if inst := q.mods[i].Instance; inst != "" {
			return m.views[inst]
		}
		return m.pkgView[normPkg(q.mods[i].PackageID)]
	}
	// owner picks, among the definers of something lost, the Mod with a row latest in the
	// save's order (10 §1.5); -1 when none has one.
	pkgOf := func(inst string) string {
		if q.names != nil {
			if p, _ := q.names(inst); p != "" {
				return normPkg(p)
			}
		}
		return normPkg(m.pkgs[inst][0])
	}
	owner := func(insts []string) int {
		best := -1
		for _, inst := range insts {
			if i, ok := issueAt[pkgOf(inst)]; ok && i > best {
				best = i
			}
		}
		return best
	}
	own := func(i int) *SaveOwned {
		o := out.owned[i]
		if o == nil {
			o = &SaveOwned{}
			out.owned[i] = o
		}
		return o
	}

	// Defs, pawns and factions.
	type pooled struct {
		races    map[string]*OwnedRace
		factions []string
		defs     OwnedDefs
		classes  []string
	}
	pool := pooled{races: map[string]*OwnedRace{}}
	if len(q.issues) > 0 && defsOK {
		names := make([]string, 0, len(f.Defs)+len(f.Colonists)+len(f.Factions))
		for n := range f.Defs {
			names = append(names, n)
		}
		for _, p := range f.Colonists {
			names = append(names, p.Race)
		}
		for _, fa := range f.Factions {
			names = append(names, fa.Def)
		}
		if err := m.loadDefs(a.Store, names); err != nil {
			return nil, err
		}
		var unknownPkgs []string
		for _, n := range names {
			for _, d := range m.defs[n] {
				if _, ok := m.pkgs[d.Instance]; !ok && pkgOf(d.Instance) == "" {
					unknownPkgs = append(unknownPkgs, d.Instance)
				}
			}
		}
		if err := m.loadPkgs(a.Store, unknownPkgs); err != nil {
			return nil, err
		}
		// lost: nothing the list loads defines it. Returns the Mod with a row owning it, -1
		// for nobody installed (the pool), -2 for an installed Mod without a row (not
		// counted: 10 §1.5).
		lost := func(name string) (int, []string, bool) {
			ds := m.defs[name]
			var insts, types []string
			for _, d := range ds {
				if q.loaded[d.Instance] {
					return 0, nil, false
				}
				insts = append(insts, d.Instance)
			}
			if len(ds) == 0 {
				return -1, nil, true
			}
			i := owner(insts)
			if i < 0 {
				return -2, nil, true
			}
			for _, d := range ds {
				if pkgOf(d.Instance) == normPkg(q.mods[i].PackageID) {
					types = append(types, d.Type)
				}
			}
			return i, types, true
		}
		special := map[string]bool{}
		races := map[int]map[string]*OwnedRace{}
		for _, p := range f.Colonists {
			special[p.Race] = true
			i, _, ok := lost(p.Race)
			if !ok || i == -2 || p.Race == "" {
				continue
			}
			byRace := pool.races
			if i >= 0 {
				if races[i] == nil {
					races[i] = map[string]*OwnedRace{}
				}
				byRace = races[i]
			}
			r := byRace[p.Race]
			if r == nil {
				r = &OwnedRace{Race: p.Race}
				byRace[p.Race] = r
			}
			if p.Dead {
				r.Dead++
			} else {
				r.Alive++
			}
		}
		for i, rs := range races {
			own(i).Colonists = sortedRaces(rs)
		}
		for _, fa := range f.Factions {
			special[fa.Def] = true
			i, _, ok := lost(fa.Def)
			if !ok || i == -2 || fa.Def == "" {
				continue
			}
			name := fa.Name
			if name == "" {
				name = fa.Def
			}
			if i >= 0 {
				own(i).Factions = append(own(i).Factions, name)
			} else {
				pool.factions = append(pool.factions, name)
			}
		}
		defs := map[int]map[string]*OwnedDefs{}
		for n, uses := range f.Defs {
			if special[n] {
				continue
			}
			i, types, ok := lost(n)
			if !ok || i == -2 {
				continue
			}
			if i < 0 {
				pool.defs.Defs++
				pool.defs.Uses += uses
				pool.defs.note(n, uses)
				continue
			}
			if defs[i] == nil {
				defs[i] = map[string]*OwnedDefs{}
			}
			for _, t := range dedupe(types) {
				d := defs[i][t]
				if d == nil {
					d = &OwnedDefs{Type: t}
					defs[i][t] = d
				}
				d.Defs++
				d.Uses += uses
				d.note(n, uses)
			}
		}
		pool.defs.finish()
		for i, ds := range defs {
			own(i).Defs = sortedDefs(ds)
		}
	}

	// Classes: every save, since a class can be missing without any Mod row.
	classes := make([]string, 0, len(f.Classes))
	for c := range f.Classes {
		if c != "" {
			classes = append(classes, c)
		}
	}
	sort.Strings(classes)
	if err := m.loadClasses(a.Store, classes, gameOK); err != nil {
		return nil, err
	}
	var ownerInst []string
	for _, c := range classes {
		ownerInst = append(ownerInst, m.classOwners(c)...)
	}
	if err := m.loadPkgs(a.Store, ownerInst); err != nil {
		return nil, err
	}
	for _, c := range classes {
		ok, insts := m.classOK(c, q.loaded)
		if ok {
			continue
		}
		if i := owner(insts); i >= 0 {
			own(i).Classes = append(own(i).Classes, c)
			continue
		}
		if !gameOK || !dllOK {
			continue // a class only an unindexed Mod or the game defines would look missing
		}
		if len(insts) == 0 {
			pool.classes = append(pool.classes, c)
			continue
		}
		pkg, name := "", ""
		if q.names != nil {
			pkg, name = q.names(insts[0])
		}
		if pkg == "" {
			pkg, name = m.pkgs[insts[0]][0], m.pkgs[insts[0]][1]
		}
		out.missing = append(out.missing, missingClass{Class: c, Mod: SaveMod{PackageID: pkg, Name: name, Instance: insts[0]}})
	}

	// The pool goes to the Mods with a row and no facts of that kind; with none, its
	// classes are missing with no Mod known and its Defs are only "source not found".
	var defPool, classPool []int
	for _, i := range q.issues {
		v := facts(i)
		if !v.Defs {
			defPool = append(defPool, i)
		}
		if !v.DLLs {
			classPool = append(classPool, i)
		}
		own(i).NoFacts = !v.Defs && !v.DLLs
		out.alone[i] = own(i).clone()
	}
	out.defPool, out.classPool = defPool, classPool
	out.pool = SaveOwned{Colonists: sortedRaces(pool.races), Factions: pool.factions, Classes: pool.classes}
	if pool.defs.Uses > 0 {
		out.pool.Defs = []OwnedDefs{pool.defs}
	}
	hasDefPool := len(pool.races) > 0 || len(pool.factions) > 0 || pool.defs.Uses > 0
	if hasDefPool {
		for _, i := range defPool {
			o := own(i)
			o.Colonists = mergeRaces(o.Colonists, pool.races)
			o.Factions = append(o.Factions, pool.factions...)
			if pool.defs.Uses > 0 {
				o.Defs = append(o.Defs, pool.defs)
			}
			o.Inferred = true
			o.Among = max(o.Among, len(defPool)-1)
		}
	}
	if len(pool.classes) > 0 {
		if len(classPool) == 0 {
			for _, c := range pool.classes {
				out.missing = append(out.missing, missingClass{Class: c})
			}
		}
		for _, i := range classPool {
			o := own(i)
			o.Classes = append(o.Classes, pool.classes...)
			o.Inferred = true
			o.Among = max(o.Among, len(classPool)-1)
		}
	}

	for _, i := range q.issues {
		o := own(i)
		sort.Strings(o.Classes)
		o.Classes = dedupe(o.Classes)
		o.DefsUnchecked = !defsOK
		o.ClassesUnchecked = !gameOK || !dllOK
	}
	sort.SliceStable(out.missing, func(i, j int) bool {
		if a, b := out.missing[i].Mod.PackageID, out.missing[j].Mod.PackageID; a != b {
			return a < b
		}
		return out.missing[i].Class < out.missing[j].Class
	})
	return out, nil
}

func (m *saveOwnerMemo) loadViews(s *store.Store, insts []string) error {
	var todo []string
	for _, i := range insts {
		if _, ok := m.views[i]; !ok && i != "" {
			todo = append(todo, i)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	got, err := s.FactViews(todo)
	if err != nil {
		return err
	}
	for _, i := range todo {
		m.views[i] = got[i]
	}
	return nil
}

func (m *saveOwnerMemo) loadPkgViews(s *store.Store, pkgs []string) error {
	var todo []string
	for _, p := range pkgs {
		if _, ok := m.pkgView[p]; !ok {
			todo = append(todo, p)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	got, err := s.PackageFactViews(todo)
	if err != nil {
		return err
	}
	for _, p := range todo {
		m.pkgView[p] = got[p]
	}
	return nil
}

func (m *saveOwnerMemo) loadPkgs(s *store.Store, insts []string) error {
	var todo []string
	seen := map[string]bool{}
	for _, i := range insts {
		if _, ok := m.pkgs[i]; !ok && i != "" && !seen[i] {
			seen[i] = true
			todo = append(todo, i)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	got, err := s.InstancePackages(todo)
	if err != nil {
		return err
	}
	for _, i := range todo {
		m.pkgs[i] = got[i]
	}
	return nil
}

// loadDefs looks up the definers of the names not in the memo; a name nobody defines
// takes the definers of the Def it is implied from.
func (m *saveOwnerMemo) loadDefs(s *store.Store, names []string) error {
	var todo []string
	seen := map[string]bool{}
	for _, n := range names {
		if _, ok := m.defs[n]; !ok && !seen[n] {
			seen[n] = true
			todo = append(todo, n)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	got, err := s.DefDefiners(todo)
	if err != nil {
		return err
	}
	var bases []string
	for _, n := range todo {
		m.defs[n] = got[n]
		if len(got[n]) == 0 {
			if b := impliedBase(n); b != "" {
				bases = append(bases, b)
			}
		}
	}
	if len(bases) == 0 {
		return nil
	}
	if err := m.loadDefs(s, bases); err != nil {
		return err
	}
	for _, n := range todo {
		if len(m.defs[n]) == 0 {
			if b := impliedBase(n); b != "" {
				m.defs[n] = m.defs[b]
			}
		}
	}
	return nil
}

// classParts splits a generic Class value into its definition and arguments:
// "Verse.Pair`2[Verse.Thing,System.Int32]" → "Verse.Pair`2", "Verse.Thing", "System.Int32".
func classParts(c string) []string {
	i := strings.IndexByte(c, '[')
	if i <= 0 || !strings.HasSuffix(c, "]") {
		return []string{c}
	}
	out := []string{c[:i]}
	arg := func(s string) {
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") { // [Name, Assembly…]
			s = s[1 : len(s)-1]
			if k := strings.Index(s, ","); k > 0 && !strings.Contains(s[:k], "[") {
				s = s[:k]
			}
		}
		if s != "" {
			out = append(out, classParts(s)...)
		}
	}
	depth, start := 0, i+1
	for j := i + 1; j < len(c)-1; j++ {
		switch c[j] {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				arg(c[start:j])
				start = j + 1
			}
		}
	}
	arg(c[start : len(c)-1])
	return out
}

// loadClasses resolves the parts of the classes not in the memo: the game first (when its
// types are indexed), then Mod assemblies by every form the game tries (07 §7 item 20).
func (m *saveOwnerMemo) loadClasses(s *store.Store, classes []string, gameOK bool) error {
	var parts []string
	seen := map[string]bool{}
	for _, c := range classes {
		for _, p := range classParts(c) {
			if _, ok := m.classes[p]; !ok && !seen[p] {
				seen[p] = true
				parts = append(parts, p)
			}
		}
	}
	if len(parts) == 0 {
		return nil
	}
	forms := map[string][]string{}
	var names []string
	for _, p := range parts {
		if strings.HasPrefix(p, "System.") || strings.HasPrefix(p, "UnityEngine.") {
			m.classes[p] = classInfo{builtin: true} // mscorlib and Unity: not indexed, always there
			continue
		}
		if gameOK {
			own, err := s.GameClassOwners(p)
			if err != nil {
				return err
			}
			if len(own) > 0 {
				m.classes[p] = classInfo{builtin: true}
				continue
			}
		}
		fs := []string{p}
		for _, ns := range store.SaveNamespaces {
			fs = append(fs, ns+"."+p)
		}
		forms[p] = fs
		names = append(names, fs...)
	}
	got, err := s.TypeDefiners(names)
	if err != nil {
		return err
	}
	var simple []string
	for p, fs := range forms {
		var owners []string
		for _, f := range fs {
			owners = append(owners, got[f]...)
		}
		if len(owners) == 0 && !strings.ContainsAny(p, ".+`") {
			simple = append(simple, p)
		}
		sort.Strings(owners)
		m.classes[p] = classInfo{owners: dedupe(owners)}
	}
	if len(simple) > 0 {
		nested, err := s.NestedTypeDefiners(simple)
		if err != nil {
			return err
		}
		for _, p := range simple {
			m.classes[p] = classInfo{owners: nested[p]}
		}
	}
	return nil
}

// classOwners lists every instance defining a part of the class.
func (m *saveOwnerMemo) classOwners(c string) []string {
	var out []string
	for _, p := range classParts(c) {
		out = append(out, m.classes[p].owners...)
	}
	return out
}

// classOK: every part of the class is defined by the game or a loaded instance; else the
// instances defining the parts that are not.
func (m *saveOwnerMemo) classOK(c string, loaded map[string]bool) (bool, []string) {
	ok := true
	var insts []string
	for _, p := range classParts(c) {
		ci := m.classes[p]
		if ci.builtin {
			continue
		}
		here := false
		for _, o := range ci.owners {
			if loaded[o] {
				here = true
				break
			}
		}
		if !here {
			ok = false
			insts = append(insts, ci.owners...)
		}
	}
	return ok, dedupe(insts)
}

func dedupe(s []string) []string {
	if len(s) < 2 {
		return s
	}
	seen := map[string]bool{}
	out := s[:0:0]
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func sortedRaces(m map[string]*OwnedRace) []OwnedRace {
	out := make([]OwnedRace, 0, len(m))
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := out[i].Alive+out[i].Dead, out[j].Alive+out[j].Dead; a != b {
			return a > b
		}
		return out[i].Race < out[j].Race
	})
	return out
}

func mergeRaces(have []OwnedRace, add map[string]*OwnedRace) []OwnedRace {
	if len(add) == 0 {
		return have
	}
	m := map[string]*OwnedRace{}
	for _, r := range have {
		r := r
		m[r.Race] = &r
	}
	for k, r := range add {
		if e := m[k]; e != nil {
			e.Alive += r.Alive
			e.Dead += r.Dead
		} else {
			c := *r
			m[k] = &c
		}
	}
	return sortedRaces(m)
}

func sortedDefs(m map[string]*OwnedDefs) []OwnedDefs {
	out := make([]OwnedDefs, 0, len(m))
	for _, d := range m {
		d.finish()
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Uses != out[j].Uses {
			return out[i].Uses > out[j].Uses
		}
		return out[i].Type < out[j].Type
	})
	return out
}
