package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Moving in from another manager, part by part (07 §6.5, 16 §2.18). The Import page shows
// what each manager found on this computer holds, grouped by kind of data, and what each
// part would add; the player picks parts, and each part is brought over on its own. A
// record of earlier imports (when, the file's signature, the list a list file became)
// lets the page say what came in before and what is new since.

// Kinds of data a manager's sources hold, as the page groups them.
const (
	MigrateConfigs = "configs" // RimCrow's profiles: a Game Config each
	MigratePlayer  = "player"  // notes, aliases, tags and colours
	MigrateRules   = "rules"   // user rules and interlocks
	MigrateLists   = "lists"   // saved lists and backups
)

// Part states.
const (
	PartNew     = "new"     // something to bring over
	PartHad     = "had"     // already in RimArcho, nothing new
	PartEmpty   = "empty"   // the source has none
	PartBlocked = "blocked" // cannot come over here (Why says why)
	PartListed  = "listed"  // a list file imported before; comes again only when picked
)

// MigrationPart is one thing the player can tick.
type MigrationPart struct {
	ID      string         `json:"id"` // what StartMigration takes
	Source  string         `json:"source"`
	Path    string         `json:"path"`
	Part    string         `json:"part"`           // Part*, "profile:<id>", "" for a list or rules file
	Name    string         `json:"name,omitempty"` // a profile's or a list's name
	Meta    string         `json:"meta,omitempty"` // a profile's game folder
	New     int            `json:"new"`
	Same    int            `json:"same"`
	State   string         `json:"state"`
	Why     string         `json:"why,omitempty"`
	Default bool           `json:"default"` // picking the manager turns it on
	List    *MigrationList `json:"list,omitempty"`
}

// MigrationList is a list file's facts and what became of it before.
type MigrationList struct {
	Mods    int    `json:"mods"`
	Missing int    `json:"missing"`
	Plan    int64  `json:"plan,omitempty"` // the list it became (still there)
	Name    string `json:"name,omitempty"`
	Changed bool   `json:"changed,omitempty"` // the file changed since
}

// MigrationGroup is one kind of data from one source (all lists of a manager together).
type MigrationGroup struct {
	Kind  string          `json:"kind"`
	Path  string          `json:"path,omitempty"`
	Parts []MigrationPart `json:"parts"`
	Error string          `json:"error,omitempty"`
}

// MigrationManager is one manager found, with when it was last imported.
type MigrationManager struct {
	Name     string           `json:"name"`
	Groups   []MigrationGroup `json:"groups"`
	Imported int64            `json:"imported,omitempty"` // unix ms
}

// MigrationPick is a part the player picked; Mode is for a list imported before: "copy"
// makes another list, "update" replaces the content of the list it became (the window
// does that, as the list editor's import does).
type MigrationPick struct {
	Source, Path, Part, Mode string
}

// MigrationID joins a part's source, path and part.
func MigrationID(source, path, part string) string { return source + "|" + part + "|" + path }

// ParseMigrationID splits it again.
func ParseMigrationID(id string) (MigrationPick, error) {
	f := strings.SplitN(id, "|", 3)
	if len(f) != 3 || f[0] == "" || f[2] == "" {
		return MigrationPick{}, fmt.Errorf("not a part: %q", id)
	}
	return MigrationPick{Source: f[0], Part: f[1], Path: f[2]}, nil
}

// ---- the record of earlier imports ----

const importHistoryKey = "import.history"

// ImportRecord is one source imported before.
type ImportRecord struct {
	When int64  `json:"when"` // unix ms
	Sig  string `json:"sig"`  // the file's signature then
	Plan int64  `json:"plan,omitempty"`
}

func historyKey(path string) string { return strings.ToLower(filepath.Clean(path)) }

// ImportHistory is every source imported before, by path.
func (a *App) ImportHistory() map[string]ImportRecord {
	out := map[string]ImportRecord{}
	if v := a.Store.Meta(importHistoryKey); v != "" {
		json.Unmarshal([]byte(v), &out)
	}
	return out
}

// RecordImport notes that path came in now; plan is the list a list file became (0 keeps
// the one recorded: a copy does not move the record).
func (a *App) RecordImport(path string, plan int64) error {
	h := a.ImportHistory()
	k := historyKey(path)
	r := h[k]
	r.When, r.Sig = a.Now().UnixMilli(), FileSignature(path)
	if plan != 0 {
		r.Plan = plan
	}
	h[k] = r
	b, err := json.Marshal(h)
	if err != nil {
		return err
	}
	return a.Store.SetMeta(importHistoryKey, string(b))
}

// ImportedPlan is the list a list file became when it was imported (0: none recorded).
func (a *App) ImportedPlan(path string) int64 { return a.ImportHistory()[historyKey(path)].Plan }

