package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// RimSort's local Mods folders come from settings.json: the legacy top-level one and each
// instance's; missing folders, relative paths and repeats are left out (07 §6.2).
func TestManagerModFoldersReadsRimSortSettings(t *testing.T) {
	root := t.TempDir()
	rs := filepath.Join(root, "RimSort")
	local, other := filepath.Join(root, "D-Mods"), filepath.Join(root, "More Mods")
	for _, d := range []string{rs, filepath.Join(local, "ExtraPack"), other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	js := func(p string) string { b, _ := json.Marshal(p); return strings.Trim(string(b), `"`) }
	settings := "\xef\xbb\xbf" + `{"local_folder": "` + js(local) + `", "instances": {
		"Default": {"local_folder": "` + js(local) + `", "game_folder": "x"},
		"Second": {"local_folder": "` + js(other) + `"},
		"Gone": {"local_folder": "` + js(filepath.Join(root, "gone")) + `"},
		"Relative": {"local_folder": "mods"}}}`
	if err := os.WriteFile(filepath.Join(rs, "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ManagerModFolders([]string{rs, filepath.Join(root, "missing")})
	want := []ManagerFolder{{"RimSort", local}, {"RimSort", other}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	// a broken settings file is no folder, not an error
	os.WriteFile(filepath.Join(rs, "settings.json"), []byte("{"), 0o644)
	if got := ManagerModFolders([]string{rs}); len(got) != 0 {
		t.Fatalf("broken: %+v", got)
	}
}
