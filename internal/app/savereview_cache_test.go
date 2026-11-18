package app_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Both runs use a fresh listing; the second also reopens the application, so no
// in-memory page or parser facts can explain a hit. Timings exclude fixture setup.
func TestSaveReviewLibraryCache300x40(t *testing.T) {
	e := testkit.New(t)
	mods := []string{"ludeon.rimworld"}
	var body strings.Builder
	for i := range 40 {
		pkg := fmt.Sprintf("tester.review%d", i)
		mods = append(mods, pkg)
		var defs strings.Builder
		defs.WriteString("<Defs>")
		for j := range 20 {
			fmt.Fprintf(&defs, "<ThingDef><defName>Review%d_%d</defName><category>Item</category></ThingDef>", i, j)
		}
		defs.WriteString("</Defs>")
		e.AddWorkshop(fmt.Sprint(3100000000+i), testkit.Mod{PackageID: pkg, Name: fmt.Sprintf("Review %d", i), Files: map[string]string{"Defs/Review.xml": defs.String()}})
		fmt.Fprintf(&body, "<li><def>Review%d_0</def></li>", i)
	}
	e.WriteModsConfig("1.6.4633 rev1261", mods...)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	for i := range 300 {
		e.WriteSave(fmt.Sprintf("Fort %03d.rws", i), testkit.SaveSpec{Mods: mods, Faction: "Fort", Seed: "review", PRV: 1,
			Body: "<things>" + body.String() + "</things><padding>" + strings.Repeat("synthetic ", 4096) + "</padding>"})
	}
	run := func() (*app.SaveReview, time.Duration) {
		t.Helper()
		start := time.Now()
		l, err := e.App.ListSaves()
		if err != nil {
			t.Fatal(err)
		}
		r, err := e.App.SaveReview(l.Colonies, app.ReviewOpts{Read: true})
		if err != nil {
			t.Fatal(err)
		}
		return r, time.Since(start)
	}
	first, cold := run()
	if first.Saves != 300 || first.Cached != 0 {
		t.Fatalf("first: %+v", first)
	}
	var bytes int64
	if err := e.App.Store.DB.QueryRow(`SELECT coalesce(sum(length(result)),0) FROM save_review_cache`).Scan(&bytes); err != nil {
		t.Fatal(err)
	}
	e.Open()
	restart := time.Now()
	// Full facts and attribution inputs are inaccessible after listing. A cache hit
	// must still succeed without a full read, attribution, or catalog reconstruction.
	l, err := e.App.ListSaves()
	if err != nil {
		t.Fatal(err)
	}
	listing := time.Since(restart)
	for _, q := range []string{
		`UPDATE save_cache SET facts=x'00'`,
		`CREATE TEMP TRIGGER reject_full_read BEFORE UPDATE ON save_cache BEGIN SELECT RAISE(ABORT,'unexpected full read'); END`,
	} {
		if _, err := e.App.Store.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.App.Store.DB.Exec(`DELETE FROM def_fact`); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	second, err := e.App.SaveReview(l.Colonies, app.ReviewOpts{Read: true})
	warm := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if second.Cached != 300 || second.Saves != 300 || !reflect.DeepEqual(first.Mods, second.Mods) {
		t.Fatalf("restart did not exclusively merge kept results: cached=%d saves=%d", second.Cached, second.Saves)
	}
	t.Logf("300 saves / 40 Mods: first (listing + full reads + attribution) %s; second after restart %s (listing %s + merge %s); compressed results %d bytes (%.0f/save)", cold, listing+warm, listing, warm, bytes, float64(bytes)/300)
}

func TestSaveReviewCatalogSharingAndOrder(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("3100000001", testkit.Mod{PackageID: "tester.a", Name: "A", Files: map[string]string{"Defs/A.xml": `<Defs><ThingDef><defName>Shared</defName><category>Item</category></ThingDef><ThingDef MayRequire="tester.guard"><defName>Guarded</defName><category>Item</category></ThingDef></Defs>`}})
	e.AddWorkshop("3100000002", testkit.Mod{PackageID: "tester.b", Name: "B", Files: map[string]string{"Defs/B.xml": `<Defs><ThingDef><defName>Shared</defName><category>Building</category></ThingDef></Defs>`}})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.a", "tester.b")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	for i, spec := range []struct {
		name string
		mods []string
		body string
	}{
		{"AB", []string{"ludeon.rimworld", "tester.a", "tester.b"}, `<things><li><def>Shared</def></li></things>`},
		{"BA", []string{"ludeon.rimworld", "tester.b", "tester.a"}, `<things><li><def>Shared</def></li></things>`},
		{"Guard", []string{"ludeon.rimworld", "tester.a", "tester.b", "tester.guard"}, `<things><li><def>Shared</def></li><li><def>Guarded</def></li></things>`},
		{"Empty", []string{"ludeon.rimworld", "tester.a", "tester.b"}, ""},
	} {
		e.WriteSave(spec.name+".rws", testkit.SaveSpec{Mods: spec.mods, Seed: spec.name, PRV: i + 1, Faction: spec.name, Body: spec.body})
	}
	run := func(name string, cached int) *app.SaveReview {
		t.Helper()
		l, err := e.App.ListSaves()
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range l.Colonies {
			if c.Latest().File != name+".rws" {
				continue
			}
			r, err := e.App.SaveReview([]*app.Colony{c}, app.ReviewOpts{Read: true})
			if err != nil || r.Cached != cached {
				t.Fatalf("%s cached %d: %+v %v", name, cached, r, err)
			}
			return r
		}
		t.Fatalf("missing colony %s", name)
		return nil
	}
	check := func(r *app.SaveReview, aAdds, aSeen, bAdds, bSeen int) {
		t.Helper()
		counts := map[string][2]int{}
		for _, m := range r.Mods {
			counts[m.PackageID] = [2]int{m.Adds, m.Seen}
		}
		if counts["tester.a"] != [2]int{aAdds, aSeen} || counts["tester.b"] != [2]int{bAdds, bSeen} {
			t.Fatalf("ownership: %v", counts)
		}
	}
	check(run("AB", 0), 0, 0, 1, 1)
	check(run("BA", 0), 1, 1, 0, 0)
	check(run("Guard", 0), 1, 1, 1, 1)
	check(run("Empty", 0), 0, 0, 1, 0)
	var n int
	if err := e.App.Store.DB.QueryRow(`SELECT count(*) FROM save_review_catalog`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("sharing across colonies: %d %v", n, err)
	}
	var raw []byte
	if err := e.App.Store.DB.QueryRow(`SELECT result FROM save_review_cache WHERE path LIKE '%empty.rws'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) > 200 {
		t.Fatalf("empty save row contains library-sized data: %d bytes", len(raw))
	}
	z, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(z)
	z.Close()
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(plain, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["Catalog"] != nil || fields["Mods"] != nil || fields["CatalogKey"] == nil {
		t.Fatalf("unexpected per-save fields: %s", plain)
	}
	e.Open()
	check(run("AB", 1), 0, 0, 1, 1)
	check(run("BA", 1), 1, 1, 0, 0)
	check(run("Guard", 1), 1, 1, 1, 1)
	// Corrupt catalogs cause a cache miss, then repair from current L1 facts.
	if _, err := e.App.Store.DB.Exec(`UPDATE save_review_catalog SET catalog=x'00'`); err != nil {
		t.Fatal(err)
	}
	check(run("AB", 0), 0, 0, 1, 1)
	check(run("AB", 1), 0, 0, 1, 1)
}

func TestSaveReviewProgressAndCancellation(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	for i := range 3 {
		e.WriteSave(fmt.Sprintf("Fort %d.rws", i), testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}, ModTime: e.Now().Add(time.Duration(i-4) * time.Hour)})
	}
	l, err := e.App.ListSaves()
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately reverse the caller's saves; the newest must still be first.
	c := l.Colonies[0]
	c.Saves[0], c.Saves[2] = c.Saves[2], c.Saves[0]
	var partial *app.SaveReview
	updates := 0
	r, err := e.App.SaveReview(l.Colonies, app.ReviewOpts{Read: true, Progress: func(done, total int) {
		if done == 1 {
			if c.Saves[2].Summary == nil || c.Saves[0].Summary != nil {
				t.Error("not newest first")
			}
			time.Sleep(510 * time.Millisecond)
		}
	}, Partial: func(p *app.SaveReview) { partial = p; updates++ }})
	if err != nil || updates != 1 || partial.Done != 1 || partial.Total != 3 || partial.Saves != 1 || r.Done != 3 {
		t.Fatalf("progress: updates=%d partial=%+v final=%+v err=%v", updates, partial, r, err)
	}
	// Progress has already been merged when Stop is checked, including the last save.
	done := 0
	_, err = e.App.SaveReview(l.Colonies, app.ReviewOpts{Read: true, Progress: func(d, total int) { done = d }, Stop: func() bool { return done == 1 }})
	if !errors.Is(err, context.Canceled) || done != 1 {
		t.Fatalf("cancel: %d %v", done, err)
	}
}

func TestSaveReviewCacheInvalidation(t *testing.T) {
	e := testkit.New(t)
	defs := "<Defs><ThingDef><defName>Cap</defName><category>Item</category></ThingDef></Defs>"
	e.AddWorkshop("3100000000", testkit.Mod{PackageID: "tester.review", Name: "Review", Files: map[string]string{"Defs/Review.xml": defs}})
	mods := []string{"ludeon.rimworld", "tester.review"}
	e.WriteModsConfig("1.6.4633 rev1261", mods...)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	path := e.WriteSave("Fort.rws", testkit.SaveSpec{Mods: mods, Body: "<things><li><def>Cap</def></li></things>"})
	run := func(want int) *app.SaveReview {
		t.Helper()
		l, err := e.App.ListSaves()
		if err != nil {
			t.Fatal(err)
		}
		r, err := e.App.SaveReview(l.Colonies, app.ReviewOpts{Read: true})
		if err != nil || r.Cached != want {
			t.Fatalf("cached want %d: %+v, %v", want, r, err)
		}
		return r
	}
	run(0)
	run(1)
	// A parser mismatch in a kept attribution is never accepted.
	if _, err := e.App.Store.DB.Exec(`UPDATE save_review_cache SET parser=0`); err != nil {
		t.Fatal(err)
	}
	run(0)
	run(1)
	at := e.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
	run(0)
	run(1)
	e.WriteFile(filepath.Join(e.Workshop, "3100000000", "Defs", "Review.xml"), strings.Replace(defs, "</Defs>", "<ThingDef><defName>Helm</defName><category>Item</category></ThingDef></Defs>", 1))
	e.MustRun("scan")
	e.MustRun("analyze")
	r := run(0)
	for _, m := range r.Mods {
		if m.PackageID == "tester.review" && (m.Adds != 2 || m.Seen != 1) {
			t.Fatalf("stale content: %+v", m)
		}
	}
	run(1)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	// The review only reads; the index tidies what belonged to a save that is gone.
	run(0)
	var n int
	if err := e.App.Store.DB.QueryRow(`SELECT count(*) FROM save_review_cache`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("the review tidied: %d %v", n, err)
	}
	e.MustRun("saves", "index")
	if err := e.App.Store.DB.QueryRow(`SELECT count(*) FROM save_review_cache`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("gone save retained: %d %v", n, err)
	}
}

// The kept attributions rest on what each save resolves its Mods with (12 §3.4), not on
// the Game Config shown or the lists open: switching Game Config back and forth, and
// editing another list, keep every save's row (one row a save, so a key that named the
// current Game Config would overwrite it on each switch).
func TestSaveReviewCacheAcrossGameConfigs(t *testing.T) {
	e := testkit.New(t)
	defs := "<Defs><ThingDef><defName>Cap</defName><category>Item</category></ThingDef></Defs>"
	e.AddWorkshop("3100000000", testkit.Mod{PackageID: "tester.review", Name: "Review", Files: map[string]string{"Defs/Review.xml": defs}})
	mods := []string{"ludeon.rimworld", "tester.review"}
	e.WriteModsConfig("1.6.4633 rev1261", mods...)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	e.WriteSave("Fort.rws", testkit.SaveSpec{Mods: mods, Faction: "Fort", Seed: "fort", PRV: 1, Body: "<things><li><def>Cap</def></li></things>"})
	e.WriteSave("Camp.rws", testkit.SaveSpec{Mods: mods, Faction: "Camp", Seed: "camp", PRV: 2, Body: "<things><li><def>Cap</def></li></things>"})
	// The window's readers name the Game Config they read (gui refreshInventorySession).
	use := func(name string) {
		t.Helper()
		e.MustRun("env", "use", name)
		e.App.EnvName = name
	}
	run := func(step string, want int) {
		t.Helper()
		l, err := e.App.ListSaves()
		if err != nil {
			t.Fatal(err)
		}
		r, err := e.App.SaveReview(l.Colonies, app.ReviewOpts{Read: true})
		if err != nil || r.Cached != want || r.Saves != 2 {
			t.Fatalf("%s: cached want %d: %+v, %v", step, want, r, err)
		}
	}
	// Adding a Game Config changes the index (its copies' generation), and its first use
	// makes its list: before the runs.
	e.MustRun("env", "add", "sibling", "--game", e.Game, "--saves", "own")
	use("sibling")
	e.MustRun("list")
	use("main")
	run("first", 0)
	run("again", 2)
	use("sibling")
	run("in the other Game Config", 2)
	use("main")
	run("back", 2)
	use("sibling")
	run("there again", 2)
	// a list of the other Game Config changes; the saves do not load it
	if res, err := e.App.Store.DB.Exec(`UPDATE workspace SET entries='[]' WHERE plan_id IN (SELECT id FROM plan WHERE environment_id<>(SELECT id FROM environment WHERE name='main'))`); err != nil {
		t.Fatal(err)
	} else if n, _ := res.RowsAffected(); n == 0 {
		t.Fatal("no list of the other Game Config to edit")
	}
	use("main")
	run("after editing another list", 2)
}
