package view

import "github.com/ToumanLin/RimArcho/internal/l10ndata"

type localeTable = l10ndata.LocaleTable

var locales = l10ndata.Locales

func localeExtra(lang, key string) string { return l10ndata.Extra(lang, key) }

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
	labels := []string{T("follow system"), l10ndata.ChineseLanguageName, "English"}
	labels = append(labels, languageChoices()[3:]...)
	return labels
}
