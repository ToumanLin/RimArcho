package source

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A test binary never reaches the shell's recycle bin (17 §4): Trash refuses and the
// folder stays where it is, so a test that forgets its fake bin fails instead of filling
// the player's Recycle Bin.
func TestTrashRefusesRealBinInTests(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "recycled-by-test")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Trash(dir); !errors.Is(err, ErrTestRecycle) {
		t.Fatalf("Trash in a test = %v, want ErrTestRecycle", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("the folder moved:", err)
	}
}
