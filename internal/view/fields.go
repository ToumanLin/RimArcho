package view

import (
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// The detail's one field list (06 §4.1, §2.9): the plan editor's detail and a copy's
// detail on Mod library show the same list, one row per field. In the window a value copies
// itself when clicked ("copied"); Act is the row's own button.

// Field kinds.
const (
	FieldID        = "id"
	FieldPackage   = "package"
	FieldPath      = "path"
	FieldWorkshop  = "workshop"
	FieldTags      = "tags"
	FieldAlias     = "alias"
	FieldNote      = "note"
	FieldState     = "state"
	FieldUsed      = "used"
	FieldSize      = "size"
	FieldVersions  = "versions"
	FieldUpdated   = "updated"
	FieldPlayed    = "played"
	FieldTextures  = "textures" // Texture optimisation: the DDS RimArcho wrote (Mod library)
	FieldLanguages = "languages"
	FieldDeps      = "deps"
	FieldConflicts = "conflicts"
	FieldSettings  = "settings" // the Mod's settings files; the act opens Mod library › Mod settings
)

// Field acts: what the row's button does.
const (
	ActFolder = "folder" // open the location
	ActURL    = "url"    // open the page
	ActSteam  = "steam"  // Steam client page
	ActTags   = "tags"   // edit (the tag bar)
	// ActSettings opens Mod library › Mod settings at Target's files (06 §2.9)
	ActSettings = "settings"
)

// Field is one row of the field list.
type Field struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	// Value is what the row shows and what a click copies; List the parts of a list value
	// (tags, languages), Items the mods of deps / conflicts with their state.
	Value   string        `json:"value"`
	List    []string      `json:"list,omitempty"`
	Items   []Dep         `json:"items,omitempty"`
	Act     string        `json:"act,omitempty"`
	ActAt   string        `json:"actLabel,omitempty"`
	URL     string        `json:"url,omitempty"`
	Extra   string        `json:"extra,omitempty"`  // the alias's original name
	Target  string        `json:"target,omitempty"` // the instance ActSettings opens
	Actions []FieldAction `json:"actions,omitempty"`
}

// FieldAction is an additional button beside a field's primary action.
type FieldAction struct {
	Act   string `json:"act"`
	Label string `json:"label"`
}

func workshopField(id string) Field {
	return Field{Kind: FieldWorkshop, Label: T("workshop"), Value: id,
		Act: ActURL, ActAt: T("web page"), URL: WorkshopURL(id),
		Actions: []FieldAction{{Act: ActSteam, Label: T("steam page")}}}
}

// ModFields: Archo ID, package, location [open], Workshop [open], tags [edit], alias and
// note when set, languages, dependencies, conflicts (06 §4.1).
func ModFields(a *app.App, st *app.LoadoutState, mod *store.Mod, d *DetailData) []Field {
	fs := headFields(st, mod)
	if a != nil {
		fs = appendSubscriptionFields(fs, a.SubscriptionOf(mod))
	}
	fs = append(fs, Field{Kind: FieldTags, Label: T("tags"), Value: strings.Join(mod.Tags, "  "), List: mod.Tags, Act: ActTags, ActAt: T("edit")})
	if mod.Alias != "" {
		fs = append(fs, Field{Kind: FieldAlias, Label: T("alias"), Value: mod.Alias, Extra: mod.Name})
	}
	if mod.Note != "" {
		fs = append(fs, Field{Kind: FieldNote, Label: T("note"), Value: mod.Note})
	}
	return append(fs, tailFields(a, st, mod, d)...)
}

func headFields(st *app.LoadoutState, mod *store.Mod) []Field {
	ref := mod.ID
	if st != nil {
		ref = st.Ix.Ref(mod.ID)
	}
	fs := []Field{{Kind: FieldID, Label: T("Archo ID"), Value: ref}}
	if mod.PackageID != "" {
		fs = append(fs, Field{Kind: FieldPackage, Label: T("package"), Value: mod.PackageID})
	}
	if st != nil && mod.Status == identity.Live {
		if p := st.Ix.ModDir(mod); p != "" {
			fs = append(fs, Field{Kind: FieldPath, Label: T("path"), Value: p, Act: ActFolder, ActAt: T("location")})
		}
	}
	if ws := mod.WorkshopID(); IsNumber(ws) {
		fs = append(fs, workshopField(ws))
	}
	return fs
}

