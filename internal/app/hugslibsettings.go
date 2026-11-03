package app

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// HugsLib settings packs (06 §2.9 "Mod settings", 03 §6.7.1). HugsLib keeps the settings of
// the Mods built on it in <user data>/HugsLib/ModSettings.xml: one element per settings pack,
// one child per stored value, as text. Like the game, it stores only values that differ from
// the default. A pack is named by its Mod's settings identifier: the Mod's package id unless
// the Mod names another, so a pack under another name is the Mod's only by Archo Monitor's
// HugsLib record, which also gives each setting's title and default.

// HugsLibSettingsFile is HugsLib's settings file under the save-data root.
var HugsLibSettingsFile = filepath.Join("HugsLib", "ModSettings.xml")

// SettingsKindHugsLib marks a ModSettingsFile that is a HugsLib settings pack.
const SettingsKindHugsLib = "hugslib"

// hugsStored is one pack of HugsLib's settings file.
type hugsStored struct {
	ID     string // as the file spells it
	Values []SettingValue
}

// readHugsLibFile reads HugsLib's settings file: lower-case pack id → what it stores. nil
// without the file.
func readHugsLibFile(path string) (map[string]*hugsStored, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseHugsLib(io.LimitReader(f, 64<<20))
}

func parseHugsLib(r io.Reader) (map[string]*hugsStored, error) {
	d := xml.NewDecoder(r)
	d.Strict = false
	out := map[string]*hugsStored{}
	depth := 0
	var pack *hugsStored
	var cur *SettingValue
	var text strings.Builder
	var c childCount
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
				if t.Name.Local != "settings" {
					return nil, errors.New("not HugsLib's settings file: root is <" + t.Name.Local + ">")
				}
			case 2:
				pack = &hugsStored{ID: t.Name.Local}
				out[strings.ToLower(pack.ID)] = pack
			case 3:
				if pack != nil && len(pack.Values) < settingsValuesMax {
					cur = &SettingValue{Key: t.Name.Local}
					text.Reset()
					c = childCount{}
				}
			case 4:
				if cur != nil {
					c.kids++
				}
			}
		case xml.EndElement:
			if depth == 3 && cur != nil {
				classifySetting(cur, text.String(), c)
				pack.Values = append(pack.Values, *cur)
				cur = nil
			}
			depth--
		case xml.CharData:
			if depth == 3 && cur != nil {
				text.Write(t)
			}
		}
	}
	return out, nil
}

// readHugsLibPack is what HugsLib's settings file stores for pack id: no values when it
// stores none.
func readHugsLibPack(path, id string) (*SettingsContent, error) {
	packs, err := readHugsLibFile(path)
	if err != nil {
		return nil, err
	}
	out := &SettingsContent{Simple: true}
	if p := packs[strings.ToLower(id)]; p != nil {
		out.Values = p.Values
		for _, v := range p.Values {
			if !v.Simple() {
				out.Simple = false
			}
		}
	}
	return out, nil
}

// HugsLibRow is one setting of a HugsLib pack as the page shows it: the record's title,
// description and default joined with the value the file stores.
type HugsLibRow struct {
	Name        string // the element name
	Title       string // HugsLib's dialog title ("" unknown)
	Description string
	Type        string // the value type's simple name ("" unknown)
	Value       SettingValue
	Default     SettingValue // valid when Recorded
	Recorded    bool         // the record names it
	Stored      bool         // the file stores it: it differs from the default
}

// HugsLibRows joins pack's record (nil: none) with what the file stores: the record's
// settings in its order, then values the file stores that the record does not name. Settings
// the Mod keeps hidden are left out; hidden counts them.
func HugsLibRows(rec *SeenHugsLibPack, sc *SettingsContent) (rows []HugsLibRow, hidden int) {
	stored := map[string]SettingValue{}
	for _, v := range sc.Values {
		stored[v.Key] = v
	}
	named := map[string]bool{}
	if rec != nil {
		for _, h := range rec.Settings {
			named[h.Name] = true
			if h.Hidden {
				hidden++
				continue
			}
			row := HugsLibRow{Name: h.Name, Title: h.Title, Description: h.Description, Type: h.Type, Recorded: true,
				Default: SettingValue{Key: h.Name}}
			classifySetting(&row.Default, h.Default, childCount{})
			row.Value = row.Default
			if v, ok := stored[h.Name]; ok {
				row.Value, row.Stored = v, true
			}
			rows = append(rows, row)
		}
	}
	for _, v := range sc.Values {
		if !named[v.Key] {
			rows = append(rows, HugsLibRow{Name: v.Key, Value: v, Stored: true})
		}
	}
	return rows, hidden
}
