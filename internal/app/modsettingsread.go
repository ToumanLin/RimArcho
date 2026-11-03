package app

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Reading Mod settings (06 §2.9 "Mod settings", 05 §4): which Mods of the current list have
// settings, and what a settings file stores. Only flat values (on/off, numbers, text) are
// shown as values; lists, dictionaries and nested objects are summarised and stay the game's
// to change. A file stores only values that differ from the Mod's defaults, so a value missing
// from it is not a missing setting.

// ModSettingsList is the current list's Mods that have settings.
type ModSettingsList struct {
	Plan   string // the list's name
	Config string // the Config folder read
	Mods   []ModSettingsMod
}

// ModSettingsMod is one Mod and its settings files, one per Mod class.
type ModSettingsMod struct {
	Instance  string
	Name      string
	PackageID string
	Folder    string // the folder the game sees it under
	Files     []ModSettingsFile
}

// ModSettingsFile is one Mod class's settings file.
type ModSettingsFile struct {
	Class  string    // the Mod class: the file name's last part
	Title  string    // the settings window's title, from Archo Monitor ("" unknown)
	File   string    // name in Config/
	Path   string    // full path
	Exists bool      // false: the Mod uses its defaults (nothing changed yet)
	Size   int64     // bytes, when it exists
	Seen   time.Time // when Archo Monitor last recorded it (zero: never)
	// Window is what the settings window drew when the player last opened it (Archo Monitor);
	// nil when never seen.
	Window *SeenSettingsWindow
}

// SettingValue is one value stored in a settings file.
type SettingValue struct {
	Key   string
	Kind  string // bool, number, text; list, map, object, null (read-only)
	Value string // the stored text, for bool, number and text
	Items int    // entries of a list, map or object
}

// Simple reports whether RimArcho can show the value as a value (on/off, number, text).
func (v SettingValue) Simple() bool {
	return v.Kind == "bool" || v.Kind == "number" || v.Kind == "text"
}

// SettingsContent is what a settings file stores.
type SettingsContent struct {
	Class  string // the ModSettings Class attribute (the settings type)
	Values []SettingValue
	Simple bool // every stored value is on/off, a number or text
}

// settingsValuesMax bounds the values read from one file.
const settingsValuesMax = 5000

// ReadSettingsFile reads a standard settings file (<SettingsBlock><ModSettings>…).
func ReadSettingsFile(path string) (*SettingsContent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseSettings(io.LimitReader(f, 64<<20))
}

func parseSettings(r io.Reader) (*SettingsContent, error) {
	d := xml.NewDecoder(r)
	d.Strict = false
	out := &SettingsContent{Simple: true}
	depth := 0
	var cur *SettingValue
	var text strings.Builder
	var c childCount
	root := false
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch depth {
			case 1:
				if t.Name.Local != "SettingsBlock" {
					return nil, fmt.Errorf("not a settings file: root is <%s>", t.Name.Local)
				}
			case 2:
				if t.Name.Local == "ModSettings" {
					root = true
					out.Class = attr(t, "Class")
				}
			case 3:
				if root && len(out.Values) < settingsValuesMax {
					cur = &SettingValue{Key: t.Name.Local}
					if strings.EqualFold(attr(t, "IsNull"), "true") {
						cur.Kind = "null"
					}
					text.Reset()
					c = childCount{}
				}
			case 4:
				if cur != nil {
					c.kids++
					switch t.Name.Local {
					case "li":
						c.lis++
					case "keys":
						c.keys, c.inKeys = true, true
					case "values":
						c.values = true
					}
				}
			case 5:
				if cur != nil && c.inKeys && t.Name.Local == "li" {
					c.keyItems++
				}
			}
		case xml.EndElement:
			if depth == 4 && t.Name.Local == "keys" {
				c.inKeys = false
			}
			if depth == 3 && cur != nil {
				classifySetting(cur, text.String(), c)
				if !cur.Simple() {
					out.Simple = false
				}
				out.Values = append(out.Values, *cur)
				cur = nil
			}
			depth--
		case xml.CharData:
			if depth == 3 && cur != nil {
				text.Write(t)
			}
		}
	}
	if !root {
		return nil, errors.New("not a settings file: no <ModSettings>")
	}
	return out, nil
}

