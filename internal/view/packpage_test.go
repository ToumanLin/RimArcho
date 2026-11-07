package view_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestBuildPackPageCheck(t *testing.T) {
	view.SetLang("en")
	t.Cleanup(func() { view.SetLang("en") })
	e := testkit.New(t)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.AddEnv(false)
	e.MustRun("scan")
	game, err := e.App.Env()
	if err != nil {
		t.Fatal(err)
	}
	game.GameVersion = "1.6.4871 rev590"
	if err := e.App.Store.UpdateEnv(game); err != nil {
		t.Fatal(err)
	}
	p := app.Pack{Format: 2, Name: "page", Env: game.Name, Game: "1.6.4633 rev1261", Entries: []app.PackEntry{{PackageID: "ludeon.rimworld", Name: "Core", Official: true}, {PackageID: "test.missing.copy", Name: "Missing Copy", Folder: "missing", Bytes: 100}}}
	write := func() {
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		e.WriteFile(filepath.Join(e.App.PacksDir(), p.Name, "pack.json"), string(b))
	}
	write()
	pg, err := view.BuildPackPage(e.App, p.Name)
	if err != nil {
		t.Fatal(err)
	}
	if pg.Check == nil || pg.Blocked || pg.Check.Blocked() || len(pg.Check.Items) != 1 || pg.Check.Items[0].Level != finding.Warning {
		t.Fatalf("page comparison (must not hash missing copy): %+v", pg)
	}
	if !strings.Contains(pg.Check.Items[0].Text, "1.6.4633 rev1261") {
		t.Fatalf("lost full version: %+v", pg.Check)
	}
	p.Entries = append(p.Entries, app.PackEntry{PackageID: "ludeon.rimworld.biotech", Name: "Biotech", Official: true})
	write()
	view.SetLang("zh")
	pg, err = view.BuildPackPage(e.App, p.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !pg.Blocked || !pg.Check.Blocked() || !strings.Contains(pg.Why, "缺 DLC：Biotech") || !strings.Contains(pg.Check.Items[0].Text, "为 1.6.4633 rev1261 创建") {
		t.Fatalf("translated blocked page: %+v", pg)
	}
}

func TestPackPageEntryIdentitySurvivesReordering(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.AddEnv(false)
	p := app.Pack{Format: 2, Name: "identity", Game: "1.6.4633 rev1261", Entries: []app.PackEntry{
		{PackageID: "fixture.a", Name: "Same Mod", Folder: "folder one", Bytes: 100},
		{PackageID: "fixture.b", Name: "Same Mod", Folder: "folder two", Bytes: 200},
	}}
	read := func() *view.PackPage {
		t.Helper()
		data, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		e.WriteFile(filepath.Join(e.App.PacksDir(), p.Name, "pack.json"), string(data))
		page, err := view.BuildPackPage(e.App, p.Name)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	before := read()
	if before.Mods[0].Key == "" || before.Mods[0].Key == before.Mods[1].Key {
		t.Fatalf("duplicate display names need distinct record identities: %+v", before.Mods)
	}
	p.Entries[0], p.Entries[1] = p.Entries[1], p.Entries[0]
	after := read()
	if before.Mods[0].Key != after.Mods[1].Key || before.Mods[1].Key != after.Mods[0].Key {
		t.Fatalf("entry identity changed with load order: before=%+v after=%+v", before.Mods, after.Mods)
	}
}
