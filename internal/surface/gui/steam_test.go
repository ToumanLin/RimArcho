package gui

import (
	"errors"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSteamWorkshop(t *testing.T) {
	_, api, _ := fixture(t)
	b := api.Board()
	var alpha, beta string
	for _, col := range b.Cols {
		for _, it := range col {
			if it.Name == "Alpha" {
				alpha = it.ID
			}
			if it.Name == "Beta" {
				beta = it.ID
			}
		}
	}
	if alpha == "" || beta == "" {
		t.Fatal("missing sample mods")
	}
	var opened string
	api.open = func(u string) error { opened = u; return nil }
	for _, ref := range []string{alpha, "ws:1001", "ws:5300"} {
		if _, err := api.OpenSteamWorkshop(ref); err != nil {
			t.Fatalf("%s: %v", ref, err)
		}
		want := "steam://url/CommunityFilePage/1001"
		if ref == "ws:5300" {
			want = "steam://url/CommunityFilePage/5300"
		}
		if opened != want {
			t.Fatalf("%s opened %q, want %q", ref, opened, want)
		}
	}
	for _, ref := range []string{beta, "unknown", "", "ws:", "ws:1/../../run/294100", "steam://run/294100"} {
		opened = ""
		if _, err := api.OpenSteamWorkshop(ref); err == nil || opened != "" {
			t.Fatalf("invalid reference %q: opened %q, error %v", ref, opened, err)
		}
	}
	launchErr := errors.New("Steam launch failed")
	api.open = func(string) error { return launchErr }
	if _, err := api.OpenSteamWorkshop(alpha); !errors.Is(err, launchErr) {
		t.Fatalf("lost launch error: %v", err)
	}
}

func TestDetailPageActions(t *testing.T) {
	e, api, _ := fixture(t)
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	var id, ref string
	for _, l := range lib.Lineages {
		for _, i := range l.Instances {
			if i.WorkshopID == "1001" && i.Mod != nil {
				id, ref = i.Mod.ID, i.Ref
			}
		}
	}
	if id == "" {
		t.Fatal("missing workshop mod")
	}
	detail, err := api.Detail(id, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := api.LibraryFields(ref)
	if err != nil {
		t.Fatal(err)
	}
	// The editor and library use the same pair of adjacent Workshop actions.
	for page, fs := range map[string][]view.Field{"editor": detail.Fields, "library": fields} {
		found := false
		for _, f := range fs {
			if f.Kind == view.FieldPath && f.ActAt != view.T("location") {
				t.Errorf("%s path label: %q", page, f.ActAt)
			}
			if f.Kind != view.FieldWorkshop {
				continue
			}
			found = true
			if f.Act != view.ActURL || f.ActAt != view.T("web page") || f.URL != view.WorkshopURL("1001") || len(f.Actions) != 1 || f.Actions[0].Act != view.ActSteam || f.Actions[0].Label != view.T("steam page") {
				t.Errorf("%s workshop actions: %+v", page, f)
			}
		}
		if !found {
			t.Errorf("%s has no workshop actions", page)
		}
	}
}
