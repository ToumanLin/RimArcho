package view

import (
	"fmt"
	"testing"
)

func TestParseQuery(t *testing.T) {
	t.Parallel()
	got := ParseQuery(`har -tag:ui author:"Brrainz X" id:#abc type:workshop bogus:x -`).terms
	want := []term{{"", "har", false}, {"tag", "ui", true}, {"author", "brrainz x", false}, {"id", "#abc", false}, {"type", "workshop", false}, {"", "bogus:x", false}, {"", "-", false}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func TestParseListsQuery(t *testing.T) {
	t.Parallel()
	got := ParseQuery(`lists:"Main colony" -lists:"OTHER colony" lists:solo`).terms
	want := []term{{"lists", "main colony", false}, {"lists", "other colony", true}, {"lists", "solo", false}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	item := Item{Lists: []string{"Main colony", "Solo"}}
	for query, want := range map[string]bool{
		`lists:"MAIN COLONY" lists:solo`:    true,
		`lists:"Main colony" lists:missing`: false,
		`-lists:missing`:                    true,
		`-lists:"Main colony"`:              false,
		`lists:Main`:                        false,
		`lists:missing`:                     false,
	} {
		if got := ParseQuery(query).Match(&item, MatchFacts{}); got != want {
			t.Errorf("%s: got %v, want %v", query, got, want)
		}
	}
}
