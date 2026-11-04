package gui

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestUseInstalledAPIAndNeedsAction(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	root := filepath.Join(e.Root, "Extra")
	for _, name := range []string{"one", "two"} {
		e.WriteMod(filepath.Join(root, name), testkit.Sample("tester."+name, name))
	}
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.one", "tester.absent", "tester.two")
	e.AddEnv(false)
	if _, err := e.App.FolderAdd(source.Extra, root); err != nil {
		t.Fatal(err)
	}
	e.MustRun("scan")
	s := &API{a: e.App, needsClosed: true}
	before, _ := e.App.Loadout()
	head := e.App.Store.LastStepID(before.Plan.ID, "undo")
	var action view.Action
	checkNeeds := func(lang string) {
		t.Helper()
		view.SetLang(lang)
		s.needs = nil
		n := s.needsLocked()
		found, absent := false, false
		for _, item := range n.Items {
			if len(item.Actions) == 0 {
				continue
			}
			if item.Actions[0].Kind == view.ActUseInstalled {
				found = true
				action = item.Actions[0]
				want := "2 mods in the list can use copies already installed"
				if lang == "zh" {
					want = "2 个列表中的 Mod 可以使用已安装的副本"
				}
				if item.Text != want || !item.Counted || !item.Fold || len(item.Parts) != 2 {
					t.Fatal(item)
				}
				if action.Confirm != "" || action.Warn != "" || !action.Undo || strings.Join(action.Refs, ",") != "tester.one,tester.two" {
					t.Fatal(action)
				}
				for _, a := range item.Actions {
					if a.Kind == view.ActSubscribe {
						t.Fatal("installed copies offered subscription")
					}
				}
			} else if strings.Contains(item.Text, "tester.absent") {
				absent = true
				for _, a := range item.Actions {
					if a.Kind == view.ActDisable && strings.Join(a.Refs, ",") != "pkg:tester.absent" {
						t.Fatal(a)
					}
				}
			}
		}
		if !found || !absent {
			t.Fatalf("split absent: %v", n.Items)
		}
	}
	checkNeeds("zh")
	checkNeeds("en")
	// First edit via the direct frontend method; then undo and execute the needs action.
	r, err := s.UseInstalled(action.Refs)
	if err != nil || !r.Undo || r.Text != "using installed copies" || r.Problems == nil {
		t.Fatal(r, err)
	}
	for _, item := range r.Problems.Items {
		for _, a := range item.Actions {
			if a.Kind == view.ActUseInstalled {
				t.Fatal("result retained fixed problem")
			}
		}
	}
	raw, _ := json.Marshal(r)
	for _, field := range []string{`"text"`, `"undo":true`, `"problems"`} {
		if !strings.Contains(string(raw), field) {
			t.Fatal(string(raw))
		}
	}
	after, _ := e.App.Loadout()
	for i, entry := range before.List {
		if entry.Missing == "tester.one" || entry.Missing == "tester.two" {
			if after.List[i].Instance == "" {
				t.Fatalf("unbound position %d", i)
			}
		} else if after.List[i] != entry {
			t.Fatalf("position %d changed", i)
		}
	}
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	restored, _ := e.App.Loadout()
	if !restored.List.Equal(before.List) || e.App.Store.LastStepID(before.Plan.ID, "undo") != head {
		t.Fatal("undo did not restore list/history")
	}
	if done, err := s.DoAction(action, 0); err != nil || done != action.Done {
		t.Fatal(done, err)
	}
	redone, _ := e.App.Loadout()
	if !redone.List.Equal(after.List) {
		t.Fatal("needs action differs from direct API")
	}
}
