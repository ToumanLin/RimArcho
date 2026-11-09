package view_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSaveContentContract(t *testing.T) {
	cat := app.SaveContentCategory{Key: "factions", Count: 1, Uses: 3, Hidden: 1, Names: []app.SaveContentName{{Name: "Clan", Uses: 3, Hidden: true}}}
	c := view.SaveContentOf(&app.SaveContentResult{Read: true, Defs: 1, Totals: []app.SaveContentCategory{cat}, Mods: []app.SaveContentMod{{Name: "A", PackageID: "tester.a", Library: "instance", Defs: 1, Cats: []app.SaveContentCategory{cat}}}, Unchecked: []string{"B"}})
	if len(c.Limits) != 4 || len(c.Totals[0].Names) != 0 || c.Mods[0].Cats[0].Names[0] != "Clan (hidden)" {
		t.Fatalf("contract: %+v", c)
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{`"read":true`, `"packageId":"tester.a"`, `"library":"instance"`, `"hidden":1`, `"uses":3`, `"unchecked":["B"]`} {
		if !strings.Contains(string(b), text) {
			t.Errorf("missing %s: %s", text, b)
		}
	}
	c = view.SaveContentOf(&app.SaveContentResult{Note: "This save has not been read yet."})
	b, err = json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"read":false,"note":"This save has not been read yet.","defs":0,"totals":[],"mods":[]}` {
		t.Fatalf("unread JSON: %s", b)
	}
}
