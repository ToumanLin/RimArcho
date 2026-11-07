package view

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ToumanLin/RimArcho/internal/l10ndata"
)

func TestTips(t *testing.T) {
	t.Setenv("ARCHO_LANG", "")
	before := Lang()
	t.Cleanup(func() { SetLang(before) })
	SetLang("en")
	english := Tips()
	if len(english) < 30 || len(english) > 40 {
		t.Fatalf("got %d tips, want 30 to 40", len(english))
	}
	seen := map[string]bool{}
	first := 0
	for _, tip := range english {
		if strings.TrimSpace(tip.Text) == "" || utf8.RuneCountInString(tip.Text) > 90 {
			t.Errorf("empty or overlong English tip: %q", tip.Text)
		}
		if seen[tip.Text] {
			t.Errorf("duplicate English tip: %q", tip.Text)
		}
		seen[tip.Text] = true
		switch tip.When {
		case "first":
			first++
		case "any":
		default:
			t.Errorf("invalid context %q for %q", tip.When, tip.Text)
		}
	}
	if first < 6 || first > 10 {
		t.Errorf("got %d first-run tips, want 6 to 10", first)
	}

	SetLang("zh")
	chinese := Tips()
	if len(chinese) != len(english) {
		t.Fatalf("translated tip count = %d, want %d", len(chinese), len(english))
	}
	translations := l10ndata.Locales["zh"]().Text
	seen = map[string]bool{}
	for i, tip := range chinese {
		source := english[i]
		translation, ok := translations[source.Text]
		if !ok || strings.TrimSpace(translation) == "" || translation == source.Text {
			t.Errorf("missing Chinese translation for %q", source.Text)
		}
		if tip.Text != translation || tip.When != source.When {
			t.Errorf("translated tip %d = %+v, want text %q and context %q", i, tip, translation, source.When)
		}
		if seen[tip.Text] {
			t.Errorf("duplicate Chinese tip: %q", tip.Text)
		}
		seen[tip.Text] = true
	}
}
