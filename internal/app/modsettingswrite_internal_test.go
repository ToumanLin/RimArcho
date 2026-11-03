package app

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

func ptr(s string) *string { return &s }

// 06 §2.9: a change touches only the value's bytes; reset removes its line; what RimArcho
// does not understand is refused and stays.
func TestEditConfigBytes(t *testing.T) {
	t.Parallel()
	const file = "\ufeff<?xml version=\"1.0\" encoding=\"utf-8\"?>\r\n<SettingsBlock>\r\n  <ModSettings Class=\"A.S\">\r\n" +
		"    <on>True</on>\r\n    <scale>0.5</scale>\r\n    <count>3</count>\r\n    <name>Bob &amp; Co</name>\r\n    <empty />\r\n" +
		"    <names>\r\n      <li>a</li>\r\n    </names>\r\n  </ModSettings>\r\n</SettingsBlock>\r\n"
	raw := []byte(file)
	edit := func(ed SettingEdit) (string, *SettingChange, error) {
		out, ch, err := editConfig(raw, ed)
		return string(out), ch, err
	}
	for _, c := range []struct {
		ed       SettingEdit
		old, new string // the bytes replaced
		from, to string
		removed  bool
	}{
		{SettingEdit{Name: "on", Value: "off"}, "<on>True</on>", "<on>False</on>", "True", "False", false},
		{SettingEdit{Name: "count", Value: " 4 "}, "<count>3</count>", "<count>4</count>", "3", "4", false},
		{SettingEdit{Name: "scale", Value: "1.25"}, "<scale>0.5</scale>", "<scale>1.25</scale>", "0.5", "1.25", false},
		{SettingEdit{Name: "name", Value: "X <y>"}, "<name>Bob &amp; Co</name>", "<name>X &lt;y&gt;</name>", "Bob & Co", "X <y>", false},
		{SettingEdit{Name: "empty", Value: "hi"}, "<empty />", "<empty>hi</empty>", "", "hi", false},
		{SettingEdit{Name: "scale", Reset: true}, "\r\n    <scale>0.5</scale>", "", "0.5", "", true},
		{SettingEdit{Name: "on", Value: "on", Old: ptr("True")}, "", "", "", "", false}, // no change
	} {
		out, ch, err := edit(c.ed)
		if err != nil {
			t.Fatalf("%+v: %v", c.ed, err)
		}
		if c.old == "" {
			if out != file || ch != nil {
				t.Fatalf("%+v changed: %q %+v", c.ed, out, ch)
			}
			continue
		}
		if want := strings.Replace(file, c.old, c.new, 1); out != want {
			t.Fatalf("%+v:\n%q\nwant\n%q", c.ed, out, want)
		}
		if ch == nil || ch.From != c.from || ch.To != c.to || ch.Removed != c.removed {
			t.Fatalf("%+v: change %+v", c.ed, ch)
		}
		if _, err := parseSettings(strings.NewReader(out)); err != nil {
			t.Fatalf("%+v: the result does not read back: %v", c.ed, err)
		}
	}
	for _, c := range []struct {
		ed   SettingEdit
		want string
	}{
		{SettingEdit{Name: "count", Value: "2.5"}, "whole number"},
		{SettingEdit{Name: "scale", Value: "1,5"}, "decimal point"},
		{SettingEdit{Name: "on", Value: "maybe"}, "on or off"},
		{SettingEdit{Name: "name", Value: "a\nb"}, "one line"},
		{SettingEdit{Name: "names", Value: "x"}, "change it in the game"},
		{SettingEdit{Name: "names", Reset: true}, "change it in the game"},
		{SettingEdit{Name: "other", Value: "1"}, "does not store it"},
		{SettingEdit{Name: "on", Value: "off", Old: ptr("False")}, "changed since it was read"},
	} {
		if _, _, err := edit(c.ed); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%+v: %v, want %q", c.ed, err, c.want)
		}
	}
	if out, ch, err := edit(SettingEdit{Name: "other", Reset: true}); err != nil || ch != nil || out != file {
		t.Fatalf("resetting a value not stored: %v %+v", err, ch)
	}
}

