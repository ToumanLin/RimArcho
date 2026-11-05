package view_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// savesEnv: Core, Alpha and Beta enabled, Gamma installed but not enabled, a launch of the
// list an hour ago. Saves: Tundra with the list's Mods; Tundra 2 after the launch, in
// another order, with Gamma and a Workshop Mod that is not installed; its autosave; a
// RimSaves save; one cut off by a crash; and one of another game version matching no list,
// with seven Mods that are not installed.
func savesEnv(t *testing.T) *testkit.Env {
	t.Helper()
	e := testkit.New(t)
	e.AddWorkshop("3000000002", testkit.Sample("tester.alpha", "Alpha"))
	e.AddWorkshop("3000000003", testkit.Sample("tester.beta", "Beta"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Gamma"), testkit.Sample("tester.gamma", "Gamma"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.alpha", "tester.beta")
	e.AddEnv(false)
	e.MustRun("scan")
	now := e.Now()
	e.RecordLaunch(now.Add(-time.Hour))
	same := testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.alpha", "tester.beta"}, ModTime: now.Add(-2 * time.Hour)}
	e.WriteSave("Tundra.rws", same)
	later := testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.beta", "tester.alpha", "tester.gamma", "tester.gone"},
		Names: []string{"Core", "Beta", "Alpha", "Gamma", "Gone Mod"}, SteamIDs: []string{"", "3000000003", "3000000002", "", "99"},
		ModTime: now.Add(-30 * time.Minute)}
	e.WriteSave("Tundra 2.rws", later)
	later.ModTime = now.Add(-20 * time.Minute)
	e.WriteSave("Autosave-1.rws", later)
	same.ModTime = now.Add(-3 * time.Hour)
	e.WriteSave("Ashfall#§#Landing.rws", same)
	e.WriteSave("Broken.rws", testkit.SaveSpec{Mods: same.Mods, CutOff: true, ModTime: now.Add(-10 * time.Minute)})
	old := testkit.SaveSpec{Game: "1.5.4104 rev435", Mods: []string{"ludeon.rimworld"}, Names: []string{"Core"}, ModTime: now.Add(-5 * time.Hour)}
	for i := range 7 {
		old.Mods = append(old.Mods, fmt.Sprintf("old.m%d", i))
		old.Names = append(old.Names, fmt.Sprintf("Old %d", i))
	}
	e.WriteSave("Sandy.rws", old)
	return e
}

func TestSavesRows(t *testing.T) {
	t.Parallel()
	e := savesEnv(t)
	l, err := e.App.ListSaves()
	if err != nil {
		t.Fatal(err)
	}
	p := view.SavesRows(l)
	var cols []string
	for _, c := range p.Colonies {
		var names []string
		for _, s := range c.Saves {
			names = append(names, s.Name)
			if s.When == "" || s.Ref == "" {
				t.Fatalf("save row %+v", s)
			}
		}
		cols = append(cols, fmt.Sprintf("%s(%d): %s", c.Name, len(c.Saves), strings.Join(names, ", ")))
	}
	want := []string{"Broken(1): Broken", "Tundra(3): Autosave-1, Tundra 2, Tundra", "Ashfall(1): Landing", "Sandy(1): Sandy"}
	if !slices.Equal(cols, want) || p.Saves != 6 || p.Lead != "4 colonies  6 saves" || p.Empty != "" || len(p.Folders) != 1 {
		t.Fatalf("left card %q, want %q; %+v", cols, want, p)
	}
	if !p.Colonies[1].Saves[0].Auto {
		t.Fatal("the autosave is not marked")
	}

	empty := view.SavesRows(&app.SaveList{})
	if empty.Saves != 0 || empty.Empty == "" || empty.Lead != "" {
		t.Fatalf("no saves: %+v", empty)
	}
}

