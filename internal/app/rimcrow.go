package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/order"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// RimCrow import (07 §6.2, U11). RimCrow (D:\…\RimModManager) keeps player data in
// <home>/data/mod_manager.db (peewee, keyed by packageId) and user rules in
// <home>/data/rules/user_rules.json. Both are read from copies; the originals are never
// opened for writing.

// RCItem is one line of the import report.
type RCItem struct{ Key, Reason string }

// RimCrowReport is the outcome of one import (or dry run).
type RimCrowReport struct {
	DB, Rules string
	Profiles  []RCProfile
	DryRun    bool
	// NotesAndTags: the source has only notes and tags (RimSort's local database), so a
	// report leaves out aliases and rules
	NotesAndTags bool

	Records, ByPath, ByPackage int // mod records carrying data, and how they mapped
	Unmapped                   []RCItem

	AliasNew, AliasSame, NoteNew, NoteSame int
	Conflicts                              []RCItem // RimArcho already has a different value

	Groups, Memberships, TagNew, TagSame int
	TagColors                            int // tags given a colour (a Mod's colour mark, a group's colour)

	RulesTotal, RuleNew, RuleSame int
	RulesDisabled                 bool // user rules are switched off in RimCrow
	RulesSkipped                  []RCItem

	NotMapped map[string]int // RimCrow fields with no RimArcho equivalent

	// Parts counts each part a migration can bring over on its own (07 §6.5): what is
	// new and what RimArcho already has.
	Parts map[string]*PartCount

	// Items are a dry run's values one by one, for the Import page's "view" of a part:
	// what each Mod gets, and whether RimArcho has it already.
	Items []PartItem `json:",omitempty"`

	// About-disabled (04 §1.2): RimCrow renames About.xml to About.xml.disabled.
	CrowDisabled  int            // modasset rows with disabled=1
	AboutDisabled []string       // RimArcho instances marked unloadable: about-disabled
	DisabledKinds map[string]int // by source kind
}

// RimCrowOptions select the files and what to import.
type RimCrowOptions struct {
	DB, Rules            string
	SelectProfiles       bool
	ProfileIDs           []string
	DryRun               bool
	IncludeDisabledRules bool
	Tags                 ManagerTagNames
	// Parts limits the Mod data and rules to these parts (Part*); nil takes them all, an
	// empty map none (profiles only).
	Parts map[string]bool
}

// The parts of another manager's data that can be brought over one by one (07 §6.5).
const (
	PartAliases    = "aliases"
	PartNotes      = "notes"
	PartGroups     = "groups"
	PartTags       = "tags"
	PartColours    = "colours"
	PartIgnored    = "ignored"
	PartRules      = "rules"      // user rules (RimCrow, RimSort)
	PartRulesOff   = "rules-off"  // RimCrow's user rules while they are switched off there
	PartInterlocks = "interlocks" // RimCrow's interlocks, as attach rules
)

// PartCount is one part of a migration: values RimArcho does not have yet, and values it
// already has.
type PartCount struct{ New, Same int }

// PartItem is one value of a part: a Mod (by lineage) and what it gets; Other is a rule's
// second Mod. Had: RimArcho already has a value there (its own stays).
type PartItem struct {
	Part  string `json:"part"`
	Lin   string `json:"-"`
	Other string `json:"-"`
	Mod   string `json:"mod"`
	Value string `json:"value"`
	With  string `json:"with,omitempty"` // a rule's second Mod, by name
	Had   bool   `json:"had"`
}

// maxPartItems bounds a dry run's items: a view shows a part, not a database
const maxPartItems = 20000

// item records one value of a part during a dry run
func (r *RimCrowReport) item(part, lin, value, other string, had bool) {
	if !r.DryRun || len(r.Items) >= maxPartItems {
		return
	}
	r.Items = append(r.Items, PartItem{Part: part, Lin: lin, Value: value, Other: other, Had: had})
}

func (r *RimCrowReport) part(p string) *PartCount {
	if r.Parts == nil {
		r.Parts = map[string]*PartCount{}
	}
	if r.Parts[p] == nil {
		r.Parts[p] = &PartCount{}
	}
	return r.Parts[p]
}

