package app

import (
	"os"
	"path/filepath"
	"testing"
)

// The game marks a Mod made for another version in its running list; the list goes on
// after such a line (1.7: every Mod after the first marked one was reported as not loaded).
func TestRunningModsIncompatibleMark(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "Player.log")
	log := "Initializing new game with mods:\n  - Ludeon.RimWorld\n  - Lat.ApparelRetexture (incompatible version)\n  - VanillaExpanded.VWE\n\nnext\n  - not.a.mod\n"
	if err := os.WriteFile(p, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runningMods(p)
	if len(got) != 3 || !got["ludeon.rimworld"] || !got["lat.apparelretexture"] || !got["vanillaexpanded.vwe"] {
		t.Fatalf("running %v", got)
	}
}
