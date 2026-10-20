//go:build !windows

package view

import "os"

func systemLang() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "en"
}
