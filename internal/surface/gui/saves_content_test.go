package gui

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestSaveContentAPI(t *testing.T) {
	e, api, _ := fixture(t)
	e.WriteSave("Content.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.a"}, Body: `<x><def>UnknownContent</def></x>`})
	l, err := e.App.ListSaves()
	if err != nil {
		t.Fatal(err)
	}
	api.saves.mu.Lock()
	api.saves.list = l
	api.saves.mu.Unlock()
	v, err := api.SaveContent("Content")
	if err != nil || v.Read || v.Note == "" {
		t.Fatalf("unread: %+v %v", v, err)
	}
	s, err := l.Find("Content")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.SaveFacts(s, true); err != nil {
		t.Fatal(err)
	}
	v, err = api.SaveContent(s.Ref)
	if err != nil || !v.Read || v.Unknown == nil || v.Unknown.Count != 1 || len(v.Limits) != 4 {
		t.Fatalf("read: %+v %v", v, err)
	}
}
