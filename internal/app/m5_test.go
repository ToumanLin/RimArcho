package app_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rimarcho/rimarcho/internal/testkit"
)

// U10 / 04 §6, and the inspector's test_copy_local_creates_clean_independent_git_baseline.
func TestCopyLocal(t *testing.T) {
	e := testkit.New(t)
	ws := e.AddWorkshop("123456", testkit.Sample("example.mod", "Example Mod"))
	e.WriteFile(filepath.Join(ws, ".gitignore"), "ignored.txt\n")
	e.WriteFile(filepath.Join(ws, "ignored.txt"), "must be in baseline\n")
	e.WriteFile(filepath.Join(ws, ".git", "upstream-marker"), "do not copy\n")
	outside := filepath.Join(e.Root, "elsewhere")
	e.WriteFile(filepath.Join(outside, "secret.txt"), "outside the mod\n")
	e.Junction(filepath.Join(ws, "Textures", "External"), outside)
	e.Junction(filepath.Join(ws, "DefsAlias"), filepath.Join(ws, "Defs"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "example.mod")
	e.AddEnv(false)
	e.MustRun("scan")
	before := e.ReadModsConfig()
	orig := ref(t, e, "Example Mod")
	out := e.MustRun("copy-local", orig, "--git")
	dst := filepath.Join(e.Game, "Mods", "123456_local")
	for _, want := range []string{"copied to", "git: repository with a baseline commit", "skipped: 1 links", "Textures/External", "loadout: unchanged", "archo swap " + orig + " #"} {
		if !strings.Contains(e.Scrub(out), want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, ".git", "upstream-marker")); err == nil {
		t.Fatal("upstream .git copied")
	}
	if _, err := os.Stat(filepath.Join(dst, "Textures", "External")); err == nil {
		t.Fatal("followed a link outside the mod folder")
	}
	if _, err := os.Stat(filepath.Join(dst, "DefsAlias", "ThingDefs", "examplemod.xml")); err != nil {
		t.Fatal("a link inside the mod folder is copied as content")
	}
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dst
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, b)
		}
		return strings.TrimSpace(string(b))
	}
	if git("ls-files", "ignored.txt") != "ignored.txt" || git("ls-files", ".rimarcho/origin.json") == "" || git("status", "--short") != "" {
		t.Fatal("baseline must hold every file, ignored ones and the origin marker included, and leave a clean tree")
	}
	var o map[string]string
	b, _ := os.ReadFile(filepath.Join(dst, ".rimarcho", "origin.json"))
	if err := json.Unmarshal(b, &o); err != nil || !strings.HasPrefix(o["origin_instance"], strings.TrimPrefix(orig, "#")) || o["workshop_id"] != "123456" || o["origin_lineage"] == "" || o["origin_snapshot"] == "" || o["copied_at"] == "" {
		t.Fatalf("origin.json: %s", b)
	}
	if e.ReadModsConfig() != before {
		t.Fatal("ModsConfig.xml changed")
	}
	if st := e.MustRun("status"); !strings.Contains(st, "same as") && !strings.Contains(st, "0 changes") && strings.Contains(st, "unsaved") {
		t.Fatalf("workspace changed:\n%s", st)
	}
	ix, _ := e.App.Index()
	var lin []string
	for _, m := range ix.Mods {
		if m.Name == "Example Mod" {
			lin = append(lin, m.LineageID)
		}
	}
	if len(lin) != 2 || lin[0] != lin[1] {
		t.Fatalf("copy joins the origin's lineage: %v", lin)
	}
	if _, code := e.Run("copy-local", orig, "--git"); code != 1 {
		t.Fatal("an existing folder must not be overwritten")
	}
	if _, code := e.Run("copy-local", orig, "--name", `..\up`); code != 1 {
		t.Fatal("--name must be a plain folder name")
	}
}