// wantPart: parts nil takes every part.
func wantPart(parts map[string]bool, p string) bool { return parts == nil || parts[p] }

// openCopy copies a SQLite database (and its WAL) to a temp dir and opens the copy
// read-only.
func (a *App) openCopy(path string) (*sql.DB, func(), error) {
	dir, err := os.MkdirTemp(filepath.Join(a.Home), "rimcrow-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	dst := filepath.Join(dir, "mod_manager.db")
	for _, suf := range []string{"", "-wal"} {
		if err := copyPlain(path+suf, dst+suf); err != nil {
			if suf == "" {
				cleanup()
				return nil, nil, err
			}
		}
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dst)+"?mode=ro")
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return db, func() { db.Close(); cleanup() }, nil
}

func copyPlain(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

type crowMod struct {
	alias, note, color string
	ignore             bool
	tags, groups       []string
}

// crowImport holds what the steps of ImportRimCrow share.
type crowImport struct {
	a          *App
	opt        RimCrowOptions
	rep        *RimCrowReport
	db         *sql.DB
	ix         *Index
	byPath     map[string]string   // normalised path → mod ID, for the path-first mapping
	assets     map[string][]string // packageId → RimCrow asset paths
	mods       map[string]*crowMod // packageId → per-mod data
	interlocks []string
	groupColor map[string]string // group name → its colour (RimCrow's default white left out)
	existing   []*store.UserRule
}

// ImportRimCrow maps RimCrow's aliases, notes, groups (as tags), tags, interlocks (as
// attach rules) and user rules onto lineages. Running it again changes nothing.
func (a *App) ImportRimCrow(opt RimCrowOptions) (*RimCrowReport, error) {
	rep := &RimCrowReport{DB: opt.DB, DryRun: opt.DryRun, NotMapped: map[string]int{}, DisabledKinds: map[string]int{}}
	db, done, err := a.openCopy(opt.DB)
	if err != nil {
		return nil, fmt.Errorf("RimCrow database: %w", err)
	}
	defer done()
	if err := a.importCrowProfiles(db, opt, rep); err != nil {
		return nil, err
	}
	if opt.Parts != nil && len(opt.Parts) == 0 {
		return rep, nil // profiles only
	}
	var assetsTable int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='modasset'").Scan(&assetsTable); err != nil {
		return nil, err
	}
	if assetsTable == 0 && len(rep.Profiles) > 0 {
		return rep, nil
	}
	ix, err := a.Index()
	if err != nil {
		if opt.DryRun && len(rep.Profiles) > 0 {
			rep.Unmapped = append(rep.Unmapped, RCItem{"annotations", "add a Game Config before importing Mod annotations"})
			return rep, nil
		}
		return nil, err
	}
	ci := &crowImport{a: a, opt: opt, rep: rep, db: db, ix: ix}
	ci.mapPaths()
	if err := ci.readAssets(); err != nil {
		return nil, err
	}
	ci.countAboutDisabled()
	if err := ci.readUserModData(); err != nil {
		return nil, err
	}
	if err := ci.readGroups(); err != nil {
		return nil, err
	}
	if err := ci.applyModData(); err != nil {
		return nil, err
	}

	// Rules: interlock chains become attach rules; user_rules.json becomes after/before/incompatible.
	if ci.existing, err = a.Store.Rules(ix.Env.ID); err != nil {
		return nil, err
	}
	if err := ci.importInterlocks(); err != nil {
		return nil, err
	}
	if err := ci.importUserRules(); err != nil {
		return nil, err
	}
	return rep, nil
}

// want reports whether the import takes part p.
func (ci *crowImport) want(p string) bool { return wantPart(ci.opt.Parts, p) }

// mapPaths records the paths RimArcho knows, for the path-first mapping.
func (ci *crowImport) mapPaths() {
	ix := ci.ix
	ci.byPath = map[string]string{}
	for _, m := range ix.Mods {
		if !m.TombstoneAt.IsZero() {
			continue
		}
		for _, p := range []string{ix.ModDir(m), gamePath(ix, m)} {
			if p != "" {
				ci.byPath[normPath(p)] = m.ID
			}
		}
	}
}

