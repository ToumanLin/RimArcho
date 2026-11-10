// Package l10ndata holds shared language data needed below the view layer.
package l10ndata

import (
	"embed"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"unicode"
)

// englishPinnedTags is the English preset of common tags (16 §3.8).
const englishPinnedTags = "RimWorld,framework,visuals,apparel,races,weapons,buildings,furniture,QoL,balance,story,performance,language packs,testing,to delete"

// A locale may carry its own preset in default-tags.txt; without one it gets English.
//
//go:embed locale/*/default-tags.txt
var defaultTagFiles embed.FS

// DefaultPinnedTags is the preset of common tags for a language code (from ResolveLang).
func DefaultPinnedTags(lang string) string {
	if b, err := defaultTagFiles.ReadFile("locale/" + lang + "/default-tags.txt"); err == nil {
		return strings.TrimSpace(string(b))
	}
	return englishPinnedTags
}

// ResolveLang turns a ui.lang value into a bundled locale code, or en: ARCHO_LANG overrides
// the value and auto (or empty) uses the system language.
func ResolveLang(v string) string {
	if e := os.Getenv("ARCHO_LANG"); e != "" {
		v = e
	}
	if v == "" || v == "auto" {
		v = systemLang()
	}
	v = strings.ToLower(v)
	if strings.HasPrefix(v, "zh") {
		v = "zh"
	}
	if _, ok := Locales[v]; ok {
		return v
	}
	return "en"
}

//go:embed locale/en/legacy-accounts.json
var legacyAccounts string

var legacyEnglish = sync.OnceValue(func() map[string]string {
	var table map[string]string
	if err := json.Unmarshal([]byte(legacyAccounts), &table); err != nil {
		panic(err)
	}
	return table
})

// LegacyEnglish reads old account statuses and diagnostics without changing current text.
// Normal English calls never parse the compatibility data.
func LegacyEnglish(text string) string {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			if english, ok := legacyEnglish()[text]; ok {
				return english
			}
			break
		}
	}
	return text
}
