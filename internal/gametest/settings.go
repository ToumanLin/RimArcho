package gametest

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

// prepareSaves removes what a save left by an earlier run would start instead of this
// run's target (autostart.rws, loaded at startup in dev mode) or mistake for this run's
// autosave, then copies save, if any, in as autostart.rws (20 §2). Other files stay.
func prepareSaves(dir, save string) error {
	old, err := filepath.Glob(filepath.Join(dir, "Autosave*.rws"))
	if err != nil {
		return err
	}
	for _, path := range append(old, filepath.Join(dir, "autostart.rws")) {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("gametest: remove %q: %w", path, err)
		}
	}
	if save == "" {
		return nil
	}
	in, err := os.Open(save)
	if err != nil {
		return fmt.Errorf("gametest: read save: %w", err)
	}
	defer in.Close()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	out, err := os.Create(filepath.Join(dir, "autostart.rws"))
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in) // saves reach 100 MB: no whole copy in memory
	return errors.Join(err, out.Close())
}

// writeRunPreferences writes the Prefs.xml of a run that plays (20 §2): a small window,
// running unfocused, never pausing by itself, English, no sound; dev mode for a save run
// (the game loads autostart.rws only in dev mode); and, with ticks, an autosave interval
// of that many ticks (autosaveIntervalDays × 60,000), so the first autosave marks them
// played. The skill's sandbox writes the same window and pause settings (20 §4).
func writeRunPreferences(data string, devMode bool, ticks int) error {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<PrefsData>
  <screenWidth>1280</screenWidth>
  <screenHeight>720</screenHeight>
  <fullscreen>False</fullscreen>
  <uiScale>1</uiScale>
  <runInBackground>True</runInBackground>
  <pauseOnLoad>False</pauseOnLoad>
  <pauseOnError>False</pauseOnError>
  <automaticPauseMode>Never</automaticPauseMode>
  <adaptiveTrainingEnabled>False</adaptiveTrainingEnabled>
  <langFolderName>English</langFolderName>
  <resetModsConfigOnCrash>False</resetModsConfigOnCrash>
  <volumeMaster>0</volumeMaster>
`)
	if devMode {
		b.WriteString("  <devMode>True</devMode>\n")
	}
	if ticks > 0 {
		fmt.Fprintf(&b, "  <autosaveIntervalDays>%s</autosaveIntervalDays>\n", strconv.FormatFloat(float64(ticks)/60000, 'f', -1, 64))
	}
	b.WriteString("</PrefsData>\n")
	return os.WriteFile(filepath.Join(data, "Config", "Prefs.xml"), []byte(b.String()), 0o644)
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
