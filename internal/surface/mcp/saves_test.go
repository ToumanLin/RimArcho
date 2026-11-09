package mcp_test

import (
	"encoding/base64"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// savesFixture has the shape of internal/app's: Core, Alpha and Beta enabled, Gamma
// installed but not enabled, a launch an hour ago; a save with the same Mods, one after the
// launch with another order plus Gamma and a Workshop Mod that is not installed, its
// autosave, and one cut off by a crash.
func savesFixture(t *testing.T) *testkit.Env {
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
	later := testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "Tester.Beta", "tester.alpha", "tester.gamma", "tester.gone"},
		Names: []string{"Core", "Beta", "Alpha", "Gamma", "Gone Mod"}, SteamIDs: []string{"", "3000000003", "3000000002", "", "99"},
		ModTime: now.Add(-30 * time.Minute)}
	e.WriteSave("Tundra 2.rws", later)
	later.ModTime = now.Add(-20 * time.Minute)
	e.WriteSave("Autosave-1.rws", later)
	e.WriteSave("Broken.rws", testkit.SaveSpec{Mods: same.Mods, CutOff: true, ModTime: now.Add(-10 * time.Minute)})
	return e
}

// playedSave writes Rimward.rws: a colony read in full has a world, a faction, colonists, a
// letter and a wealth series (the element paths internal/savefile reads).
func playedSave(e *testkit.Env) {
	raw := make([]byte, 8)
	binary.LittleEndian.PutUint32(raw, math.Float32bits(9000))
	binary.LittleEndian.PutUint32(raw[4:], math.Float32bits(12345))
	pawn := func(id, nick, extra string) string {
		return `<thing Class="Pawn"><def>Human</def><id>` + id + `</id><faction>Faction_1</faction><kindDef>Colonist</kindDef>` +
			`<name Class="NameTriple"><first>` + nick + `</first><nick>` + nick + `</nick><last>Lee</last></name><story />` + extra + `</thing>`
	}
	body := `<info><realPlayTimeInteracting>7300</realPlayTimeInteracting></info>` +
		`<storyteller><def>Cassandra</def><difficulty>Rough</difficulty></storyteller>` +
		`<storyWatcher><statsRecord><numRaidsEnemy>4</numRaidsEnemy></statsRecord></storyWatcher>` +
		`<history><archive><archivables><li Class="StandardLetter"><def>ThreatBig</def><label>Raid</label><arrivalTick>600000</arrivalTick></li></archivables></archive>` +
		`<autoRecorderGroups><li><def>Wealth</def><recorders><li><def>Wealth_Total</def><records>` + base64.StdEncoding.EncodeToString(raw) +
		`</records></li></recorders></li></autoRecorderGroups></history>` +
		`<maps><li><uniqueID>0</uniqueID><things>` + pawn("Human1", "Mara", "") + pawn("Human2", "Kari", `<guest><guestStatus>Slave</guestStatus></guest>`) +
		pawn("Human3", "Ozzie", `<healthTracker><healthState>Dead</healthState></healthTracker>`) + `</things></li></maps>`
	e.WriteSave("Rimward.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.alpha", "tester.beta"}, World: "Ennawrin", Seed: "hollow", PRV: -7,
		Faction: "New Arrivals", Ticks: 25*60000 + 1234, Body: body, ModTime: e.Now().Add(-5 * time.Hour)})
}

