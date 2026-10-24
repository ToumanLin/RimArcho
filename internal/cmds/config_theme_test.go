package cmds_test

import (
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"slices"
	"testing"
)

func TestThemeSetting(t *testing.T) {
	e := testkit.New(t)
	choices := []string{"auto", "light", "dark"}
	if !slices.Equal(cmds.SettingChoices("ui.theme"), choices) || cmds.Setting(e.App, "ui.theme") != "auto" {
		t.Fatal("unexpected theme choices/default")
	}
	for _, value := range choices {
		e.MustRun("config", "set", "ui.theme", value)
		e.Open()
		if got := cmds.Setting(e.App, "ui.theme"); got != value {
			t.Fatalf("reopened theme: %q, want %q", got, value)
		}
	}
	for _, value := range []string{"", "Dark", "system", "invalid"} {
		if _, code := e.Run("config", "set", "ui.theme", value); code != cmds.ExitUsage {
			t.Errorf("invalid theme %q: exit %d", value, code)
		}
		if got := cmds.Setting(e.App, "ui.theme"); got != "dark" {
			t.Fatalf("invalid theme overwrote saved value: %q", got)
		}
	}
	e.MustRun("config", "set", "ui.theme", "auto")
	if got := cmds.Setting(e.App, "ui.theme"); got != "auto" {
		t.Fatalf("restore auto: %q", got)
	}
}
