package view

import (
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// The three list columns (06 §5, 08 §1.1): not enabled | enabled | staging.

// Column is a list column.
type Column int

const (
	Inactive Column = iota
	Active
	Staging
)

// Row status, worst first wins (06 §3: one status icon at most).
const (
	StatusNone = iota
	StatusUnloadable
	StatusWarn
	StatusLog
	StatusErr
)

// Item is one row of a column.
type Item struct {
	ID       string   `json:"id"`
	Ref      string   `json:"ref"`
	Name     string   `json:"name"` // the alias when there is one
	Kind     string   `json:"kind"` // official, workshop, local, copy, git (06 §5.5 type:)
	SteamCMD bool     `json:"steamcmd,omitempty"`
	Pos      int      `json:"pos,omitempty"`     // 1-based load position (enabled column)
	Unsaved  bool     `json:"unsaved,omitempty"` // changed since the last save
	Dim      bool     `json:"dim,omitempty"`     // cannot be loaded
	Missing  bool     `json:"missing,omitempty"` // a ModsConfig entry with no installed mod
	Tags     []string `json:"tags,omitempty"`
	Alias    bool     `json:"alias,omitempty"`
	Note     string   `json:"-"`
	Package  string   `json:"-"`
	Workshop string   `json:"-"`
	Author   string   `json:"-"`

	Lower string `json:"-"` // sort key
	// search fields (06 §5.5), lower case
	names, author, text string
}

// Board is the three columns and the counts around them.
type Board struct {
	St          *app.LoadoutState `json:"-"`
	Cols        [3][]Item         `json:"cols"`
	EnvName     string            `json:"env"`
	GameVersion string            `json:"gameVersion"`
	Unsaved     int               `json:"unsaved"`   // workspace changes since the last save
	Missing     int               `json:"missing"`   // entries with no installed mod
	Questions   int               `json:"questions"` // open identity questions
	Changes     int               `json:"changes"`   // events of the last scan
	LastEvents  map[string]int    `json:"lastEvents,omitempty"`
}

// BuildBoard builds the columns from the workspace (synced with ModsConfig.xml) and
// staging. On error the board still carries the state when there is one.
func BuildBoard(a *app.App) (*Board, error) {
	staged, st, err := a.Staged()
	b := &Board{St: st}
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
	for i, e := range st.List {
		if e.Instance == "" {
			b.Missing++
			b.Cols[Active] = append(b.Cols[Active], Item{Pos: i + 1, Ref: "pkg:" + e.Missing, Name: e.Missing, Lower: e.Missing, Missing: true, Unsaved: unsaved[e.Key()]})
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
	ina := b.Cols[Inactive]
	sort.SliceStable(ina, func(i, j int) bool { return ina[i].Lower < ina[j].Lower })
	if qs, err := a.Store.Questions(st.Ix.Env.ID, false); err == nil {
		b.Questions = len(qs)
	}
	if sc, ss, _ := a.LastScan(st.Ix.Env.ID); sc != nil && ss != nil {
		b.LastEvents = ss.Events
		for _, n := range ss.Events {
			b.Changes += n
		}
	}
	return b, nil
}

// ModItem is the row of one installed mod.
func ModItem(st *app.LoadoutState, mod *store.Mod) Item {
	it := Item{ID: mod.ID, Ref: st.Ix.Ref(mod.ID), Name: mod.DisplayName()}
	it.Lower = strings.ToLower(it.Name)
	it.names = strings.ToLower(it.Name + " " + mod.Name)
	it.Author, it.author = mod.Authors, strings.ToLower(mod.Authors)
	it.text = it.names + " " + it.author
	it.Package, it.Workshop = mod.PackageID, mod.WorkshopID()
	it.Tags, it.Note, it.Alias = mod.Tags, mod.Note, mod.Alias != ""
	it.Kind, it.SteamCMD = SourceKind(mod), mod.Kind == source.SteamCMD
	it.Dim = mod.Unloadable != "" || mod.Residue || !st.Reg.Known(mod.ID) || !st.Reg.Loadable(mod.ID)
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

// SourceKind is the type: value of a mod's source (06 §5.5).
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
