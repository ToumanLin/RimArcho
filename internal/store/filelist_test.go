package store

import (
	"database/sql"
	"reflect"
	"testing"
)

func TestFileListCodec(t *testing.T) {
	es := []listEntry{{"", 0}, {"A", 1}, {"Textures/中文/图.png", 1 << 40}, {"Textures/中文/图2.png", 3}, {"z", 0}}
	data, err := encodeList(es)
	if err != nil {
		t.Fatal(err)
	}
	var got []listEntry
	if err := decodeList(data, func(rel string, size int64) { got = append(got, listEntry{rel, size}) }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, es) {
		t.Fatalf("%v", got)
	}
	if err := decodeList(data[:len(data)/2], func(string, int64) {}); err == nil {
		t.Fatal("a cut block decoded")
	}
}

// likeMatch is SQLite's LIKE: % and _, ASCII letters without case, other letters exact.
func TestLikeMatchesSQLite(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	subjects := []string{"", "a", "A", "Textures/a.png", "textures/A.PNG", "x/Textures/b/c.png", "Ü.png", "ü.png", "中文/图.png", "a_b.dll", "aXb.dll",
		"100%.txt", "Languages/English/Keyed/A.xml", "Languages//Keyed/x", "abcabc", "ab", "mod.DLL"}
	patterns := []string{"", "%", "_", "a", "%Textures/%", "%.png", "%a_b%", "a%b%c", "%abc", "abc%", "%Ü%", "%ü%", "_.png", "__", "%中文%", "%100%%",
		"Languages/_%/Keyed/%", "%%", "%_%", "%.dll", "%mod.dll", "a%"}
	for _, p := range patterns {
		for _, s := range subjects {
			var want bool
			if err := db.QueryRow(`SELECT ? LIKE ?`, s, p).Scan(&want); err != nil {
				t.Fatal(err)
			}
			if got := likeMatch(p, s); got != want {
				t.Errorf("%q LIKE %q = %v, SQLite says %v", s, p, got, want)
			}
		}
	}
}
