package view

import "testing"

// Every edit name the app records reads in Chinese in the history.
func TestEditNamesTranslated(t *testing.T) {
	SetLang("zh")
	defer SetLang("en")
	for _, op := range []string{"complete dependencies and sort", "replace recommended mods", "use installed copies", "sort"} {
		w := op
		if s, ok := stepWords[op]; ok {
			w = s
		}
		if T(w) == w {
			t.Errorf("%q has no Chinese", op)
		}
	}
}
