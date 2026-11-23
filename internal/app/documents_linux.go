package app

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// documentsDir is the player's Documents folder: XDG_DOCUMENTS_DIR from user-dirs.dirs,
// the file xdg-user-dir DOCUMENTS reads (localised names such as ~/Dokumente included), else
// ~/Documents.
func documentsDir() string {
	home, _ := os.UserHomeDir()
	config := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(config) {
		config = filepath.Join(home, ".config")
	}
	if p := userDirsDocuments(filepath.Join(config, "user-dirs.dirs"), home); p != "" {
		return p
	}
	return filepath.Join(home, "Documents")
}

// userDirsDocuments reads XDG_DOCUMENTS_DIR from a user-dirs.dirs file. Its values are
// "$HOME/<path>" or an absolute path in double quotes (xdg-user-dirs' format); anything
// else, or no entry, gives "".
func userDirsDocuments(file, home string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	out := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		value, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "XDG_DOCUMENTS_DIR=")
		if !ok || len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
			continue
		}
		value = strings.ReplaceAll(value[1:len(value)-1], `\"`, `"`)
		switch {
		case value == "$HOME" || strings.HasPrefix(value, "$HOME/"):
			if home == "" {
				continue
			}
			out = filepath.Join(home, strings.TrimPrefix(value, "$HOME"))
		case filepath.IsAbs(value):
			out = filepath.Clean(value)
		}
	}
	return out
}
