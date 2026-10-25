package app

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/fsscan"
	"github.com/ToumanLin/RimArcho/internal/source"
)

func TestPackDirSizeLinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("pack data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "outside"), make([]byte, 100), 0600); err != nil {
		t.Fatal(err)
	}
	check := func(t *testing.T, path string) {
		t.Helper()
		if got, want := dirSize(path), dirSizeWithLinks(path); got != want {
			t.Fatalf("dirSize(%q) = %d, WalkDir = %d", path, got, want)
		}
	}
	check(t, root)
	check(t, file)
	check(t, filepath.Join(root, "missing"))
	t.Run("directory-link", func(t *testing.T) {
		link := filepath.Join(root, "junction")
		if err := source.CreateLink(link, target); err != nil {
			t.Skipf("directory link unavailable: %v", err)
		}
		check(t, root)
		check(t, link)
	})
	t.Run("file-link", func(t *testing.T) {
		link := filepath.Join(root, "symlink")
		if err := os.Symlink(file, link); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		check(t, root)
		check(t, link)
	})
}

// BenchmarkPackDirSize reads real data only when RIMARCHO_PACK_SIZE_BENCH_ROOT is set.
func BenchmarkPackDirSize(b *testing.B) {
	root := os.Getenv("RIMARCHO_PACK_SIZE_BENCH_ROOT")
	if root == "" {
		b.Skip("set RIMARCHO_PACK_SIZE_BENCH_ROOT to a workshop root")
	}
	// One reference walk also finds the largest immediate mod by byte size.
	sizes := map[string]int64{}
	var links int
	var linkBytes int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			links++
			linkBytes += fi.Size()
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name, _, nested := strings.Cut(filepath.ToSlash(rel), "/")
		if nested {
			sizes[name] += fi.Size()
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
	largest := ""
	for name, size := range sizes {
		if largest == "" || size > sizes[largest] {
			largest = name
		}
	}
	if largest == "" {
		b.Fatal("no mod subdirectories found")
	}
	b.Logf("largest=%s bytes=%d; nonregular entries=%d bytes=%d; workers=%d", largest, sizes[largest], links, linkBytes, fsscan.Workers(root))
	for _, tree := range []struct{ name, path string }{
		{"largest-" + largest, filepath.Join(root, largest)},
		{"workshop", root},
	} {
		want := dirSizeWithLinks(tree.path)
		if got := dirSize(tree.path); got != want {
			b.Fatalf("%s: dirSize=%d, WalkDir=%d", tree.path, got, want)
		}
		if _, got, err := fsscan.Size(context.Background(), tree.path, 0); err != nil || got != want {
			b.Fatalf("%s: fsscan.Size=%d, err=%v, WalkDir=%d", tree.path, got, err, want)
		}
		// Alternate old/new samples to reduce bias from other activity on the machine.
		for sample := range 3 {
			for _, method := range []struct {
				name string
				fn   func(string) int64
			}{{"before", dirSizeWithLinks}, {"after", dirSize}} {
				b.Run(tree.name+"/"+method.name+"/"+fmt.Sprint(sample+1), func(b *testing.B) {
					b.ReportAllocs()
					for range b.N {
						if got := method.fn(tree.path); got != want {
							b.Fatalf("size changed: got %d, want %d", got, want)
						}
					}
				})
			}
		}
	}
}