func TestSaveDetail(t *testing.T) {
	t.Parallel()
	e := savesEnv(t)
	current := false
	detail := func(name string, running bool) *view.SaveDetail {
		t.Helper()
		s, _, err := e.App.FindSave(name)
		if err != nil {
			t.Fatal(err)
		}
		c, err := e.App.CheckSave(s)
		if err != nil {
			t.Fatal(err)
		}
		return view.SaveDetailOf(c, running, current)
	}
	rows := func(d *view.SaveDetail) []string {
		var out []string
		for _, is := range d.Issues {
			r := is.Group + " " + is.Level + " " + is.Sign + " " + is.Head
			for _, f := range is.Fixes {
				r += " [" + f.Label + "]"
			}
			out = append(out, r)
		}
		return out
	}

	// loads as it is: green, Launch, nothing listed
	d := detail("Tundra", false)
	if d.Level != "" || d.Verdict != "loads with default as it is" || d.Quick == nil || d.Quick.Action != app.FixLaunch || d.Quick.Label != "launch" || len(d.Issues) != 0 {
		t.Fatalf("same Mods: %+v %v", d, rows(d))
	}
	if d.ListKind != "list" || d.List != "default" || d.ListID == 0 || d.Match != "the same Mods as a version of this list" || d.Game != "1.6.4633 rev1261" || d.Env == "" {
		t.Fatalf("where it belongs: %+v", d)
	}
	if d.Play != nil || d.Reading || d.PlayNote != "" {
		t.Fatal("play facts from the head: the right card is the caller's (gui savePlay)")
	}

	// Mods missing and reordered: one row each, with its consequence and its own fix; the
	// bar offers no Fix and launch (16 §2.11)
	current = true
	d = detail("Tundra 2", false)
	current = false
	want := []string{
		"mods warning − Gamma [enable]",
		"mods warning − Gone Mod [subscribe and enable]",
		"mods info ↕ Beta is in another order [use the save's order]",
	}
	if !slices.Equal(rows(d), want) {
		t.Fatalf("issues %q\nwant %q", rows(d), want)
	}
	if d.Verdict != "content will be lost" || d.Quick != nil {
		t.Fatalf("verdict %q quick %+v", d.Verdict, d.Quick)
	}
	if d.Match != "saved after a launch of this list" || d.Against != "against the current list default" {
		t.Fatalf("match %q against %q", d.Match, d.Against)
	}
	if d.Issues[0].Title != "installed but not in the list" || d.Issues[1].Details == nil || len(d.Issues[1].Details.Mods) != 1 ||
		d.Issues[1].Details.Mods[0] != (view.SaveDetailMod{Name: "Gone Mod", PackageID: "tester.gone", Note: "Workshop 99"}) {
		t.Fatalf("hover text and details: %+v", d.Issues[:2])
	}
	for _, is := range d.Issues {
		if is.Consequence == "" {
			t.Fatalf("no consequence: %+v", is)
		}
	}
	if len(d.Mods) != 5 || !d.Mods[4].Missing || !d.Mods[3].Off || !d.Mods[1].Moved {
		t.Fatalf("Mod list: %+v", d.Mods)
	}
	if d.NewList == nil || d.NewList.Label != "make a list from this save" {
		t.Fatalf("make a list: %+v", d.NewList)
	}
	if d := detail("Tundra 2", false); d.Against != "against list default" {
		t.Fatalf("against a list that is not current: %q", d.Against)
	}

	// cut off: Recycle first, and the colony's previous readable save
	d = detail("Broken", false)
	if d.Level != app.LevelError || d.Verdict != "this save cannot be loaded" || d.Quick.Action != app.FixRecycle || d.Quick.Label != "recycle…" {
		t.Fatalf("cut off: %+v %v", d, rows(d))
	}
	if r := rows(d)[0]; r != "file error  the file is cut off [recycle…]" {
		t.Fatalf("cut-off row %q", r)
	}

	// no list, another game version, seven Mods not installed: the game's row first, then
	// one row for the seven
	d = detail("Sandy", false)
	got := rows(d)
	want = []string{
		"game warning  the game version changed since this save (1.5.4104 rev435 → 1.6.4633 rev1261)",
		"mods warning − Old 0, Old 1, Old 2, Old 3, Old 4, Old 5 and 1 more",
		"mods info  no list matches this save [make a list from this save]",
	}
	if d.ListKind != "" || d.Quick.Action != app.FixNewList || !slices.Equal(got, want) {
		t.Fatalf("no list: %+v\n%q\nwant %q", d, got, want)
	}
	if g := d.Issues[1]; len(g.Mods) != 7 || len(g.Details.Mods) != 7 || g.Details.Mods[0].Note != "no known Workshop page" ||
		g.Consequence != "what they stored in the save is removed when the save loads" {
		t.Fatalf("the Mods not installed: %+v", g)
	}

	// a running game locks rename and recycle, with the reason
	if d := detail("Tundra", true); d.Locked == "" {
		t.Fatal("not locked while the game runs")
	}
}

