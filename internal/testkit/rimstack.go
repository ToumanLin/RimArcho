package testkit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// RimStackNote is one Mod's entry in RimStack's notes.json, as RimStack writes it: tags and
// note always, the others only when set.
type RimStackNote struct {
	Tags      []string `json:"tags"`
	Note      string   `json:"note"`
	VersionOK bool     `json:"versionOk,omitempty"`
	Category  string   `json:"category,omitempty"`
	Ignored   []string `json:"ignored,omitempty"`
	Sub       string   `json:"sub,omitempty"` // an older RimStack's group
}

// RimStackData writes a synthetic RimStack data folder (07 §6.6) at dir: notes.json keyed by
// packageId, rules.json as {first, then} pairs, and saved lists under lists/ and snapshots/
// ({name, ids}, the file named after the list). A nil part is not written.
func RimStackData(t testing.TB, dir string, notes map[string]RimStackNote, rules [][2]string, lists, snapshots map[string][]string) {
	t.Helper()
	write := func(path string, v any) {
		t.Helper()
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if notes != nil {
		write(filepath.Join(dir, "notes.json"), notes)
	}
	if rules != nil {
		pairs := []map[string]string{}
		for _, r := range rules {
			pairs = append(pairs, map[string]string{"first": r[0], "then": r[1]})
		}
		write(filepath.Join(dir, "rules.json"), pairs)
	}
	for sub, set := range map[string]map[string][]string{"lists": lists, "snapshots": snapshots} {
		for name, ids := range set {
			write(filepath.Join(dir, sub, name+".json"), map[string]any{"name": name, "ids": ids})
		}
	}
}
