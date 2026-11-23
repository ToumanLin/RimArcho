package view_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func needsOf(t *testing.T, e *testkit.Env) *view.Needs {
	t.Helper()
	b, err := view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return view.BuildNeeds(e.App, b.St, ps)
}

// GitHub #10 part b: entries Steam is still downloading are a warning of their own, "still
// downloading in Steam", not "not installed" (04 §1.6).
func TestDownloadingEntriesAreNotMissing(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony", "tester.chess", "tester.seed", "tester.gone")
	e.AddEnv(false)
	e.MustRun("scan")
	e.WriteFile(filepath.Join(e.App.Home, "cache", "steamDB.index.json"),
		`{"tester.chess":["3001","Chess"],"tester.seed":["3002","Seed"]}`)
	e.Subscribe("3001")
	e.Subscribe("3002")

	var downloading, missing []view.Need
	for _, it := range needsOf(t, e).Items {
		switch {
		case strings.Contains(it.Text, "still downloading"):
			downloading = append(downloading, it)
		case strings.Contains(it.Text, "not installed"):
			missing = append(missing, it)
		}
	}
	if len(downloading) != 1 || downloading[0].Text != "2 mods in the list are still downloading in Steam" ||
		downloading[0].Status != view.StatusWarn || len(downloading[0].Parts) != 2 {
		t.Fatalf("downloading: %+v", downloading)
	}
	if len(missing) != 1 || missing[0].Text != "the list has tester.gone, but it is not installed" {
		t.Fatalf("missing: %+v", missing)
	}

	// One left: its own card, with the Workshop page and removal.
	e.Unsubscribe("3002")
	for _, it := range needsOf(t, e).Items {
		if strings.Contains(it.Text, "still downloading") {
			if it.Text != "Chess is still downloading in Steam" || len(it.Actions) != 2 || it.Actions[1].Kind != view.ActDisable {
				t.Fatalf("one downloading: %+v", it)
			}
			return
		}
	}
	t.Fatal("no downloading card for Chess")
}

// Player feedback 2026-11-18: another tool edited ModsConfig.xml, which belonged to list
// default (edited since). On list b the card offers to ignore it there, and b's launch is
// not stopped (07 §4.1, §4.3).
func TestOtherListOutsideChangeIsSettledHere(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	e.AddWorkshop("1001", testkit.Sample("tester.a", "A"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("save")                   // default writes the file
	e.MustRun("enable", "pkg:tester.a") // and is edited after it
	e.MustRun("plan", "new", "b", "--from", "v1")
	e.MustRun("plan", "use", "b")
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld") // another tool

	var card *view.Need
	n := needsOf(t, e)
	for i, it := range n.Items {
		if strings.Contains(it.Text, "list default keeps it as") {
			card = &n.Items[i]
		}
	}
	if card == nil || len(card.Actions) != 2 || card.Actions[0].Kind != view.ActDismissConflict || card.Actions[0].Refs[0] != "default" ||
		!strings.Contains(card.Why, "launching this list is not stopped") {
		t.Fatalf("card: %+v", card)
	}
	if out, code := e.Run("save", "--dry-run"); code != 0 {
		t.Fatalf("b's save is stopped: %s", out)
	}
	if _, err := view.Do(e.App, card.Actions[0], 0); err != nil {
		t.Fatal(err)
	}
	for _, it := range needsOf(t, e).Items {
		if strings.Contains(it.Text, "keeps it as") {
			t.Fatalf("still shown after ignoring: %+v", it)
		}
	}
	if p, _ := e.App.CurrentPlan(); p.Name != "b" {
		t.Fatalf("ignoring switched the list to %s", p.Name)
	}
}