// HugsLib: a value the record names is added to its pack (or a new pack, or a new file);
// its default removes it; a choice is one of the enum's names.
func TestEditHugsLibBytes(t *testing.T) {
	t.Parallel()
	f := &ModSettingsFile{Kind: SettingsKindHugsLib, Class: "Pack", HugsLib: &SeenHugsLibPack{HugsLibPack: logs.HugsLibPack{ID: "Pack", Settings: []logs.HugsLibSetting{
		{Name: "raids", Title: "Raids", Value: "True", Default: "False", Type: "Boolean"},
		{Name: "mode", Title: "Mode", Value: "Normal", Default: "Normal", Type: "Mode", Choices: []string{"Easy", "Normal", "Hard"}},
		{Name: "pawns", Title: "Pawns", Value: "20", Default: "20", Type: "Int32"},
		{Name: "thing", Value: "x", Default: "x", Type: "ThingDef"},
		{Name: "seen", Value: "3", Default: "0", Type: "Int32", Hidden: true},
	}}}}
	read := func(raw []byte) map[string]string {
		t.Helper()
		packs, err := parseHugsLib(strings.NewReader(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for id, p := range packs {
			for _, v := range p.Values {
				got[id+"."+v.Key] = v.Value
			}
		}
		return got
	}
	// no file yet
	out, ch, err := editHugsLib(nil, f, SettingEdit{Name: "Mode", Value: "hard"})
	if err != nil || ch == nil || ch.To != "Hard" || read(out)["pack.mode"] != "Hard" {
		t.Fatalf("a new file: %v %+v\n%s", err, ch, out)
	}
	// a pack with values: the new one follows them, indented alike
	const file = "\ufeff<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<settings>\n  <Other>\n    <x>1</x>\n  </Other>\n  <Pack>\n    <raids>True</raids>\n  </Pack>\n</settings>"
	out, _, err = editHugsLib([]byte(file), f, SettingEdit{Name: "pawns", Value: "25"})
	if err != nil || !strings.Contains(string(out), "    <raids>True</raids>\n    <pawns>25</pawns>\n  </Pack>") {
		t.Fatalf("added to the pack: %v\n%s", err, out)
	}
	// its default removes it
	out, ch, err = editHugsLib([]byte(file), f, SettingEdit{Name: "raids", Value: "off"})
	if err != nil || ch == nil || !ch.Removed || strings.Contains(string(out), "raids") || !strings.Contains(string(out), "<Pack>\n  </Pack>") {
		t.Fatalf("set to its default: %v %+v\n%s", err, ch, out)
	}
	// no pack yet: a new one after the others
	other := strings.Replace(file, "  <Pack>\n    <raids>True</raids>\n  </Pack>\n", "", 1)
	out, _, err = editHugsLib([]byte(other), f, SettingEdit{Name: "mode", Value: "Easy"})
	if got := read(out); err != nil || got["pack.mode"] != "Easy" || got["other.x"] != "1" || !strings.Contains(string(out), "  </Other>\n  <Pack>\n    <mode>Easy</mode>\n  </Pack>\n</settings>") {
		t.Fatalf("a new pack: %v\n%s", err, out)
	}
	for _, c := range []struct {
		ed   SettingEdit
		want string
	}{
		{SettingEdit{Name: "mode", Value: "Brutal"}, "not one of Easy, Normal, Hard"},
		{SettingEdit{Name: "thing", Value: "y"}, "cannot change this kind"},
		{SettingEdit{Name: "seen", Value: "1"}, "no such setting"},
		{SettingEdit{Name: "raids", Value: "on", Old: ptr("")}, "changed since it was read"},
	} {
		if _, _, err := editHugsLib([]byte(file), f, c.ed); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%+v: %v, want %q", c.ed, err, c.want)
		}
	}
}
