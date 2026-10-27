package main

import (
	"database/sql"
	"encoding/json"
	"github.com/ToumanLin/RimArcho/internal/app"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixtureCanonicalAndReadOnly(t *testing.T) {
	root := t.TempDir()
	left, right := filepath.Join(root, "left"), filepath.Join(root, "right")
	fixture(left)
	fixture(right)
	a, err := app.Open(filepath.Join(left, "home"))
	if err != nil {
		t.Fatal(err)
	}
	sorted, err := a.Sort(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(sorted.MissingDependencies) != 3 || len(sorted.MissingMods) != 2 || len(sorted.Replacements) != 1 || !sorted.Replacements[0].CanReplace {
		t.Fatalf("fixture does not cover notices: %+v", sorted.SortNotices)
	}
	if !sorted.MissingMods[1].CanEnable {
		t.Fatal("installed-copy case missing")
	}
	a.Close()
	db := filepath.Join(left, "home", "archo.db")
	before, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	observed := snapshot(filepath.Join(left, "home")).(map[string]any)
	instances := map[string]bool{}
	for _, m := range observed["instances"].([]map[string]any) {
		instances[m["id"].(string)] = true
	}
	owners := observed["problems"].(map[string]any)["mods"].(map[string]any)
	if len(owners) == 0 {
		t.Fatal("fixture must contain per-instance problems")
	}
	for id := range owners {
		if !instances[id] {
			t.Fatalf("problem ownership key was not canonicalized: %s", id)
		}
	}
	l, err := json.Marshal(observed)
	if err != nil {
		t.Fatal(err)
	}
	r, err := json.Marshal(snapshot(filepath.Join(right, "home")))
	if err != nil {
		t.Fatal(err)
	}
	if string(l) != string(r) {
		t.Fatal("canonical snapshots depend on root or random identities")
	}
	after, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("observer wrote database")
	}
}

func TestSnapshotLegacyHistoryReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "fixture")
	fixture(root)
	home := filepath.Join(root, "home")
	path := filepath.Join(home, "archo.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE workspace_undo DROP COLUMN staging; UPDATE meta SET value='20' WHERE key='schema_version'`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	observed := snapshot(home).(map[string]any)
	if len(observed["plans"].([]map[string]any)) != 3 {
		t.Fatal("legacy observation lost plans")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("legacy observer migrated or wrote DB")
	}
}

func TestScaleFixture(t *testing.T) {
	root := filepath.Join(t.TempDir(), "scale")
	fixtureSized(root, true)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(root, "home", "archo.db"))+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM instance WHERE status='live'").Scan(&count); err != nil || count != 4000 {
		t.Fatalf("installed=%d: %v", count, err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM environment").Scan(&count); err != nil || count != 2 {
		t.Fatalf("games=%d: %v", count, err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM plan").Scan(&count); err != nil || count != 3 {
		t.Fatalf("lists=%d: %v", count, err)
	}
	marker, err := os.ReadFile(filepath.Join(root, "isolated.json"))
	if err != nil || !strings.Contains(string(marker), `"synthetic":true`) {
		t.Fatalf("synthetic marker: %s: %v", marker, err)
	}
	var entries string
	if err := db.QueryRow("SELECT w.entries FROM workspace w JOIN plan p ON p.id=w.plan_id WHERE p.name='default'").Scan(&entries); err != nil {
		t.Fatal(err)
	}
	var list []map[string]any
	if err := json.Unmarshal([]byte(entries), &list); err != nil || len(list) != 481 {
		t.Fatalf("default=%d: %v", len(list), err)
	}
	for _, e := range list {
		if e["i"] == nil || e["i"] == "" {
			t.Fatal("unbound installed entry")
		}
	}
	audit(filepath.Join(root, "home"), root)
}
