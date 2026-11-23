//go:build !windows

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/source"
)

// testTrashData is this test process's own XDG data folder. Without it, a test that
// recycles (trash, pack updates, uninstall) moved its synthetic folders into the real
// user's ~/.local/share/Trash (Linux-Test VM, 2026-11-23). The tests run in parallel, so
// t.Setenv cannot do this per test.
var testTrashData string

const testTrashPrefix = "archo-app-test-data-"

func TestMain(m *testing.M) {
	// A helper process (the test binary run again as a child) keeps its parent's folder.
	if x := os.Getenv("XDG_DATA_HOME"); strings.HasPrefix(filepath.Base(x), testTrashPrefix) {
		testTrashData = x
		os.Exit(m.Run())
	}
	dir, err := os.MkdirTemp("", testTrashPrefix)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	testTrashData = dir
	os.Setenv("XDG_DATA_HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// A test's recycled folder lands in the test process's trash, never the user's.
func TestRecycleStaysInTestData(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "recycled-by-test")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := source.Trash(dir); err != nil {
		t.Fatal(err)
	}
	if !source.SameVolume(filepath.Dir(dir), testTrashData) {
		t.Skip("the temporary folder is on another file system; its trash is the volume's own")
	}
	if _, err := os.Lstat(filepath.Join(testTrashData, "Trash", "files", "recycled-by-test")); err != nil {
		t.Fatalf("not in the test's trash: %v", err)
	}
}
