package cmds_test

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestAccentSetting(t *testing.T) {
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
	for _, value := range []string{"", "Blue", "not-a-colour", "#0090ff"} {
		if _, code := e.Run("config", "set", "ui.accent", value); code != cmds.ExitUsage {
			t.Errorf("invalid accent %q: exit %d", value, code)
		}
		if got := cmds.Setting(e.App, "ui.accent"); got != "blue" {
			t.Fatalf("invalid accent overwrote the saved value: %q", got)
		}
	}
	e.MustRun("config", "set", "ui.accent", "default")
	if got := cmds.Setting(e.App, "ui.accent"); got != "default" {
		t.Fatalf("could not restore default: %s", got)
	}
}