// tailFields: settings files, languages, dependencies, conflicts.
func tailFields(a *app.App, st *app.LoadoutState, mod *store.Mod, d *DetailData) []Field {
	var fs []Field
	if st != nil {
		if n, ok := app.StoredSettingsOf(st, mod); ok {
			fs = append(fs, Field{Kind: FieldSettings, Label: T("mod settings"), Value: Tf("%d stored values", n),
				Act: ActSettings, ActAt: T("open"), Target: mod.ID})
		}
	}
	if a != nil && mod.SnapID != 0 {
		if ls, err := a.Store.Languages(mod.SnapID); err == nil && len(ls) > 0 {
			fs = append(fs, Field{Kind: FieldLanguages, Label: T("languages"), Value: strings.Join(ls, "  "), List: ls})
		}
	}
	if deps := Deps(st, d); len(deps) > 0 {
		fs = append(fs, Field{Kind: FieldDeps, Label: T("requires"), Value: depNames(deps), Items: deps})
	}
	if cs := Conflicts(st, d); len(cs) > 0 {
		fs = append(fs, Field{Kind: FieldConflicts, Label: T("conflicts with"), Value: depNames(cs), Items: cs})
	}
	return fs
}

func depNames(ds []Dep) string {
	names := make([]string, len(ds))
	for i, x := range ds {
		names[i] = x.Name
	}
	return strings.Join(names, "  ")
}

// Conflicts: the declared incompatibilities (plus the current version's), each once, with
// the state of the mod named: enabled (a clash), installed but not enabled, or not here.
func Conflicts(st *app.LoadoutState, d *DetailData) []Dep {
	if d == nil || st == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []Dep
	for _, p := range append(append([]string{}, d.Decl.IncompatibleWith...), d.Decl.IncompatibleByVersion["v"+st.GameMM()]...) {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		x := Dep{Name: p, State: DepMissing}
		for _, m := range st.Ix.Mods {
			if m.PackageID != p || m.Status != identity.Live {
				continue
			}
			if x.State == DepMissing {
				x = Dep{Name: m.DisplayName(), State: DepDisabled}
			}
			if st.List.Index(m.ID) >= 0 {
				x = Dep{Name: m.DisplayName(), State: DepEnabled}
			}
		}
		out = append(out, x)
	}
	return out
}

// LibFields is a copy's field list on Mod library: the editor's fields with the library's in
// between (state, used in, size, versions, update date, last played). The note is above
// the list (CopyNote), not a row.
func LibFields(a *app.App, lib *app.Library, i *app.LibInstance, d *DetailData) []Field {
	st := lib.St
	var fs []Field
	if i.Mod != nil {
		fs = headFields(st, i.Mod)
		fs = append(fs, Field{Kind: FieldTags, Label: T("tags"), Value: strings.Join(i.Mod.Tags, "  "), List: i.Mod.Tags, Act: ActTags, ActAt: T("edit")})
	} else if IsNumber(i.WorkshopID) {
		fs = append(fs, workshopField(i.WorkshopID))
	}
	var states []string
	fs = appendSubscriptionFields(fs, i.Subscription)
	for _, s := range i.States {
		states = append(states, LibStateText(s, lib.GameMM))
	}
	add := func(kind, label, v string, list []string) {
		if v != "" {
			fs = append(fs, Field{Kind: kind, Label: label, Value: v, List: list})
		}
	}
	add(FieldState, T("state"), strings.Join(states, "  "), states)
	var uses []string
	for _, u := range i.UsedIn {
		uses = append(uses, libUseText(u.Value()))
	}
	add(FieldUsed, T("used in"), strings.Join(uses, "  "), uses)
	if i.Sized {
		add(FieldSize, T("size"), render.Size(i.Bytes), nil)
	}
	add(FieldVersions, T("supported versions"), strings.Join(i.Versions, " "), nil)
	if !i.Updated.IsZero() {
		add(FieldUpdated, T("update date"), LongDate(i.Updated), nil)
	}
	if !i.Played.IsZero() {
		add(FieldPlayed, T("last played"), LongDate(i.Played), nil)
	}
	add(FieldTextures, T("textures"), TextureField(i), nil)
	if i.Mod != nil {
		fs = append(fs, tailFields(a, st, i.Mod, d)...)
	}
	return fs
}

// LongDate is a localized date with the year, such as Oct 21, 2026.
func LongDate(t time.Time) string {
	t = t.Local()
	if Lang() == "zh" {
		return t.Format(localeExtra("zh", "@date.long"))
	}
	return t.Format("Jan 2, 2006")
}
