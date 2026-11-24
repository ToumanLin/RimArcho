package app

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/store"
)

// RimSort's local database (07 §6.4): every RimSort instance keeps aux_metadata.db
// (SQLAlchemy, SQLite) in <RimSort data>\instances\<instance>. Its table
// auxiliary_metadata has one row per Mod folder, keyed by the folder's absolute path, with
// the player's notes, colour and "ignore warnings"; mod_tags and tags_table hold the tags.
// The rest of a row (ACF and Workshop times, outdated) is RimSort's own cache.

// RimSortAuxOptions select the database and the names of the tags it becomes.
type RimSortAuxOptions struct {
	DB     string
	DryRun bool
	Tags   ManagerTagNames
	Parts  map[string]bool // notes, tags, colours, ignored (Part*); nil takes them all
}

type rimSortMod struct {
	path, workshop, note, color string
	ignore                      bool
	tags                        []string
}

// ImportRimSortAux maps RimSort's notes and tags onto lineages; a colour becomes a tag
// with that colour and "ignore warnings" a tag, as RimArcho keeps both as tags. It reads a
// copy; running it again changes nothing.
func (a *App) ImportRimSortAux(opt RimSortAuxOptions) (*RimCrowReport, error) {
	rep := &RimCrowReport{DB: opt.DB, DryRun: opt.DryRun, NotMapped: map[string]int{}, NotesAndTags: true}
	db, done, err := a.openCopy(opt.DB)
	if err != nil {
		return nil, fmt.Errorf("RimSort database: %w", err)
	}
	defer done()
	mods, err := readRimSortMods(db, opt, rep)
	if err != nil {
		return nil, err
	}

	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	lins := newRimSortLineages(ix)
	pd, err := a.Store.PlayerDataAll(ix.Env.ID)
	if err != nil {
		return nil, err
	}
	tags, err := a.newTagImport(ix.Env.ID, opt.DryRun, rep)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(mods))
	for p, m := range mods {
		if m.note != "" || m.color != "" || m.ignore || len(m.tags) > 0 {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	for _, p := range paths {
		m := mods[p]
		rep.Records++
		lin, byPathOK, why := lins.lineageOf(m)
		if lin == "" {
			rep.Unmapped = append(rep.Unmapped, RCItem{p, why})
			continue
		}
		if byPathOK {
			rep.ByPath++
		} else {
			rep.ByPackage++
		}
		cur := pd[lin]
		if cur == nil {
			cur = &store.PlayerData{}
			pd[lin] = cur
		}
		if err := a.applyRimSortMod(opt, rep, ix.Env.ID, lin, m, cur, tags); err != nil {
			return nil, err
		}
	}
	return rep, nil
}

// readRimSortMods reads auxiliary_metadata and the tags, keyed by folder path, keeping
// only the chosen parts.
func readRimSortMods(db *sql.DB, opt RimSortAuxOptions, rep *RimCrowReport) (map[string]*rimSortMod, error) {
	cols, err := tableColumns(db, "auxiliary_metadata")
	if err != nil {
		return nil, err
	}
	if !cols["path"] {
		return nil, fmt.Errorf("this is not RimSort's aux_metadata.db")
	}
	text := func(c string) string {
		if cols[c] {
			return "coalesce(" + c + ",'')"
		}
		return "''"
	}
	ignore := "0"
	if cols["ignore_warnings"] {
		ignore = "coalesce(ignore_warnings,0)"
	}
	mods := map[string]*rimSortMod{}
	rows, err := db.Query(`SELECT path, ` + text("published_file_id") + `, ` + text("user_notes") + `, ` + text("color_hex") + `, ` + ignore + ` FROM auxiliary_metadata`)
	if err != nil {
		return nil, fmt.Errorf("RimSort database: %w", err)
	}
	for rows.Next() {
		m := &rimSortMod{}
		if err := rows.Scan(&m.path, &m.workshop, &m.note, &m.color, &m.ignore); err != nil {
			rows.Close()
			return nil, err
		}
		m.keepParts(opt.Parts, rep)
		mods[m.path] = m
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := readRimSortTags(db, opt, mods); err != nil {
		return nil, err
	}
	return mods, nil
}

// keepParts trims a row and clears what the chosen parts leave out; a colour RimArcho
// cannot read is counted as not mapped.
func (m *rimSortMod) keepParts(parts map[string]bool, rep *RimCrowReport) {
	m.note, m.workshop = strings.TrimSpace(m.note), strings.TrimSpace(m.workshop)
	if !wantPart(parts, PartNotes) {
		m.note = ""
	}
	if !wantPart(parts, PartIgnored) {
		m.ignore = false
	}
	if m.color != "" && managerHex(m.color) == "" {
		rep.NotMapped["color_hex"]++ // not a colour RimArcho can read
		m.color = ""
	}
	if !wantPart(parts, PartColours) {
		m.color = ""
	}
}

// readRimSortTags adds each Mod's tags from tags_table and mod_tags, when they are there
// and wanted.
func readRimSortTags(db *sql.DB, opt RimSortAuxOptions, mods map[string]*rimSortMod) error {
	tags, _ := tableColumns(db, "tags_table")
	if !tags["left_id"] || !tags["right_id"] || !wantPart(opt.Parts, PartTags) {
		return nil
	}
	rows, err := db.Query(`SELECT t.left_id, g.tag FROM tags_table t JOIN mod_tags g ON g.id = t.right_id ORDER BY g.tag`)
	if err != nil {
		return fmt.Errorf("RimSort database: %w", err)
	}
	for rows.Next() {
		var path, tag string
		if err := rows.Scan(&path, &tag); err != nil {
			rows.Close()
			return err
		}
		if m := mods[path]; m != nil {
			m.tags = append(m.tags, tag)
		}
	}
	rows.Close()
	return nil
}

// rimSortLineages maps RimSort's Mod folders onto lineages.
type rimSortLineages struct {
	ix     *Index
	byPath map[string]string // normalised path → lineage
}

// newRimSortLineages records the paths RimArcho knows.
func newRimSortLineages(ix *Index) *rimSortLineages {
	byPath := map[string]string{}
	for _, m := range ix.Mods {
		if !m.TombstoneAt.IsZero() {
			continue
		}
		for _, p := range []string{ix.ModDir(m), gamePath(ix, m)} {
			if p != "" {
				byPath[normPath(p)] = m.LineageID
			}
		}
	}
	return &rimSortLineages{ix: ix, byPath: byPath}
}

// lineageOf maps a RimSort Mod by its path first; by its Workshop ID when the folder moved
// (a Workshop copy RimSort saw under another library, say). It returns the lineage, whether
// the path found it, and why none was found.
func (l *rimSortLineages) lineageOf(m *rimSortMod) (string, bool, string) {
	if lin := l.byPath[normPath(m.path)]; lin != "" {
		return lin, true, ""
	}
	if m.workshop == "" || m.workshop == "0" {
		return "", false, "not installed here"
	}
	var lins []string
	for _, x := range l.ix.Mods {
		if x.TombstoneAt.IsZero() && x.WorkshopID() == m.workshop && !contains(lins, x.LineageID) {
			lins = append(lins, x.LineageID)
		}
	}
	switch len(lins) {
	case 0:
		return "", false, "not installed here"
	case 1:
		return lins[0], false, ""
	}
	return "", false, "more than one installed Mod matches this Workshop ID"
}

// applyRimSortMod brings one Mod's note, tags, colour and ignored mark over to lineage lin,
// whose current data is cur.
func (a *App) applyRimSortMod(opt RimSortAuxOptions, rep *RimCrowReport, envID, lin string, m *rimSortMod, cur *store.PlayerData, tags *tagImport) error {
	if err := a.importNote(rep, envID, lin, m.path, "RimSort", m.note, cur); err != nil {
		return err
	}
	for _, t := range m.tags {
		if err := tags.add(PartTags, lin, cur, t); err != nil {
			return err
		}
	}
	if hex := managerHex(m.color); hex != "" {
		name := opt.Tags.color("RimSort", hex)
		if err := tags.add(PartColours, lin, cur, name); err != nil {
			return err
		}
		if err := tags.color(name, hex); err != nil {
			return err
		}
	}
	if m.ignore {
		if err := tags.add(PartIgnored, lin, cur, opt.Tags.ignore("RimSort")); err != nil {
			return err
		}
	}
	return nil
}

// tableColumns lists a table's columns (none when the table is missing).
func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out[strings.ToLower(c)] = true
	}
	return out, rows.Err()
}
