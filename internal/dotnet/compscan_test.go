package dotnet

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestCompEdits(t *testing.T) {
	a, err := Open(filepath.Join("..", "..", "testdata", "dotnet", "bin", "ModCompRemover.dll"))
	if err != nil {
		t.Fatal(err)
	}
	got := a.CompEdits()
	want := []CompEdit{
		{Type: "CompRemover.CompSelfRemover", Method: "CompTick", Ops: []string{"GenList.Swap", "AllComps.Remove"}, Tick: "CompTick"},
		{Type: "CompRemover.Gizmo", Method: "Attach", Ops: []string{"AllComps.Add"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CompEdits\n got %+v\nwant %+v", got, want)
	}
	if cs := a.CompClassesSet(); cs["CompRemover.CompProperties_Remover"] != "CompRemover.CompSelfRemover" {
		t.Errorf("CompClassesSet %v", cs)
	}
	// The thrower only reads the list: no edits, but its prefix enumerates AllComps.
	b, err := Open(filepath.Join("..", "..", "testdata", "dotnet", "bin", "ModCompThrower.dll"))
	if err != nil {
		t.Fatal(err)
	}
	if e := b.CompEdits(); len(e) != 0 {
		t.Errorf("thrower edits %+v", e)
	}
	for _, typ := range []string{"CompThrower.Tick_Patch"} {
		if got := b.Enumerated(typ, "Prefix"); !reflect.DeepEqual(got, []string{"Verse.ThingWithComps.AllComps"}) || !IsCompList(got[0]) {
			t.Errorf("Enumerated(%s) = %v", typ, got)
		}
	}
	if got := b.Enumerated("CompThrower.Nope", "Prefix"); got != nil {
		t.Errorf("unknown type: %v", got)
	}
}
