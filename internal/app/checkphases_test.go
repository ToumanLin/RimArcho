package app

import (
	"reflect"
	"testing"
)

func TestCheckPhasesRoundTrip(t *testing.T) {
	p := CheckPhases{Game: "1.6", Active: 312, Total: 1354, Automatic: true}
	for _, ph := range []CheckPhase{{"read", 320}, {"defs", 1200}, {"patches", 3400}, {"harmony", 64_000}, {"save", 0}} {
		p.Add(ph.Name, ph.Ms)
	}
	d := p.Detail()
	if want := "1.6, 312/1354 Mods, automatic: read 320 ms, defs 1.2 s, patches 3.4 s, harmony 1 min 4 s, save 0 ms"; d != want {
		t.Fatalf("detail %q", d)
	}
	got, ok := ParseCheckPhases(d + "; disk full")
	if !ok || !reflect.DeepEqual(got, p) {
		t.Fatalf("parsed %+v %v", got, ok)
	}
	if _, ok := ParseCheckPhases("method direct"); ok {
		t.Fatal("parsed a launch detail")
	}
}
