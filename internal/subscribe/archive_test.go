package subscribe_test

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

func TestImportArchivePaths(t *testing.T) {
	for _, name := range []string{"../outside", `..\outside`, "/absolute", `C:\absolute`, "//server/share", "link"} {
		t.Run(name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "mod.zip")
			f, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			z := zip.NewWriter(f)
			h := &zip.FileHeader{Name: name}
			if name == "link" {
				h.SetMode(os.ModeSymlink | 0o777)
			}
			w, err := z.CreateHeader(h)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte("outside")); err != nil {
				t.Fatal(err)
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			f.Close()
			if _, err := subscribe.Import(archive, t.TempDir()); err == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}

func TestImportFolder(t *testing.T) {
	for _, wrapper := range []string{"", "Release"} {
		t.Run(wrapper, func(t *testing.T) {
			from, dest := t.TempDir(), t.TempDir()
			about := filepath.Join(from, wrapper, "About", "About.xml")
			if err := os.MkdirAll(filepath.Dir(about), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(about, []byte("<ModMetaData/>"), 0o644); err != nil {
				t.Fatal(err)
			}
			mod, err := subscribe.Import(from, dest)
			if err != nil || source.AboutState(mod) != source.AboutPresent {
				t.Fatalf("folder import: %s %v", mod, err)
			}
			if _, err := subscribe.Import(from, filepath.Join(from, "recursive")); err == nil {
				t.Fatal("recursive folder import accepted")
			}
		})
	}
	if _, err := subscribe.Import(t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("non-mod folder accepted")
	}
}
