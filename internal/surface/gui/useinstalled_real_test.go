package gui

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Opt-in acceptance on an external SQLite backup, never on the user's live home.
func TestUseInstalledRealCopy(t *testing.T) {
	home := os.Getenv("RIMARCHO_HOME")
	if home == "" {
		t.Skip("set RIMARCHO_HOME to an isolated backup with a use-installed-check-copy marker")
	}
	if _, err := os.Stat(filepath.Join(home, "use-installed-check-copy")); err != nil {
		t.Fatal("isolated backup marker required")
	}
	config, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(filepath.Clean(home), filepath.Join(config, "RimArcho")) {
		t.Fatal("refusing live user data")
	}
	a, err := app.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.LazyLog, a.LogCacheOnly = true, true
	a.HTTP = &http.Client{Transport: needsOffline{}}
	s := &API{a: a, needsClosed: true}
	view.SetLang("en")
	before, err := a.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	count := func(st *app.LoadoutState) int {
		n := 0
		for _, e := range st.List {
			if e.Missing != "" {
				n++
			}
		}
		return n
	}
	missing := a.MissingMods(before)
	kinds := map[string]int{}
	var packages []string
	for _, item := range missing {
		if !item.Installed || !item.CanEnable {
			t.Fatalf("expected an available installed copy: %+v", item)
		}
		m := before.Ix.ByID[item.Instance]
		if before.Reg.Resolve(item.PackageID).Instance != "" || before.Reg.Known(m.ID) || !before.Deployable(m) {
			t.Fatalf("unexpected resolution: %+v", item)
		}
		if _, err := os.Stat(filepath.Join(m.RealPath, "About", "About.xml")); err != nil {
			t.Fatal(err)
		}
		kinds[string(m.Kind)]++
		packages = append(packages, item.PackageID)
		if item.PackageID == "better.plants" {
			t.Logf("EVIDENCE package=%s instance=%s kind=%s source=%s path=%s lineage=%s registered=%v deployable=%v", item.PackageID, m.ID, m.Kind, m.SourceLabel, m.RealPath, m.LineageID, before.Reg.Known(m.ID), before.Deployable(m))
		}
	}
	if len(packages) == 0 {
		t.Fatal("no unresolved installed copies to verify")
	}
	logNeeds := func(phase string) int {
		t.Helper()
		n := s.needsLocked()
		cards := 0
		for _, item := range n.Items {
			if strings.Contains(item.Text, "mods in the list") || strings.Contains(item.Text, "not installed:") {
				t.Logf("%s needs: %s", phase, item.Text)
			}
			for _, act := range item.Actions {
				if act.Kind == view.ActUseInstalled {
					cards++
					t.Logf("%s action=%s refs=%d undo=%v confirm=%q", phase, act.Kind, len(act.Refs), act.Undo, act.Confirm)
				}
			}
		}
		return cards
	}
	t.Logf("BEFORE environment=%s plan=%q entries=%d enabled=%d missing=%d canEnable=%d kinds=%v", before.Ix.Env.Name, before.Plan.Name, len(before.List), len(before.List)-count(before), count(before), len(packages), kinds)
	if logNeeds("BEFORE") != 1 {
		t.Fatal("expected installed-copy needs item")
	}
	head := a.Store.LastStepID(before.Plan.ID, "undo")
	r, err := s.UseInstalled(packages)
	if err != nil || !r.Undo || r.Problems == nil {
		t.Fatal(r, err)
	}
	after, err := a.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if count(after) != 0 || len(after.List) != len(before.List) {
		t.Fatal("missing entries remain or positions changed")
	}
	for i, e := range before.List {
		if e.Missing != "" {
			if m := after.Ix.ByID[after.List[i].Instance]; m == nil || m.PackageID != e.Missing {
				t.Fatalf("wrong copy at %d", i)
			}
		} else if after.List[i] != e {
			t.Fatalf("position %d changed", i)
		}
	}
	t.Logf("AFTER entries=%d enabled=%d missing=%d result.text=%q undo=%v problems=%v", len(after.List), len(after.List)-count(after), count(after), r.Text, r.Undo, r.Problems != nil)
	if logNeeds("AFTER") != 0 {
		t.Fatal("fixed needs item remains")
	}
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	restored, err := a.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if !restored.List.Equal(before.List) || a.Store.LastStepID(before.Plan.ID, "undo") != head {
		t.Fatal("one undo did not restore exact list/history")
	}
	t.Logf("UNDO entries=%d enabled=%d missing=%d exactListAndHistory=true", len(restored.List), len(restored.List)-count(restored), count(restored))
	if logNeeds("UNDO") != 1 {
		t.Fatal("undo did not restore needs item")
	}
}
