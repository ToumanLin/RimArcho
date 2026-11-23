package view_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestReviewPageOf(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.WriteSave("Fort.rws", testkit.SaveSpec{Faction: "Fort", Seed: "alpha", PRV: 1, Ticks: 12 * 60000})
	l, err := e.App.ListSaves()
	if err != nil || len(l.Colonies) != 1 {
		t.Fatalf("colonies: %+v, %v", l, err)
	}
	if _, err := e.App.SaveFacts(l.Colonies[0].Latest(), true); err != nil {
		t.Fatal(err)
	}
	l, err = e.App.ListSaves()
	if err != nil {
		t.Fatal(err)
	}
	r := &app.SaveReview{Colonies: 1, Saves: 1, Done: 3, Total: 7, Mods: []app.ReviewMod{
		{Name: "Quests and buildings", PackageID: "tester.mixed", Library: "mixed-copy", Colonies: 1, Saves: 1, Adds: 8, Seen: 5,
			Cats: []app.ReviewCat{
				{Key: "quests", Adds: 2, Seen: 1, Under: true, NotSeen: []string{"Quest B"}},
				{Key: "buildings", Adds: 6, Seen: 4, NotSeen: []string{"Bench", "Lamp"}},
			}},
		{Name: "Buildings", PackageID: "tester.buildings", Library: "building-copy", Colonies: 1, Saves: 1, Adds: 3,
			Cats: []app.ReviewCat{{Key: "buildings", Adds: 3, NotSeen: []string{"Wall", "Door", "Table"}}}},
		{Name: "Menus", PackageID: "tester.menus", Library: "menu-copy", Functional: true},
	}}
	tags := []string{"Adventure", "Favourites"}
	p := view.ReviewPageOf(r, l.Colonies, l.Colonies[0].Key, map[string][]string{"mixed-copy": tags})
	if p.Done != 3 || p.Total != 7 || p.Colony != l.Colonies[0].Key || len(p.Colonies) != 1 || p.Colonies[0].Key != p.Colony ||
		p.Colonies[0].Name != "Fort" || p.Colonies[0].Line != "day 12, 1 save" || p.All != "1 colony, 1 save" {
		t.Fatalf("colony cards: %+v", p)
	}
	if len(p.Groups) != 2 || p.Groups[0].Key != "meet" || p.Groups[1].Key != "rest" ||
		len(p.Groups[0].Mods) != 1 || len(p.Groups[1].Mods) != 1 {
		t.Fatalf("map groups: %+v", p.Groups)
	}
	m := p.Groups[0].Mods[0]
	if m.PackageID != "tester.mixed" || m.Size != 2 || m.Part != 1 || m.Adds != 8 || m.Seen != 5 ||
		m.Sum != "5 of 8 seen" || m.Where != "1 colony, 1 save" || m.Tip != "quests 1/2" || m.Main != "buildings" ||
		!reflect.DeepEqual(m.Tags, tags) {
		t.Errorf("mixed Mod: %+v", m)
	}
	if len(m.Cats) != 2 || m.Cats[0].Label != "buildings" || m.Cats[1].Label != "quests" ||
		!m.Cats[1].Under || !reflect.DeepEqual(m.Cats[1].Names, []string{"Quest B"}) {
		t.Errorf("categories by amount added: %+v", m.Cats)
	}
	b := p.Groups[1].Mods[0]
	if b.PackageID != "tester.buildings" || b.Size != 3 || b.Part != 0 || b.Tip != "0 of 3 seen" ||
		p.Groups[1].Hint != "1 with nothing in the saves (outlined)" {
		t.Errorf("buildings Mod: %+v; hint: %s", b, p.Groups[1].Hint)
	}
	if !strings.HasPrefix(p.Foot, "1 interface, performance or fix Mod") || strings.Contains(p.Foot, "Menus") {
		t.Errorf("functional count: %q", p.Foot)
	}
	if empty := view.ReviewPageOf(&app.SaveReview{}, nil, "", nil); len(empty.Groups) != 0 || empty.Foot != "" || len(empty.Colonies) != 0 {
		t.Errorf("empty review: %+v", empty)
	}
}