// childCount is what a value's element holds: Scribe writes a list as <li> items and a
// dictionary as <keys> and <values> lists (research 2026-11-02_mod-settings §4).
type childCount struct {
	kids, lis, keyItems  int
	keys, values, inKeys bool
}

func classifySetting(v *SettingValue, text string, c childCount) {
	switch {
	case v.Kind == "null":
	case c.kids == 0:
		s := strings.TrimSpace(text)
		v.Value = s
		switch {
		case strings.EqualFold(s, "true") || strings.EqualFold(s, "false"):
			v.Kind = "bool"
		case isNumber(s):
			v.Kind = "number"
		default:
			v.Kind = "text"
		}
	case c.lis == c.kids:
		v.Kind, v.Items = "list", c.lis
	case c.kids == 2 && c.keys && c.values:
		v.Kind, v.Items = "map", c.keyItems
	default:
		v.Kind, v.Items = "object", c.kids
	}
}

func isNumber(s string) bool {
	if s == "" || strings.ContainsAny(s, "nNiI") { // NaN, Inf: text, as a person reads them
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func attr(t xml.StartElement, name string) string {
	for _, a := range t.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// ModSettings lists the current list's Mods that have settings: those Archo Monitor saw
// with a settings window or stored settings, and those with a settings file under the folder
// the game sees them under.
func (a *App) ModSettings() (*ModSettingsList, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	root := st.Ix.Env.UserDataRoot
	if root == "" {
		return nil, errors.New("the environment has no user data directory; set one with archo env set --user-data <dir>")
	}
	out := &ModSettingsList{Plan: PlanRef(st.Plan), Config: filepath.Join(root, "Config")}
	var listed []string
	for _, en := range st.List {
		listed = append(listed, st.Reg.Folder(en.Instance))
	}
	r := a.settingsReader(st.Ix, st.Aliases, listed)
	for _, en := range st.List {
		m := st.Ix.ByID[en.Instance]
		if m == nil || m.Kind == source.Official || m.PackageID == "" || !st.Reg.Loadable(m.ID) {
			continue
		}
		if sm := r.of(m, st.Reg.Folder(m.ID)); len(sm.Files) > 0 {
			out.Mods = append(out.Mods, sm)
		}
	}
	sort.SliceStable(out.Mods, func(i, j int) bool { return strings.ToLower(out.Mods[i].Name) < strings.ToLower(out.Mods[j].Name) })
	return out, nil
}

// ModSettingsOf is one Mod's settings under the folder the game sees it under (another
// tool's link to it, else its deployment name), whether or not the current list uses it. It
// reads the index only: unlike Loadout, it never takes in an outside change of the list.
func (a *App) ModSettingsOf(m *store.Mod) (*ModSettingsMod, error) {
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	if ix.Env.UserDataRoot == "" {
		return nil, errors.New("the environment has no user data directory; set one with archo env set --user-data <dir>")
	}
	aliases, err := a.Store.AllAliases(ix.Env.ID)
	if err != nil {
		return nil, err
	}
	folder := ""
	for _, c := range Candidates(ix, aliases) {
		if c.ID == m.ID {
			folder = c.Folder
		}
	}
	if folder == "" && !gameVisible(m.Kind) {
		folder = DeployName(m)
	}
	sm := a.settingsReader(ix, aliases, []string{folder}).of(m, folder)
	return &sm, nil
}

// settingsReader holds what finding a Mod's settings files reads once: the Config folder's
// files and Archo Monitor's records.
type settingsReader struct {
	cfg     string
	seen    []SeenSettingsFile
	windows []SeenSettingsWindow
	files   map[string]map[string]settingsFileInfo // stem → class → file
}

// settingsReader reads ix's environment's Config folder; folders are those asked about.
func (a *App) settingsReader(ix *Index, aliases map[string][]store.Alias, folders []string) *settingsReader {
	root := ix.Env.UserDataRoot
	ms := a.monitorSeen(root)
	r := &settingsReader{cfg: filepath.Join(root, "Config"), seen: ms.Files, windows: ms.Windows}
	known := settingsFolders(ix, aliases)
	addSeenFolders(known, r.seen)
	stems := map[string]bool{}
	for _, f := range append(allFolders(known), folders...) {
		if f != "" {
			stems[strings.ToLower(settingsStem(f))] = true
		}
	}
	r.files = readSettingsFiles(r.cfg, stems)
	return r
}

func (r *settingsReader) of(m *store.Mod, folder string) ModSettingsMod {
	sm := ModSettingsMod{Instance: m.ID, Name: m.DisplayName(), PackageID: strings.ToLower(m.PackageID), Folder: folder}
	if folder == "" {
		return sm
	}
	pkg := packageBase(sm.PackageID)
	byClass := map[string]*ModSettingsFile{}
	var order []string
	add := func(class string) *ModSettingsFile {
		k := strings.ToLower(class)
		if f := byClass[k]; f != nil {
			return f
		}
		f := &ModSettingsFile{Class: class, File: SettingsFile(folder, class)}
		byClass[k] = f
		order = append(order, k)
		return f
	}
	// Archo Monitor's records of this package: the window title, from this folder's record
	// when there is one, else the newest.
	for _, s := range r.seen {
		if packageBase(strings.ToLower(s.PackageID)) != pkg || s.Title == "" && !s.Stored {
			continue
		}
		f := add(s.Class)
		here := strings.EqualFold(s.Folder, folder)
		if s.Seen.After(f.Seen) || here {
			if here || f.Title == "" || s.Title != "" {
				f.Title = s.Title
			}
			f.Seen = s.Seen
		}
	}
	for _, fi := range r.files[strings.ToLower(settingsStem(folder))] {
		add(fi.class)
	}
	// a window the player opened: its class has settings even before Monitor's record of it
	for i := range r.windows {
		w := &r.windows[i]
		if packageBase(strings.ToLower(w.PackageID)) != pkg {
			continue
		}
		f := add(w.Class)
		f.Window = w
		if f.Title == "" {
			f.Title = w.Title
		}
	}
	sort.Strings(order)
	for _, k := range order {
		f := byClass[k]
		f.Path = filepath.Join(r.cfg, f.File)
		if fi, err := os.Stat(f.Path); err == nil && fi.Mode().IsRegular() {
			f.Exists, f.Size = true, fi.Size()
		}
		sm.Files = append(sm.Files, *f)
	}
	return sm
}

// SettingsFilesOf is the paths of m's settings files under the folder the game sees it under
// (st's registry), from the Config folder alone: no Archo Monitor record, so a detail can ask
// on every cursor move (06 §4.1). nil when the game does not see m or there are none.
func SettingsFilesOf(st *LoadoutState, m *store.Mod) []string {
	folder, root := st.Reg.Folder(m.ID), st.Ix.Env.UserDataRoot
	if folder == "" || root == "" {
		return nil
	}
	stem := strings.ToLower(settingsStem(folder))
	stems := map[string]bool{stem: true}
	for _, f := range allFolders(settingsFolders(st.Ix, st.Aliases)) {
		stems[strings.ToLower(settingsStem(f))] = true
	}
	var paths []string
	for _, fi := range readSettingsFiles(filepath.Join(root, "Config"), stems)[stem] {
		paths = append(paths, filepath.Join(fi.dir, fi.name))
	}
	sort.Strings(paths)
	return paths
}
