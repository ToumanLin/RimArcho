package view

import (
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// Mod library › Mod settings (06 §2.9 "Mod settings"): the current list's Mods that have
// settings, one row per settings file, and what one file stores. Read only: values change
// in the game's settings window.

// ModSettingsPage is the page's table.
type ModSettingsPage struct {
	Plan   string           `json:"plan"`
	Config string           `json:"config"` // the Config folder read
	Rows   []ModSettingsRow `json:"rows"`
}

// ModSettingsRow is one Mod class's settings file.
type ModSettingsRow struct {
	Key      string `json:"key"` // instance and class: unique on the page
	Instance string `json:"instance"`
	Name     string `json:"name"`
	Title    string `json:"title"` // the settings window's title; "—" unknown
	Class    string `json:"class"`
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
			r := ModSettingsRow{Key: m.Instance + "|" + f.Class, Instance: m.Instance, Name: m.Name, Title: f.Title,
				Class: f.Class, File: f.File, Exists: f.Exists, Bytes: f.Size, Size: "—", Count: -1, Values: T("defaults")}
			if r.Title == "" {
				r.Title = "—"
			}
			if f.Exists {
				r.Size = SizeText(f.Size)
				if sc, err := app.ReadSettingsFile(f.Path); err != nil {
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

// ModSettingsDetail is what one settings file stores.
type ModSettingsDetail struct {
	Key    string       `json:"key"`
	Name   string       `json:"name"`
	Title  string       `json:"title"`
	Class  string       `json:"class"` // the Mod class (the file name's last part)
	Type   string       `json:"type"`  // the settings type the file names
	File   string       `json:"file"`
	Path   string       `json:"path"`
	Folder string       `json:"folder"` // the folder the game sees the Mod under
	Exists bool         `json:"exists"`
	Error  string       `json:"error,omitempty"`
	Values []SettingRow `json:"values"`
	Notes  []string     `json:"notes"`
}

// SettingRow is one stored value as the page shows it.
type SettingRow struct {
	Key  string `json:"key"`
	Kind string `json:"kind"` // bool, number, text; list, map, object, null
	Text string `json:"text"`
	// Fixed: a list, dictionary, nested value or null; shown as a count, never changed here.
	Fixed bool `json:"fixed,omitempty"`
}

// ModSettingsFileDetail reads one of sm's files (class: its Mod class).
func ModSettingsFileDetail(sm *app.ModSettingsMod, class string) *ModSettingsDetail {
	d := &ModSettingsDetail{Key: sm.Instance + "|" + class, Name: sm.Name, Class: class, Folder: sm.Folder, Values: []SettingRow{}}
	var f *app.ModSettingsFile
	for i := range sm.Files {
		if strings.EqualFold(sm.Files[i].Class, class) {
			f = &sm.Files[i]
		}
	}
	if f == nil {
		d.Error = T("no settings file of this name any more")
		return d
	}
	d.Title, d.File, d.Path, d.Exists = f.Title, f.File, f.Path, f.Exists
	if !f.Exists {
		d.Notes = []string{T("no file yet: the Mod uses its defaults until its settings are changed in the game")}
		return d
	}
	sc, err := app.ReadSettingsFile(f.Path)
	if err != nil {
		d.Error = Tf("cannot read the file: %v", err)
		return d
	}
	d.Type = sc.Class
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
	d.Notes = append(d.Notes, T("read only: change values in the game's Mod settings window"))
	return d
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
