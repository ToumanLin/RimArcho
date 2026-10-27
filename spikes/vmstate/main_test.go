package main

import (
	"encoding/json"
	"github.com/ToumanLin/RimArcho/internal/app"
	"os"
	"path/filepath"
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
	l, err := json.Marshal(snapshot(filepath.Join(left, "home")))
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
