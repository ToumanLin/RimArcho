package view

import (
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/order"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// The three list columns (16 §2.5, 16 §1): not enabled | enabled | staging.

// Column is a list column.
type Column int

const (
	Inactive Column = iota
	Active
	Staging
)

// Row status, worst first wins (16 §7: one status icon at most).
const (
	StatusNone = iota
	StatusUnloadable
	StatusWarn
	StatusLog
	StatusErr
)

// StatusHint is informational and sorts below warnings without changing existing wire values.
const StatusHint = -1

// StatusVersion marks a row whose only reminder is that it does not declare the game's
// version (version.unsupported, 16 §5): a quieter icon than a warning, below hints.
const StatusVersion = -2

// Item is one row of a column.
type Item struct {
	Lists       []string          `json:"-"`
	Lineage     string            `json:"lineage,omitempty"`
	CopyLabel   string            `json:"copyLabel,omitempty"`
	CopyCount   int               `json:"copyCount,omitempty"`
	Sources     []string          `json:"-"`
	SourceNames map[string]string `json:"-"`
	ID          string            `json:"id"`
	Ref         string            `json:"ref"`
	Name        string            `json:"name"` // the alias when there is one
	Kind        string            `json:"kind"` // official, workshop, local, copy, git (16 §3.7 type:)
	SteamCMD    bool              `json:"steamcmd,omitempty"`
	Pos         int               `json:"pos,omitempty"`     // 1-based load position (enabled column)
	Unsaved     bool              `json:"unsaved,omitempty"` // changed since the last save
	Dim         bool              `json:"dim,omitempty"`     // cannot be loaded
	Missing     bool              `json:"missing,omitempty"` // a ModsConfig entry with no installed mod
	Gone        bool              `json:"gone,omitempty"`    // its folder is no longer on disk (a tombstone)
	CanUse      bool              `json:"canUse,omitempty"`  // missing, but an installed copy can take its place
	Tags        []string          `json:"tags,omitempty"`
	Alias       bool              `json:"alias,omitempty"`
	Note        string            `json:"-"`
	Package     string            `json:"-"`
	Workshop    string            `json:"-"`
	Author      string            `json:"-"`
	Versions    []string          `json:"-"` // the game versions it supports ("1.6")
	Location    string            `json:"-"` // the registered source root, including for linked mods

	// Cat is the Mod's category (app.CategoryKeys, 16 §2.5): the row's marker and the
	// category: filter; CatSet when the player chose it.
	Cat    string `json:"cat,omitempty"`
	CatSet bool   `json:"catSet,omitempty"`

	Lower string `json:"-"` // sort key
	// New: a scan first found the Mod within app.NewFor, after the game's first scan
	// (16 §2.5); added are its added: values.
	New    bool `json:"new,omitempty"`
	added  []string
	sortAt int64 // the Disabled sort's time for its Mod (sortDisabled)
	// search fields (16 §3.7), lower case
	names, author, text string
}

// Board is the three columns and the counts around them.
type Board struct {
	Lists       *app.ListMembership `json:"-"`
	St          *app.LoadoutState   `json:"-"`
	Cols        [3][]Item           `json:"cols"`
	EnvName     string              `json:"env"`
	GameVersion string              `json:"gameVersion"`
	Unsaved     int                 `json:"unsaved"`   // workspace changes since the last save
	Missing     int                 `json:"missing"`   // entries with no installed mod
	Questions   int                 `json:"questions"` // open identity questions
	Changes     int                 `json:"changes"`   // events of the last scan
	LastEvents  map[string]int      `json:"lastEvents,omitempty"`
	// TagColors is every coloured tag's colour, by lower-cased tag: a row is marked with
	// its first tag's (16 §2.9 Tag manager)
	TagColors map[string]string `json:"tagColors,omitempty"`
}

// BuildBoard builds the columns from the workspace (synced with ModsConfig.xml) and
// staging. On error the board still carries the state when there is one.
func BuildBoard(a *app.App) (*Board, error) {
	staged, st, err := a.Staged()
	b := &Board{St: st}
	if err != nil {
		return b, err
	}
	b.Lists, err = a.ListsFor(st)
	if err != nil {
		return b, err
	}
	b.EnvName, b.GameVersion = st.Ix.Env.Name, st.Ix.Env.GameVersion
	chs := st.Changes()
	b.Unsaved = len(chs)
	unsaved := map[string]bool{}
	for _, c := range chs {
		unsaved[c.Entry.Key()] = true
	}
	inList := map[string]bool{}
	var missingRows map[string]app.MissingMod
	for i, e := range st.List {
		if e.Instance == "" {
			b.Missing++
			// Show the mod's name (import, earlier copy, steamDB), not only its packageId.
			if missingRows == nil {
				missingRows = a.MissingByToken(st)
			}
			m := missingRows[e.Missing]
			name := m.Name
			if name == "" {
				name = e.Missing
			}
			b.Cols[Active] = append(b.Cols[Active], Item{Pos: i + 1, Ref: "pkg:" + e.Missing, Name: name, Lower: strings.ToLower(name + " " + e.Missing), Missing: true, CanUse: m.CanEnable, Unsaved: unsaved[e.Key()]})
			continue
		}
		inList[e.Instance] = true
		if mod := st.Ix.ByID[e.Instance]; mod != nil {
			it := ModItem(st, mod)
			it.Pos, it.Unsaved = i+1, unsaved[e.Key()]
			b.Cols[Active] = append(b.Cols[Active], it)
		}
	}
	isStaged := map[string]bool{}
	for _, s := range staged {
		isStaged[s.Instance] = true
		b.Cols[Staging] = append(b.Cols[Staging], ModItem(st, st.Ix.ByID[s.Instance]))
	}
	for _, mod := range st.Ix.Mods {
		if inList[mod.ID] || isStaged[mod.ID] || mod.Status != identity.Live {
			continue
		}
		b.Cols[Inactive] = append(b.Cols[Inactive], ModItem(st, mod))
	}
	counts := map[string]int{}
	for _, m := range st.Ix.Mods {
		if m.Status == identity.Live {
			counts[m.LineageID]++
		}
	}
	// Different releases can have different About.xml names. Use one heading per
	// lineage, preferring the name already used in the enabled list.
	names := map[string]string{}
	for _, k := range []Column{Active, Staging, Inactive} {
		for _, it := range b.Cols[k] {
			if it.Lineage != "" && names[it.Lineage] == "" {
				names[it.Lineage] = it.Name
			}
		}
	}
	added, newSince, now := app.Added(st), a.NewSince(st.Ix.Env.GameID), a.Now()
	cats := st.Ix.Categories()
	for k := range b.Cols {
		for i := range b.Cols[k] {
			it := &b.Cols[k][i]
			it.Lists = b.Lists.ByInstance[it.ID]
			m := st.Ix.ByID[it.ID]
			if m == nil {
				continue
			}
			it.New = app.IsNew(added[m.LineageID], newSince)
			c := cats[m.ID]
			it.Cat, it.CatSet = c.Key, c.Player
			it.added = app.AddedValues(added[m.LineageID], it.New, now)
			if k == int(Inactive) {
				it.Name = names[it.Lineage]
				it.Lower = strings.ToLower(it.Name)
				it.names += " " + it.Lower
				it.text += " " + it.Lower
			}
			it.CopyCount = counts[m.LineageID]
			it.CopyLabel = CopyCaption(a, st, m)
			s := st.Ix.Source(m.SourceID)
			it.Sources = a.DirectoryValues(s)
			it.SourceNames = map[string]string{}
			if s != nil && a.DirectoryRole(s) == "other" {
				it.SourceNames[app.DirectoryAlias(s)] = app.DirectoryName(s)
			}
		}
	}
	sortDisabled(st, b.Cols[Inactive], added, a.Store.Meta(DisabledSortKey))
	if c, err := a.Store.TagColors(); err == nil && len(c) > 0 {
		b.TagColors = c
	}
	if qs, err := a.Store.Questions(st.Ix.Env.GameID, false); err == nil {
		b.Questions = len(qs)
	}
	if sc, ss, _ := a.LastScan(st.Ix.Env.GameID); sc != nil && ss != nil {
		b.LastEvents = ss.Events
		for _, n := range ss.Events {
			b.Changes += n
		}
	}
	return b, nil
}

// ModItem is the row of one installed mod.
func ModItem(st *app.LoadoutState, mod *store.Mod) Item {
	it := Item{Lineage: mod.LineageID, ID: mod.ID, Ref: st.Ix.Ref(mod.ID), Name: mod.DisplayName()}
	it.Lower = strings.ToLower(it.Name)
	it.names = strings.ToLower(it.Name + " " + mod.Name)
	it.Author, it.author = mod.Authors, strings.ToLower(mod.Authors)
	it.text = it.names + " " + it.author
	it.Package, it.Workshop = mod.PackageID, mod.WorkshopID()
	it.Tags, it.Note, it.Alias = mod.Tags, mod.Note, mod.Alias != ""
	it.Versions = strings.Fields(mod.SupportedVersions)
	it.Kind, it.SteamCMD = SourceKind(mod), mod.Kind == source.SteamCMD
	if s := st.Ix.Source(mod.SourceID); s != nil {
		it.Location = s.Root
	}
	it.Gone = mod.Status != identity.Live
	it.Dim = mod.Unloadable != "" || mod.Residue || (!st.Reg.Known(mod.ID) && !st.Deployable(mod)) || (st.Reg.Known(mod.ID) && !st.Reg.Loadable(mod.ID))
	return it
}

// RowStatus is the row's status icon: problems from the last analysis (status, by
// instance), missing entries, unloadable mods.
func RowStatus(it *Item, status map[string]int) int {
	s := status[it.ID]
	if it.Missing {
		s = StatusErr
	}
	if s == StatusNone && it.Dim {
		s = StatusUnloadable
	}
	return s
}

// SourceKind is the type: value of a mod's source (16 §3.7).
func SourceKind(m *store.Mod) string {
	switch {
	case m.Kind == source.Official:
		return "official"
	case m.Kind.WorkshopLike():
		return "workshop"
	case m.GitHead != "":
		return "git"
	case m.Origin != "":
		return "copy"
	}
	return "local"
}

// SourceText names the source in the detail.
func SourceText(m *store.Mod) string {
	switch SourceKind(m) {
	case "official":
		return T("official")
	case "workshop":
		return T("workshop")
	case "git":
		return "Git"
	case "copy":
		return T("local copy")
	}
	return T("local")
}

// DisabledSortKey keeps the Disabled column's order (16 §2.5): "" or "name", "updated"
// (the Workshop's update time), "modified" (the folder's modification time) or "added"
// (when a scan first found the Mod).
const DisabledSortKey = "ui.gui.plan.disabledsort"

// DisabledSorts are the Disabled column's orders, the default first.
var DisabledSorts = []string{"name", "updated", "modified", "added"}

// sortDisabled orders Not enabled (16 §2.5): new Mods first in every order, then by the
// chosen time, newest first (a Mod's newest copy counts, so its copies stay together; Mods
// without the time follow), then the official ones, by name and by copy. Only "modified"
// reads the disk, one stat per disabled copy.
func sortDisabled(st *app.LoadoutState, ina []Item, added map[string]time.Time, by string) {
	if by != "" && by != "name" {
		at := map[string]int64{}
		for i := range ina {
			m := st.Ix.ByID[ina[i].ID]
			if m == nil {
				continue
			}
			var t int64
			switch by {
			case "updated":
				t = m.ACFTimeUpdated
			case "modified":
				if fi, err := os.Stat(st.Ix.ModDir(m)); err == nil {
					t = fi.ModTime().Unix()
				}
			case "added":
				if f := added[m.LineageID]; !f.IsZero() {
					t = f.Unix()
				}
			}
			if t > at[ina[i].Lineage] {
				at[ina[i].Lineage] = t
			}
		}
		for i := range ina {
			ina[i].sortAt = at[ina[i].Lineage]
		}
	}
	sort.SliceStable(ina, func(i, j int) bool {
		if ina[i].New != ina[j].New {
			return ina[i].New
		}
		if ina[i].sortAt != ina[j].sortAt {
			return ina[i].sortAt > ina[j].sortAt
		}
		if c := order.CompareOfficial(ina[i].Package, ina[j].Package); c != 0 {
			return c < 0
		}
		if ina[i].Lower != ina[j].Lower {
			return ina[i].Lower < ina[j].Lower
		}
		if ina[i].Lineage != ina[j].Lineage {
			return ina[i].Lineage < ina[j].Lineage
		}
		return ina[i].CopyLabel < ina[j].CopyLabel
	})
}
