package tui

import tea "github.com/charmbracelet/bubbletea"

// imeKeys: what a Chinese or Japanese input method in its own mode sends for the
// punctuation keys the TUI uses (shift+1-8, ?, :, /, \ and others). The keys are
// those of the US layout (16 9.1), so a player who left the input method on Chinese
// found shift+1-8, ? and : dead (VM round 2026-11-10). Text boxes get the key itself.
var imeKeys = map[string]string{
	"\uff01":       "!",
	"\uff20":       "@",
	"\uff03":       "#",
	"\uffe5":       "$", // Chinese
	"\u00a5":       "$",
	"\uff04":       "$", // Japanese
	"\uff05":       "%",
	"\u2026\u2026": "^", // shift+6, Chinese
	"\u2026":       "^", // shift+6 as two keys
	"\uff3e":       "^",
	"\uff06":       "&",
	"\uff0a":       "*",
	"\uff1f":       "?",
	"\uff1a":       ":",
	"\uff0f":       "/",
	"\u3001":       "\\", // the backslash key, Chinese
	"\uff3c":       "\\",
	"\uff08":       "(",
	"\uff09":       ")",
	"\u3010":       "[",
	"\u3011":       "]",
	"\uff0c":       ",",
	"\u3002":       ".",
	"\uff1b":       ";",
	"\u300a":       "<",
	"\u300b":       ">",
	"\uff5e":       "~",
}

// keyName is the key's name for the TUI's commands: k.String(), with an input method's
// full-width punctuation read as the US key it sits on.
func keyName(k tea.KeyMsg) string {
	s := k.String()
	if us, ok := imeKeys[s]; ok && !k.Paste {
		return us
	}
	return s
}
