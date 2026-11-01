package l10ndata

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
)

//go:embed locale/*/*.json
var localeFiles embed.FS

//go:embed locale/zh/name.txt
var ChineseLanguageName string

type LocaleTable struct {
	Name      string
	Text      map[string]string
	ExtraText map[string]string
	// clashes keeps both values of conflicting keys, including duplicates within a file.
	Clashes map[string][2]string
}

// Discover directory names without reading or parsing their JSON at startup.
// A new language only needs a new locale directory with feature JSON files.
var Locales = func() map[string]func() *LocaleTable {
	dirs, err := fs.ReadDir(localeFiles, "locale")
	if err != nil {
		panic(err)
	}
	out := make(map[string]func() *LocaleTable, len(dirs))
	for _, dir := range dirs {
		if !dir.IsDir() || dir.Name() == "en" {
			continue
		}
		name := dir.Name()
		out[name] = sync.OnceValue(func() *LocaleTable {
			table, err := readLocale(localeFiles, "locale/"+name)
			if err != nil {
				panic(err)
			}
			table.Name = name
			return table
		})
	}
	return out
}()

func Extra(lang, key string) string {
	if load, ok := Locales[lang]; ok {
		if text, ok := load().ExtraText[key]; ok {
			return text
		}
	}
	return key
}

func readLocale(files fs.FS, dir string) (*LocaleTable, error) {
	entries, err := fs.ReadDir(files, dir)
	if err != nil {
		return nil, err
	}
	table := &LocaleTable{Text: map[string]string{}, ExtraText: map[string]string{}, Clashes: map[string][2]string{}}
	seen := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		file := path.Join(dir, entry.Name())
		data, err := fs.ReadFile(files, file)
		if err != nil {
			return nil, err
		}
		target := table.Text
		if strings.HasPrefix(entry.Name(), "_") {
			target = table.ExtraText
		}
		part := map[string]string{}
		if err := readLocaleObject(data, part, table.Clashes); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		for key, value := range part {
			if old, exists := seen[key]; exists && old != value {
				table.Clashes[key] = [2]string{old, value}
			}
			seen[key], target[key] = value, value
		}
	}
	return table, nil
}

// Decode pairs individually: decoding straight into a map would hide duplicate JSON keys.
func readLocaleObject(data []byte, target map[string]string, clashes map[string][2]string) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return fmt.Errorf("expected a translation object")
	}
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return fmt.Errorf("expected a string key")
		}
		token, err = dec.Token()
		if err != nil {
			return err
		}
		value, ok := token.(string)
		if !ok {
			return fmt.Errorf("expected a string translation for %q", key)
		}
		if old, exists := target[key]; exists && old != value {
			clashes[key] = [2]string{old, value}
		}
		target[key] = value
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("unexpected data after translation object: %v", err)
	}
	return nil
}

// LanguageCodes is shared by settings validation and the interface; no JSON is parsed.
func LanguageCodes() []string {
	codes := []string{"en"}
	for code := range Locales {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return append([]string{"auto"}, codes...)
}
