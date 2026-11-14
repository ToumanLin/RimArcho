package store_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/store"
)

func TestUpgradeSaveReviewCache(t *testing.T) {
	s, path := openTemp(t)
	legacyGameSchema(t, s)
	if err := s.PutSaveCache(&store.SaveCache{Path: "synthetic", Size: 1, MTime: 2, Parser: 3, Facts: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`DROP TABLE save_review_cache`, `UPDATE meta SET value='35' WHERE key='schema_version'`} {
		if _, err := s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	u, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	want := bytes.Repeat([]byte(`{"seen":["SyntheticDef"]}`), 20)
	if err := u.PutSaveReviewCache(&store.SaveReviewCache{Path: "synthetic", Size: 1, MTime: 2, Parser: 3, InputKey: "input", Result: want}); err != nil {
		t.Fatal(err)
	}
	got, err := u.SaveReviewCache("synthetic", 1, 2, 3, "input")
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("round trip: %s %v", got, err)
	}
	for _, c := range []struct {
		size, mtime int64
		parser      int
		key         string
	}{{2, 2, 3, "input"}, {1, 3, 3, "input"}, {1, 2, 4, "input"}, {1, 2, 3, "changed"}} {
		got, err := u.SaveReviewCache("synthetic", c.size, c.mtime, c.parser, c.key)
		if err != nil || got != nil {
			t.Fatalf("stale entry: %s %v", got, err)
		}
	}
	if _, err := u.DB.Exec(`UPDATE save_review_cache SET result=x'00'`); err != nil {
		t.Fatal(err)
	}
	if got, err := u.SaveReviewCache("synthetic", 1, 2, 3, "input"); got != nil || err != nil {
		t.Fatalf("corrupt entry: %s %v", got, err)
	}
	if err := u.DeleteSaveCache([]string{"synthetic"}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := u.DB.QueryRow(`SELECT count(*) FROM save_review_cache`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("orphan: %d %v", n, err)
	}
}

func TestUpgradeSharedReviewCatalog(t *testing.T) {
	s, path := openTemp(t)
	legacyGameSchema(t, s)
	if err := s.PutSaveCache(&store.SaveCache{Path: "legacy", Size: 1, MTime: 2, Parser: 3, Facts: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutSaveReviewCache(&store.SaveReviewCache{Path: "legacy", Size: 1, MTime: 2, Parser: 3, InputKey: "old-format", Result: []byte(`{"Catalog":{}}`)}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`DROP INDEX save_review_catalog_ref`, `ALTER TABLE save_review_cache DROP COLUMN catalog_key`, `DROP TABLE save_review_catalog`, `UPDATE meta SET value='36' WHERE key='schema_version'`} {
		if _, err := s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	u, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	if err := u.PruneSaveReviewFormat("review2:"); err != nil {
		t.Fatal(err)
	}
	if c, err := u.SaveFacts("legacy"); err != nil || c == nil || string(c.Facts) != "{}" {
		t.Fatalf("migration lost full facts: %+v %v", c, err)
	}
	var n int
	if err := u.DB.QueryRow(`SELECT count(*) FROM save_review_cache`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("legacy catalog format retained: %d %v", n, err)
	}
}

func TestSharedReviewCatalogLifecycle(t *testing.T) {
	s, _ := openTemp(t)
	put := func(path string, catalog []byte, key string) error {
		if err := s.PutSaveCache(&store.SaveCache{Path: path, Size: 1, MTime: 2, Parser: 3, Facts: []byte(`{}`)}); err != nil {
			return err
		}
		return s.PutSaveReviewCache(&store.SaveReviewCache{Path: path, Size: 1, MTime: 2, Parser: 3, InputKey: "review2:input", Result: []byte(`{}`), CatalogKey: key, Catalog: catalog})
	}
	first := []byte(`{"Names":{"copy":{"items":["Shared"]}}}`)
	firstKey := fmt.Sprintf("%x", sha256.Sum256(first))
	second := []byte(`{"Names":{"copy":{"items":["Changed"]}}}`)
	secondKey := fmt.Sprintf("%x", sha256.Sum256(second))
	count := func(want int) {
		t.Helper()
		var n int
		if err := s.DB.QueryRow(`SELECT count(*) FROM save_review_catalog`).Scan(&n); err != nil || n != want {
			t.Fatalf("catalog count: %d want %d, %v", n, want, err)
		}
	}
	if err := put("a", first, firstKey); err != nil {
		t.Fatal(err)
	}
	if err := put("b", nil, firstKey); err != nil {
		t.Fatal(err)
	}
	count(1)
	if got, err := s.SaveReviewCatalog(firstKey); err != nil || !bytes.Equal(got, first) {
		t.Fatalf("round trip: %s %v", got, err)
	}
	if err := put("a", second, secondKey); err != nil {
		t.Fatal(err)
	}
	count(2) // b still needs the original catalog
	if err := s.DeleteSaveCache([]string{"b"}); err != nil {
		t.Fatal(err)
	}
	count(1)
	if _, err := s.DB.Exec(`UPDATE save_review_catalog SET catalog=x'00'`); err != nil {
		t.Fatal(err)
	}
	if got, err := s.SaveReviewCatalog(secondKey); got != nil || err != nil {
		t.Fatalf("corrupt catalog accepted: %s %v", got, err)
	}
	if err := put("a", second, secondKey); err != nil {
		t.Fatal(err)
	} // repair a corrupt row
	if got, err := s.SaveReviewCatalog(secondKey); err != nil || !bytes.Equal(got, second) {
		t.Fatalf("repair: %s %v", got, err)
	}
	if err := put("a", first, secondKey); err == nil {
		t.Fatal("wrong content address accepted")
	}
	count(1)
	// A failed save insertion rolls back its new catalog too.
	if err := s.PutSaveReviewCache(&store.SaveReviewCache{Path: "no-parent", InputKey: "review2:input", Result: []byte(`{}`), CatalogKey: firstKey, Catalog: first}); err == nil {
		t.Fatal("missing parent accepted")
	}
	count(1)
	if err := s.PruneSaveReviewFormat("review3:"); err != nil {
		t.Fatal(err)
	}
	count(0)
}
