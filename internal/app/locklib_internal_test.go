package app

import (
	"os"
	"path/filepath"
	"testing"
)

// Moving a locked copy to another volume (04 §7): copy everything (.git and the origin
// marker included), check it, then send the original to the recycle bin.
func TestCopyMove(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "Mods", "5100_local")
	for p, c := range map[string]string{"About/About.xml": "<ModMetaData/>", ".rimarcho/origin.json": "{}", ".git/HEAD": "ref", "Defs/a.xml": "<Defs/>"} {
		os.MkdirAll(filepath.Dir(filepath.Join(from, p)), 0o755)
		os.WriteFile(filepath.Join(from, p), []byte(c), 0o644)
	}
	to := filepath.Join(root, "lib", "5100_local")
	os.MkdirAll(filepath.Dir(to), 0o755)
	var trashed []string
	a := &App{TrashFunc: func(p string) error { trashed = append(trashed, p); return os.RemoveAll(p) },
		trashAllow: func(string) error { return nil }}
	if err := a.copyMove(from, to); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"About/About.xml", ".rimarcho/origin.json", ".git/HEAD", "Defs/a.xml"} {
		if _, err := os.Stat(filepath.Join(to, p)); err != nil {
			t.Errorf("%s missing after the move", p)
		}
	}
	if len(trashed) != 1 || trashed[0] != from {
		t.Fatalf("trashed %v", trashed)
	}
	if _, err := os.Stat(to + ".archo-tmp"); err == nil {
		t.Fatal("temporary folder left behind")
	}
}
