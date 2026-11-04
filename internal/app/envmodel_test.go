package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// secondGame makes another RimWorld install outside any Steam library.
func secondGame(e *testkit.Env, name string) string { return e.OtherGame(name, "1.5.4409 rev1118") }

func sourcesOf(t *testing.T, e *testkit.Env) map[string]*source.Source {
	t.Helper()
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*source.Source{}
	for _, s := range ix.Sources {
		out[s.Label] = s
	}
	return out
}

// 02 §0: a second game shares the Workshop folder: the same instances, lineages and player
// data in both environments; Data/ and Mods/ are each game's own; plans are shared by every game.
func TestEnvModelSharedWorkshop(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("5100", testkit.Sample("tester.shared", "Shared"))
	e.AddEnv(false)
	scanNow(t, e)
	ws := refID(t, e, "Shared", source.Workshop)
	e.MustRun("tag", "add", "#"+ws[:6], "keeper")
	e.MustRun("enable", "#"+ws[:6])

	g2 := secondGame(e, "Game15")
	if out, code := e.Run("env", "add", "again", "--game", e.Game); code == 0 || !strings.Contains(out, "one environment per game folder") {
		t.Fatalf("a second environment for one game folder: exit %d\n%s", code, out)
	}
	out := e.MustRun("env", "add", "old", "--game", g2, "--saves", "own")
	if !strings.Contains(out, "saves: own") || strings.Contains(out, "src:workshop") {
		t.Fatalf("env add: the Workshop folder is already there, not added again:\n%s", out)
	}
	e.MustRun("env", "use", "old")
	// A copy of the shared mod in this game's Mods/, read by a game-only scan: it joins the
	// Workshop mod's lineage although the Workshop folder is not read.
	e.CopyTree(filepath.Join(e.Workshop, "5100"), filepath.Join(g2, "Mods", "Shared Copy"))
	r, err := e.App.ScanWith(app.ScanOptions{GameOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range r.Stats.Sources {
		if s.Kind == string(source.Workshop) {
			t.Fatalf("a game-only scan read the Workshop folder: %+v", r.Stats.Sources)
		}
	}
	ix, _ := e.App.Index()
	m := ix.ByID[ws]
	if m == nil || m.Status != identity.Live || !slices.Contains(m.Tags, "keeper") {
		t.Fatalf("the shared mod, with its tag, is the same in the other game: %+v", m)
	}
	var cp *store.Mod
	for _, x := range ix.Mods {
		if x.Rel == "Shared Copy" {
			cp = x
		}
	}
	if cp == nil || cp.LineageID != m.LineageID || cp.LineageBasis != identity.BasisInferred {
		t.Fatalf("the copy in Mods/ joins the shared lineage: %+v", cp)
	}
	for _, x := range ix.Mods {
		if x.Kind == source.Official && strings.Contains(strings.ToLower(x.RealPath), strings.ToLower(e.Game)) {
			t.Fatal("the other game's Data/ is not this one's")
		}
	}
	// Lists are shared by every game; the first game's list still uses the shared mod.
	e.MustRun("env", "use", "main")
	st, _ := e.App.Loadout()
	if st.List.Index(ws) < 0 {
		t.Fatal("main's plan lost the shared mod")
	}
	// Mod library: "used in" names each list once, whichever game uses it.
	e.MustRun("env", "use", "old")
	e.MustRun("enable", "#"+ws[:6])
	e.MustRun("env", "use", "main")
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	l, err := lib.Lineage("L#" + m.LineageID[:6])
	if err != nil {
		t.Fatal(err)
	}
	var uses []string
	for _, u := range l.UsedIn() {
		uses = append(uses, u.Name)
	}
	if !slices.Contains(uses, "default") {
		t.Fatalf("used in: %v", uses)
	}
	// Removing a game keeps the shared mod and what was said about it.
	e.MustRun("env", "remove", "old", "--yes")
	ix, _ = e.App.Index()
	if m := ix.ByID[ws]; m == nil || !slices.Contains(m.Tags, "keeper") {
		t.Fatal("env remove dropped a shared mod or its tag")
	}
}

// 02 §0 item 5: own saves live in <data>/userdata/<env> and the game starts with
// -savedatafolder; shared saves are the game's default folder.
func TestEnvModelSaves(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	g2 := secondGame(e, "Game15")
	e.MustRun("env", "add", "main", "--game", g2, "--saves", "own", "--launch", "direct")
	own := filepath.Join(e.Home, "userdata", "main")
	if st, err := os.Stat(own); err != nil || !st.IsDir() {
		t.Fatal("own saves folder not made")
	}
	scanNow(t, e)
	e.MustRun("launch")
	if len(e.Launched) != 1 || !strings.Contains(e.Launched[0], "-savedatafolder="+own) {
		t.Fatalf("launched: %v", e.Launched)
	}
	out := e.MustRun("env", "set", "main", "--saves", "shared")
	if !strings.Contains(out, "saves: shared") || !strings.Contains(e.Scrub(out), "AppData/LocalLow") {
		t.Fatalf("back to the default folder:\n%s", out)
	}
	if _, code := e.Run("env", "set", "main", "--saves", "own", "--user-data", e.UserData); code == 0 {
		t.Fatal("--saves with --user-data must be refused")
	}
}

// 02 §0 items 3, 12: shared mod folders are added and removed in one place; a second
// Workshop folder replaces the first (re-bound); RimArcho's own folders stay; folders
// must not contain each other.
func TestFolderCommands(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("5100", testkit.Sample("tester.shared", "Shared"))
	e.AddEnv(false)
	scanNow(t, e)
	extra := filepath.Join(e.Root, "MoreMods")
	e.WriteMod(filepath.Join(extra, "Extra One"), testkit.Sample("tester.extra", "Extra One"))
	out := e.MustRun("folder", "add", "extra", extra)
	if !strings.Contains(out, "src:extra  added") {
		t.Fatalf("folder add:\n%s", out)
	}
	if out := e.MustRun("folder", "add", "extra", extra); !strings.Contains(out, "unchanged") {
		t.Fatalf("adding again:\n%s", out)
	}
	if out, code := e.Run("folder", "add", "extra", filepath.Join(extra, "Extra One")); code == 0 || !strings.Contains(out, "overlaps") {
		t.Fatalf("a folder inside another: exit %d\n%s", code, out)
	}
	if out, code := e.Run("folder", "add", "extra", filepath.Join(e.Game, "Mods")); code == 0 || !strings.Contains(out, "overlaps") {
		t.Fatalf("a game's Mods/: exit %d\n%s", code, out)
	}
	scanNow(t, e)
	list := e.MustRun("folder", "list")
	if !strings.Contains(list, "src:workshop") || !strings.Contains(list, "src:extra") || !strings.Contains(list, "1 mods") {
		t.Fatalf("folder list:\n%s", list)
	}
	if out, code := e.Run("folder", "remove", "src:local"); code == 0 || !strings.Contains(out, "comes and goes with its environment") {
		t.Fatalf("removing a game's Mods/: exit %d\n%s", code, out)
	}
	e.MustRun("folder", "remove", "src:extra")
	sid := scanNow(t, e)
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvDisappeared: 1})

	// The Workshop folder moves (the Steam library moved): same instances.
	ws := refID(t, e, "Shared", source.Workshop)
	moved := filepath.Join(e.Root, "Library2", "steamapps", "workshop", "content", "294100")
	e.Move(e.Workshop, moved)
	if out := e.MustRun("folder", "add", "workshop", moved); !strings.Contains(out, "rebound") {
		t.Fatalf("a second Workshop folder replaces the first:\n%s", out)
	}
	scanNow(t, e)
	if refID(t, e, "Shared", source.Workshop) != ws {
		t.Fatal("the moved Workshop folder kept its instances")
	}
}

