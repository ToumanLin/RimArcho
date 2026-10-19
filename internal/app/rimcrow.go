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

	"github.com/rimarcho/rimarcho/internal/order"
	"github.com/rimarcho/rimarcho/internal/scan"
	"github.com/rimarcho/rimarcho/internal/source"
	"github.com/rimarcho/rimarcho/internal/store"
)

// RimCrow import (04 §7.2, U11). RimCrow (D:\…\RimModManager) keeps player data in
// <home>/data/mod_manager.db (peewee, keyed by packageId) and user rules in
// <home>/data/rules/user_rules.json. Both are read from copies; the originals are never
// opened for writing.

// RCItem is one line of the import report.
type RCItem struct{ Key, Reason string }

// RimCrowReport is the outcome of one import (or dry run).
type RimCrowReport struct {
	DB, Rules string
	DryRun    bool

	Records, ByPath, ByPackage int // mod records carrying data, and how they mapped
	Unmapped                   []RCItem

	AliasNew, AliasSame, NoteNew, NoteSame int
	Conflicts                              []RCItem // RimArcho already has a different value

	Groups, Memberships, TagNew, TagSame int

	RulesTotal, RuleNew, RuleSame int
	RulesDisabled                 bool // user rules are switched off in RimCrow
	RulesSkipped                  []RCItem

	NotMapped map[string]int // RimCrow fields with no RimArcho equivalent

	// About-disabled (02 §4.1): RimCrow renames About.xml to About.xml.disabled.
	CrowDisabled  int            // modasset rows with disabled=1
	AboutDisabled []string       // RimArcho instances marked unloadable: about-disabled
	DisabledKinds map[string]int // by source kind
}

// RimCrowOptions select the files and what to import.
type RimCrowOptions struct {
	DB, Rules            string
	DryRun               bool
	IncludeDisabledRules bool
}

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
	alias, note string
	tags        []string
}