// I1 for the save tools: list_saves, get_save and manage_saves give the text of archo saves,
// read-only calls and previews alike (10 SV15). get_save reads its save in full the first
// time (the MCP call) and from the cache after (the CLI): the text is the same.
func TestSaveToolsCLIParity(t *testing.T) {
	t.Parallel()
	e := savesFixture(t)
	playedSave(e)
	cs := connect(t, e)
	backups := filepath.Join(e.Home, "backups", "saves")
	cases := []struct {
		tool string
		args map[string]any
		cli  []string
	}{
		{"list_saves", nil, []string{"saves", "list"}},
		{"list_saves", map[string]any{"attention": true}, []string{"saves", "list", "--attention"}},
		{"list_saves", map[string]any{"mod": "tester.gone"}, []string{"saves", "list", "--mod", "tester.gone"}},
		{"list_saves", map[string]any{"mod": "pkg:tester.gamma", "colony": "tundra", "all": true}, []string{"saves", "list", "--colony", "tundra", "--mod", "pkg:tester.gamma", "--all"}},
		{"list_saves", map[string]any{"colony": "nope"}, []string{"saves", "list", "--colony", "nope"}},
		{"get_save", map[string]any{"save": "Tundra 2"}, []string{"saves", "show", "Tundra 2"}},
		{"get_save", map[string]any{"save": "Tundra 2.rws", "mods": true, "all": true}, []string{"saves", "show", "Tundra 2.rws", "--mods", "--all"}},
		{"get_save", map[string]any{"save": "Broken"}, []string{"saves", "show", "Broken"}},
		{"get_save", map[string]any{"save": "Nope"}, []string{"saves", "show", "Nope"}},
		{"get_save", map[string]any{"save": "Rimward"}, []string{"saves", "show", "Rimward"}},
		{"get_save", map[string]any{"save": "Rimward", "play": true}, []string{"saves", "show", "Rimward", "--play"}},
		{"get_save", map[string]any{"save": "Rimward", "content": true}, []string{"saves", "show", "Rimward", "--content"}},
		{"get_save", map[string]any{"save": "Rimward.rws", "play": true, "mods": true, "all": true}, []string{"saves", "show", "Rimward.rws", "--mods", "--play", "--all"}},
		{"list_saves", map[string]any{"colony": "New Arrivals"}, []string{"saves", "list", "--colony", "New Arrivals"}},
		{"manage_saves", map[string]any{"action": "fix", "saves": []string{"Tundra 2"}, "dry_run": true}, []string{"saves", "fix", "Tundra 2", "--dry-run"}},
		{"manage_saves", map[string]any{"action": "fix", "saves": []string{"Tundra 2"}, "mods": []string{"tester.gamma"}, "order": true, "disable_added": true, "launch": true, "dry_run": true},
			[]string{"saves", "fix", "Tundra 2", "--mod", "tester.gamma", "--disable-added", "--order", "--launch", "--dry-run"}},
		{"manage_saves", map[string]any{"action": "backup", "saves": []string{"Tundra", "Broken"}}, []string{"saves", "backup", "Tundra", "Broken"}},
		{"manage_saves", map[string]any{"action": "trash", "saves": []string{"Broken"}, "confirm": true, "dry_run": true}, []string{"saves", "trash", "Broken"}},
		{"manage_saves", map[string]any{"action": "trash", "saves": []string{"Broken"}}, []string{"saves", "trash", "Broken"}},
	}
	for _, c := range cases {
		got, isErr := call(t, cs, c.tool, c.args)
		want, code := e.Run(c.cli...)
		if got != want {
			t.Errorf("%s %v: MCP text differs from `archo %s`:\n--- mcp\n%s\n--- cli\n%s", c.tool, c.args, strings.Join(c.cli, " "), got, want)
		}
		if isErr != (code == cmds.ExitError || code == cmds.ExitUsage) {
			t.Errorf("%s %v: isError=%v, exit %d", c.tool, c.args, isErr, code)
		}
		if n := strings.Count(strings.TrimRight(got, "\n"), "\n") + 1; n > 40 {
			t.Errorf("%s %v: %d lines (I5: at most 40)", c.tool, c.args, n)
		}
	}
	// env= keeps one game's saves, as the CLI's global --env does.
	got, _ := call(t, cs, "list_saves", map[string]any{"env": "main"})
	e.App.EnvName = "main"
	want, _ := e.Run("saves", "list")
	e.App.EnvName = ""
	if got != want || !strings.Contains(got, "(env:main)") {
		t.Errorf("list_saves env=main:\n--- mcp\n%s\n--- cli\n%s", got, want)
	}
	// Previews wrote nothing.
	if _, err := os.Stat(backups); err == nil {
		t.Error("a backup preview wrote the backups folder")
	}
	if _, err := os.Stat(filepath.Join(e.SavesDir(), "Broken.rws")); err != nil {
		t.Errorf("a trash preview moved the save: %v", err)
	}
	if out := e.MustRun("loadout", "export"); strings.Contains(out, "tester.gamma") {
		t.Errorf("a fix dry run changed the plan:\n%s", out)
	}
	// The played save was read in full: the list gives its day, its colony is its world's.
	if got, _ := call(t, cs, "list_saves", nil); !strings.Contains(got, "New Arrivals  1 save") || !strings.Contains(got, "day 25") {
		t.Errorf("list_saves after get_save:\n%s", got)
	}
	if got, _ := call(t, cs, "get_save", map[string]any{"save": "Rimward", "play": true}); !strings.Contains(got, "play: New Arrivals, day 25") ||
		!strings.Contains(got, "colonists: 2 (1 slave), 1 dead") || !strings.Contains(got, "Ozzie  Human  dead") || !strings.Contains(got, "day 10  Raid") {
		t.Errorf("get_save play=true:\n%s", got)
	}
}

