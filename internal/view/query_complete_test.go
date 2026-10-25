package view

import "testing"

func TestTagMatch(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		tag, v string
		want   bool
	}{
		{"Facial Hair", "facial", true},
		{"Facial Animation", "Facial", true},
		{"Facial Animation", "animation", true},
		{"Facial Animation", "anim", true},
		{"Facial Hair", "facial hair", true},
		{"Facial Hair", "air", false},
		{"Mod Library", "library", true},
		{"UI", "ui", true},
		{"Quality-of-Life", "life", true},
		{"Hygiene", "", false},
	} {
		if got := TagMatch(c.tag, c.v); got != c.want {
			t.Errorf("TagMatch(%q, %q) = %v, want %v", c.tag, c.v, got, c.want)
		}
	}
	// the query: tag:Facial finds both Facial tags, tag:Animation only the one
	items := []Item{{Name: "a", Tags: []string{"Facial Hair"}}, {Name: "b", Tags: []string{"Facial Animation"}}, {Name: "c", Tags: []string{"Hair"}}}
	for q, want := range map[string]int{"tag:Facial": 2, "tag:animation": 1, "tag:hair": 2, "-tag:facial": 1, `tag:"facial hair"`: 1} {
		if got := len(ParseQuery(q).Filter(items, MatchFacts{})); got != want {
			t.Errorf("%s: %d matches, want %d", q, got, want)
		}
	}
}

func TestCompleteQuery(t *testing.T) {
	t.Parallel()
	tags := []string{"Facial Hair", "Facial Animation", "Hygiene", "Hair Style"}
	for q, want := range map[string]string{
		"au":               "thor:",
		"a":                "",
		"harmony au":       "thor:",
		"-ta":              "g:",
		"t":                "",
		"ty":               "pe:",
		"na":               "me:",
		"author:":          "",
		"type:wo":          "rkshop",
		"has:pr":           "oblem",
		"tag:fac":          "ial",
		"tag:Fac":          "ial",
		"tag:hai":          "r",
		"tag:facial":       "",
		"zz":               "",
		"au ":              "",
		"":                 "",
		`name:"half op`:    "",
		"bogus:x":          "",
		"author:brrainz x": "",
	} {
		if got := CompleteQuery(q, tags); got != want {
			t.Errorf("CompleteQuery(%q) = %q, want %q", q, got, want)
		}
	}
}
