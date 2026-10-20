package view

import (
	"fmt"
	"testing"
)

func TestParseQuery(t *testing.T) {
	got := ParseQuery(`har -tag:ui author:"Brrainz X" id:#abc type:workshop bogus:x -`).terms
	want := []term{{"", "har", false}, {"tag", "ui", true}, {"author", "brrainz x", false}, {"id", "#abc", false}, {"type", "workshop", false}, {"", "bogus:x", false}, {"", "-", false}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}
