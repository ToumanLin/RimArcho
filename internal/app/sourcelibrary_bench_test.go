package app_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Compare the previous Settings source-size walk with persisted accounting on
// the same synthetic 300-assembly, 6,000-file tree. Setup is outside timed work.
func BenchmarkSourceLibrarySize(b *testing.B) {
	e := testkit.New(b)
	for i := 0; i < 300; i++ {
		dir := fmt.Sprintf("mods/test.mod/Assembly %06x", i)
		for j := 0; j < 20; j++ {
			e.WriteFile(filepath.Join(e.App.SourceDir(), filepath.FromSlash(dir), fmt.Sprintf("Type%d.cs", j)), "class Synthetic {}\n")
		}
		_, err := e.App.Store.ClaimSourceArtifact(store.SourceArtifact{Hash: fmt.Sprint(i), Kind: "mod", Folder: dir, DLLName: "Assembly.dll", Created: e.App.Now(), Bytes: 380})
		if err != nil {
			b.Fatal(err)
		}
	}
	b.Run("previous-walk", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var size int64
			filepath.WalkDir(e.App.SourceDir(), func(_ string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					if fi, err := d.Info(); err == nil {
						size += fi.Size()
					}
				}
				return nil
			})
			if size != 114000 {
				b.Fatal(size)
			}
		}
	})
	b.Run("stored-accounting", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if n := e.App.Store.SourceLibraryBytes(); n != 114000 {
				b.Fatal(n)
			}
		}
	})
}
