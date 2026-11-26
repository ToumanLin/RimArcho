package l10ndata

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestGameConfigCopyTranslated(t *testing.T) {
	// Scan production copy: parity alone misses keys absent from both locales.
	literal := regexp.MustCompile(`\bTf?\("([^"\n]*)"`)
	var keys []string
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range literal.FindAllStringSubmatch(string(b), -1) {
			low := strings.ToLower(m[1])
			if strings.Contains(low, "game config") || strings.Contains(low, "game folder") || strings.Contains(low, "last run's log was not found") {
				keys = append(keys, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"zh", "ja", "ru", "ko"} {
		table := Locales[lang]()
		for _, key := range keys {
			if text := table.Text[key]; text == "" || text == key {
				t.Errorf("%s lacks Game Config copy %q", lang, key)
			}
		}
	}
}
