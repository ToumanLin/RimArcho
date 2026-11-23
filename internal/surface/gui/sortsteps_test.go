//go:build windows || cgo

package gui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// sortStepsFixture: after a sort, every kind of step has something to do (16 §6.1): an entry
// whose copy is only in an extra folder, an enabled Mod with an installed replacement, a
// required Mod installed but not enabled, a replacement to get from the Workshop, and an
// entry nobody knows where to get.
func sortStepsFixture(t *testing.T) (*testkit.Env, *API) {
	t.Helper()
	view.SetLang("en")
	e := testkit.New(t)
	mods := filepath.Join(e.Game, "Mods")
	e.WriteMod(filepath.Join(mods, "Dep"), testkit.Sample("tester.dep", "Dep"))
	e.WriteMod(filepath.Join(mods, "User"), testkit.Mod{PackageID: "tester.user", Name: "User", Deps: []string{"tester.dep"}})
	e.WriteMod(filepath.Join(mods, "Old"), testkit.Sample("tester.old", "Old"))
	e.WriteMod(filepath.Join(mods, "New"), testkit.Sample("tester.new", "New"))
	e.WriteMod(filepath.Join(mods, "Stale"), testkit.Sample("tester.stale", "Stale"))
	extra := filepath.Join(e.Root, "Extra")
	e.WriteMod(filepath.Join(extra, "One"), testkit.Sample("tester.one", "One"))
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.user", "tester.old", "tester.one", "tester.stale", "tester.lost")
	e.WriteFile(filepath.Join(e.Home, "cache", "replacements.json.gz"), `{"rules":[
		{"oldPackageId":"tester.old","newPackageId":"tester.new"},
		{"oldPackageId":"tester.stale","newPackageId":"tester.fork","newWorkshopId":"7002","newName":"Stale Fork"}]}`)
	e.AddEnv(false)
	if _, err := e.App.FolderAdd(source.Extra, extra); err != nil {
		t.Fatal(err)
	}
	e.MustRun("scan")
	return e, &API{a: e.App, needsClosed: true}
}

func stepKinds(s *view.SortSteps) string {
	var k []string
	for _, st := range s.Steps {
		k = append(k, st.Kind)
	}
	return strings.Join(k, ",")
}