// ImportRimCrow maps RimCrow's aliases, notes, groups (as tags), tags, interlocks (as
// attach rules) and user rules onto lineages. Running it again changes nothing.
func (a *App) ImportRimCrow(opt RimCrowOptions) (*RimCrowReport, error) {
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	rep := &RimCrowReport{DB: opt.DB, DryRun: opt.DryRun, NotMapped: map[string]int{}, DisabledKinds: map[string]int{}}
	db, done, err := a.openCopy(opt.DB)
	if err != nil {
		return nil, fmt.Errorf("RimCrow database: %w", err)
	}
	defer done()

	// Paths RimArcho knows, for the path-first mapping.
	byPath := map[string]string{}
	for _, m := range ix.Mods {
		if !m.TombstoneAt.IsZero() {
			continue
		}
		for _, p := range []string{ix.ModDir(m), gamePath(ix, m)} {
			if p != "" {
				byPath[normPath(p)] = m.ID
			}
		}
	}
	assets := map[string][]string{} // packageId → RimCrow asset paths
	rows, err := db.Query(`SELECT package_id, path, disabled, source FROM modasset`)
	if err != nil {
		return nil, fmt.Errorf("RimCrow database: %w (is this RimCrow's mod_manager.db?)", err)
	}
	for rows.Next() {
		var pkg, path, src string
		var disabled bool
		if err := rows.Scan(&pkg, &path, &disabled, &src); err != nil {
			rows.Close()
			return nil, err
		}
		pkg = strings.ToLower(pkg)
		assets[pkg] = append(assets[pkg], path)
		if disabled {
			rep.CrowDisabled++
		}
	}
	rows.Close()
	for _, m := range ix.Mods {
		if m.TombstoneAt.IsZero() && m.Unloadable == scan.UnloadableAboutDisabled {
			rep.AboutDisabled = append(rep.AboutDisabled, m.ID)
			rep.DisabledKinds[string(m.Kind)]++
		}
	}
	lineageOf := func(pkg string) (string, string, string) {
		lins := map[string]bool{}
		for _, p := range assets[pkg] {
			if id := byPath[normPath(p)]; id != "" {
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

	// Per-mod data: usermoddata + group memberships.
	mods := map[string]*crowMod{}
	get := func(pkg string) *crowMod {
		pkg = strings.ToLower(pkg)
		if mods[pkg] == nil {
			mods[pkg] = &crowMod{}
		}
		return mods[pkg]
	}
	var interlocks []string
	rows, err = db.Query(`SELECT mod_id, coalesce(alias_name,''), coalesce(notes,''), coalesce(tags,'[]'), coalesce(sign_color,''), coalesce(user_mod_type,''), coalesce(ignored_issues,'[]'), coalesce(interlock_id,'') FROM usermoddata`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, alias, note, tags, color, typ, ign, lock string
		if err := rows.Scan(&id, &alias, &note, &tags, &color, &typ, &ign, &lock); err != nil {
			rows.Close()
			return nil, err
		}
		var tl, il []string
		json.Unmarshal([]byte(tags), &tl)
		json.Unmarshal([]byte(ign), &il)
		if strings.TrimSpace(alias) != "" || strings.TrimSpace(note) != "" || len(tl) > 0 {
			m := get(id)
			m.alias, m.note = strings.TrimSpace(alias), strings.TrimSpace(note)
			m.tags = append(m.tags, tl...)
		}
		if color != "" {
			rep.NotMapped["sign_color"]++
		}
		if typ != "" {
			rep.NotMapped["user_mod_type"]++
		}
		if len(il) > 0 {
			rep.NotMapped["ignored_issues"]++
		}
		if lock != "" && !contains(interlocks, lock) {
			interlocks = append(interlocks, lock)
		}
	}
	rows.Close()
	groups := map[string]string{}
	rows, err = db.Query(`SELECT group_id, name FROM groupdata`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, name string
		rows.Scan(&id, &name)
		groups[id] = strings.TrimSpace(name)
	}
	rows.Close()
	rep.Groups = len(groups)
	rows, err = db.Query(`SELECT group_id, mod_id FROM groupmod ORDER BY group_id, sort_index`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var g, id string
		rows.Scan(&g, &id)
		if name := groups[g]; name != "" {
			m := get(id)
			m.tags = append(m.tags, name)
			rep.Memberships++
		}
	}
	rows.Close()

	pd, err := a.Store.PlayerDataAll(ix.Env.ID)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(mods))
	for k := range mods {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, pkg := range keys {
		cm := mods[pkg]
		rep.Records++
		lin, how, why := lineageOf(pkg)
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
		apply := func(what, v, have string, set func(env, lin, v string) error, n, same *int) error {
			switch {
			case v == "":
			case have == v:
				*same++
			case have != "":
				rep.Conflicts = append(rep.Conflicts, RCItem{"pkg:" + pkg, fmt.Sprintf("%s: RimArcho has %q, RimCrow %q (kept RimArcho's)", what, have, v)})
			default:
				*n++
				if !opt.DryRun {
					return set(ix.Env.ID, lin, v)
				}
			}
			return nil
		}
		if err := apply("alias", cm.alias, cur.Alias, a.Store.SetAlias, &rep.AliasNew, &rep.AliasSame); err != nil {
			return nil, err
		}
		if cm.alias != "" && cur.Alias == "" {
			cur.Alias = cm.alias
		}
		if err := apply("note", cm.note, cur.Note, a.Store.SetNote, &rep.NoteNew, &rep.NoteSame); err != nil {
			return nil, err
		}
		if cm.note != "" && cur.Note == "" {
			cur.Note = cm.note
		}
		for _, t := range cm.tags {
			if t = strings.TrimSpace(t); t == "" {
				continue
			}
			if containsFold(cur.Tags, t) {
				rep.TagSame++
				continue
			}
			cur.Tags = append(cur.Tags, t)
			rep.TagNew++
			if !opt.DryRun {
				if _, err := a.Store.AddTag(ix.Env.ID, lin, t); err != nil {
					return nil, err
				}
			}
		}
	}

	// Rules: interlock chains become attach rules; user_rules.json becomes after/before/incompatible.
	existing, err := a.Store.Rules(ix.Env.ID)
	if err != nil {
		return nil, err
	}
	addRule := func(kind, pa, pb, label string) error {
		rep.RulesTotal++
		la, _, wa := lineageOf(pa)
		lb, _, wb := lineageOf(pb)
		switch {
		case la == "":
			rep.RulesSkipped = append(rep.RulesSkipped, RCItem{label, "pkg:" + pa + " " + wa})
			return nil
		case lb == "":
			rep.RulesSkipped = append(rep.RulesSkipped, RCItem{label, "pkg:" + pb + " " + wb})
			return nil
		}
		for _, r := range existing {
			if r.Kind == kind && r.A == la && r.B == lb {
				rep.RuleSame++
				return nil
			}
		}
		rep.RuleNew++
		r := &store.UserRule{Kind: kind, A: la, B: lb, Note: "imported from RimCrow", Created: a.Now().UTC()}
		existing = append(existing, r)
		if opt.DryRun {
			return nil
		}
		_, err := a.Store.AddRule(ix.Env.ID, r)
		return err
	}
	for _, lock := range interlocks {
		var chain string
		if err := db.QueryRow(`SELECT chain FROM modinterlock WHERE id=?`, lock).Scan(&chain); err != nil {
			continue
		}
		var c []string
		json.Unmarshal([]byte(chain), &c)
		for i := 1; i < len(c); i++ {
			if err := addRule(order.RuleAttach, strings.ToLower(c[i]), strings.ToLower(c[i-1]), "interlock "+lock); err != nil {
				return nil, err
			}
		}
	}
	rulesPath := opt.Rules
	if rulesPath == "" {
		rulesPath = filepath.Join(filepath.Dir(opt.DB), "rules", "user_rules.json")
	}
	if b, err := os.ReadFile(rulesPath); err == nil {
		rep.Rules = rulesPath
		var f struct {
			Settings struct {
				Enabled *bool `json:"user_mod_rules_enabled"`
			} `json:"settings"`
			ModRules map[string]map[string]json.RawMessage `json:"mod_rules"`
		}
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("%s: %w", rulesPath, err)
		}
		rep.RulesDisabled = f.Settings.Enabled != nil && !*f.Settings.Enabled
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
				kind := map[string]string{"loadAfter": order.RuleAfter, "loadBefore": order.RuleBefore, "incompatibleWith": order.RuleIncompatible}[k]
				if kind == "" {
					rep.NotMapped["rule "+k]++ // loadTop / loadBottom: no RimArcho equivalent
					continue
				}
				var targets map[string]json.RawMessage
				json.Unmarshal(kinds[k], &targets)
				var bs []string
				for b := range targets {
					bs = append(bs, b)
				}
				sort.Strings(bs)
				for _, pb := range bs {
					label := fmt.Sprintf("%s %s %s", strings.ToLower(pa), k, strings.ToLower(pb))
					if rep.RulesDisabled && !opt.IncludeDisabledRules {
						rep.RulesTotal++
						rep.RulesSkipped = append(rep.RulesSkipped, RCItem{label, "user rules are switched off in RimCrow (--include-disabled-rules)"})
						continue
					}
					if err := addRule(kind, strings.ToLower(pa), strings.ToLower(pb), label); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return rep, nil
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
