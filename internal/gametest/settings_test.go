package gametest

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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
		for _, name := range []string{"Prefs.xml", "keyprefs.XML"} {
			if _, err := os.Lstat(filepath.Join(o.DataDir, "Config", name)); !os.IsNotExist(err) {
				t.Errorf("old preference %s reached launch: %v", name, err)
			}
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