// readAssets reads RimCrow's modasset table: each packageId's paths, and how many are
// disabled.
func (ci *crowImport) readAssets() error {
	ci.assets = map[string][]string{}
	rows, err := ci.db.Query(`SELECT package_id, path, disabled, source FROM modasset`)
	if err != nil {
		return fmt.Errorf("RimCrow database: %w (is this RimCrow's mod_manager.db?)", err)
	}
	for rows.Next() {
		var pkg, path, src string
		var disabled bool
		if err := rows.Scan(&pkg, &path, &disabled, &src); err != nil {
			rows.Close()
			return err
		}
		pkg = strings.ToLower(pkg)
		ci.assets[pkg] = append(ci.assets[pkg], path)
		if disabled {
			ci.rep.CrowDisabled++
		}
	}
	rows.Close()
	return nil
}

// countAboutDisabled reports the RimArcho instances marked unloadable: about-disabled.
func (ci *crowImport) countAboutDisabled() {
	for _, m := range ci.ix.Mods {
		if m.TombstoneAt.IsZero() && m.Unloadable == scan.UnloadableAboutDisabled {
			ci.rep.AboutDisabled = append(ci.rep.AboutDisabled, m.ID)
			ci.rep.DisabledKinds[string(m.Kind)]++
		}
	}
}

// lineageOf maps a RimCrow packageId to a lineage: by its paths first, then by package ID.
// It returns the lineage and how it was found, or why it could not be.
func (ci *crowImport) lineageOf(pkg string) (string, string, string) {
	ix := ci.ix
	lins := map[string]bool{}
	for _, p := range ci.assets[pkg] {
		if id := ci.byPath[normPath(p)]; id != "" {
			lins[ix.ByID[id].LineageID] = true
		}
	}
	if len(lins) == 1 {
		for l := range lins {
			return l, "path", ""
		}
	}
	if len(lins) > 1 {
		return "", "", fmt.Sprintf("its RimCrow paths are %d different lineages here", len(lins))
	}
	var cands []string
	for _, m := range ix.Mods {
		if m.TombstoneAt.IsZero() && strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam") == pkg && !contains(cands, m.LineageID) {
			cands = append(cands, m.LineageID)
		}
	}
	switch len(cands) {
	case 0:
		return "", "", "not installed here"
	case 1:
		return cands[0], "packageId", ""
	}
	var refs []string
	for _, l := range cands {
		refs = append(refs, ix.LRef(l))
	}
	return "", "", "packageId has several lineages (" + strings.Join(refs, ", ") + "); not imported"
}

// mod returns the per-mod data of a packageId, creating it.
func (ci *crowImport) mod(pkg string) *crowMod {
	pkg = strings.ToLower(pkg)
	if ci.mods[pkg] == nil {
		ci.mods[pkg] = &crowMod{}
	}
	return ci.mods[pkg]
}

// readUserModData reads RimCrow's usermoddata rows into per-mod data and interlock IDs.
func (ci *crowImport) readUserModData() error {
	ci.mods = map[string]*crowMod{}
	rows, err := ci.db.Query(`SELECT mod_id, coalesce(alias_name,''), coalesce(notes,''), coalesce(tags,'[]'), coalesce(sign_color,''), coalesce(user_mod_type,''), coalesce(ignored_issues,'[]'), coalesce(interlock_id,'') FROM usermoddata`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, alias, note, tags, color, typ, ign, lock string
		if err := rows.Scan(&id, &alias, &note, &tags, &color, &typ, &ign, &lock); err != nil {
			rows.Close()
			return err
		}
		ci.addUserModData(id, alias, note, tags, color, typ, ign, lock)
	}
	rows.Close()
	return nil
}

