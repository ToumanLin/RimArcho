package view

import (
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/logs"
)

// Mod library › Mod settings (06 §2.9 "Mod settings"): the current list's Mods that have
// settings, one row per settings file or HugsLib settings pack, and what one stores. Read
// only: values change in the game's settings window.

// ModSettingsPage is the page's table.
type ModSettingsPage struct {
	Plan   string           `json:"plan"`
	Config string           `json:"config"` // the Config folder read
	Rows   []ModSettingsRow `json:"rows"`
}

// ModSettingsRow is one Mod class's settings file, or one HugsLib pack.
type ModSettingsRow struct {
	Key      string `json:"key"` // instance and class: unique on the page
	Instance string `json:"instance"`
	Name     string `json:"name"`
	Title    string `json:"title"` // the settings window's title; "—" unknown
	Class    string `json:"class"` // the file's key among the Mod's (app.ModSettingsFile.Key)
	File     string `json:"file"`
	Exists   bool   `json:"exists"`
	Bytes    int64  `json:"bytes"`
	Size     string `json:"size"`
	Count    int    `json:"count"`  // stored values; -1 no file or unreadable
	Values   string `json:"values"` // Count as shown: "12", "defaults", "cannot read"
}

// ModSettingsRows makes the table from app.ModSettings. Each file is read for its count
// (bounded by app.ReadSettingsFile).
func ModSettingsRows(l *app.ModSettingsList) *ModSettingsPage {
	p := &ModSettingsPage{Plan: strings.TrimPrefix(l.Plan, "plan:"), Config: l.Config, Rows: []ModSettingsRow{}}
	for _, m := range l.Mods {
		for _, f := range m.Files {
			r := ModSettingsRow{Key: m.Instance + "|" + f.Key(), Instance: m.Instance, Name: m.Name, Title: settingsTitle(&f),
				Class: f.Key(), File: f.File, Exists: f.Exists, Bytes: f.Size, Size: "—", Count: -1, Values: T("defaults")}
			if f.Exists {
				if f.Kind != app.SettingsKindHugsLib {
					r.Size = SizeText(f.Size)
				}
				if sc, err := f.Read(); err != nil {
					r.Values = T("cannot read")
				} else {
					r.Count = len(sc.Values)
					r.Values = strconv.Itoa(r.Count)
				}
			}
			p.Rows = append(p.Rows, r)
		}
	}
	return p
}

// settingsTitle is the title column: the settings window's title, or HugsLib and the pack's
// entry; "—" unknown.
func settingsTitle(f *app.ModSettingsFile) string {
	switch {
	case f.Kind == app.SettingsKindHugsLib && f.Title != "":
		return Tf("HugsLib: %s", f.Title)
	case f.Kind == app.SettingsKindHugsLib:
		return "HugsLib"
	case f.Title == "":
		return "—"
	}
	return f.Title
}

// ModSettingsDetail is what one settings file or HugsLib pack stores.
type ModSettingsDetail struct {
	Key    string       `json:"key"`
	Kind   string       `json:"kind,omitempty"` // "hugslib": a HugsLib pack
	Name   string       `json:"name"`
	Title  string       `json:"title"`
	Class  string       `json:"class"` // the Mod class (the file name's last part)
	Type   string       `json:"type"`  // the settings type the file names
	File   string       `json:"file"`
	Path   string       `json:"path"`
	Folder string       `json:"folder"` // the folder the game sees the Mod under
	Exists bool         `json:"exists"`
	Error  string       `json:"error,omitempty"`
	Stored int          `json:"stored"` // values the file stores
	Values []SettingRow `json:"values"`
	Notes  []string     `json:"notes"`
	// Window is what the game's settings window drew when the player last opened it (Archo
	// Monitor, 03 §6.7.1); nil when never seen.
	Window *SettingsWindowView `json:"window,omitempty"`
}

// SettingsWindowView is a settings window as Archo Monitor saw it.
type SettingsWindowView struct {
	Seen     string      `json:"seen"` // the date it was seen
	Full     bool        `json:"full"` // false: the window drew more than Monitor keeps
	Controls []WindowRow `json:"controls"`
}

// WindowRow is one thing the window drew: a heading (Kind label), a button, or a control
// with its value as text.
type WindowRow struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Text  string `json:"text"`
}

// SettingRow is one stored value as the page shows it.
type SettingRow struct {
	Key  string `json:"key"`
	Kind string `json:"kind"` // bool, number, text; list, map, object, null
	Text string `json:"text"`
	// Fixed: a list, dictionary, nested value or null; shown as a count, never changed here.
	Fixed bool `json:"fixed,omitempty"`
	// HugsLib packs with Archo Monitor's record: Default is the default of a changed value
	// ("" at its default or unknown), AtDefault a setting the file does not store, Help its
	// description, Name its stored name when Key is its title.
	Default   string `json:"default,omitempty"`
	AtDefault bool   `json:"atDefault,omitempty"`
	Help      string `json:"help,omitempty"`
	Name      string `json:"name,omitempty"`
}

