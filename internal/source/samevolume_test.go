package source

import (
	"path/filepath"
	"testing"
)

func TestSameVolume(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if !SameVolume(dir, filepath.Join(dir, "new", "destination")) {
		t.Fatal("nonexistent destination must use its nearest existing ancestor")
	}
}
