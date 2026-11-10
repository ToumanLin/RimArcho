package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// holdLocked opens path with no sharing until the test ends: it cannot be deleted.
func holdLocked(t *testing.T, path string) {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { windows.CloseHandle(h) })
}

// A real locked file: the uninstall/delete path removes the rest, reports a partial
// deletion and leaves the game link removed.
func TestLockedFileDeletionKeepsLinksRemoved(t *testing.T) {
	t.Parallel()
	a, target, link := partialFixture(t)
	holdLocked(t, filepath.Join(target, "About", "About.xml"))
	err := a.withIncomingLinks(target, "", func() error { return removeModTree(target) })
	var partly *partlyRemovedError
	if !errors.As(err, &partly) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "Defs")); !os.IsNotExist(err) {
		t.Fatal("deletion stopped early", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("game linked to a half-deleted Mod", err)
	}
	settledLinks(t, a)

	// Only the locked file: nothing could be removed, so the link comes back.
	b, target2, link2 := partialFixture(t)
	os.RemoveAll(filepath.Join(target2, "Defs"))
	holdLocked(t, filepath.Join(target2, "About", "About.xml"))
	if err := b.withIncomingLinks(target2, "", func() error { return removeModTree(target2) }); err == nil || errors.As(err, &partly) {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link2); err != nil {
		t.Fatal("link not put back", err)
	}
	settledLinks(t, b)
}
