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
	// HugsLib keeps tester.a's settings under its package id; Archo Monitor recorded the pack
	e.WriteFile(filepath.Join(e.UserData, "HugsLib", "ModSettings.xml"),
		`<?xml version="1.0" encoding="utf-8"?><settings><tester.a><raids>True</raids></tester.a><Other><x>1</x></Other></settings>`)
	e.WriteFile(filepath.Join(e.UserData, "ArchoMonitor.log"),
		`{"timestamp":"2026-11-03 10:00:00.000","level":"INFO","message":"[Archo Monitor] session 0123456789ab","details":"","count":1,"context":{"session":"0123456789ab","version":"0.11.0","game":"1.6.4633 rev1261"},"source":"archo-monitor","schema":1,"kind":"session"}`+"\n"+
			`{"timestamp":"2026-11-03 10:01:00.000","level":"INFO","message":"[Archo Monitor] hugslib 1","details":"","count":1,"context":{"hugslib":[{"id":"tester.a","packageId":"tester.a","folder":"1001","title":"Alpha options","full":true,"settings":[{"name":"raids","title":"Raids","description":"More raids","value":"True","default":"False","type":"Boolean"},{"name":"mode","title":"Mode","value":"Normal","default":"Normal","type":"Mode"},{"name":"seen","value":"3","default":"0","type":"Int32","hidden":true}]}]},"source":"archo-monitor","schema":1,"kind":"hugslib"}`+"\n")
	p, err := api.ModSettings()
	if err != nil {
		t.Fatal(err)
	}
	if p.Plan != "default" || len(p.Rows) != 2 {
		t.Fatalf("page: %+v", p)
	}
	if h := p.Rows[1]; h.Class != "hugslib:tester.a" || h.Title != "HugsLib: Alpha options" || h.Count != 1 || h.Size != "—" {
		t.Fatalf("HugsLib row: %+v", h)
	}
	hd, err := api.ModSettingsFile(p.Rows[1].Instance, p.Rows[1].Class)
	if err != nil || hd.Kind != "hugslib" || hd.Stored != 1 || len(hd.Values) != 2 {
		t.Fatalf("HugsLib detail: %+v %v", hd, err)
	}
	if v := hd.Values[0]; v.Key != "Raids" || v.Name != "raids" || v.Text != "on" || v.Default != "off" || v.AtDefault || v.Help != "More raids" {
		t.Fatalf("a changed HugsLib value: %+v", v)
	}
	if v := hd.Values[1]; v.Key != "Mode" || v.Text != `"Normal"` || v.Default != "" || !v.AtDefault {
		t.Fatalf("a HugsLib value at its default: %+v", v)
	}
	if !strings.Contains(strings.Join(hd.Notes, "\n"), "1 settings the Mod keeps out of its window") {
		t.Fatalf("HugsLib notes: %q", hd.Notes)
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
	if row == nil || row.Value != "4 stored values" || row.Act != view.ActSettings || row.Target != r.Instance {
		t.Fatalf("detail settings row: %+v", row)
	}
}
