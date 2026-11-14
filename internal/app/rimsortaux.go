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
		m.note, m.workshop = strings.TrimSpace(m.note), strings.TrimSpace(m.workshop)
		if m.color != "" && managerHex(m.color) == "" {
			rep.NotMapped["color_hex"]++ // not a colour RimArcho can read
			m.color = ""
		}
		mods[m.path] = m
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if tags, _ := tableColumns(db, "tags_table"); tags["left_id"] && tags["right_id"] {
		rows, err := db.Query(`SELECT t.left_id, g.tag FROM tags_table t JOIN mod_tags g ON g.id = t.right_id ORDER BY g.tag`)
		if err != nil {
			return nil, fmt.Errorf("RimSort database: %w", err)
		}
		for rows.Next() {
			var path, tag string
			if err := rows.Scan(&path, &tag); err != nil {
				rows.Close()
				return nil, err
			}
			if m := mods[path]; m != nil {
				m.tags = append(m.tags, tag)
			}
		}
		rows.Close()
	}

	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
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
	// The path first; a Workshop ID when the folder moved (a Workshop copy RimSort saw
	// under another library, say).
	lineageOf := func(m *rimSortMod) (string, bool, string) {
		if l := byPath[normPath(m.path)]; l != "" {
			return l, true, ""
		}
		if m.workshop == "" || m.workshop == "0" {
			return "", false, "not installed here"
		}
		var lins []string
		for _, x := range ix.Mods {
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
		lin, byPathOK, why := lineageOf(m)
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
		switch {
		case m.note == "":
		case cur.Note == m.note:
			rep.NoteSame++
		case cur.Note != "":
			rep.Conflicts = append(rep.Conflicts, RCItem{p, fmt.Sprintf("note: RimArcho has %q, RimSort %q (kept RimArcho's)", cur.Note, m.note)})
		default:
			rep.NoteNew++
			cur.Note = m.note
			if !opt.DryRun {
				if err := a.Store.SetNote(ix.Env.ID, lin, m.note); err != nil {
					return nil, err
				}
			}
		}
		for _, t := range m.tags {
			if err := tags.add(lin, cur, t); err != nil {
				return nil, err
			}
		}
		if hex := managerHex(m.color); hex != "" {
			name := opt.Tags.color("RimSort", hex)
			if err := tags.add(lin, cur, name); err != nil {
				return nil, err
			}
			if err := tags.color(name, hex); err != nil {
				return nil, err
			}
		}
		if m.ignore {
			if err := tags.add(lin, cur, opt.Tags.ignore("RimSort")); err != nil {
				return nil, err
			}
		}
	}
	return rep, nil
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
