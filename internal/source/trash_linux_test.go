package source

import (
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestFreedesktopTrash(t *testing.T) {
	home := t.TempDir()
	data := filepath.Join(home, "data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", data)
	root := filepath.Join(data, "Trash")
	for i := 1; i <= 2; i++ {
		file := filepath.Join(home, "a #save.rws")
		if err := os.WriteFile(file, []byte("save"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Trash(file); err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(file)
		if i > 1 {
			name += "." + strconv.Itoa(i)
		}
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatal("original remains:", err)
		}
		if b, err := os.ReadFile(filepath.Join(root, "files", name)); err != nil || string(b) != "save" {
			t.Fatalf("trashed content: %q %v", b, err)
		}
		if b, err := os.ReadFile(filepath.Join(root, "info", name+".trashinfo")); err != nil || !strings.Contains(string(b), "Path="+(&url.URL{Path: file}).EscapedPath()+"\nDeletionDate=") {
			t.Fatalf("trash metadata: %q %v", b, err)
		}
	}
	// A link in place of info must fail before the original can be moved.
	badData := filepath.Join(home, "bad-data")
	badRoot := filepath.Join(badData, "Trash")
	if err := os.MkdirAll(badRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(home, filepath.Join(badRoot, "info")); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(home, "keep.rws")
	if err := os.WriteFile(file, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", badData)
	if err := Trash(file); err == nil {
		t.Fatal("unsafe trash directory was accepted")
	}
	if b, err := os.ReadFile(file); err != nil || string(b) != "keep" {
		t.Fatalf("failure changed the original: %q %v", b, err)
	}
	t.Setenv("XDG_DATA_HOME", data)
	t.Run("other-volume", func(t *testing.T) {
		other, err := os.MkdirTemp("/dev/shm", "rimarcho-safety-")
		if err != nil {
			t.Skip("no writable second filesystem:", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(other) })
		if SameVolume(home, other) {
			t.Skip("/dev/shm is on the home volume")
		}
		mount, err := trashMount(other)
		if err != nil {
			t.Fatal(err)
		}
		trash := filepath.Join(mount, ".Trash-"+strconv.Itoa(os.Geteuid()))
		// Reserve a fresh synthetic trash; never read or modify an existing one.
		if err := os.Mkdir(trash, 0o700); err != nil {
			t.Skip("cannot reserve isolated volume trash:", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(trash) })
		file := filepath.Join(other, "other save.rws")
		if err := os.WriteFile(file, []byte("other"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Trash(file); err != nil {
			t.Fatal(err)
		}
		if b, err := os.ReadFile(filepath.Join(trash, "files", filepath.Base(file))); err != nil || string(b) != "other" {
			t.Fatalf("other-volume content: %q %v", b, err)
		}
		rel, _ := filepath.Rel(mount, file)
		if b, err := os.ReadFile(filepath.Join(trash, "info", filepath.Base(file)+".trashinfo")); err != nil || !strings.Contains(string(b), "Path="+(&url.URL{Path: rel}).EscapedPath()+"\n") {
			t.Fatalf("other-volume relative path: %q %v", b, err)
		}
		if info, err := os.Stat(trash); err != nil || info.Mode().Perm() != 0o700 {
			t.Fatal("volume trash is not private:", err)
		}
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatal("other-volume original remains:", err)
		}
	})
}
