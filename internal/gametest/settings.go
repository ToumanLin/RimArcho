package gametest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

const ClassSettings = "settings"

// PreferenceFile identifies game settings whose schema can differ between versions.
func PreferenceFile(name string) bool {
	return strings.EqualFold(name, "Prefs.xml") || strings.EqualFold(name, "KeyPrefs.xml")
}

// Cached test data may contain preferences written by a different game version.
// Let the target game create its own defaults, including language and key bindings.
func resetPreferences(data string) error {
	cfg := filepath.Join(data, "Config")
	entries, err := os.ReadDir(cfg)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !PreferenceFile(entry.Name()) {
			continue
		}
		path := filepath.Join(cfg, entry.Name())
		if entry.IsDir() {
			return fmt.Errorf("gametest: preference path %q is a directory", path)
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("gametest: reset preferences %q: %w", path, err)
		}
	}
	return nil
}

var preferenceType = regexp.MustCompile(`\b(?:type|class|loading|reading)\s+(?:Verse\.)?(?:PrefsData|KeyPrefs(?:Data)?)\b|\b(?:Prefs|KeyPrefs)\.xml\b`)

// SettingsError identifies XML failures about the game's preference schemas.
func SettingsError(e *logs.Entry) bool {
	if logs.Classify(e) != logs.ClassXMLLoad {
		return false
	}
	if preferenceType.MatchString(e.Message + "\n" + strings.Join(e.Extra, "\n")) {
		return true
	}
	for _, frame := range e.Frames {
		switch strings.TrimPrefix(frame.Type, "Verse.") {
		case "PrefsData", "KeyPrefs", "KeyPrefsData":
			return true
		}
	}
	return false
}
