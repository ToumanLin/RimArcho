package cmds_test

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestAccentSetting(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	values := cmds.SettingChoices("ui.accent")
	if len(values) < 9 || values[0] != "default" || cmds.Setting(e.App, "ui.accent") != "default" {
		t.Fatalf("accent choices/default: %v", values)
	}
	for _, value := range values {
		e.MustRun("config", "set", "ui.accent", value)
		if got := cmds.Setting(e.App, "ui.accent"); got != value {
			t.Fatalf("set %s, got %s", value, got)
		}
	}
	e.MustRun("config", "set", "ui.accent", "blue")
	for _, value := range []string{"", "Blue", "not-a-colour", "0090ff", "#0090f", "#0090ff0", "#gg0000", "#fff"} {
		if _, code := e.Run("config", "set", "ui.accent", value); code != cmds.ExitUsage {
			t.Errorf("invalid accent %q: exit %d", value, code)
		}
		if got := cmds.Setting(e.App, "ui.accent"); got != "blue" {
			t.Fatalf("invalid accent overwrote the saved value: %q", got)
		}
	}
	// a custom accent is one #rrggbb colour, stored in lower case
	e.MustRun("config", "set", "ui.accent", "#3A7BD5")
	if got := cmds.Setting(e.App, "ui.accent"); got != "#3a7bd5" {
		t.Fatalf("custom accent: %q", got)
	}
	if _, code := e.Run("config", "set", "ui.accent", "#3a7bd"); code != cmds.ExitUsage || cmds.Setting(e.App, "ui.accent") != "#3a7bd5" {
		t.Fatalf("an invalid colour replaced the custom accent: exit %d", code)
	}
	if cmds.HexColour("#00FF7f") != "#00ff7f" || cmds.HexColour("#00ff7") != "" || cmds.HexColour("blue") != "" {
		t.Fatal("HexColour")
	}
	e.MustRun("config", "set", "ui.accent", "default")
	if got := cmds.Setting(e.App, "ui.accent"); got != "default" {
		t.Fatalf("could not restore default: %s", got)
	}
}