// addUserModData takes one usermoddata row, keeping only the chosen parts.
func (ci *crowImport) addUserModData(id, alias, note, tags, color, typ, ign, lock string) {
	rep := ci.rep
	var tl, il []string
	json.Unmarshal([]byte(tags), &tl)
	json.Unmarshal([]byte(ign), &il)
	if color != "" && managerHex(color) == "" {
		rep.NotMapped["sign_color"]++ // not a colour RimArcho can read
		color = ""
	}
	if !ci.want(PartAliases) {
		alias = ""
	}
	if !ci.want(PartNotes) {
		note = ""
	}
	if !ci.want(PartTags) {
		tl = nil
	}
	if !ci.want(PartColours) {
		color = ""
	}
	if !ci.want(PartIgnored) {
		il = nil
	}
	// a colour mark and ignored issues become tags (07 §6.4); RimCrow's issue IDs
	// themselves have no RimArcho equivalent
	if strings.TrimSpace(alias) != "" || strings.TrimSpace(note) != "" || len(tl) > 0 || color != "" || len(il) > 0 {
		m := ci.mod(id)
		m.alias, m.note = strings.TrimSpace(alias), strings.TrimSpace(note)
		m.tags = append(m.tags, tl...)
		m.color, m.ignore = color, len(il) > 0
	}
	if typ != "" {
		rep.NotMapped["user_mod_type"]++
	}
	if lock != "" && ci.want(PartInterlocks) && !contains(ci.interlocks, lock) {
		ci.interlocks = append(ci.interlocks, lock)
	}
}

// readGroups reads RimCrow's groups, their colours and their members.
func (ci *crowImport) readGroups() error {
	groups := map[string]string{}
	ci.groupColor = map[string]string{}
	rows, err := ci.db.Query(`SELECT group_id, name, coalesce(color,'') FROM groupdata`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, name, color string
		rows.Scan(&id, &name, &color)
		groups[id] = strings.TrimSpace(name)
		if hex := managerHex(color); hex != "" && hex != "#ffffff" {
			ci.groupColor[strings.TrimSpace(name)] = hex
		}
	}
	rows.Close()
	ci.rep.Groups = len(groups)
	rows, err = ci.db.Query(`SELECT group_id, mod_id FROM groupmod ORDER BY group_id, sort_index`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var g, id string
		rows.Scan(&g, &id)
		if name := groups[g]; name != "" && ci.want(PartGroups) {
			m := ci.mod(id)
			m.groups = append(m.groups, name)
			ci.rep.Memberships++
		}
	}
	rows.Close()
	return nil
}

// applyModData maps each RimCrow mod onto a lineage and brings over its data, in
// packageId order.
func (ci *crowImport) applyModData() error {
	a, ix, rep := ci.a, ci.ix, ci.rep
	pd, err := a.Store.PlayerDataAll(ix.Env.ID)
	if err != nil {
		return err
	}
	tagger, err := a.newTagImport(ix.Env.ID, ci.opt.DryRun, rep)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(ci.mods))
	for k := range ci.mods {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, pkg := range keys {
		rep.Records++
		lin, how, why := ci.lineageOf(pkg)
		if lin == "" {
			rep.Unmapped = append(rep.Unmapped, RCItem{"pkg:" + pkg, why})
			continue
		}
		if how == "path" {
			rep.ByPath++
		} else {
			rep.ByPackage++
		}
		cur := pd[lin]
		if cur == nil {
			cur = &store.PlayerData{}
			pd[lin] = cur
		}
		if err := ci.applyMod(pkg, lin, ci.mods[pkg], cur, tagger); err != nil {
			return err
		}
	}
	return nil
}

// applyMod brings one mod's alias, note, tags, groups, colour and ignored mark over to
// lineage lin, whose current data is cur.
func (ci *crowImport) applyMod(pkg, lin string, cm *crowMod, cur *store.PlayerData, tagger *tagImport) error {
	a, opt := ci.a, ci.opt
	rep := ci.rep
	if err := ci.applyValue(pkg, lin, "alias", cm.alias, cur.Alias, a.Store.SetAlias, &rep.AliasNew, &rep.AliasSame); err != nil {
		return err
	}
	if cm.alias != "" && cur.Alias == "" {
		cur.Alias = cm.alias
	}
	if err := ci.applyValue(pkg, lin, "note", cm.note, cur.Note, a.Store.SetNote, &rep.NoteNew, &rep.NoteSame); err != nil {
		return err
	}
	if cm.note != "" && cur.Note == "" {
		cur.Note = cm.note
	}
	for _, t := range cm.tags {
		if err := tagger.add(PartTags, lin, cur, t); err != nil {
			return err
		}
	}
	for _, t := range cm.groups {
		if err := tagger.add(PartGroups, lin, cur, t); err != nil {
			return err
		}
		if err := tagger.color(t, ci.groupColor[strings.TrimSpace(t)]); err != nil {
			return err
		}
	}
	if hex := managerHex(cm.color); hex != "" {
		name := opt.Tags.color("RimCrow", hex)
		if err := tagger.add(PartColours, lin, cur, name); err != nil {
			return err
		}
		if err := tagger.color(name, hex); err != nil {
			return err
		}
	}
	if cm.ignore {
		if err := tagger.add(PartIgnored, lin, cur, opt.Tags.ignore("RimCrow")); err != nil {
			return err
		}
	}
	return nil
}

