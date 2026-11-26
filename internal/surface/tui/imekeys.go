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

// The Russian (JCUKEN) layout puts Cyrillic letters on the letter keys and a few
// punctuation keys; each is read as the US key it sits on (ruKeys, same order as usKeys).
// Its punctuation that is itself ASCII (shift+2 sends ", shift+4 ;) cannot be told apart
// from the US key and stays as typed.
const (
	ruKeys = "\u0439\u0446\u0443\u043a\u0435\u043d\u0433\u0448\u0449\u0437\u0445\u044a\u0444\u044b\u0432\u0430\u043f\u0440\u043e\u043b\u0434\u0436\u044d\u044f\u0447\u0441\u043c\u0438\u0442\u044c\u0431\u044e\u0451\u0419\u0426\u0423\u041a\u0415\u041d\u0413\u0428\u0429\u0417\u0425\u042a\u0424\u042b\u0412\u0410\u041f\u0420\u041e\u041b\u0414\u0416\u042d\u042f\u0427\u0421\u041c\u0418\u0422\u042c\u0411\u042e\u0401\u2116"
	usKeys = "qwertyuiop[]asdfghjkl;'zxcvbnm,.`QWERTYUIOP{}ASDFGHJKL:\"ZXCVBNM<>~#"
)

func init() {
	us := []rune(usKeys)
	for i, r := range []rune(ruKeys) {
		imeKeys[string(r)] = string(us[i])
	}
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
