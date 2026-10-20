package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

func parse(t *testing.T, s string) *app.ImportedList {
	t.Helper()
	l, err := app.ParseList([]byte(s))
	if err != nil {
		t.Fatalf("%v\n%s", err, s)
	}
	return l
}

// X1: RimSort's XML export, its JSON, clipboard text and Rentry markdown.
func TestParseRimSortFormats(t *testing.T) {
	cases := map[string]struct {
		in     string
		format string
	}{
		"xml": {`<?xml version="1.0" encoding="utf-8"?>
<ModsConfigData><version>1.6.4633 rev1261</version><activeMods><li>brrainz.harmony</li><li>Ludeon.RimWorld</li><li>tester.a_steam</li></activeMods><knownExpansions/></ModsConfigData>`, app.FormatModsConfig},
		"json": {`{"version":"1.6","activeMods":["brrainz.harmony","ludeon.rimworld","tester.a_steam"],"knownExpansions":[]}`, "rimsort-json"},
		"clipboard": {`Created with RimSort
Harmony [brrainz.harmony][https://steamcommunity.com/sharedfiles/filedetails/?id=2009463077]
Core [ludeon.rimworld][No url specified]
A [tester.a_steam][https://steamcommunity.com/sharedfiles/filedetails/?id=1001]`, "rimsort-text"},
		"rentry": {`# RimWorld mod list
1. ![](https://x/y.png) [Harmony](https://steamcommunity.com/sharedfiles/filedetails/?id=2009463077 packageid: brrainz.harmony)
2. [Core](https://rimworldgame.com) {packageid: ludeon.rimworld}
3. ![](p) [A](https://steamcommunity.com/sharedfiles/filedetails/?id=1001 packageid: tester.a_steam)`, "rentry"},
	}
	for name, c := range cases {
		l := parse(t, c.in)
		if l.Format != c.format || strings.Join(l.Tokens, ",") != "brrainz.harmony,ludeon.rimworld,tester.a_steam" {
			t.Errorf("%s: format %s tokens %v", name, l.Format, l.Tokens)
		}
	}
	if l := parse(t, cases["clipboard"].in); l.Workshop["brrainz.harmony"] != "2009463077" || l.Names["brrainz.harmony"] != "Harmony" {
		t.Errorf("clipboard extras: %v %v", l.Workshop, l.Names)
	}
}

// X3: RimWorld's .rml and RimCrow's ModList.xml, JSON and share code; the writers round-trip;
// a damaged share code is refused.
func TestRimCrowAndRMLFormats(t *testing.T) {
	tokens := []string{"brrainz.harmony", "ludeon.rimworld", "tester.a"}
	ws := map[string]string{"brrainz.harmony": "2009463077", "tester.a": "1001"}
	names := map[string]string{"brrainz.harmony": "Harmony", "ludeon.rimworld": "Core", "tester.a": "A & <B>"}
	check := func(what string, l *app.ImportedList) {
		t.Helper()
		if strings.Join(l.Tokens, ",") != strings.Join(tokens, ",") || l.Workshop["tester.a"] != "1001" || l.Workshop["ludeon.rimworld"] != "" || l.Names["tester.a"] != "A & <B>" {
			t.Errorf("%s: %s %v %v %v", what, l.Format, l.Tokens, l.Workshop, l.Names)
		}
	}
	rml := parse(t, string(app.RML("1.6.4633 rev1261", tokens, ws, names)))
	check("rml", rml)
	if rml.Format != app.FormatRML || rml.Game != "1.6.4633 rev1261" {
		t.Errorf("rml: %s %s", rml.Format, rml.Game)
	}
	code, err := app.ShareCode("my list", "1.6", tokens, ws, names)
	if err != nil {
		t.Fatal(err)
	}
	sc := parse(t, code)
	check("share code", sc)
	if sc.Title != "my list" || sc.Game != "1.6" {
		t.Errorf("share code: %q %q", sc.Title, sc.Game)
	}
	parse(t, "share://RC/"+strings.TrimPrefix(code, ""))
	bad := code[:3] + map[bool]string{true: "1", false: "0"}[code[3] == '0'] + code[4:]
	if _, err := app.ParseList([]byte(bad)); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("damaged code: %v", err)
	}
	check("modlist", parse(t, `<ModList><Name>x</Name><modIds><li>brrainz.harmony</li><li>ludeon.rimworld</li><li>tester.a</li></modIds>
<modNames><li>Harmony</li><li>Core</li><li>A &amp; &lt;B&gt;</li></modNames><modSteamWorkshopIds><li>2009463077</li><li>0</li><li>1001</li></modSteamWorkshopIds></ModList>`))
	check("rimcrow json", parse(t, `{"format":"rimcrow_json","package_ids":["x"],"modlist":[{"package_id":"brrainz.harmony","name":"Harmony","workshop_id":"2009463077"},
{"package_id":"ludeon.rimworld","name":"Core"},{"package_id":"tester.a","name":"A & <B>","workshop_id":"1001"}]}`))
}

// X2: export in every format and import it back: the same list. A list of workshop IDs
// resolves against installed mods.
func TestExportImportRoundTrip(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("disable", "pkg:gone.mod")
	want := strings.Join(active(t, e), ", ")
	for _, f := range []string{"text", "modsconfig", "rimsort", "rml", "rimcrow"} {
		out := filepath.Join(e.Root, "export."+f)
		e.MustRun("loadout", "export", "--format", f, "--out", out)
		e.MustRun("disable", "pkg:tester.a")
		e.MustRun("loadout", "import", out)
		if got := strings.Join(active(t, e), ", "); got != want {
			t.Errorf("%s: %s, want %s", f, got, want)
		}
	}
	code := strings.TrimSpace(e.MustRun("loadout", "export", "--format", "rimcrow"))
	e.MustRun("disable", "pkg:tester.a")
	e.MustRun("loadout", "import", code)
	if got := strings.Join(active(t, e), ", "); got != want {
		t.Errorf("share code argument: %s", got)
	}
	ids := filepath.Join(e.Root, "ids.txt")
	os.WriteFile(ids, []byte("2009463077\nhttps://steamcommunity.com/sharedfiles/filedetails/?id=1001\n7777777\n"), 0o644)
	out := e.MustRun("loadout", "import", ids)
	if got := strings.Join(active(t, e), ", "); got != "Harmony, A" || !strings.Contains(out, "skipped: 1 workshop IDs") {
		t.Errorf("workshop IDs: %s\n%s", got, out)
	}
}
