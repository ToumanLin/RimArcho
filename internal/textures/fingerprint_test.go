package textures

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFingerprintTracksUnscannedTextureChanges(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Textures")
	a := filepath.Join(dir, "a.png")
	writePNG(t, a, 32, 64, false)
	fingerprint := func() [32]byte {
		t.Helper()
		sum, err := Fingerprint(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}
		return sum
	}
	first := fingerprint()
	if fingerprint() != first {
		t.Fatal("unchanged files changed fingerprint")
	}
	parent, _ := os.Stat(dir)
	info, _ := os.Stat(a)
	// Changing an existing file need not change its containing directory's timestamp.
	if err := os.Chtimes(a, info.ModTime(), info.ModTime().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dir, parent.ModTime(), parent.ModTime()); err != nil {
		t.Fatal(err)
	}
	second := fingerprint()
	if first == second {
		t.Fatal("missed an unscanned overwrite")
	}
	b := filepath.Join(root, "1.7", "Textures", "b.png")
	writePNG(t, b, 128, 64, true)
	third := fingerprint()
	if second == third {
		t.Fatal("missed a new version folder")
	}
	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	}
	if fingerprint() == third {
		t.Fatal("missed removal")
	}
	writeFile(t, filepath.Join(dir, "Source", "ignored.png"), "author source")
	before := fingerprint()
	writeFile(t, filepath.Join(dir, "Source", "ignored.png"), "changed author source")
	if fingerprint() != before {
		t.Fatal("author sources affect fingerprint")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Fingerprint(ctx, root); err == nil {
		t.Fatal("cancelled fingerprint succeeded")
	}
}
