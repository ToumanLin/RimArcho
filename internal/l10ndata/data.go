// Package l10ndata holds shared language data needed below the view layer.
package l10ndata

import (
	_ "embed"
	"encoding/json"
	"sync"
	"unicode"
)

// Keep the existing preset: saved tags and newly offered tags must still match.
//
//go:embed locale/zh/default-tags.txt
var DefaultPinnedTags string

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