// FileSignature is a file's content hash ("" when it cannot be read).
func FileSignature(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ---- what each manager holds ----

// SurveyMigration reads every source found (dry runs only) and says, part by part, what it
// would add. It writes nothing.
func (a *App) SurveyMigration(found []ManagerImport, tags ManagerTagNames) []MigrationManager {
	hist := a.ImportHistory()
	byName := map[string]*MigrationManager{}
	var order []string
	for _, f := range found {
		m := byName[f.Manager]
		if m == nil {
			m = &MigrationManager{Name: f.Manager}
			byName[f.Manager] = m
			order = append(order, f.Manager)
		}
		if r, ok := hist[historyKey(f.Path)]; ok && r.When > m.Imported {
			m.Imported = r.When
		}
		switch f.Kind {
		case "list":
			part := a.surveyList(f, hist)
			var lists *MigrationGroup
			for i := range m.Groups {
				if m.Groups[i].Kind == MigrateLists {
					lists = &m.Groups[i]
				}
			}
			if lists == nil {
				m.Groups = append(m.Groups, MigrationGroup{Kind: MigrateLists})
				lists = &m.Groups[len(m.Groups)-1]
			}
			lists.Parts = append(lists.Parts, part)
		case "rimcrow":
			m.Groups = append(m.Groups, a.surveyRimCrow(f, tags)...)
		case "rimsort-aux":
			g := MigrationGroup{Kind: MigratePlayer, Path: f.Path}
			rep, err := a.ImportRimSortAux(RimSortAuxOptions{DB: f.Path, DryRun: true, Tags: tags})
			if err != nil {
				g.Error = err.Error()
			} else {
				for _, p := range []string{PartNotes, PartTags, PartColours, PartIgnored} {
					g.Parts = append(g.Parts, dataPart(f, p, rep.Parts[p], true))
				}
			}
			m.Groups = append(m.Groups, g)
		case "rimsort-rules":
			g := MigrationGroup{Kind: MigrateRules, Path: f.Path}
			rep, err := a.ImportRimSortRules(f.Path, true)
			if err != nil {
				g.Error = err.Error()
			} else {
				g.Parts = append(g.Parts, dataPart(f, PartRules, &PartCount{rep.RuleNew, rep.RuleSame}, true))
			}
			m.Groups = append(m.Groups, g)
		}
	}
	out := make([]MigrationManager, 0, len(order))
	for _, n := range order {
		m := byName[n]
		// a group that could not be read has no parts: an empty list, never null, so the
		// page draws its error and the rest of the manager (B4b VM round)
		for i := range m.Groups {
			if m.Groups[i].Parts == nil {
				m.Groups[i].Parts = []MigrationPart{}
			}
		}
		sort.SliceStable(m.Groups, func(i, j int) bool { return groupRank(m.Groups[i].Kind) < groupRank(m.Groups[j].Kind) })
		out = append(out, *m)
	}
	return out
}

func groupRank(kind string) int {
	return map[string]int{MigrateConfigs: 0, MigratePlayer: 1, MigrateRules: 2, MigrateLists: 3}[kind]
}

func dataPart(f ManagerImport, part string, c *PartCount, def bool) MigrationPart {
	p := MigrationPart{ID: MigrationID(f.Kind, f.Path, part), Source: f.Kind, Path: f.Path, Part: part, Default: def}
	if c != nil {
		p.New, p.Same = c.New, c.Same
	}
	switch {
	case p.New > 0:
		p.State = PartNew
	case p.Same > 0:
		p.State = PartHad
	default:
		p.State = PartEmpty
	}
	return p
}

func (a *App) surveyRimCrow(f ManagerImport, tags ManagerTagNames) []MigrationGroup {
	rep, err := a.ImportRimCrow(RimCrowOptions{DB: f.Path, DryRun: true, IncludeDisabledRules: true, Tags: tags})
	if err != nil {
		return []MigrationGroup{{Kind: MigratePlayer, Path: f.Path, Error: err.Error()}}
	}
	var out []MigrationGroup
	if len(rep.Profiles) > 0 {
		g := MigrationGroup{Kind: MigrateConfigs, Path: f.Path}
		for _, pr := range rep.Profiles {
			p := MigrationPart{ID: MigrationID(f.Kind, f.Path, "profile:"+pr.ID), Source: f.Kind, Path: f.Path, Part: "profile:" + pr.ID, Name: pr.Name, Meta: pr.Game}
			switch {
			case pr.Status == "ready":
				p.State, p.Default, p.New = PartNew, true, 1
			case strings.HasPrefix(pr.Status, "already imported"):
				p.State, p.Same, p.Why = PartHad, 1, pr.Environment
			default:
				p.State, p.Why = PartBlocked, pr.Status
			}
			g.Parts = append(g.Parts, p)
		}
		out = append(out, g)
	}
	data := MigrationGroup{Kind: MigratePlayer, Path: f.Path}
	for _, p := range []string{PartAliases, PartNotes, PartGroups, PartTags, PartColours, PartIgnored} {
		data.Parts = append(data.Parts, dataPart(f, p, rep.Parts[p], true))
	}
	for _, u := range rep.Unmapped {
		if u.Key == "annotations" {
			data.Error = u.Reason // no Game Config yet
		}
	}
	out = append(out, data)
	rules := MigrationGroup{Kind: MigrateRules, Path: f.Path}
	if rep.RulesDisabled {
		rules.Parts = append(rules.Parts, dataPart(f, PartRulesOff, rep.Parts[PartRulesOff], false))
	} else {
		rules.Parts = append(rules.Parts, dataPart(f, PartRules, rep.Parts[PartRules], true))
	}
	rules.Parts = append(rules.Parts, dataPart(f, PartInterlocks, rep.Parts[PartInterlocks], true))
	return append(out, rules)
}

func (a *App) surveyList(f ManagerImport, hist map[string]ImportRecord) MigrationPart {
	p := MigrationPart{ID: MigrationID(f.Kind, f.Path, ""), Source: f.Kind, Path: f.Path, Name: listName(f.Path), State: PartNew, Default: true}
	il, err := readListFile(f.Path)
	if err != nil {
		p.State, p.Why, p.Default = PartBlocked, err.Error(), false
		return p
	}
	if il.Title != "" {
		p.Name = il.Title
	}
	l := &MigrationList{Mods: len(il.Tokens) + len(il.WorkshopOnly)}
	if st, err := a.Loadout(); err == nil {
		have := map[string]bool{}
		ws := map[string]bool{}
		for _, m := range st.Ix.Mods {
			if m.Status == identity.Live {
				have[strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")] = true
				if id := m.WorkshopID(); id != "" {
					ws[id] = true
				}
			}
		}
		for _, t := range il.Tokens {
			if !have[strings.TrimSuffix(strings.ToLower(t), "_steam")] {
				l.Missing++
			}
		}
		for _, id := range il.WorkshopOnly {
			if !ws[id] {
				l.Missing++
			}
		}
	}
	if r, ok := hist[historyKey(f.Path)]; ok && r.Plan != 0 {
		if pl, err := a.Store.PlanByID(r.Plan); err == nil && pl != nil {
			l.Plan, l.Name, l.Changed = pl.ID, pl.Name, r.Sig != FileSignature(f.Path)
			p.State, p.Default, p.Same = PartListed, false, 1
		}
	}
	if p.State == PartNew {
		p.New = 1
	}
	p.List = l
	return p
}

// listName is what a list file is called: its name, or for the game's ModsConfig.xml (a
// RimSort instance's) the folder it is in too.
func listName(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	switch strings.ToLower(base) {
	case "modsconfig":
		return filepath.Base(filepath.Dir(path)) + " " + base
	}
	return base
}

// ---- bringing one part over ----

// MigrationOutcome is what one part came to.
type MigrationOutcome struct {
	Report  *RimCrowReport // data, rules and profiles
	Count   PartCount      // the part's own count
	Plan    *store.Plan    // a list file's new list
	Missing int            // its Mods not installed
}

// MigratePart brings one part over and records the import. A list file becomes a new list
// (Mode "" or "copy"); "update" is the window's (it replaces a list's content in the
// editor's way).
func (a *App) MigratePart(p MigrationPick, tags ManagerTagNames) (*MigrationOutcome, error) {
	out := &MigrationOutcome{}
	var plan int64
	switch {
	case p.Source == "list":
		il, err := readListFile(p.Path)
		if err != nil {
			return nil, err
		}
		base := listName(p.Path)
		if il.Title != "" {
			base = il.Title
		}
		name, err := a.freePlanName(base)
		if err != nil {
			return nil, err
		}
		r, err := a.NewPlanFromImport(name, il, importLabel(p.Path, il))
		if err != nil {
			return nil, err
		}
		out.Plan, out.Missing = r.Plan, len(r.Unresolved)
		if p.Mode != "copy" {
			plan = r.Plan.ID
		}
	case p.Source == "rimcrow" && strings.HasPrefix(p.Part, "profile:"):
		rep, err := a.ImportRimCrow(RimCrowOptions{DB: p.Path, SelectProfiles: true, ProfileIDs: []string{strings.TrimPrefix(p.Part, "profile:")}, Parts: map[string]bool{}, Tags: tags})
		if err != nil {
			return nil, err
		}
		out.Report = rep
		for _, pr := range rep.Profiles {
			if "profile:"+pr.ID == p.Part && pr.Status != "" && !strings.HasPrefix(pr.Status, "imported") && !strings.HasPrefix(pr.Status, "already imported") {
				return out, errors.New(pr.Status)
			}
		}
	case p.Source == "rimcrow":
		rep, err := a.ImportRimCrow(RimCrowOptions{DB: p.Path, SelectProfiles: true, Parts: map[string]bool{p.Part: true}, Tags: tags})
		if err != nil {
			return nil, err
		}
		out.Report = rep
	case p.Source == "rimsort-aux":
		rep, err := a.ImportRimSortAux(RimSortAuxOptions{DB: p.Path, Parts: map[string]bool{p.Part: true}, Tags: tags})
		if err != nil {
			return nil, err
		}
		out.Report = rep
	case p.Source == "rimsort-rules":
		rep, err := a.ImportRimSortRules(p.Path, false)
		if err != nil {
			return nil, err
		}
		rep.part(PartRules).New, rep.part(PartRules).Same = rep.RuleNew, rep.RuleSame
		out.Report = rep
	default:
		return nil, fmt.Errorf("cannot bring %q over", p.Source)
	}
	if out.Report != nil && out.Report.Parts[p.Part] != nil {
		out.Count = *out.Report.Parts[p.Part]
	}
	return out, a.RecordImport(p.Path, plan)
}

// freePlanName is base, or base 2, base 3 … when a list has that name.
func (a *App) freePlanName(base string) (string, error) {
	base = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(base), "-"))
	if base == "" {
		base = "import"
	}
	for utf8.RuneCountInString(base) > 60 {
		_, size := utf8.DecodeLastRuneInString(base)
		base = base[:len(base)-size]
	}
	var err error
	for i := 1; i < 100; i++ {
		n := base
		if i > 1 {
			n = fmt.Sprintf("%s %d", base, i)
		}
		if n, err = a.checkPlanName(n, 0); err == nil {
			return n, nil
		}
	}
	return "", err
}

