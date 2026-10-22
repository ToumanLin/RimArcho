package view

import "github.com/ToumanLin/RimArcho/internal/app"

// LaunchHint is the most likely trouble of a launch (06 §2.3): the first code of
// app.LaunchChecks (most predictive first, 03 §4.3.1) that has a counted item in 能不能跑, in
// that item's words, with the check's one-line reason from the log corpus. It never blocks
// a launch; it says where to look. Only items 能不能跑 shows are used, so following the hint
// always lands on something the player can act on.
type LaunchHint struct {
	Text  string `json:"text"`  // the item's title, "… and 2 more like it" when there are several
	Why   string `json:"why"`   // the check's reason, with the corpus numbers
	Code  string `json:"code"`  // the problem code (for tests and the CLI's wording)
	Count int    `json:"count"` // counted items of this code
}

func launchHint(items []Need) *LaunchHint {
	for _, c := range app.LaunchChecks {
		var first *Need
		n := 0
		for i := range items {
			it := &items[i]
			if it.code != c.Code || !it.Counted || SectionOf(it.Group) != SectionBreak {
				continue
			}
			if first == nil {
				first = it
			}
			n++
		}
		if first == nil {
			continue
		}
		text := first.Title
		if text == "" {
			text = first.Text
		}
		if n > 1 {
			text = Tf("%s, and %d more like it", text, n-1)
		}
		return &LaunchHint{Text: text, Why: T(c.Why), Code: c.Code, Count: n}
	}
	return nil
}
