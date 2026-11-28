package gametest

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

func TestRunResetsVersionSpecificPreferences(t *testing.T) {
	o := testOptions(t, "idle", false)
	prefs, err := os.ReadFile("testdata/Prefs.xml")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(o.DataDir, "Config", "Prefs.xml"), string(prefs))
	writeTestFile(t, filepath.Join(o.DataDir, "Config", "keyprefs.XML"), "<KeyPrefs><unknown>1</unknown></KeyPrefs>")
	modSettings := filepath.Join(o.DataDir, "Config", "ModSettings.xml")
	writeTestFile(t, modSettings, "<Settings><keep>1</keep></Settings>")
	stopped := errors.New("synthetic stop before launching")
	o.Start = func(*exec.Cmd) error {
		if _, err := os.Lstat(filepath.Join(o.DataDir, "Config", "keyprefs.XML")); !os.IsNotExist(err) {
			t.Errorf("old key preferences reached launch: %v", err)
		}
		// The run's own Prefs.xml replaces the old one (writeRunPreferences).
		if b, err := os.ReadFile(filepath.Join(o.DataDir, "Config", "Prefs.xml")); err != nil ||
			strings.Contains(string(b), "windowPositionX") || strings.Contains(string(b), "devMode") {
			t.Errorf("old preferences reached launch: %q %v", b, err)
		}
		return stopped
	}
	if _, err := Run(context.Background(), o); !errors.Is(err, stopped) {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		modSettings: "<Settings><keep>1</keep></Settings>",
		filepath.Join(o.DataDir, "ShaderCache", "reused"): "cache",
	} {
		b, err := os.ReadFile(path)
		if err != nil || string(b) != want {
			t.Errorf("preserved file %s: %q %v", path, b, err)
		}
	}
}

func TestSettingsError(t *testing.T) {
	for _, tc := range []struct {
		message string
		want    bool
	}{
		{`XML error: <windowPositionX>-27</windowPositionX> doesn't correspond to any field in type PrefsData`, true},
		{`XML error: <windowPositionY>42</windowPositionY> doesn't correspond to any field in type Verse.PrefsData`, true},
		{`XML error: <scaleRelativeToSystem>True</scaleRelativeToSystem> doesn't correspond to any field in type PrefsData`, true},
		{`XML error: unknown field in type KeyPrefs`, true},
		{`XML error: unknown field in type KeyPrefsData`, true},
		{`XML error: unknown field in type ModPrefsData`, false},
		{`XML error: <PrefsData> isn't a field in ThingDef`, false},
		{`Exception reading Config/KeyPrefs.xml: invalid XML`, true},
		{`XML error: unknown field in type ThingDef`, false},
		{`Loaded PrefsData`, false},
	} {
		t.Run(tc.message, func(t *testing.T) {
			if got := SettingsError(&logs.Entry{Message: tc.message}); got != tc.want {
				t.Fatalf("SettingsError = %v, want %v", got, tc.want)
			}
		})
	}
	for _, e := range []*logs.Entry{
		{Message: "XML error: unknown field", Extra: []string{"type PrefsData"}},
		{Message: "XmlException: invalid configuration", Frames: []logs.Frame{{Type: "Verse.KeyPrefs", Method: "Load"}}},
	} {
		if !SettingsError(e) {
			t.Errorf("preference context was ignored: %+v", e)
		}
	}
}

// Every run writes Prefs.xml with runInBackground, so an unfocused window keeps loading;
// dev mode only in a save run, an autosave interval only with ticks (20 §2).
func TestRunPreferencesByKind(t *testing.T) {
	for _, tc := range []struct {
		name       string
		set        func(*Options)
		dev, ticks bool
	}{
		{"menu", func(*Options) {}, false, false},
		{"map", func(o *Options) { o.Map = true }, false, false},
		{"probe", func(o *Options) { o.Probe, o.Map = true, true }, false, false},
		{"map ticks", func(o *Options) { o.Map, o.Ticks = true, 3000 }, false, true},
		{"save", func(o *Options) { o.Save = "Colony.rws" }, true, false},
		{"save ticks", func(o *Options) { o.Save, o.Ticks = "Colony.rws", 3000 }, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			o := Options{DataDir: filepath.Join(base, "data"), RunDir: filepath.Join(base, "run")}
			tc.set(&o)
			if o.Save != "" {
				o.Save = filepath.Join(base, o.Save)
				writeTestFile(t, o.Save, "<savegame/>")
			}
			// An earlier run's or another version's preferences, which the reset removes.
			writeTestFile(t, filepath.Join(o.DataDir, "Config", "Prefs.xml"), "<PrefsData><devMode>True</devMode><autosaveIntervalDays>9</autosaveIntervalDays></PrefsData>")
			if err := prepareData(o); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(filepath.Join(o.DataDir, "Config", "Prefs.xml"))
			if err != nil {
				t.Fatal(err)
			}
			prefs := string(b)
			if !strings.Contains(prefs, "<runInBackground>True</runInBackground>") ||
				strings.Contains(prefs, "<devMode>True</devMode>") != tc.dev ||
				strings.Contains(prefs, "<autosaveIntervalDays>0.05</autosaveIntervalDays>") != tc.ticks ||
				strings.Count(prefs, "<autosaveIntervalDays>") > 1 || strings.Contains(prefs, ">9<") {
				t.Fatalf("prefs %s", prefs)
			}
		})
	}
}

// A log that stops growing while the game loads, or until a timeout, is a stall (20 §2).
func TestNoteStall(t *testing.T) {
	launch := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	at := func(s int) time.Time { return launch.Add(time.Duration(s) * time.Second) }
	g := &gameRun{launch: launch}
	g.noteStall(at(5), false) // nothing logged yet: quiet since launch
	g.tail.lastWrite = at(10)
	g.noteStall(at(10), false)
	g.noteStall(at(80), false)
	g.tail.lastWrite = at(81)
	g.noteStall(at(81), false)
	if g.stall != 70*time.Second {
		t.Fatalf("stall %v, want the 10-80 s quiet", g.stall)
	}
	// On the map the quiet log is arrival, not a stall, until a timeout ends the run.
	g = &gameRun{launch: launch}
	g.tail.newAt, g.tail.lastWrite = at(20), at(20)
	g.noteStall(at(200), false)
	if g.stall != 0 {
		t.Fatalf("quiet map counted: %v", g.stall)
	}
	g.noteStall(at(200), true)
	if g.stall != 180*time.Second {
		t.Fatalf("timeout quiet %v", g.stall)
	}
}
