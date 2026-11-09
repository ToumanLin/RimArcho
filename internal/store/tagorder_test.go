package store_test

import (
	"fmt"
	"testing"
)

// A Mod's tags keep the order they were added in until the player reorders them; a merge
// puts the merged tag where the first source was and carries a colour over (16 §2.9).
func TestTagOrderAndColors(t *testing.T) {
	s, _ := openTemp(t)
	tags := func(lin string) string {
		pd, err := s.PlayerDataAll("")
		if err != nil {
			t.Fatal(err)
		}
		if pd[lin] == nil {
			return "[]"
		}
		return fmt.Sprint(pd[lin].Tags)
	}
	for _, tag := range []string{"Medieval", "framework", "Gravship"} {
		if _, err := s.AddTag("", "l1", tag); err != nil {
			t.Fatal(err)
		}
	}
	if got := tags("l1"); got != "[Medieval framework Gravship]" {
		t.Errorf("added order: %s", got)
	}
	if err := s.SetTagOrder("l1", []string{"gravship", "nope"}); err != nil {
		t.Fatal(err)
	}
	if got := tags("l1"); got != "[Gravship Medieval framework]" {
		t.Errorf("after reordering: %s", got)
	}
	if err := s.SetTagColor("gravship", "blue"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTagColor("Medieval", "#ff8800"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MergeTags([]string{"Gravship"}, "Space"); err != nil {
		t.Fatal(err)
	}
	if got := tags("l1"); got != "[Space Medieval framework]" {
		t.Errorf("after a rename: %s", got)
	}
	c, err := s.TagColors()
	if err != nil {
		t.Fatal(err)
	}
	if c["space"] != "blue" || c["gravship"] != "" || c["medieval"] != "#ff8800" {
		t.Errorf("colours after a rename: %v", c)
	}
	if _, err := s.DeleteTags([]string{"medieval"}); err != nil {
		t.Fatal(err)
	}
	if c, _ := s.TagColors(); c["medieval"] != "" {
		t.Errorf("a deleted tag keeps its colour: %v", c)
	}
	if err := s.SetTagColor("space", ""); err != nil {
		t.Fatal(err)
	}
	if c, _ := s.TagColors(); len(c) != 0 {
		t.Errorf("colours left: %v", c)
	}
}