// ModSettingsFileDetail reads one of sm's files (class: its Mod class).
func ModSettingsFileDetail(sm *app.ModSettingsMod, class string) *ModSettingsDetail {
	d := &ModSettingsDetail{Key: sm.Instance + "|" + class, Name: sm.Name, Class: class, Folder: sm.Folder, Values: []SettingRow{}}
	var f *app.ModSettingsFile
	for i := range sm.Files {
		if strings.EqualFold(sm.Files[i].Key(), class) {
			f = &sm.Files[i]
		}
	}
	if f == nil {
		d.Error = T("no settings file of this name any more")
		return d
	}
	d.Title, d.File, d.Path, d.Exists = f.Title, f.File, f.Path, f.Exists
	if f.Kind == app.SettingsKindHugsLib {
		hugsLibDetail(d, f)
		return d
	}
	if f.Window != nil {
		d.Window = windowView(f.Window)
	}
	if !f.Exists {
		d.Notes = []string{T("no file yet: the Mod uses its defaults until its settings are changed in the game")}
		if d.Window != nil {
			d.Notes = append(d.Notes, T("the window part shows the game's window as it was when you last opened it"))
		}
		return d
	}
	sc, err := app.ReadSettingsFile(f.Path)
	if err != nil {
		d.Error = Tf("cannot read the file: %v", err)
		return d
	}
	d.Type, d.Stored = sc.Class, len(sc.Values)
	for _, v := range sc.Values {
		d.Values = append(d.Values, SettingRow{Key: v.Key, Kind: v.Kind, Text: SettingText(v), Fixed: !v.Simple()})
	}
	if len(sc.Values) == 0 {
		d.Notes = append(d.Notes, T("the file stores no value: every value is at its default"))
	} else {
		d.Notes = append(d.Notes, T("a value missing from the file is at its default"))
	}
	if !sc.Simple {
		d.Notes = append(d.Notes, T("lists, dictionaries and nested values are shown as counts"))
	}
	if d.Window != nil {
		d.Notes = append(d.Notes, T("the window part shows the game's window as it was when you last opened it"))
	}
	d.Notes = append(d.Notes, T("read only: change values in the game's Mod settings window"))
	return d
}

// hugsLibDetail fills d with HugsLib pack f: with Archo Monitor's record, every setting with
// its title and default; without, the values the file stores.
func hugsLibDetail(d *ModSettingsDetail, f *app.ModSettingsFile) {
	d.Kind = app.SettingsKindHugsLib
	sc, err := f.Read()
	if err != nil {
		d.Error = Tf("cannot read the file: %v", err)
		return
	}
	d.Stored = len(sc.Values)
	rows, hidden := app.HugsLibRows(f.HugsLib, sc)
	for _, r := range rows {
		row := SettingRow{Key: r.Name, Kind: r.Value.Kind, Text: SettingText(r.Value), Fixed: !r.Value.Simple(), Help: r.Description, AtDefault: !r.Stored}
		if r.Title != "" {
			row.Key, row.Name = r.Title, r.Name
		}
		if r.Stored && r.Recorded {
			row.Default = SettingText(r.Default)
		}
		d.Values = append(d.Values, row)
	}
	switch {
	case f.HugsLib != nil:
		d.Notes = append(d.Notes, Tf("titles and defaults as Archo Monitor saw them (%s); a grey value is at its default", LongDate(f.HugsLib.Seen)))
	case len(sc.Values) == 0:
		d.Notes = append(d.Notes, T("HugsLib stores no value of this Mod: every value is at its default"))
	default:
		d.Notes = append(d.Notes, T("a value missing from the file is at its default"),
			T("with Archo Monitor, the game's next start adds every setting's title and default"))
	}
	if hidden > 0 {
		d.Notes = append(d.Notes, Tf("%d settings the Mod keeps out of its window are not shown", hidden))
	}
	d.Notes = append(d.Notes, T("read only: change values in HugsLib's Mod settings window"))
}

func windowView(w *app.SeenSettingsWindow) *SettingsWindowView {
	v := &SettingsWindowView{Seen: LongDate(w.Seen), Full: w.Full, Controls: []WindowRow{}}
	for _, c := range w.Controls {
		v.Controls = append(v.Controls, WindowRow{Kind: c.Kind, Label: c.Label, Text: WindowText(c)})
	}
	return v
}

// WindowText is a control's value as shown: on/off, a chosen choice, a slider's value and
// range, text in quotes; "" for headings and buttons.
func WindowText(c logs.SettingsControl) string {
	switch c.Kind {
	case "checkbox":
		if strings.EqualFold(c.Value, "true") {
			return T("on")
		}
		return T("off")
	case "radio":
		if strings.EqualFold(c.Value, "true") {
			return T("chosen")
		}
		return ""
	case "slider":
		if c.Min != "" {
			return Tf("%s (%s to %s)", c.Value, c.Min, c.Max)
		}
		return c.Value
	case "text":
		return `"` + c.Value + `"`
	case "number":
		return c.Value
	}
	return ""
}

// SettingText is a stored value as the page shows it.
func SettingText(v app.SettingValue) string {
	switch v.Kind {
	case "bool":
		if strings.EqualFold(v.Value, "true") {
			return T("on")
		}
		return T("off")
	case "number":
		return v.Value
	case "text":
		return `"` + v.Value + `"`
	case "null":
		return T("not set")
	case "list":
		return Tf("list of %d", v.Items)
	case "map":
		return Tf("dictionary of %d", v.Items)
	default:
		return Tf("nested, %d fields", v.Items)
	}
}