// manage_saves acts only when asked: backup and trash need confirm=true, a fix is one
// undoable edit, rename changes the file and has no dry run.
func TestManageSavesActions(t *testing.T) {
	t.Parallel()
	e := savesFixture(t)
	cs := connect(t, e)
	var trashed []string
	e.App.TrashFunc = func(p string) error { trashed = append(trashed, filepath.Base(p)); return os.Remove(p) }

	out, isErr := call(t, cs, "manage_saves", map[string]any{"action": "backup", "saves": []string{"Tundra", "Broken"}, "confirm": true})
	dir := filepath.Join(e.Home, "backups", "saves", e.Now().Format("20060102-150405"))
	if isErr || !strings.HasPrefix(out, "backed up 2 saves") {
		t.Errorf("backup confirm=true:\n%s", out)
	}
	for _, f := range []string{"Tundra.rws", "Broken.rws"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("backup: %v", err)
		}
	}

	out, isErr = call(t, cs, "manage_saves", map[string]any{"action": "trash", "saves": []string{"Broken"}})
	if isErr || len(trashed) != 0 || !strings.Contains(out, "recycle preview, nothing moved") {
		t.Errorf("trash without confirm moved %v:\n%s", trashed, out)
	}
	out, isErr = call(t, cs, "manage_saves", map[string]any{"action": "trash", "saves": []string{"Broken"}, "confirm": true})
	if isErr || !slices.Equal(trashed, []string{"Broken.rws"}) || !strings.HasPrefix(out, "moved to the recycle bin: 1 save") {
		t.Errorf("trash confirm=true: trashed %v\n%s", trashed, out)
	}

	before := e.MustRun("loadout", "export")
	out, isErr = call(t, cs, "manage_saves", map[string]any{"action": "fix", "saves": []string{"Tundra 2"}, "mods": []string{"tester.gamma"}})
	if isErr || !strings.Contains(out, "enabled:") || !strings.Contains(e.MustRun("loadout", "export"), "tester.gamma") {
		t.Errorf("fix:\n%s", out)
	}
	if _, isErr := call(t, cs, "edit_loadout", map[string]any{"action": "undo"}); isErr || e.MustRun("loadout", "export") != before {
		t.Error("undo did not take the fix back")
	}

	if out, isErr := call(t, cs, "manage_saves", map[string]any{"action": "rename", "saves": []string{"Tundra"}, "name": "Tundra old", "dry_run": true}); !isErr {
		t.Errorf("rename dry_run ran:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(e.SavesDir(), "Tundra.rws")); err != nil {
		t.Fatal("rename dry_run renamed the file")
	}
	out, isErr = call(t, cs, "manage_saves", map[string]any{"action": "rename", "saves": []string{"Tundra"}, "name": "Tundra old"})
	if _, err := os.Stat(filepath.Join(e.SavesDir(), "Tundra old.rws")); isErr || err != nil {
		t.Errorf("rename: %v\n%s", err, out)
	}
	out, isErr = call(t, cs, "manage_saves", map[string]any{"action": "copy", "saves": []string{"Tundra old"}})
	if _, err := os.Stat(filepath.Join(e.SavesDir(), "Tundra old (copy).rws")); isErr || err != nil {
		t.Errorf("copy: %v\n%s", err, out)
	}
	if out, isErr := call(t, cs, "manage_saves", map[string]any{"action": "delete", "saves": []string{"Tundra old"}}); !isErr || !strings.Contains(out, "unknown action") {
		t.Errorf("unknown action:\n%s", out)
	}
}

// Each manage_saves action maps to one archo saves command line.
func TestManageSavesArgv(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		args string
		want []string
	}{
		{`{"action":"fix","saves":["sv:4be1c0"],"mods":["a.b","c.d"],"launch":true}`, []string{"saves", "fix", "sv:4be1c0", "--mod", "a.b", "--mod", "c.d", "--launch"}},
		{`{"action":"rename","saves":["sv:4be1c0"],"name":"First day"}`, []string{"saves", "rename", "sv:4be1c0", "First day"}},
		{`{"action":"copy","saves":["Tundra"]}`, []string{"saves", "copy", "Tundra"}},
		{`{"action":"backup","saves":["a","b"]}`, []string{"saves", "backup", "a", "b"}},
		{`{"action":"backup","saves":["a"],"confirm":true}`, []string{"saves", "backup", "a", "--yes"}},
		{`{"action":"trash","saves":["a"],"confirm":true,"dry_run":true}`, []string{"saves", "trash", "a"}},
		{`{"action":"trash","saves":["a"],"confirm":true}`, []string{"saves", "trash", "a", "--yes"}},
	} {
		got, err := mcp.ArgvFor("manage_saves", []byte(c.args))
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%s → %q, %v; want %q", c.args, got, err, c.want)
		}
	}
	if got, _ := mcp.ArgvFor("get_save", []byte(`{"save":"Tundra 2","mods":true}`)); !slices.Equal(got, []string{"saves", "show", "Tundra 2", "--mods"}) {
		t.Errorf("get_save → %q", got)
	}
	if got, _ := mcp.ArgvFor("get_save", []byte(`{"save":"Tundra 2","play":true,"all":true}`)); !slices.Equal(got, []string{"saves", "show", "Tundra 2", "--play", "--all"}) {
		t.Errorf("get_save play → %q", got)
	}
	if got, _ := mcp.ArgvFor("list_saves", []byte(`{"mod":"#z3bw","attention":true}`)); !slices.Equal(got, []string{"saves", "list", "--mod", "#z3bw", "--attention"}) {
		t.Errorf("list_saves → %q", got)
	}
}
