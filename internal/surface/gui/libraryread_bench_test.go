package gui

import (
	"fmt"
	"os"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// RIMARCHO_BENCH_HOME may point to a disposable DB backup for local measurements.
// Never point it at the live home: edits below change its workspace.
func BenchmarkLibraryCopyAfterEdit(b *testing.B) {
	var a *app.App
	if home := os.Getenv("RIMARCHO_BENCH_HOME"); home != "" {
		var err error
		a, err = app.Open(home)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { a.Close() })
	} else {
		e := testkit.New(b)
		for i := 0; i < 500; i++ {
			e.AddWorkshop(fmt.Sprint(10000+i), testkit.Sample(fmt.Sprintf("test.mod%d", i), fmt.Sprintf("Mod %d", i)))
		}
		e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "test.mod0")
		e.AddEnv(false)
		e.MustRun("scan")
		// A substantial file table without creating thousands of extra disk files.
		_, err := e.App.Store.DB.Exec(`WITH RECURSIVE files(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM files WHERE n<100)
   INSERT INTO snapshot_file(snapshot_id,rel_path,size,mtime) SELECT id,'Defs/Fixture'||n||'.xml',1,1 FROM snapshot CROSS JOIN files`)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := e.App.Store.DB.Exec(`UPDATE snapshot SET kinds = kinds | ?`, store.HasDefsPatch); err != nil {
			b.Fatal(err)
		}
		a = e.App
	}
	a.LazyLog, a.LogCacheOnly = true, true
	api := newAPI(a)
	b.Cleanup(api.Close)
	board := api.Board()
	if board.Error != "" {
		b.Fatal(board.Error)
	}
	if len(board.Cols[view.Active]) < 2 {
		b.Fatal("need an enabled non-Core mod")
	}
	id := board.Cols[view.Active][1].ID
	toggle := func() {
		st, err := a.Loadout()
		if err != nil {
			b.Fatal(err)
		}
		col := view.Active
		if st.List.Index(id) < 0 {
			col = view.Inactive
		}
		if _, err := api.Toggle(int(col), []string{id}, nil); err != nil {
			b.Fatal(err)
		}
	}
	b.Run("full-library", func(b *testing.B) {
		for b.Loop() {
			b.StopTimer()
			toggle()
			b.StartTimer()
			lib, err := a.Library()
			if err != nil {
				b.Fatal(err)
			}
			if view.LibCopy(lib, id) == nil {
				b.Fatal("missing row")
			}
		}
	})
	b.Run("one-lineage", func(b *testing.B) {
		for b.Loop() {
			b.StopTimer()
			toggle()
			b.StartTimer()
			if row, err := api.LibraryCopy(id); err != nil || row == nil {
				b.Fatalf("copy: %v", err)
			}
		}
	})
	b.Run("warm-copy", func(b *testing.B) {
		if _, err := api.LibraryCopy(id); err != nil {
			b.Fatal(err)
		}
		for b.Loop() {
			if _, err := api.LibraryCopy(id); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("warm-detail", func(b *testing.B) {
		if _, err := api.Detail(id, false, false, false); err != nil {
			b.Fatal(err)
		}
		for b.Loop() {
			if _, err := api.Detail(id, false, false, false); err != nil {
				b.Fatal(err)
			}
		}
	})
}
