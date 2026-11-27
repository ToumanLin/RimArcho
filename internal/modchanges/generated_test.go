package modchanges_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 16 §2.9 "Generated Mods": RimArcho Looks Preference appearing and being rebuilt is
// RimArcho's own doing: Changes shows neither as a Mod that arrived nor as one that changed.
func TestGeneratedModIsNoChange(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	tex := func(pkg, name, content string) testkit.Mod {
		return testkit.Mod{PackageID: pkg, Name: name, Files: map[string]string{"Textures/Things/Gun.png": content, "Textures/Things/Hat.png": content + "hat"}}
	}
	e.AddWorkshop("1001", tex("looks.a", "A", "first"))
	e.AddWorkshop("1002", tex("looks.b", "B", "second"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "looks.a", "looks.b")
	e.MustRun("scan")
	e.Advance(time.Hour)
	e.RecordLaunch(e.Now())
	e.Advance(time.Hour)
	ctx := context.Background()
	if _, err := e.App.LooksApply(ctx, "things/gun", "pkg:looks.a", nil, app.LooksPlan, false); err != nil {
		t.Fatal(err)
	}
	e.Advance(time.Hour)
	if _, err := e.App.LooksApply(ctx, "things/hat", "pkg:looks.a", nil, app.LooksPlan, false); err != nil {
		t.Fatal(err)
	}
	e.MustRun("scan")
	// a real change beside it, so the page is not empty for another reason
	b := tex("looks.b", "B", "second")
	b.Files["Defs/New.xml"] = "<Defs><ThingDef><defName>B_New</defName></ThingDef></Defs>\n"
	e.WriteMod(filepath.Join(e.Workshop, "1002"), b)
	e.MustRun("scan")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	p, err := modchanges.Build(e.App, st, modchanges.Opts{Library: true}, modchanges.English(time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	rows := append([]modchanges.Row{}, p.Rows...)
	if p.Arrivals != nil {
		rows = append(rows, p.Arrivals.Rows...)
	}
	for _, d := range p.Older {
		rows = append(rows, d.Rows...)
	}
	seenB := false
	for _, r := range rows {
		if r.Mod == app.LooksName {
			t.Errorf("Changes lists %s: %s", r.Mod, r.Text)
		}
		seenB = seenB || r.Mod == "B"
	}
	if !seenB {
		t.Fatalf("fixture: B's change is not listed: %+v", p)
	}
}
