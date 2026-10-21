package app

import (
	"path/filepath"
	"testing"
)

// A game folder matches its own processes only: "RimWorld2" is not inside "RimWorld".
func TestProcessIn(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "RimWorld")
	b := filepath.Join(root, "RimWorld2")
	procs := []*GameRun{{PID: 2, Exe: filepath.Join(b, "RimWorldWin64.exe")}, {PID: 1, Exe: filepath.Join(a, "RimWorldWin64.exe")}}
	if r := processIn(procs, a); r == nil || r.PID != 1 {
		t.Fatalf("game folder a: %+v", r)
	}
	if r := processIn(procs, b); r == nil || r.PID != 2 {
		t.Fatalf("game folder b: %+v", r)
	}
	if r := processIn(procs, filepath.Join(root, "Other")); r != nil {
		t.Fatalf("no game: %+v", r)
	}
	if r := processIn(procs, ""); r != nil {
		t.Fatal("an environment without a game folder has no game")
	}
}
