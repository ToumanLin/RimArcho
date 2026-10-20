package cmds_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Settings with free values (06 §12.3, §12.4): a default, a check, and set/get.
func TestFreeSettings(t *testing.T) {
	e := testkit.New(t)
	if v := cmds.Setting(e.App, "tags.pinned"); !strings.HasPrefix(v, "框架,美化,") {
		t.Fatalf("tags.pinned default: %q", v)
	}
	if v := cmds.Setting(e.App, "ui.preview_dwell_ms"); v != "0" {
		t.Fatalf("dwell default: %q", v)
	}
	if _, code := e.Run("config", "set", "ui.preview_dwell_ms", "soon"); code == cmds.ExitOK {
		t.Fatal("a word was accepted as milliseconds")
	}
	e.MustRun("config", "set", "ui.preview_dwell_ms", "120")
	e.MustRun("config", "set", "tags.pinned", "a,b")
	if out := e.MustRun("config", "get"); !strings.Contains(out, "120") || !strings.Contains(out, "tags.pinned") || !strings.Contains(out, "ui.previews") {
		t.Fatalf("config get:\n%s", out)
	}
	if v := cmds.Setting(e.App, "tags.pinned"); v != "a,b" {
		t.Fatalf("tags.pinned: %q", v)
	}
}
