package view

import (
	"strconv"
	"strings"
	"testing"
)

// Russian counts take one of three noun forms by the number (1 and 21; 2–4 and 22; 5–20 and 0).
func TestRussianPluralForms(t *testing.T) {
	t.Setenv("ARCHO_LANG", "")
	old := Lang()
	defer SetLang(old)
	SetLang("ru")
	noun := func(n int) string { return strings.TrimPrefix(Plural(n, "Mod"), strconv.Itoa(n)+" ") }
	one, few, many := noun(1), noun(3), noun(5)
	if one == few || few == many || one == many || one == T("Mod") && few == T("Mod") {
		t.Fatalf("forms not distinct: %q %q %q", one, few, many)
	}
	for n, want := range map[int]string{0: many, 2: few, 4: few, 11: many, 12: many, 14: many, 21: one, 22: few, 25: many, 101: one, 111: many} {
		if got := noun(n); got != want {
			t.Errorf("%d: %q, want %q", n, got, want)
		}
	}
	if Plural(1, "Mod") != Plural(1, "mod") {
		t.Error("Mod and mod count alike")
	}
	SetLang("ko")
	if got := Plural(3, "mod"); !strings.Contains(got, "3") || strings.HasPrefix(got, "3 mod") {
		t.Errorf("ko count %q", got)
	}
}