// applyValue brings one alias or note over: counted as new or same, a different value of
// RimArcho's own kept as a conflict; set writes it unless this is a dry run.
func (ci *crowImport) applyValue(pkg, lin, what, v, have string, set func(env, lin, v string) error, n, same *int) error {
	rep := ci.rep
	pn := map[string]string{"alias": PartAliases, "note": PartNotes}[what]
	part := rep.part(pn)
	if v != "" {
		rep.item(pn, lin, v, "", have != "")
	}
	switch {
	case v == "":
	case have == v:
		*same++
		part.Same++
	case have != "":
		part.Same++ // RimArcho's own value stays
		rep.Conflicts = append(rep.Conflicts, RCItem{"pkg:" + pkg, fmt.Sprintf("%s: RimArcho has %q, RimCrow %q (kept RimArcho's)", what, have, v)})
	default:
		*n++
		part.New++
		if !ci.opt.DryRun {
			return set(ci.ix.Env.ID, lin, v)
		}
	}
	return nil
}

// addRule brings one rule over unless a mod is not here or RimArcho has it already.
func (ci *crowImport) addRule(part, kind, pa, pb, label string) error {
	rep := ci.rep
	rep.RulesTotal++
	la, _, wa := ci.lineageOf(pa)
	lb, wb := "", ""
	if pb != "" { // top / bottom name one mod
		lb, _, wb = ci.lineageOf(pb)
	}
	switch {
	case la == "":
		rep.RulesSkipped = append(rep.RulesSkipped, RCItem{label, "pkg:" + pa + " " + wa})
		return nil
	case lb == "" && pb != "":
		rep.RulesSkipped = append(rep.RulesSkipped, RCItem{label, "pkg:" + pb + " " + wb})
		return nil
	}
	for _, r := range ci.existing {
		if r.Kind == kind && r.A == la && r.B == lb {
			rep.RuleSame++
			rep.part(part).Same++
			rep.item(part, la, kind, lb, true)
			return nil
		}
	}
	rep.RuleNew++
	rep.part(part).New++
	rep.item(part, la, kind, lb, false)
	r := &store.UserRule{Kind: kind, A: la, B: lb, Note: "imported from RimCrow", Created: ci.a.Now().UTC()}
	ci.existing = append(ci.existing, r)
	if ci.opt.DryRun {
		return nil
	}
	_, err := ci.a.Store.AddRule(ci.ix.Env.ID, r)
	return err
}

// importInterlocks turns each interlock chain into attach rules.
func (ci *crowImport) importInterlocks() error {
	for _, lock := range ci.interlocks {
		var chain string
		if err := ci.db.QueryRow(`SELECT chain FROM modinterlock WHERE id=?`, lock).Scan(&chain); err != nil {
			continue
		}
		var c []string
		json.Unmarshal([]byte(chain), &c)
		for i := 1; i < len(c); i++ {
			if err := ci.addRule(PartInterlocks, order.RuleAttach, strings.ToLower(c[i]), strings.ToLower(c[i-1]), "interlock "+lock); err != nil {
				return err
			}
		}
	}
	return nil
}

// crowUserRules is how user_rules.json is read.
type crowUserRules struct {
	Settings struct {
		Enabled *bool `json:"user_mod_rules_enabled"`
	} `json:"settings"`
	ModRules map[string]map[string]json.RawMessage `json:"mod_rules"`
}

