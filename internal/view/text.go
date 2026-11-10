package view

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/ToumanLin/RimArcho/internal/l10ndata"
)

// Package view holds the shared interface models and text (16 §1).
// Interface keys are English; CLI output stays English (13 §1.1).
var selectedLocale atomic.Pointer[localeTable]

// Lang returns the selected locale directory name, or en for English.
func Lang() string {
	if table := selectedLocale.Load(); table != nil {
		return table.Name
	}
	return "en"
}

// Table returns the selected language's interface text. Callers must not modify it.
func Table() map[string]string {
	if table := selectedLocale.Load(); table != nil {
		return table.Text
	}
	return enText
}

// SetLang selects a locale, with ARCHO_LANG overriding the setting and auto using the OS.
// Each locale is parsed once, only when selected. English never parses Chinese resources.
func SetLang(v string) {
	if load, ok := locales[l10ndata.ResolveLang(v)]; ok {
		selectedLocale.Store(load())
		return
	}
	selectedLocale.Store(nil)
}

// T translates an interface string, falling back to its English source.
func T(s string) string {
	if table := selectedLocale.Load(); table != nil {
		if text, ok := table.Text[s]; ok {
			return text
		}
		if text, ok := table.ExtraText[s]; ok {
			return text
		}
		return s
	}
	if text, ok := enText[s]; ok {
		return text
	}
	return s
}

// enText is English the interface says differently from the text it is given: app and
// modchanges write these in the CLI's words (plan, build), which the CLI keeps for agents;
// the interface uses the player's (16 §7.2).
var enText = map[string]string{
	"RimArcho's link for this plan":          "RimArcho's link for this list",
	"a plan needs a name":                    "a list needs a name",
	"gone; the plan uses it":                 "gone; the list uses it",
	"in no plan":                             "in no list",
	"the current plan":                       "the current list",
	"the current plan since the last scan":   "the current list since the last scan",
	"the current plan since you last played": "the current list since you last played",
	"an unfinished build":                    "an unfinished pack",
	"the build was stopped":                  "stopped; what it copied is deleted",
}

// Tf translates a format string, then formats.
func Tf(f string, a ...any) string { return fmt.Sprintf(T(f), a...) }

// ExtraText holds translations for English keys replacing older mixed-language keys.
// Table retains every original key so existing callers and translation audits stay stable.
func ExtraText() map[string]string {
	table := selectedLocale.Load()
	if table == nil {
		return nil
	}
	extra := map[string]string{}
	for key, value := range table.ExtraText {
		if !strings.HasPrefix(key, "@") {
			extra[key] = value
		}
	}
	return extra
}
