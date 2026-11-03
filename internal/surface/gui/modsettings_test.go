package gui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// Mod library › Mod settings (06 §2.9 "Mod settings"): one row per settings file of the
// current list, and what a file stores, by instance and class (the page never passes a path).
func TestModSettingsPage(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	// tester.a is active, a Workshop item the game sees as folder 1001
	e.WriteFile(filepath.Join(e.UserData, "Config", "Mod_1001_AlphaMod.xml"),
		`<SettingsBlock><ModSettings Class="Alpha.Settings"><on>True</on><scale>0.5</scale><names><li>a</li><li>b</li></names></ModSettings></SettingsBlock>`)
	// tester.b is not in the list: its file is not a row
	e.WriteFile(filepath.Join(e.UserData, "Config", "Mod_B_BetaMod.xml"), `<SettingsBlock><ModSettings Class="Beta.Settings"/></SettingsBlock>`)
	p, err := api.ModSettings()
	if err != nil {
		t.Fatal(err)
	}
	if p.Plan != "default" || len(p.Rows) != 1 {
		t.Fatalf("page: %+v", p)
	}
	r := p.Rows[0]
	if r.Name != "Alpha" || r.Class != "AlphaMod" || r.File != "Mod_1001_AlphaMod.xml" || !r.Exists || r.Count != 3 || r.Values != "3" || r.Title != "—" {
		t.Fatalf("row: %+v", r)
	}
	d, err := api.ModSettingsFile(r.Instance, r.Class)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range d.Values {
		got = append(got, v.Key+"="+v.Text)
	}
	if d.Type != "Alpha.Settings" || strings.Join(got, " ") != "on=on scale=0.5 names=list of 2" || !d.Values[2].Fixed || d.Values[0].Fixed {
		t.Fatalf("values: %+v", d)
	}
	if !strings.Contains(strings.Join(d.Notes, "\n"), "read only") {
		t.Fatalf("notes: %q", d.Notes)
	}
	if d, err := api.ModSettingsFile(r.Instance, "Gone"); err != nil || d.Error == "" {
		t.Fatalf("a class without a file: %+v %v", d, err)
	}
	if _, err := api.ModSettingsFile("nope", r.Class); err == nil {
		t.Fatal("an unknown instance is an error")
	}
	// the Mod's detail has a settings row whose button opens the page at it (06 §4.1)
	det, err := api.Detail(r.Instance, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	var row *view.Field
	for i := range det.Fields {
		if det.Fields[i].Kind == view.FieldSettings {
			row = &det.Fields[i]
		}
	}
	if row == nil || row.Value != "3 stored values" || row.Act != view.ActSettings || row.Target != r.Instance {
		t.Fatalf("detail settings row: %+v", row)
	}
}
