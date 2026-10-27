package source_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/source"
)

func TestRawRelativeSymlink(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink("target", link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink privilege unavailable: %v", err)
		}
		t.Fatal(err)
	}
	l, err := source.ReadLink(link)
	if err != nil || l.Kind != "symlink" || l.RawTarget != "target" || l.State != source.TargetMissing {
		t.Fatalf("relative symlink: %+v %v", l, err)
	}
	if err := os.Mkdir(filepath.Join(root, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := source.RemoveLinkChecked(l); err == nil {
		t.Fatal("checked removal accepted a target restored since preview")
	}
	l, err = source.ReadLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.RemoveLinkChecked(l); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "target")); err != nil {
		t.Fatal("removed target directory")
	}
}

func TestReadRawDanglingAndCheckedRemove(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "missing")
	link := filepath.Join(root, "link")
	if err := source.CreateLink(link, target); err != nil {
		t.Fatal(err)
	}
	l, err := source.ReadLink(link)
	if err != nil || l.RawTarget != target || l.State != source.TargetMissing {
		t.Fatalf("raw dangling: %+v %v", l, err)
	}
	if err := source.RemoveLinkChecked(l); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("link remains")
	}
}

func TestCheckedRemoveRefusesReplacement(t *testing.T) {
	for _, kind := range []string{"link", "same-target-link", "directory", "file"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			link := filepath.Join(root, "link")
			if err := source.CreateLink(link, filepath.Join(root, "target")); err != nil {
				t.Fatal(err)
			}
			l, err := source.ReadLink(link)
			if err != nil {
				t.Fatal(err)
			}
			if err := source.RemoveLink(link); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "link":
				err = source.CreateLink(link, filepath.Join(root, "other"))
			case "same-target-link":
				err = source.CreateLink(link, filepath.Join(root, "target"))
			case "directory":
				err = os.Mkdir(link, 0o755)
			case "file":
				err = os.WriteFile(link, []byte("keep"), 0o644)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := source.RemoveLinkChecked(l); err == nil {
				t.Fatal("replacement removed")
			}
			if _, err := os.Lstat(link); err != nil {
				t.Fatal("replacement lost")
			}
		})
	}
}

func TestReadPlainEntryCannotAuthorizeRemoval(t *testing.T) {
	for _, dir := range []bool{false, true} {
		root := t.TempDir()
		path := filepath.Join(root, "plain")
		var err error
		if dir {
			err = os.Mkdir(path, 0755)
		} else {
			err = os.WriteFile(path, []byte("keep"), 0644)
		}
		if err != nil {
			t.Fatal(err)
		}
		l, err := source.ReadLink(path)
		if err != nil || l.Kind != "plain" {
			t.Fatalf("plain: %+v %v", l, err)
		}
		if err := source.RemoveLinkChecked(l); err == nil {
			t.Fatal("plain entry accepted for removal")
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal("plain entry was touched")
		}
	}
}
