package tui

import "golang.org/x/sys/windows"

func systemLang() string {
	l, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil || len(l) == 0 {
		return "en"
	}
	return l[0]
}
