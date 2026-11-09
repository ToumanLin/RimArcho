package source

import (
	"os"
	"path/filepath"
	"testing"
)

// Work folders RimArcho leaves behind when a copy is cut short are not Mods.
func TestListSkipsWorkFolders(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, name := range []string{"Real", ".archo-tmp-123", "Real_local.archo-tmp", "Monitor.archo-old"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	entries, _, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "Real" {
		t.Fatalf("listed %+v", entries)
	}
}
