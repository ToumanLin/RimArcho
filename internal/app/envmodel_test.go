package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
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
// data in both environments; Data/ and Mods/ are each game's own; plans stay per game.
func TestEnvModelSharedWorkshop(t *testing.T) {
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
	// Plans: per game; the first game's plan still uses the shared mod.
	e.MustRun("env", "use", "main")
	st, _ := e.App.Loadout()
	if st.List.Index(ws) < 0 {
		t.Fatal("main's plan lost the shared mod")
	}
	// Mod 库存: "用在" names the other game's plans too.
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
	if !slices.Contains(uses, "default") || !slices.Contains(uses, "old/default") {
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

// 02 §0.1: a database from before the shared folders, with a second environment that had
// its own copy of the Workshop folder: after the migration every plan keeps its list and
// order, the undo stack works, a pack made before still launches and is counted as using
// the mod, and a locked copy (whose origin.json names IDs merged away) is matched by R5.
func TestMigrateV14App(t *testing.T) {
	e, _ := pinFixture(t)
	e.MustRun("env", "set", "main", "--launch", "direct")
	up := refID(t, e, "Lockable", source.Workshop)
	e.MustRun("pack", "build", "before")
	e.MustRun("pin", refOf(t, e, "Lockable", ""))
	cp := lockedAt(t, e, "5100_local")
	stBefore, _ := e.App.Loadout()
	planBefore := folders(t, e, stBefore.List)

	twin := downgradeWithTwin(t, e)
	e.Open() // migration v14

	ix, _ := e.App.Index()
	if ix.ByID[up] != nil {
		t.Fatal("the merged-away Workshop instance is still there")
	}
	newUp := twin[up]
	w := ix.ByID[newUp]
	c := ix.ByID[cp]
	if w == nil || c == nil || c.LineageID != w.LineageID {
		t.Fatalf("the locked copy and its upstream share the lineage kept: up %+v copy %+v", w, c)
	}
	st, _ := e.App.Loadout()
	if got := folders(t, e, st.List); !slices.Equal(got, planBefore) {
		t.Fatalf("main's plan changed:\n%v\n%v", planBefore, got)
	}
	// Undo: the pin's swap is undone onto the instance kept.
	e.MustRun("undo")
	if st, _ := e.App.Loadout(); st.List.Index(newUp) < 0 || st.List.Index(cp) >= 0 {
		t.Fatal("undo after the migration must bring the upstream back")
	}
	e.MustRun("redo")

	// The pack names the old ID: it launches, and counts as using the mod kept.
	e.MustRun("pack", "launch", "before", "--dry-run")
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	l, err := lib.Lineage("L#" + w.LineageID[:6])
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range l.UsedIn() {
		found = found || (u.Kind == "pack" && u.Name == "before")
	}
	if !found {
		t.Fatalf("the old pack's use: %+v", l.UsedIn())
	}

	// A scan: nothing changes; a second copy of the locked folder is a confirmed copy in the
	// lineage kept (R5 through id_alias).
	sid := scanNow(t, e)
	if evs := eventsOf(t, e, sid); len(evs) != 0 {
		t.Fatalf("the first scan after the migration changed something: %v", evs)
	}
	lockDir := filepath.Join(e.Root, "LocalAppData", "RimArcho", "locked")
	legacy := filepath.Join(lockDir, "main")
	e.CopyTree(filepath.Join(legacy, "5100_local"), filepath.Join(legacy, "5100_again"))
	sid = scanNow(t, e)
	ix, _ = e.App.Index()
	m := ix.ModAt(filepath.Join(legacy, "5100_again"))
	if m == nil || m.LineageID != w.LineageID || m.LineageBasis != identity.BasisConfirmed {
		t.Fatalf("R5 after the migration: %+v (events %v)", m, eventsOf(t, e, sid))
	}
	// The older per-environment folder still counts as the lock library.
	if s := sourcesOf(t, e)["locked-main"]; s == nil || !e.App.IsLockSource(s) {
		t.Fatalf("legacy lock folder: %+v", sourcesOf(t, e))
	}
}

func folders(t *testing.T, e *testkit.Env, l loadout.List) []string {
	t.Helper()
	ix, _ := e.App.Index()
	var out []string
	for _, x := range l {
		if m := ix.ByID[x.Instance]; m != nil {
			out = append(out, string(m.Kind)+"/"+m.Rel)
		} else {
			out = append(out, "?"+x.Key())
		}
	}
	return out
}

// downgradeWithTwin turns e's database into what schema v13 would have held with a second
// environment "two" that had its own copy of every shared folder (except the lock library):
// "two"'s instances are older, so they are the ones kept. The lock library goes back to
// <lock.dir>/main. It returns the map from main's shared instances to their twins.
func downgradeWithTwin(t *testing.T, e *testkit.Env) map[string]string {
	t.Helper()
	db := e.App.Store.DB
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	envID := ""
	db.QueryRow(`SELECT id FROM environment WHERE name='main'`).Scan(&envID)
	g2 := secondGame(e, "Game15")
	two := e.App.NewID()
	exec(`INSERT INTO environment(id, name, game_root, user_data_root, game_version, launch, created_at) VALUES(?, 'two', ?, ?, '1.6', 'direct', 1)`, two, g2, e.UserData)
	exec(`INSERT INTO source(id, environment_id, kind, label, root, status) VALUES(?, ?, 'official', 'official', ?, 'ok'), (?, ?, 'local', 'local', ?, 'ok')`,
		e.App.NewID(), two, filepath.Join(g2, "Data"), e.App.NewID(), two, filepath.Join(g2, "Mods"))

	// The lock library as it was: <lock.dir>/main, a source of main.
	lockDir := filepath.Join(e.Root, "LocalAppData", "RimArcho", "locked")
	legacy := filepath.Join(lockDir, "main")
	ents, _ := os.ReadDir(lockDir)
	e.MkdirAll(legacy)
	for _, d := range ents {
		e.Move(filepath.Join(lockDir, d.Name()), filepath.Join(legacy, d.Name()))
	}
	exec(`UPDATE source SET root=? WHERE label='locked'`, legacy)
	exec(`UPDATE instance SET real_path=replace(real_path, ?, ?) WHERE source_id IN (SELECT id FROM source WHERE label='locked')`, lockDir, legacy)

	type row struct{ id, kind, label, root string }
	var shared []row
	rows, _ := db.Query(`SELECT id, kind, label, root FROM source WHERE environment_id=''`)
	for rows.Next() {
		var r row
		rows.Scan(&r.id, &r.kind, &r.label, &r.root)
		shared = append(shared, r)
	}
	rows.Close()
	twin := map[string]string{}
	twinLin := map[string]string{}
	for _, s := range shared {
		if s.label == app.LockLabel {
			continue
		}
		ts := e.App.NewID()
		exec(`INSERT INTO source(id, environment_id, kind, label, root, status) VALUES(?, ?, ?, ?, ?, 'ok')`, ts, two, s.kind, s.label, s.root)
		type inst struct {
			id, lin, rel, real, status string
			snap                       int64
			first                      int64
		}
		var is []inst
		rows, _ := db.Query(`SELECT id, lineage_id, rel_path, real_path, status, coalesce(current_snapshot_id, 0), first_seen FROM instance WHERE source_id=?`, s.id)
		for rows.Next() {
			var x inst
			rows.Scan(&x.id, &x.lin, &x.rel, &x.real, &x.status, &x.snap, &x.first)
			is = append(is, x)
		}
		rows.Close()
		for _, x := range is {
			if twinLin[x.lin] == "" {
				twinLin[x.lin] = e.App.NewID()
				exec(`INSERT INTO lineage(id, environment_id, primary_instance_id, created_at) SELECT ?, ?, '', created_at-10 FROM lineage WHERE id=?`, twinLin[x.lin], two, x.lin)
			}
			n := e.App.NewID()
			twin[x.id] = n
			res, err := db.Exec(`INSERT INTO snapshot(instance_id, scan_id, snapshot_hash, package_id, package_id_raw, name, authors, workshop_id_declared,
				supported_versions, decl_blob, acf_manifest, acf_time_updated, acf_size, git_head, origin, file_count, total_size, semantic_count)
				SELECT ?, scan_id, snapshot_hash, package_id, package_id_raw, name, authors, workshop_id_declared, supported_versions, decl_blob,
				acf_manifest, acf_time_updated, acf_size, git_head, origin, file_count, total_size, semantic_count FROM snapshot WHERE id=?`, n, x.snap)
			if err != nil {
				t.Fatal(err)
			}
			sid, _ := res.LastInsertId()
			exec(`INSERT INTO snapshot_file(snapshot_id, rel_path, size, mtime, hash) SELECT ?, rel_path, size, mtime, hash FROM snapshot_file WHERE snapshot_id=?`, sid, x.snap)
			exec(`INSERT INTO instance(id, environment_id, lineage_id, source_id, rel_path, real_path, status, current_snapshot_id, first_seen, last_seen)
				VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, n, two, twinLin[x.lin], ts, x.rel, x.real, x.status, sid, x.first-10, x.first)
		}
	}
	exec(`UPDATE source SET environment_id=? WHERE environment_id=''`, envID)
	exec(`UPDATE instance SET environment_id=? WHERE environment_id=''`, envID)
	exec(`UPDATE lineage SET environment_id=? WHERE environment_id=''`, envID)
	exec(`UPDATE lineage_data SET environment_id=?`, envID)
	exec(`UPDATE lineage_tag SET environment_id=?`, envID)
	exec(`UPDATE user_rule SET environment_id=?`, envID)
	exec(`INSERT INTO plan(environment_id, name, created_at, updated_at) VALUES(?, 'default', 1, 1)`, two)
	exec(`DROP TABLE id_alias`)
	exec(`DROP TABLE texture_file`) // v15
	exec(`DROP TABLE texture_mod`)
	exec(`UPDATE meta SET value='13' WHERE key='schema_version'`)
	return twin
}