// rimcrowDB writes a RimCrow-shaped database (the tables import rimcrow reads).
func rimcrowDB(t *testing.T, path string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema := []string{
		`CREATE TABLE modasset (path_hash VARCHAR(255) PRIMARY KEY, package_id VARCHAR(255) NOT NULL, path VARCHAR(255) NOT NULL, source VARCHAR(255) NOT NULL, disabled INTEGER NOT NULL)`,
		`CREATE TABLE modinterlock (id VARCHAR(255) PRIMARY KEY, chain TEXT NOT NULL)`,
		`CREATE TABLE usermoddata (mod_id VARCHAR(255) NOT NULL PRIMARY KEY COLLATE NOCASE, alias_name VARCHAR(255), notes TEXT, tags TEXT NOT NULL, sign_color VARCHAR(255), user_mod_type VARCHAR(255), interlock_id VARCHAR(255), ignored_issues TEXT NOT NULL)`,
		`CREATE TABLE groupdata (group_id TEXT PRIMARY KEY, name VARCHAR(255) NOT NULL, color VARCHAR(255) NOT NULL, sort_index INTEGER NOT NULL, is_expanded INTEGER NOT NULL)`,
		`CREATE TABLE groupmod (group_id VARCHAR(255) NOT NULL, mod_id VARCHAR(255) NOT NULL, sort_index INTEGER NOT NULL, PRIMARY KEY (group_id, mod_id))`,
	}
	for _, s := range append(schema, stmts...) {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// U11 / 04 §7.2: path first, packageId second, ambiguous and missing reported, conflicts
// kept, rules mapped, idempotent, and the RimCrow files are never written.
func TestImportRimCrow(t *testing.T) {
	e := testkit.New(t)
	alpha := e.AddWorkshop("3000000002", testkit.Sample("tester.alpha", "Alpha"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "AlphaFork"), testkit.Sample("tester.alpha", "Alpha Fork")) // second lineage, same package
	e.AddWorkshop("3000000003", testkit.Sample("tester.beta", "Beta"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Gamma"), testkit.Sample("tester.gamma", "Gamma"))
	off := filepath.Join(e.Workshop, "3000000004")
	e.Subscribe("3000000004")
	e.WriteFile(filepath.Join(off, "About", "About.xml.disabled"), testkit.Mod{PackageID: "tester.off", Name: "Switched Off"}.AboutXML())
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.alpha", "tester.beta")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("alias", ref(t, e, "Gamma"), "My Gamma")

	crow := filepath.Join(e.Root, "RimCrow", "data")
	os.MkdirAll(filepath.Join(crow, "rules"), 0o755)
	dbPath := filepath.Join(crow, "mod_manager.db")
	rimcrowDB(t, dbPath,
		`INSERT INTO modasset VALUES ('h1','tester.alpha','`+alpha+`','workshop',0), ('h2','tester.beta','C:\gone\Beta','workshop',0), ('h3','tester.off','`+off+`','workshop',1)`,
		`INSERT INTO usermoddata VALUES ('Tester.Alpha','Alpha (main)','keep this one','["combat"]','#ff0000',NULL,'ia','[]'),
			('tester.beta',NULL,NULL,'[]',NULL,NULL,'ia','[]'), ('tester.gamma','Gamma Prime',NULL,'[]',NULL,NULL,NULL,'[]'),
			('tester.missing','Ghost',NULL,'[]',NULL,NULL,NULL,'[]')`,
		`INSERT INTO modinterlock VALUES ('ia','["tester.alpha","tester.beta"]')`,
		`INSERT INTO groupdata VALUES ('g1','Weapons','#fff',0,1), ('g2','UI','#fff',1,1)`,
		`INSERT INTO groupmod VALUES ('g1','tester.alpha',0), ('g1','tester.beta',1), ('g2','tester.beta',0)`)
	os.WriteFile(filepath.Join(crow, "rules", "user_rules.json"), []byte(`{"settings": {"user_mod_rules_enabled": true},
		"mod_rules": {"tester.beta": {"loadAfter": {"tester.gamma": {"name": ["Gamma"]}}, "loadBottom": {"value": true}},
		              "tester.gamma": {"incompatibleWith": {"tester.nowhere": {}}}}}`), 0o644)
	sum := func() string {
		b, _ := os.ReadFile(dbPath)
		r, _ := os.ReadFile(filepath.Join(crow, "rules", "user_rules.json"))
		return string(b) + string(r)
	}
	before := sum()

	dry := e.MustRun("import", "rimcrow", "--db", dbPath, "--dry-run")
	for _, want := range []string{
		"mods: 4 with data: 3 mapped (1 by path, 2 by packageId), 1 not mapped",
		"aliases: 1 new, 0 already set", "notes: 1 new", "tags: 4 new, 0 already present (2 groups, 3 memberships)",
		"rules: 3: 2 new, 0 already present, 1 skipped", "conflicts: 1", "pkg:tester.gamma  alias: RimArcho has \"My Gamma\", RimCrow \"Gamma Prime\"",
		"pkg:tester.missing  not installed here", "tester.gamma incompatibleWith tester.nowhere  pkg:tester.nowhere not installed here",
		"not-mapped: rule loadBottom 1, sign_color 1", "about-disabled: 1 folders", "Switched Off",
	} {
		if !strings.Contains(dry, want) {
			t.Fatalf("dry run lacks %q:\n%s", want, dry)
		}
	}
	if out := e.MustRun("tag", "list"); !strings.Contains(out, "tags: 0") {
		t.Fatalf("dry run wrote tags:\n%s", out)
	}
	// Alpha's package has two lineages; the path picks the workshop one.
	first := e.MustRun("import", "rimcrow", "--db", dbPath)
	if !strings.Contains(first, "tags: 4 new") {
		t.Fatalf("import:\n%s", first)
	}
	if out := e.MustRun("mod", "ws:3000000002"); !strings.Contains(out, "alias: Alpha (main) (About.xml name: Alpha)") || !strings.Contains(out, "tags: combat, Weapons") || !strings.Contains(out, "note: keep this one") {
		t.Fatalf("alias/tags/note on the workshop Alpha:\n%s", out)
	}
	if out := e.MustRun("list", "--filter", "tag:ui"); !strings.Contains(out, "Beta") || strings.Contains(out, "Alpha") {
		t.Fatalf("tag filter:\n%s", out)
	}
	if out := e.MustRun("rule", "list"); !strings.Contains(out, "attach") || !strings.Contains(out, "after") {
		t.Fatalf("rules:\n%s", out)
	}
	again := e.MustRun("import", "rimcrow", "--db", dbPath)
	for _, want := range []string{"aliases: 0 new, 1 already set", "tags: 0 new, 4 already present", "rules: 3: 0 new, 2 already present"} {
		if !strings.Contains(again, want) {
			t.Fatalf("second import is not a no-op (%q):\n%s", want, again)
		}
	}
	if sum() != before {
		t.Fatal("the RimCrow database or rules file changed")
	}
	if ents, _ := filepath.Glob(filepath.Join(e.Home, "rimcrow-*")); len(ents) > 0 {
		t.Fatalf("temp copies left behind: %v", ents)
	}
}

// 05 §3: loadout export / import round-trips through a text list and a ModsConfig.xml,
// and reads a save's mod list; import is one undoable step and never writes the game file.
func TestLoadoutExchange(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("3000000002", testkit.Sample("tester.alpha", "Alpha"))
	e.AddWorkshop("3000000003", testkit.Sample("tester.beta", "Beta"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Gamma"), testkit.Sample("tester.gamma", "Gamma"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.alpha", "tester.beta")
	e.AddEnv(false)
	e.MustRun("scan")
	cfg := e.ReadModsConfig()
	text := e.MustRun("loadout", "export")
	if !strings.HasPrefix(text, "# RimArcho loadout") || !strings.Contains(text, "tester.beta  # Beta\n") {
		t.Fatalf("text export:\n%s", text)
	}
	dir := t.TempDir()
	tf := filepath.Join(dir, "list.txt")
	e.MustRun("loadout", "export", "--out", tf)
	if _, code := e.Run("loadout", "export", "--out", tf); code != 1 {
		t.Fatal("export overwrote a file")
	}
	mf := filepath.Join(dir, "ModsConfig.xml")
	e.MustRun("loadout", "export", "--format", "modsconfig", "--out", mf)

	// A save from another run: Gamma first, Beta, and a mod that is not installed.
	save := filepath.Join(dir, "Colony.rws")
	os.WriteFile(save, []byte(`<?xml version="1.0" encoding="utf-8"?>
<savegame><meta><gameVersion>1.6.4633 rev1261</gameVersion>
<modIds><li>ludeon.rimworld</li><li>tester.gamma</li><li>Tester.Beta</li><li>tester.gone</li></modIds>
<modSteamIds><li>0</li><li>0</li><li>3000000003</li><li>99</li></modSteamIds>
<modNames><li>Core</li><li>Gamma</li><li>Beta</li><li>Gone Mod</li></modNames></meta>
<game><huge>…</huge></game></savegame>`), 0o644)
	out := e.MustRun("loadout", "import", save)
	if !strings.Contains(out, "imported 4 entries from a save, game 1.6.4633 rev1261") || !strings.Contains(out, "not installed: 1") || !strings.Contains(out, "tester.gone (Gone Mod)") {
		t.Fatalf("save import:\n%s", out)
	}
	if e.ReadModsConfig() != cfg {
		t.Fatal("import wrote ModsConfig.xml")
	}
	e.MustRun("undo")
	if again := e.MustRun("loadout", "export"); again[strings.Index(again, "\nludeon"):] != text[strings.Index(text, "\nludeon"):] {
		t.Fatalf("undo did not restore the list:\n%s", again)
	}
	e.MustRun("loadout", "import", save)
	for _, f := range []string{tf, mf} {
		if out := e.MustRun("loadout", "import", f); !strings.Contains(out, "imported 3 entries") {
			t.Fatalf("%s:\n%s", f, out)
		}
		if again := e.MustRun("loadout", "export"); again[strings.Index(again, "\nludeon"):] != text[strings.Index(text, "\nludeon"):] {
			t.Fatalf("%s did not round-trip:\n%s", f, again)
		}
		e.MustRun("loadout", "import", save)
	}
}

// decisions-m5: the environment's analysis and one for the log's game version coexist;
// alternating problems and log does not re-run either.
func TestAnalysisPerGameVersion(t *testing.T) {
	e := testkit.New(t)
	log := e.HarmonyLogFixture()
	os.WriteFile(filepath.Join(e.UserData, "Player.log"), []byte(strings.Replace(log, "RimWorld 1.6.4633 rev1261", "RimWorld 1.5.4409 rev1118", 1)), 0o644)
	runs := 0
	inner := e.App.AnalyzeRunner
	e.App.AnalyzeRunner = func(env string, p func(string)) error { runs++; return inner(env, p) }
	e.MustRun("problems")
	if runs != 2 {
		t.Fatalf("first problems: %d analysis runs, want 2 (1.6 and the log's 1.5)", runs)
	}
	for i := 0; i < 3; i++ {
		e.MustRun("log", "--cluster", "E1")
		e.MustRun("problems")
		e.MustRun("harmony", "--target", "Verse.Pawn.Tick")
	}
	if runs != 2 {
		t.Fatalf("alternating commands re-ran the analysis: %d runs", runs)
	}
	if out := e.MustRun("harmony", "--target", "Verse.Pawn.Tick"); !strings.Contains(out, "4 patches from 3 mods") {
		t.Fatalf("facts of the environment's version:\n%s", out)
	}
	if out := e.MustRun("log", "--cluster", "E1"); !strings.Contains(out, "analysed: as 1.5") || !strings.Contains(out, "patch.no-match: target-inactive") {
		t.Fatalf("the 1.5 analysis is used for the 1.5 log:\n%s", out)
	}
}
