package view

import "testing"

func TestEnglishDoesNotLoadChinese(t *testing.T) {
	t.Setenv("ARCHO_LANG", "")
	before, selected := locales, selectedLocale.Load()
	locales = map[string]func() *localeTable{"zh": func() *localeTable { t.Fatal("English parsed Chinese resources"); return nil }}
	t.Cleanup(func() { locales = before; selectedLocale.Store(selected) })
	SetLang("en")
	if T("loading...") != "loading..." || Table() == nil || Lang() != "en" || ExtraText() != nil {
		t.Fatal("English fallback changed")
	}
}

func TestEnglishKeysRetainChineseTranslations(t *testing.T) {
	t.Setenv("ARCHO_LANG", "")
	before := Lang()
	t.Cleanup(func() { SetLang(before) })
	SetLang("zh")
	extras := ExtraText()
	for key, value := range extras {
		if T(key) != value || value == key {
			t.Errorf("untranslated supplemental key %q", key)
		}
	}
	for _, command := range Commands {
		if command.Name() == command.En || command.Name() == command.Key {
			t.Errorf("untranslated palette command %s", command.ID)
		}
	}
}
