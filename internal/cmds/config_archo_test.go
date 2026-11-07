package cmds_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestArchoMotionSetting(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	choices := []string{"live", "still", "off"}
	if !slices.Equal(cmds.SettingChoices("ui.archo.motion"), choices) || cmds.Setting(e.App, "ui.archo.motion") != "live" {
		t.Fatal("unexpected Archo motion choices/default")
	}
	if out := e.MustRun("config", "get"); !strings.Contains(out, "ui.archo.motion: live") {
		t.Fatal(out)
	}
	for _, value := range choices {
		e.MustRun("config", "set", "ui.archo.motion", value)
		e.Open()
		if got := cmds.Setting(e.App, "ui.archo.motion"); got != value {
			t.Fatalf("reopened motion: %q, want %q", got, value)
		}
	}
	for _, value := range []string{"", "Live", "animated", "on", "invalid"} {
		if _, code := e.Run("config", "set", "ui.archo.motion", value); code != cmds.ExitUsage {
			t.Errorf("invalid motion %q: exit %d", value, code)
		}
		if got := cmds.Setting(e.App, "ui.archo.motion"); got != "off" {
			t.Fatalf("invalid motion overwrote saved value: %q", got)
		}
	}
}
