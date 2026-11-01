package view

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// The Chinese replacements live under @retired in the language resources (06 §7.2).
// retiredEn are the English words the interface no longer uses: list, game, a newer
// version, switch version, locked, go back to a version, and the
// light / full pack (a pack is always full; its list alone is a RimArcho list).
var retiredEn = regexp.MustCompile(`(?i)\b(plans?|environments?|upstream|workspace|swap(s|ped)?|pinned|not written|checkout|light packs?|full packs?)\b`)

// wordingExempt are texts whose Chinese is right although it holds a retired word.
var wordingExempt = map[string]bool{
	"decompiled C#":                     true, // decompilation
	"decompile the current list's mods": true,
	"decompiling…":                      true,
	"%d assemblies are not decompiled yet: search again to go on":            true,
	"the mod was built for another version of the game or of the target mod": true, // a DLL compiled for another version
}

// WordingTextProblems checks text from surface-specific literals as well as tables.
func WordingTextProblems(where, text string) []string {
	if strings.ContainsRune(text, '\u00b7') {
		return []string{fmt.Sprintf("%s: %q uses a middle dot (06 §7.3)", where, text)}
	}
	return nil
}

// WordingProblems lists every interface text that still uses a retired word or middle dot, and every key
// two tables translate differently (the later table wins and the other text is lost). It
// sees the tables of every package linked in, so call it where all surfaces are.
func WordingProblems() []string {
	zh := locales["zh"]()
	var out []string
	check := func(where, zh string) {
		for old, replacement := range locales["zh"]().ExtraText {
			if !strings.HasPrefix(old, "@retired.") {
				continue
			}
			old = strings.TrimPrefix(old, "@retired.")
			if strings.Contains(zh, old) {
				out = append(out, fmt.Sprintf("%s: %q uses %q (write %q)", where, zh, old, replacement))
			}
		}
	}
	checkEn := func(where, en string) {
		if w := retiredEn.FindString(en); w != "" {
			out = append(out, fmt.Sprintf("%s: the English %q uses %q", where, en, w))
		}
	}
	for k, v := range zh.Text {
		out = append(out, WordingTextProblems("key", k)...)
		out = append(out, WordingTextProblems(k, v)...)
		if !wordingExempt[k] {
			check(fmt.Sprintf("%q", k), v)
		}
		en := k
		if e, ok := enText[k]; ok {
			en = e
		}
		checkEn("key", en)
	}
	for key, value := range zh.ExtraText {
		if strings.HasPrefix(key, "@") {
			continue
		}
		out = append(out, WordingTextProblems("key", key)...)
		out = append(out, WordingTextProblems(key, value)...)
		check(key, value)
		checkEn("key", key)
	}
	for k, v := range enText {
		out = append(out, WordingTextProblems("English key", k)...)
		out = append(out, WordingTextProblems(k, v)...)
		checkEn(k, v)
	}
	for _, c := range Commands {
		out = append(out, WordingTextProblems("palette "+c.ID, localeExtra("zh", c.Key))...)
		out = append(out, WordingTextProblems("palette "+c.ID, c.En)...)
		check("palette "+c.ID, localeExtra("zh", c.Key))
		checkEn("palette "+c.ID, c.En)
	}
	for name, load := range locales {
		for k, v := range load().Clashes {
			out = append(out, fmt.Sprintf("%s: %q is translated twice: %q and %q", name, k, v[0], v[1]))
		}
	}

	sort.Strings(out)
	return out
}
