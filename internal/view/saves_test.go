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
	// search matches Mods in a colony's saves, by packageId or name, without case
	if got := view.SavesMatching(l, "GONE mod"); !slices.Equal(got, []string{p.Colonies[1].Key}) {
		t.Fatalf("search by Mod name: %v", got)
	}
	if got := view.SavesMatching(l, "old.m3"); !slices.Equal(got, []string{p.Colonies[3].Key}) {
		t.Fatalf("search by packageId: %v", got)
	}
	if got := view.SavesMatching(l, ""); len(got) != 0 {
		t.Fatalf("empty search: %v", got)
	}

	empty := view.SavesRows(&app.SaveList{})
	if empty.Saves != 0 || empty.Empty == "" || empty.Lead != "" {
		t.Fatalf("no saves: %+v", empty)
	}
}

func TestSaveDetail(t *testing.T) {
	t.Parallel()
	e := savesEnv(t)
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
		return view.SaveDetailOf(c, running)
	}
	rows := func(d *view.SaveDetail) []string {
		var out []string
		for _, is := range d.Issues {
			r := is.Level + " " + is.Sign + " " + is.Head
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
	if d.Play != nil || d.PlayNote == "" {
		t.Fatal("play facts before the full read")
	}

	// Mods missing and reordered: one row each, with its consequence and its own fix
	d = detail("Tundra 2", false)
	want := []string{
		"warning − Gamma is installed but not in the list [enable in the list]",
		"warning − Gone Mod is not installed [subscribe and enable]",
		"info ↕ Beta is in another order [use the save's order]",
	}
	if !slices.Equal(rows(d), want) {
		t.Fatalf("issues %q\nwant %q", rows(d), want)
	}
	if d.Verdict != "loads, but content will be lost" || d.Quick.Label != "fix and launch" || d.Quick.Command != "archo saves fix "+d.Ref+" --launch" {
		t.Fatalf("verdict %q quick %+v", d.Verdict, d.Quick)
	}
	if d.Match != "saved after a launch of this list" || d.Against != "against default as it is now" {
		t.Fatalf("match %q against %q", d.Match, d.Against)
	}
	for _, is := range d.Issues {
		if is.Consequence == "" {
			t.Fatalf("no consequence: %+v", is)
		}
	}
	if d.Missing != 1 || d.Moved != 1 || len(d.Mods) != 5 || !d.Mods[4].Missing || !d.Mods[3].Off || !d.Mods[1].Moved {
		t.Fatalf("Mod list: %+v", d.Mods)
	}
	if d.NewList == nil || d.NewList.Label != "make a list from this save" {
		t.Fatalf("make a list: %+v", d.NewList)
	}

	// cut off: Recycle first, and the colony's previous readable save
	d = detail("Broken", false)
	if d.Level != app.LevelError || d.Verdict != "this save cannot be loaded" || d.Quick.Action != app.FixRecycle || d.Quick.Label != "recycle…" {
		t.Fatalf("cut off: %+v %v", d, rows(d))
	}
	if r := rows(d)[0]; r != "error  the file is cut off [recycle…]" {
		t.Fatalf("cut-off row %q", r)
	}

	// no list, another game version, seven Mods not installed: four rows and one folded row
	d = detail("Sandy", false)
	got := rows(d)
	if d.ListKind != "" || d.Quick.Action != app.FixNewList || !strings.Contains(got[0], "made in another game version (1.5.4104 rev435 → 1.6.4633 rev1261)") {
		t.Fatalf("no list: %+v %q", d, got)
	}
	var folded *view.SaveIssueView
	n := 0
	for i, is := range d.Issues {
		if is.Kind == app.IssueNotInstalled {
			n++
			folded = &d.Issues[i]
		}
	}
	if n != view.ModRowsShown+1 || folded.Head != "3 more Mods are not installed" || len(folded.Mods) != 3 || !strings.HasPrefix(folded.Consequence, "Old 4, Old 5, Old 6: ") {
		t.Fatalf("folded rows: %d, %+v", n, folded)
	}

	// a running game locks rename and recycle, with the reason
	if d := detail("Tundra", true); d.Locked == "" {
		t.Fatal("not locked while the game runs")
	}
}

// Seven Mods to enable or subscribe fold into one row whose one fix names all of them.
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
	d := view.SaveDetailOf(c, false)
	var last view.SaveIssueView
	for _, is := range d.Issues {
		if is.Kind == app.IssueNotInstalled {
			last = is
		}
	}
	if len(last.Fixes) != 1 || last.Fixes[0].Action != "mods" || last.Fixes[0].Target != "more.m4 more.m5 more.m6" {
		t.Fatalf("folded fix: %+v", last)
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
