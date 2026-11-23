package view

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// One file is "1 file", not "1 files" (B4 VM round, Mod settings).
func TestModSettingsMessagesSingular(t *testing.T) {
	one := []app.SettingsCopy{{Mod: "Hats", From: "a.xml", To: "a.xml"}}
	two := append(one, app.SettingsCopy{Mod: "Bows", From: "b.xml", To: "b.xml"})
	for _, c := range []struct{ got, want string }{
		{SettingsCopyLines(&app.SettingsCopyPlan{From: "main", DryRun: true, Copies: one}).Message, "1 file to copy from main, 0 replacing a file here"},
		{SettingsCopyLines(&app.SettingsCopyPlan{From: "main", DryRun: true, Copies: two}).Message, "2 files to copy from main, 0 replacing a file here"},
		{SettingsCopyLines(&app.SettingsCopyPlan{From: "main", Copies: one}).Message, "copied 1 settings file from main"},
		{SettingsCopyLines(&app.SettingsCopyPlan{From: "main", Copies: two}).Message, "copied 2 settings files from main"},
		{SettingsRecycleText(&app.SettingsRecycle{Files: make([]app.ConfigFile, 1)}), "moved 1 settings file to the recycle bin"},
		{SettingsRecycleText(&app.SettingsRecycle{Files: make([]app.ConfigFile, 3)}), "moved 3 settings files to the recycle bin"},
		{replaceAt("C:/m", 1, "2 KB"), "replace at: C:/m (1 file, 2 KB)"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}