// 02 §0: a SteamCMD game keeps its own Workshop folder inside the game folder. It becomes a
// shared SteamCMD folder on env add (and, for an environment added earlier, on a scan); it
// never moves the Steam client's Workshop folder, and a removed one stays removed.
func TestGameWorkshopIsSteamCMD(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("5100", testkit.Sample("tester.shared", "Shared"))
	e.AddEnv(false)
	scanNow(t, e)
	wsRoot := sourcesOf(t, e)["workshop"].Root

	g2 := secondGame(e, "Game16")
	own := filepath.Join(g2, "steamapps", "workshop", "content", "294100")
	e.WriteMod(filepath.Join(own, "7000"), testkit.Sample("tester.cmd", "FromSteamCMD"))
	out := e.MustRun("env", "add", "old", "--game", g2, "--workshop", own)
	if !strings.Contains(out, "src:steamcmd") {
		t.Fatalf("env add: the game's own Workshop folder is a SteamCMD folder:\n%s", out)
	}
	if got := sourcesOf(t, e)["workshop"]; got == nil || got.Root != wsRoot {
		t.Fatalf("the Steam client's Workshop folder stays: %+v", got)
	}
	cmd := sourcesOf(t, e)["steamcmd"]
	if cmd == nil || cmd.EnvID != store.Shared || cmd.Root != own {
		t.Fatalf("a shared steamcmd folder for %s: %+v", own, cmd)
	}

	// An environment added before its game had a Workshop folder: the next scan adds it.
	g3 := secondGame(e, "Game16b")
	e.MustRun("env", "add", "third", "--game", g3)
	own3 := filepath.Join(g3, "steamapps", "workshop", "content", "294100")
	e.WriteMod(filepath.Join(own3, "7001"), testkit.Sample("tester.cmd2", "Later"))
	scanNow(t, e)
	var label string
	for l, s := range sourcesOf(t, e) {
		if s.Root == own3 {
			label = l
		}
	}
	if label == "" || sourcesOf(t, e)[label].Kind != source.SteamCMD {
		t.Fatalf("a scan registers the game's Workshop folder as steamcmd: %v", sourcesOf(t, e))
	}
	if got := sourcesOf(t, e)["workshop"]; got == nil || got.Root != wsRoot {
		t.Fatalf("the Steam client's Workshop folder stays: %+v", got)
	}
	e.MustRun("folder", "remove", "src:"+label)
	scanNow(t, e)
	for _, s := range sourcesOf(t, e) {
		if s.Root == own3 && s.Status != source.StatusRetired {
			t.Fatalf("a removed folder stays removed: %+v", s)
		}
	}

	// Removing the game and adding it again brings its own folder back as itself: the
	// same source, the same mod ID (2026-10-25).
	later := refID(t, e, "Later", "")
	e.MustRun("env", "use", "main")
	e.MustRun("env", "remove", "third", "--yes")
	out = e.MustRun("env", "add", "third", "--game", g3)
	if !strings.Contains(out, "src:steamcmd") {
		t.Fatalf("env add again: the game's own folder comes back:\n%s", out)
	}
	var back *source.Source
	for _, s := range sourcesOf(t, e) {
		if s.Root == own3 {
			back = s
		}
	}
	if back == nil || back.Status != source.StatusOK || back.Kind != source.SteamCMD {
		t.Fatalf("the re-added game's folder: %+v", back)
	}
	e.MustRun("env", "use", "third")
	scanNow(t, e)
	if got := refID(t, e, "Later", ""); got != later {
		t.Fatalf("the folder's mod came back as a new instance: %s, was %s", got, later)
	}
}

