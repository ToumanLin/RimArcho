package view

import "github.com/ToumanLin/RimArcho/internal/l10ndata"

type localeTable = l10ndata.LocaleTable

var locales = l10ndata.Locales

func localeExtra(lang, key string) string { return l10ndata.Extra(lang, key) }

// localeFormat is the current locale's supplemental format (dates, counts); English has none.
func localeFormat(key string) (string, bool) {
	if lang := Lang(); lang != "en" {
		if format := localeExtra(lang, key); format != key {
			return format, true
		}
	}
	return "", false
}

func languageChoices() []string {
	choices := []string{"auto", "zh", "en"}
	for _, code := range l10ndata.LanguageCodes() {
		if code != "auto" && code != "zh" && code != "en" {
			choices = append(choices, code)
		}
	}
	return choices
}

func languageLabels() []string {
	labels := []string{T("follow system"), l10ndata.LanguageName("zh"), "English"}
	for _, code := range languageChoices()[3:] {
		labels = append(labels, l10ndata.LanguageName(code))
	}
	return labels
}