func TestSortStepsOneAtATime(t *testing.T) {
	e, api := sortStepsFixture(t)
	if o, err := api.Run("sort"); err != nil || len(o.Refused) > 0 {
		t.Fatal(o, err)
	}
	list := func() loadout.List {
		api.mu.Lock()
		defer api.mu.Unlock()
		st, err := e.App.Loadout()
		if err != nil {
			t.Fatal(err)
		}
		return st.List.Clone()
	}
	sorted := list()

	s, err := api.SortSteps(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := stepKinds(s); got != "use-installed,replace,enable,subscribe" || s.Current != 0 {
		t.Fatalf("steps %s, current %d", got, s.Current)
	}
	// Only the current step is previewed; previewing writes nothing.
	cur := s.Steps[0]
	if !cur.Preview || len(cur.Items) != 1 || cur.Items[0].Change != "copy" || cur.Items[0].Name != "One" || cur.Items[0].At == 0 {
		t.Fatalf("use-installed preview: %+v", cur)
	}
	if s.Steps[1].Preview || s.Steps[1].Items[0].Old != "Old" || s.Steps[1].Items[0].Name != "New" {
		t.Fatalf("replace step: %+v", s.Steps[1])
	}
	if sub := s.Steps[3]; strings.Join(sub.WorkshopIDs, ",") != "7002" || sub.Items[0].Old != "Stale" {
		t.Fatalf("subscribe step: %+v", sub)
	}
	if len(s.Unfixable) != 1 || s.Unfixable[0].Name != "tester.lost" || s.Unfixable[0].Why == "" {
		t.Fatalf("unfixable: %+v", s.Unfixable)
	}
	if !list().Equal(sorted) {
		t.Fatal("a preview changed the list")
	}

	// Step 1 applies alone, as one undo step.
	r, err := api.ApplySortStep(view.StepUseInstalled)
	if err != nil || !r.Undo {
		t.Fatal(r, err)
	}
	afterCopy := list()
	if afterCopy.Equal(sorted) {
		t.Fatal("use-installed changed nothing")
	}

	// The next read starts at replace, previewed at Old's place.
	past := []string{view.StepUseInstalled}
	if s, err = api.SortSteps(past); err != nil || stepKinds(s) != "replace,enable,subscribe" || s.Current != 0 {
		t.Fatalf("%+v %v", s, err)
	}
	api.mu.Lock()
	st, _ := e.App.Loadout()
	oldAt := 0
	for i, x := range st.List {
		if m := st.Ix.ByID[x.Instance]; m != nil && m.PackageID == "tester.old" {
			oldAt = i + 1
		}
	}
	api.mu.Unlock()
	if it := s.Steps[0].Items[0]; !s.Steps[0].Preview || it.At != oldAt {
		t.Fatalf("replace preview at %d, Old at %d: %+v", it.At, oldAt, s.Steps[0])
	}

	// Skipping replace makes enable current; its preview names the requiring Mod.
	past = append(past, view.StepReplace)
	if s, err = api.SortSteps(past); err != nil || s.Current != 1 || s.Steps[1].Kind != view.StepEnable {
		t.Fatalf("%+v %v", s, err)
	}
	en := s.Steps[1]
	if !en.Preview || len(en.Items) != 1 || en.Items[0].Name != "Dep" || strings.Join(en.Items[0].For, ",") != "User" || en.Items[0].At == 0 {
		t.Fatalf("enable preview: %+v", en)
	}
	if !list().Equal(afterCopy) {
		t.Fatal("skipping changed the list")
	}

	// Enable applies; undo takes back that step only.
	if r, err = api.ApplySortStep(view.StepEnable); err != nil || !r.Undo {
		t.Fatal(r, err)
	}
	if list().Equal(afterCopy) {
		t.Fatal("enable changed nothing")
	}
	if _, err := api.Undo(); err != nil {
		t.Fatal(err)
	}
	if !list().Equal(afterCopy) {
		t.Fatal("undo did not restore the list before the enable step")
	}
	if s, err = api.SortSteps(past); err != nil || s.Steps[s.Current].Kind != view.StepEnable {
		t.Fatalf("after undo: %+v %v", s, err)
	}

	// The Workshop step is not a list edit, and a step with nothing left says so.
	if _, err := api.ApplySortStep(view.StepSubscribe); err == nil {
		t.Fatal("subscribe applied as a list step")
	}
	if r, err := api.ApplySortStep(view.StepUseInstalled); err != nil || r.Undo {
		t.Fatal(r, err)
	}
}

func TestSortStepsNothingToDo(t *testing.T) {
	_, api, _ := fixture(t)
	if _, err := api.CompleteDependencies(); err != nil {
		t.Fatal(err)
	}
	s, err := api.SortSteps(nil)
	if err != nil || len(s.Steps) != 0 || s.Current != -1 || len(s.Unfixable) != 0 {
		t.Fatalf("%+v %v", s, err)
	}
}

// Each installed name carries its instance for its details link, and a list entry that is not
// installed can be removed from the list from its row (16 §6.1; the delisted case is
// TestReadSortFixesDelisted in view).
func TestSortStepsLinksAndRemove(t *testing.T) {
	e, api := sortStepsFixture(t)
	if o, err := api.Run("sort"); err != nil || len(o.Refused) > 0 {
		t.Fatal(o, err)
	}
	s, err := api.SortSteps(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range s.Steps {
		for _, it := range st.Items {
			if st.Kind != view.StepSubscribe && it.ID == "" || it.Old != "" && it.OldID == "" {
				t.Fatalf("%s: a name without its instance: %+v", st.Kind, it)
			}
		}
	}
	if len(s.Unfixable) != 1 || s.Unfixable[0].Remove == "" {
		t.Fatalf("local-only entry: %+v", s.Unfixable)
	}
	e.MustRun("disable", s.Unfixable[0].Remove)
	if s, err = api.SortSteps(nil); err != nil || len(s.Unfixable) != 0 {
		t.Fatalf("removed entry still listed: %+v %v", s, err)
	}
}