// 06 §2.14: removing an environment is refused while its game runs; removing the current
// one makes another current, and removing the last leaves none (the guide).
func TestEnvRemoveCurrent(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	g2 := secondGame(e, "Game15")
	e.MustRun("env", "add", "old", "--game", g2)
	scanNow(t, e)
	e.App.GameProbe = func(root string) *app.GameRun {
		if root == e.Game {
			return &app.GameRun{PID: 42, Started: e.Now()}
		}
		return nil
	}
	if out, code := e.Run("env", "remove", "main", "--yes"); code == 0 || !strings.Contains(out, "running") {
		t.Fatalf("removing a running game's environment is refused: exit %d\n%s", code, out)
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	r, err := e.App.EnvRemovePreview("main")
	if err != nil || !r.Current || r.Next != "old" || r.Mods == 0 {
		t.Fatalf("dry run: %+v %v", r, err)
	}
	e.MustRun("env", "remove", "main", "--yes")
	if cur, _ := e.App.Store.CurrentEnv(); cur == nil || cur.Name != "old" {
		t.Fatalf("the other environment becomes current: %+v", cur)
	}
	out := e.MustRun("env", "remove", "old", "--yes")
	if !strings.Contains(out, "none left") {
		t.Fatalf("the last environment: none left\n%s", out)
	}
	if cur, _ := e.App.Store.CurrentEnv(); cur != nil {
		t.Fatalf("no current environment: %+v", cur)
	}
}