// Seven Mods to subscribe are one row whose one fix names all of them.
func TestSaveDetailFoldsFixes(t *testing.T) {
	t.Parallel()
	e := savesEnv(t)
	spec := testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.alpha", "tester.beta"}, ModTime: e.Now().Add(-10 * time.Second)}
	for i := range 7 {
		spec.Mods = append(spec.Mods, fmt.Sprintf("more.m%d", i))
		spec.SteamIDs = append(spec.SteamIDs, "")
	}
	spec.SteamIDs = append([]string{"", "", ""}, slices.Repeat([]string{"777"}, 7)...)
	e.WriteSave("Wide.rws", spec)
	s, _, err := e.App.FindSave("Wide")
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.App.CheckSave(s)
	if err != nil {
		t.Fatal(err)
	}
	d := view.SaveDetailOf(c, false, false)
	var gone []view.SaveIssueView
	for _, is := range d.Issues {
		if is.Kind == app.IssueNotInstalled {
			gone = append(gone, is)
		}
	}
	if len(gone) != 1 || len(gone[0].Fixes) != 1 || gone[0].Fixes[0].Action != "mods" || gone[0].Fixes[0].Label != "subscribe and enable these" ||
		gone[0].Fixes[0].Target != "more.m0 more.m1 more.m2 more.m3 more.m4 more.m5 more.m6" {
		t.Fatalf("one row, one fix: %+v", gone)
	}
}

func TestSaveFilePreviews(t *testing.T) {
	t.Parallel()
	e := savesEnv(t)
	find := func(ref string) *app.Save {
		t.Helper()
		s, _, err := e.App.FindSave(ref)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	r, err := e.App.BackupSaves([]*app.Save{find("Tundra"), find("Broken")}, false)
	if err != nil {
		t.Fatal(err)
	}
	p := view.SaveBackupPreview(r)
	if !strings.HasPrefix(p.Title, "back up 2 saves (") || len(p.Lines) != 2 || !strings.HasPrefix(p.Lines[0], "Tundra.rws  ") || !strings.Contains(p.To, filepath.Join("backups", "saves")) {
		t.Fatalf("backup preview: %+v", p)
	}
	tr, err := e.App.TrashSaves([]*app.Save{find("Broken")}, false)
	if err != nil {
		t.Fatal(err)
	}
	if p := view.SaveTrashPreview(tr); !strings.HasPrefix(p.Title, "move Broken to the recycle bin (") || p.Button != "recycle" || len(p.Lines) != 1 {
		t.Fatalf("recycle preview: %+v", p)
	}
	landing := find("Ashfall/Landing")
	for name, want := range map[string]string{
		"":             "the name is empty",
		`a/b`:          `a save name cannot hold \ / : * ? " < > |`,
		"Landing":      "",
		"First winter": "",
	} {
		if got := view.SaveNameProblem(landing, name); got != want {
			t.Errorf("name %q: %q, want %q", name, got, want)
		}
	}
	e.WriteSave("Ashfall#§#Second.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}})
	if got := view.SaveNameProblem(landing, "second"); got != "a save named second is already there" {
		t.Fatalf("a duplicate in the colony: %q", got)
	}
	if got := view.SaveNameProblem(find("Tundra"), "Ashfall#§#Second"); got == "" {
		t.Fatal("accepted a RimSaves separator")
	}
}
