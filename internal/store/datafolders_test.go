package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/store"
)

// openAt41 makes a home whose database is at schema 41, with the given data-directory
// folders, and opens it again so step 42 runs (17 §3).
func openAt41(t *testing.T, folders ...string) (string, *store.Store) {
	t.Helper()
	home := t.TempDir()
	db := filepath.Join(home, "archo.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta("schema_version", "41"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	for _, f := range folders {
		if err := os.MkdirAll(filepath.Join(home, f), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	u, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { u.Close() })
	return home, u
}

// An upgraded home keeps the packs and source library it had in the data directory.
func TestUpgradeKeepsDataFolders(t *testing.T) {
	t.Parallel()
	home, u := openAt41(t, "packs", "source")
	for _, c := range []struct{ id, key string }{{"packs", "pack.dir"}, {"source", "source.dir"}} {
		want := filepath.Join(home, c.id)
		if got := u.Meta("cfg:" + c.key); got != want {
			t.Fatalf("cfg:%s = %q, want the old folder %q", c.key, got, want)
		}
		if got := u.Meta("directory.used:" + c.id); got != want {
			t.Fatalf("directory.used:%s = %q, want %q", c.id, got, want)
		}
	}
}

// Without the old folders an upgraded home takes the new defaults; a folder the player set
// is left as it is.
func TestUpgradeWithoutDataFolders(t *testing.T) {
	t.Parallel()
	_, u := openAt41(t)
	if u.Meta("cfg:pack.dir") != "" || u.Meta("cfg:source.dir") != "" {
		t.Fatalf("a home without the old folders got pinned: %q %q", u.Meta("cfg:pack.dir"), u.Meta("cfg:source.dir"))
	}
	home := t.TempDir()
	db := filepath.Join(home, "archo.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	chosen := filepath.Join(t.TempDir(), "my packs")
	s.SetMeta("cfg:pack.dir", chosen)
	s.SetMeta("schema_version", "41")
	s.Close()
	os.MkdirAll(filepath.Join(home, "packs"), 0o755)
	u, err = store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	if got := u.Meta("cfg:pack.dir"); got != chosen {
		t.Fatalf("the player's pack folder became %q", got)
	}
}
