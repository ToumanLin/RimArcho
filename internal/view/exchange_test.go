package view_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// exchangeEnv: Core, Harmony (ws 2009463077), Alpha (ws 1001) and Beta installed; Core
// and Alpha enabled.
func exchangeEnv(t *testing.T) *testkit.Env {
	view.SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	e.AddWorkshop("1001", testkit.Sample("tester.a", "Alpha"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "B"), testkit.Sample("tester.b", "Beta"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.a")
	e.AddEnv(false)
	e.MustRun("scan")
	return e
}

func enabled(t *testing.T, e *testkit.Env) string {
	t.Helper()
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, x := range st.List {
		if m := st.Ix.ByID[x.Instance]; m != nil {
			out = append(out, m.DisplayName())
		} else {
			out = append(out, "missing:"+x.Missing)
		}
	}
	return strings.Join(out, ",")
}

const rml = `<?xml version="1.0" encoding="utf-8"?>
<savedModList>
  <meta>
    <gameVersion>1.5.4104 rev435</gameVersion>
    <modIds><li>ludeon.rimworld</li><li>brrainz.harmony</li><li>tester.b</li><li>tester.a</li><li>someone.gone</li></modIds>
    <modSteamIds><li>0</li><li>2009463077</li><li>0</li><li>1001</li><li>777777777</li></modSteamIds>
    <modNames><li>Core</li><li>Harmony</li><li>Beta</li><li>Alpha</li><li>Gone Mod</li></modNames>
  </meta>
</savedModList>`

// X-GUI1: the formats are detected (file, pasted JSON, share code, RimSort clipboard), and
// the preview lists what is installed and what is missing with its Workshop link.
func TestImportPreview(t *testing.T) {
	e := exchangeEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "list.rml")
	os.WriteFile(path, []byte(rml), 0o644)

	imp, err := view.ReadImport(e.App, view.ImportFile, path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := view.PreviewImport(e.App, imp)
	if err != nil {
		t.Fatal(err)
	}
	if p.Format != "RimWorld mod list (.rml)" || p.Total != 5 || len(p.Installed) != 4 || len(p.Missing) != 1 {
		t.Fatalf("preview: %+v", p)
	}
	m := p.Missing[0]
	if m.Name != "Gone Mod" || m.Package != "someone.gone" || !strings.HasSuffix(m.URL, "?id=777777777") {
		t.Fatalf("missing row: %+v", m)
	}
	if !strings.Contains(p.GameNote, "1.5") || !strings.Contains(p.GameNote, "1.6") {
		t.Fatalf("game note: %q", p.GameNote)
	}
	// Core, Alpha enabled → the list adds Harmony, Beta and the missing entry, and moves Alpha
	if r := p.Replace; r.Added != 3 || r.Removed != 0 || r.Count != 5 || r.Same {
		t.Fatalf("replace: %+v", r)
	}
	if r := p.Merge; r.Added != 2 || r.Removed != 0 || r.Moved != 0 || r.Count != 4 {
		t.Fatalf("merge: %+v", r)
	}
	if enabled(t, e) != "Core,Alpha" {
		t.Fatalf("the preview changed the list: %s", enabled(t, e))
	}
	// a typed path in the paste box is read as the file
	if imp2, err := view.ReadImport(e.App, view.ImportText, `"`+path+`"`); err != nil || imp2.List.Format != app.FormatRML {
		t.Fatalf("path as text: %v %+v", err, imp2)
	}

	code, err := app.ShareCode("Mine", "1.6", []string{"ludeon.rimworld", "tester.b"}, map[string]string{}, map[string]string{"tester.b": "Beta"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ text, format string }{
		{`{"activeMods": ["ludeon.rimworld", "tester.b"]}`, "RimSort JSON list"},
		{code, "RimCrow share code"},
		{"Beta [tester.b][https://steamcommunity.com/sharedfiles/filedetails/?id=5]\nCore [ludeon.rimworld][]", "RimSort clipboard list"},
		{"ludeon.rimworld\ntester.b  # Beta", "text list"},
		{"2009463077\nhttps://steamcommunity.com/sharedfiles/filedetails/?id=1001", "text list"},
	} {
		imp, err := view.ReadImport(e.App, view.ImportText, c.text)
		if err != nil {
			t.Fatalf("%q: %v", c.text, err)
		}
		p, err := view.PreviewImport(e.App, imp)
		if err != nil || p.Format != c.format || len(p.Missing) != 0 || len(p.Installed) != 2 {
			t.Fatalf("%q: %v %+v", c.text, err, p)
		}
	}
	if _, err := view.ReadImport(e.App, view.ImportText, "hello there\nnothing here"); err == nil {
		t.Fatal("prose was accepted as a list")
	}
}

// X-GUI2: applying is one undoable step, replace or add, and never writes ModsConfig.xml.
func TestImportApplyIsOneStep(t *testing.T) {
	e := exchangeEnv(t)
	before, _ := os.ReadFile(e.ModsConfig)
	imp, err := view.ReadImport(e.App, view.ImportText, rml)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := view.ApplyImport(e.App, imp, true); err != nil {
		t.Fatal(err)
	}
	if got := enabled(t, e); got != "Core,Harmony,Beta,Alpha" {
		t.Fatalf("merge: %s", got)
	}
	e.MustRun("undo")
	if got := enabled(t, e); got != "Core,Alpha" {
		t.Fatalf("undo after merge: %s", got)
	}
	text, err := view.ApplyImport(e.App, imp, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := enabled(t, e); got != "Core,Harmony,Beta,Alpha,missing:someone.gone" {
		t.Fatalf("replace: %s", got)
	}
	if !strings.Contains(text, "not saved yet") {
		t.Fatalf("status: %s", text)
	}
	e.MustRun("undo")
	if got := enabled(t, e); got != "Core,Alpha" {
		t.Fatalf("one undo after replace: %s", got)
	}
	if after, _ := os.ReadFile(e.ModsConfig); !bytes.Equal(before, after) {
		t.Fatal("import wrote ModsConfig.xml")
	}
}

// X-GUI3: a collection link pasted in the box is read as a collection (titles for the
// missing items); a Rentry link is fetched.
func TestImportLinks(t *testing.T) {
	e := exchangeEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.URL.Path {
		case "/collection":
			w.Write([]byte(`{"response":{"collectiondetails":[{"publishedfileid":"3000000001","result":1,"children":[
{"publishedfileid":"2009463077","sortorder":1,"filetype":0},{"publishedfileid":"5550001","sortorder":2,"filetype":0},{"publishedfileid":"1001","sortorder":3,"filetype":0}]}]}}`))
		case "/api":
			w.Write([]byte(`{"response":{"publishedfiledetails":[{"publishedfileid":"3000000001","title":"My Pack"},{"publishedfileid":"5550001","title":"Not Here"}]}}`))
		case "/rentry/abc/raw":
			w.Write([]byte("# Mods\n1. [Beta](https://steamcommunity.com/sharedfiles/filedetails/?id=5) {packageid: tester.b}\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	e.App.SteamAPI, e.App.SteamCollectionAPI = srv.URL+"/api", srv.URL+"/collection"
	old := app.RentryRaw
	app.RentryRaw = srv.URL + "/rentry/%s/raw"
	t.Cleanup(func() { app.RentryRaw = old })

	imp, err := view.ReadImport(e.App, view.ImportText, " https://steamcommunity.com/sharedfiles/filedetails/?id=3000000001 ")
	if err != nil {
		t.Fatal(err)
	}
	p, err := view.PreviewImport(e.App, imp)
	if err != nil {
		t.Fatal(err)
	}
	if p.Format != "Workshop collection" || p.Name != "My Pack" || len(p.Installed) != 2 || len(p.Missing) != 1 || p.Missing[0].Name != "Not Here" || !strings.HasSuffix(p.Missing[0].URL, "=5550001") {
		t.Fatalf("collection: %+v", p)
	}
	view.ApplyImport(e.App, imp, false)
	if got := enabled(t, e); got != "Harmony,Alpha" {
		t.Fatalf("collection import: %s", got)
	}

	imp, err = view.ReadImport(e.App, view.ImportText, "https://rentry.co/abc")
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := view.PreviewImport(e.App, imp); p.Format != "Rentry list" || len(p.Installed) != 1 {
		t.Fatalf("rentry: %+v", p)
	}
	if _, err := view.ReadImport(e.App, view.ImportText, "https://rentry.co/nope"); err == nil || !strings.Contains(err.Error(), "paste") {
		t.Fatalf("rentry failure: %v", err)
	}
}

// X-GUI4: export writes every format readable back, never over a file without asking and
// never to the game's ModsConfig.xml.
func TestExport(t *testing.T) {
	e := exchangeEnv(t)
	dir := t.TempDir()
	for _, f := range view.ExportFormats(e.App) {
		path := filepath.Join(dir, f.ID+"-"+f.File)
		if _, err := view.ExportTo(e.App, f.ID, path, false); err != nil {
			t.Fatalf("%s: %v", f.ID, err)
		}
		il, err := app.ReadLoadoutFile(path)
		if err != nil || strings.Join(il.Tokens, ",") != "ludeon.rimworld,tester.a" {
			t.Fatalf("%s reads back as %v %+v", f.ID, err, il)
		}
		if _, err := view.ExportTo(e.App, f.ID, path, false); err == nil {
			t.Fatalf("%s: overwrote without asking", f.ID)
		}
	}
	il, _ := app.ReadLoadoutFile(filepath.Join(dir, "rimcrow-xml-"+view.ExportFormats(e.App)[2].File))
	if il.Format != "rimcrow-modlist" || il.Workshop["tester.a"] != "1001" || il.Names["tester.a"] != "Alpha" {
		t.Fatalf("RimCrow ModList.xml: %+v", il)
	}
	before, _ := os.ReadFile(e.ModsConfig)
	if _, err := view.ExportTo(e.App, app.FormatModsConfig, e.ModsConfig, true); err == nil {
		t.Fatal("export wrote the game's ModsConfig.xml")
	}
	if after, _ := os.ReadFile(e.ModsConfig); !bytes.Equal(before, after) {
		t.Fatal("ModsConfig.xml changed")
	}
	if s, err := view.ExportText(e.App, app.FormatRimCrow); err != nil || !strings.HasPrefix(s, "RC-") {
		t.Fatalf("share code: %v %q", err, s)
	}
}