// PartView is what one part holds, for the Import page's "view" (16 §2.18): a list file to
// compare (a list, a Game Config's ModsConfig.xml), or the part's values one by one.
type PartView struct {
	List  string     `json:"list,omitempty"`
	Items []PartItem `json:"items"`
	More  bool       `json:"more,omitempty"` // there are more values than Items holds
}

// MigrationPartView reads what a part would bring over, without bringing it (dry runs).
func (a *App) MigrationPartView(p MigrationPick, tags ManagerTagNames) (*PartView, error) {
	var rep *RimCrowReport
	var err error
	switch {
	case p.Source == "list":
		return &PartView{List: p.Path, Items: []PartItem{}}, nil
	case p.Source == "rimcrow" && strings.HasPrefix(p.Part, "profile:"):
		id := strings.TrimPrefix(p.Part, "profile:")
		rep, err = a.ImportRimCrow(RimCrowOptions{DB: p.Path, DryRun: true, SelectProfiles: true, ProfileIDs: []string{id}, Parts: map[string]bool{}, Tags: tags})
		if err != nil {
			return nil, err
		}
		for _, pr := range rep.Profiles {
			if pr.ID == id && pr.UserData != "" {
				return &PartView{List: filepath.Join(pr.UserData, "Config", "ModsConfig.xml"), Items: []PartItem{}}, nil
			}
		}
		return nil, fmt.Errorf("no list found for the profile %q", id)
	case p.Source == "rimcrow":
		rep, err = a.ImportRimCrow(RimCrowOptions{DB: p.Path, DryRun: true, IncludeDisabledRules: true, Parts: map[string]bool{p.Part: true}, Tags: tags})
	case p.Source == "rimsort-aux":
		rep, err = a.ImportRimSortAux(RimSortAuxOptions{DB: p.Path, DryRun: true, Parts: map[string]bool{p.Part: true}, Tags: tags})
	case p.Source == "rimsort-rules":
		rep, err = a.ImportRimSortRules(p.Path, true)
	default:
		return nil, fmt.Errorf("cannot read %q", p.Source)
	}
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	if ix, err := a.Index(); err == nil {
		for _, m := range ix.Mods {
			if m.TombstoneAt.IsZero() && names[m.LineageID] == "" {
				names[m.LineageID] = m.DisplayName()
			}
		}
	}
	name := func(lin string) string {
		if n := names[lin]; n != "" {
			return n
		}
		return lin
	}
	out := &PartView{Items: []PartItem{}, More: len(rep.Items) >= maxPartItems}
	for _, it := range rep.Items {
		if it.Part != p.Part {
			continue
		}
		it.Mod = name(it.Lin)
		if it.Other != "" {
			it.With = name(it.Other)
		}
		out.Items = append(out.Items, it)
	}
	return out, nil
}