// crowRuleScope is what applies to all of user_rules.json's rules.
type crowRuleScope struct {
	part     string // the part they come over as
	skipUser bool   // that part is not chosen
	include  bool   // switched-off rules come over too
}

// importUserRules turns user_rules.json into after/before/incompatible and top/bottom rules.
func (ci *crowImport) importUserRules() error {
	opt, rep := ci.opt, ci.rep
	rulesPath := opt.Rules
	if rulesPath == "" {
		rulesPath = filepath.Join(filepath.Dir(opt.DB), "rules", "user_rules.json")
	}
	b, err := os.ReadFile(rulesPath)
	if err != nil {
		return nil
	}
	rep.Rules = rulesPath
	var f crowUserRules
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("%s: %w", rulesPath, err)
	}
	rep.RulesDisabled = f.Settings.Enabled != nil && !*f.Settings.Enabled
	// with parts chosen, switched-off rules come over only as their own part
	sc := crowRuleScope{part: PartRules}
	if rep.RulesDisabled {
		sc.part = PartRulesOff
	}
	sc.skipUser = opt.Parts != nil && !opt.Parts[sc.part]
	sc.include = opt.IncludeDisabledRules || opt.Parts[PartRulesOff]
	var as []string
	for k := range f.ModRules {
		as = append(as, k)
	}
	sort.Strings(as)
	for _, pa := range as {
		kinds := f.ModRules[pa]
		var ks []string
		for k := range kinds {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			if err := ci.importUserRule(sc, pa, k, kinds[k]); err != nil {
				return err
			}
		}
	}
	return nil
}

// skipSwitchedOff counts a rule left out because user rules are switched off in RimCrow
// and reports whether it was.
func (ci *crowImport) skipSwitchedOff(sc crowRuleScope, label string) bool {
	if ci.rep.RulesDisabled && !sc.include {
		ci.rep.RulesTotal++
		ci.rep.RulesSkipped = append(ci.rep.RulesSkipped, RCItem{label, "user rules are switched off in RimCrow (--include-disabled-rules)"})
		return true
	}
	return false
}

// importUserRule brings over mod pa's rules of one kind k from user_rules.json.
func (ci *crowImport) importUserRule(sc crowRuleScope, pa, k string, raw json.RawMessage) error {
	if pos := map[string]string{"loadTop": order.RuleTop, "loadBottom": order.RuleBottom}[k]; pos != "" {
		// {"value": true, "comment": "..."} → top / bottom (08 §1.5)
		var f struct {
			Value json.RawMessage `json:"value"`
		}
		json.Unmarshal(raw, &f)
		if strings.Trim(strings.ToLower(string(f.Value)), `" `) != "true" {
			return nil
		}
		label := fmt.Sprintf("%s %s", strings.ToLower(pa), k)
		if sc.skipUser || ci.skipSwitchedOff(sc, label) {
			return nil
		}
		return ci.addRule(sc.part, pos, strings.ToLower(pa), "", label)
	}
	kind := map[string]string{"loadAfter": order.RuleAfter, "loadBefore": order.RuleBefore, "incompatibleWith": order.RuleIncompatible}[k]
	if sc.skipUser {
		return nil
	}
	if kind == "" {
		ci.rep.NotMapped["rule "+k]++
		return nil
	}
	var targets map[string]json.RawMessage
	json.Unmarshal(raw, &targets)
	var bs []string
	for b := range targets {
		bs = append(bs, b)
	}
	sort.Strings(bs)
	for _, pb := range bs {
		label := fmt.Sprintf("%s %s %s", strings.ToLower(pa), k, strings.ToLower(pb))
		if ci.skipSwitchedOff(sc, label) {
			continue
		}
		if err := ci.addRule(sc.part, kind,
			strings.ToLower(pa), strings.ToLower(pb), label); err != nil {
			return err
		}
	}
	return nil
}

func normPath(p string) string {
	if real, err := source.RealPath(p); err == nil {
		p = real
	}
	return strings.ToLower(filepath.Clean(strings.ReplaceAll(p, "/", `\`)))
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func containsFold(l []string, s string) bool {
	for _, x := range l {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
