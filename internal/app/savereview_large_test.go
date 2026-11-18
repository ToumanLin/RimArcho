package app_test

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Run explicitly with -run '^$' -bench BenchmarkSaveReviewLibrary500x200x200
// -benchtime=1x. Setup and both measured reviews use only testkit data.
func BenchmarkSaveReviewLibrary500x200x200(b *testing.B) {
	for range b.N {
		b.StopTimer()
		e := testkit.New(b)
		mods := []string{"ludeon.rimworld"}
		var body strings.Builder
		for i := range 500 {
			pkg := fmt.Sprintf("tester.review%d", i)
			mods = append(mods, pkg)
			var defs strings.Builder
			defs.WriteString("<Defs>")
			for j := range 200 {
				h := sha256.Sum256([]byte(fmt.Sprintf("synthetic-%d-%d", i, j)))
				name := fmt.Sprintf("Review_%03d_%03d_%x", i, j, h[:8])
				fmt.Fprintf(&defs, "<ThingDef><defName>%s</defName><category>Item</category></ThingDef>", name)
				if j == 0 {
					fmt.Fprintf(&body, "<li><def>%s</def></li>", name)
				}
			}
			defs.WriteString("</Defs>")
			e.AddWorkshop(fmt.Sprint(3100000000+i), testkit.Mod{PackageID: pkg, Name: fmt.Sprintf("Review %d", i), Files: map[string]string{"Defs/Review.xml": defs.String()}})
		}
		e.WriteModsConfig("1.6.4633 rev1261", mods...)
		e.AddEnv(false)
		e.MustRun("scan")
		e.MustRun("analyze")
		for i := range 200 {
			col := i % 3
			e.WriteSave(fmt.Sprintf("Colony%d %03d.rws", col, i), testkit.SaveSpec{Mods: mods, Faction: fmt.Sprintf("Colony%d", col), Seed: fmt.Sprint(col), PRV: col + 1, Body: "<things>" + body.String() + "</things>"})
		}
		run := func() (*app.SaveReview, time.Duration, uint64) {
			b.Helper()
			runtime.GC()
			stop, done := make(chan struct{}), make(chan uint64, 1)
			go func() {
				tick := time.NewTicker(10 * time.Millisecond)
				defer tick.Stop()
				var peak uint64
				for {
					var mem runtime.MemStats
					runtime.ReadMemStats(&mem)
					peak = max(peak, mem.HeapAlloc)
					select {
					case <-stop:
						done <- peak
						return
					case <-tick.C:
					}
				}
			}()
			start := time.Now()
			l, err := e.App.ListSaves()
			if err != nil {
				b.Fatal(err)
			}
			r, err := e.App.SaveReview(l.Colonies, app.ReviewOpts{Read: true})
			elapsed := time.Since(start)
			close(stop)
			peak := <-done
			if err != nil {
				b.Fatal(err)
			}
			if r.Saves != 200 || r.Colonies != 3 {
				b.Fatalf("counts: %+v", r)
			}
			return r, elapsed, peak
		}
		b.StartTimer()
		first, cold, coldHeap := run()
		b.StopTimer()
		var total, minRow, maxRow int64
		if err := e.App.Store.DB.QueryRow(`SELECT sum(length(result)),min(length(result)),max(length(result)) FROM save_review_cache`).Scan(&total, &minRow, &maxRow); err != nil {
			b.Fatal(err)
		}
		var catalogBytes int64
		var catalogs int
		if err := e.App.Store.DB.QueryRow(`SELECT count(*),coalesce(sum(length(catalog)),0) FROM save_review_catalog`).Scan(&catalogs, &catalogBytes); err != nil {
			b.Fatal(err)
		}
		if catalogs != 1 {
			b.Fatalf("identical enabled sets stored %d catalogs", catalogs)
		}
		e.Open()
		b.StartTimer()
		second, warm, warmHeap := run()
		b.StopTimer()
		if first.Cached != 0 || second.Cached != 200 || !reflect.DeepEqual(first.Mods, second.Mods) {
			b.Fatalf("restart cache: first %d second %d", first.Cached, second.Cached)
		}
		b.Logf("500 Mods x 200 Defs, 200 saves, 3 colonies: rows %d bytes total, %.0f/save (%d..%d); first %s, restart second %s; sampled peak heap first %.1f MiB, second %.1f MiB", total, float64(total)/200, minRow, maxRow, cold, warm, float64(coldHeap)/(1<<20), float64(warmHeap)/(1<<20))
		b.Logf("shared catalogs %d, %d bytes; combined payload %d bytes (%.0f/save amortized), plus 64 bytes/save SQL catalog reference and SQLite overhead", catalogs, catalogBytes, total+catalogBytes, float64(total+catalogBytes)/200)
	}
}
