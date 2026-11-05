package view

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/logs"
)

// Mod library › Mod settings (16 §2.9 "Mod settings"): the current list's Mods that have
// settings, one row per settings file or HugsLib settings pack, and what one stores. Simple
// values change here while no game runs (app.EditModSettings); the rest in the game.

// ModSettingsPage is the page's table.
type ModSettingsPage struct {
	Plan   string           `json:"plan"`
	Config string           `json:"config"` // the Config folder read
	Rows   []ModSettingsRow `json:"rows"`
	Locked string           `json:"locked,omitempty"` // why values cannot change now (a game runs)
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
	Key    string `json:"key"`
	Kind   string `json:"kind,omitempty"` // "hugslib": a HugsLib pack
	Name   string `json:"name"`
	Title  string `json:"title"`
	Class  string `json:"class"` // the Mod class (the file name's last part)
	Type   string `json:"type"`  // the settings type the file names
	File   string `json:"file"`
	Path   string `json:"path"`
	Folder string `json:"folder"` // the folder the game sees the Mod under
	Exists bool   `json:"exists"`
	Error  string `json:"error,omitempty"`
	Stored int    `json:"stored"` // values the file stores
	// Editable: the values whose Edit is set can change here (the file reads; a Config file
	// exists).
	Editable bool         `json:"editable"`
	Values   []SettingRow `json:"values"`
	Notes    []string     `json:"notes"`
	// Window is what the game's settings window drew when the player last opened it (Archo
	// Monitor, 06 §7.1); nil when never seen.
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
	// Edit is how the value changes here (app.SettingEditor): bool, integer, number, text or
	// choice (Choices); "" read only. Value is the text the control starts from, Stored
	// whether the file stores it (its old text then is Value, else "").
	Edit    string   `json:"edit,omitempty"`
	Choices []string `json:"choices,omitempty"`
	Value   string   `json:"value,omitempty"`
	Stored  bool     `json:"stored,omitempty"`
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
	d.Type, d.Stored, d.Editable = sc.Class, len(sc.Values), true
	for _, v := range sc.Values {
		d.Values = append(d.Values, SettingRow{Key: v.Key, Kind: v.Kind, Text: SettingText(v), Fixed: !v.Simple(),
			Edit: app.ConfigEditor(v).Kind, Value: v.Value, Stored: true})
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
	d.Notes = append(d.Notes, T("on/off, numbers and text change here while the game is closed; the file is backed up first"))
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
	d.Stored, d.Editable = len(sc.Values), true
	rows, hidden := app.HugsLibRows(f.HugsLib, sc)
	for _, r := range rows {
		ed := app.HugsLibEditor(r)
		row := SettingRow{Key: r.Name, Kind: r.Value.Kind, Text: SettingText(r.Value), Fixed: !r.Value.Simple(), Help: r.Description, AtDefault: !r.Stored,
			Edit: ed.Kind, Choices: ed.Choices, Value: r.Value.Value, Stored: r.Stored}
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
	d.Notes = append(d.Notes, T("on/off, numbers, text and choices change here while the game is closed; the file is backed up first"))
}

// SettingErrorText is a refused change as the page says it.
func SettingErrorText(e *app.SettingInputError) string {
	switch e.Code {
	case "bool":
		return Tf("%q is not on or off", e.Input)
	case "integer":
		return Tf("%q is not a whole number (RimArcho writes this value only as one)", e.Input)
	case "number":
		return Tf("%q is not a number (write a decimal point, not a comma)", e.Input)
	case "line":
		return T("the text must be one line")
	case "choice":
		return Tf("%q is not one of %s", e.Input, strings.Join(e.Choices, ", "))
	case "fixed":
		return T("RimArcho cannot change this value: change it in the game")
	case "unknown":
		return T("the file no longer has this setting: it has been read again")
	case "changed":
		return T("the file changed since it was read: it has been read again")
	case "nofile":
		return T("the Mod has no settings file yet: change a setting in the game once")
	case "running":
		return T("the game is running: close it first, or change the value in the game")
	}
	return e.Error()
}

// SettingsWriteText says what a change wrote: label is the setting as the page names it,
// now its value as shown after.
func SettingsWriteText(w *app.SettingsWrite, label, now string) string {
	if len(w.Changes) == 0 {
		return T("nothing changed: the value was already so")
	}
	msg := Tf("%s is now %s", label, now)
	if w.Changes[0].Removed {
		msg = Tf("%s is back to its default", label)
	}
	if w.Backup != "" {
		msg = Tf("%s; the previous file is kept as %s", msg, filepath.Base(w.Backup))
	}
	return msg
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
	case "empty":
		return T("empty")
	case "list":
		return Tf("list of %d", v.Items)
	case "map":
		return Tf("dictionary of %d", v.Items)
	default:
		return Tf("nested, %d fields", v.Items)
	}
}
