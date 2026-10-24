package scan

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/source"
)

func TestComputeSkipListing(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"About/About.xml", "1.6/Defs/A.xml", "Textures/Z.png", "Textures/a.png",
		"Textures/generated.dds", ".git/config", "1.6/.svn/entries", "Textures/.rimarcho/marker",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(rel), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var want []File
	for _, rel := range []string{"1.6/Defs/A.xml", "About/About.xml", "Textures/a.png", "Textures/Z.png"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		f := File{Rel: rel, Size: fi.Size(), MTime: fi.ModTime().UnixNano()}
		if strings.HasSuffix(rel, ".xml") {
			f.Hash, err = hashFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f.HasHash = true
		}
		want = append(want, f)
	}
	generated := func(string) map[string]bool {
		return map[string]bool{"textures/generated.dds": true}
	}
	check := func(t *testing.T, dir string) {
		t.Helper()
		fp, err := ComputeSkip(dir, nil, generated)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(fp.Files, want) || fp.SnapshotHash != SnapshotHash(want) || fp.Count != len(want) || fp.Hashed != 2 {
			t.Fatalf("fingerprint = %+v; want files %+v", fp, want)
		}
		prev := map[string]File{}
		for _, f := range fp.Files {
			prev[strings.ToLower(f.Rel)] = f
		}
		reused, err := ComputeSkip(dir, prev, generated)
		if err != nil || !reflect.DeepEqual(reused.Files, want) || reused.Hashed != 0 || reused.Reused != 2 {
			t.Fatalf("reuse = %+v, err=%v", reused, err)
		}
	}
	check(t, root)
	t.Run("links", func(t *testing.T) {
		outside := t.TempDir()
		if err := os.WriteFile(filepath.Join(outside, "outside"), []byte("ignored"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := source.CreateLink(filepath.Join(root, "inside-link"), outside); err != nil {
			t.Skipf("directory links unavailable: %v", err)
		}
		link := filepath.Join(t.TempDir(), "root-link")
		if err := source.CreateLink(link, root); err != nil {
			t.Fatal(err)
		}
		real, err := source.RealPath(root)
		if err != nil {
			t.Fatal(err)
		}
		generated = func(dir string) map[string]bool {
			if dir != real {
				t.Errorf("generated callback dir = %q, want %q", dir, real)
			}
			return map[string]bool{"textures/generated.dds": true}
		}
		check(t, link)
	})
	if _, err := ComputeSkip(filepath.Join(root, "missing"), nil, nil); err == nil {
		t.Fatal("missing root succeeded")
	}
}
