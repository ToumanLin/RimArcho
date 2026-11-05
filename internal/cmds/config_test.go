package cmds_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Settings with free values (16 §3.8, §2.14): a default, a check, and set/get.
func TestFreeSettings(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	if v := cmds.Setting(e.App, "tags.pinned"); !strings.HasPrefix(v, "框架,美化,") {
		t.Fatalf("tags.pinned default: %q", v)
	}
	e.MustRun("config", "set", "tags.pinned", "a,b")
	if out := e.MustRun("config", "get"); !strings.Contains(out, "tags.pinned") {
		t.Fatalf("config get:\n%s", out)
	}
	if v := cmds.Setting(e.App, "tags.pinned"); v != "a,b" {
		t.Fatalf("tags.pinned: %q", v)
	}
}

func TestEnumeratedSettingDefaults(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	for _, tc := range []struct{ key, stored, want string }{
		{"ui.icons", "", "compat"},
		{"ui.icons", "auto", "compat"},
		{"ui.icons", "invalid", "compat"},
		{"ui.icons", "nerd", "nerd"},
		{"ui.theme", "obsolete", "auto"},
		{"ui.theme", "light", "light"},
	} {
		if err := e.App.Store.SetMeta("cfg:"+tc.key, tc.stored); err != nil {
			t.Fatal(err)
		}
		if got := cmds.Setting(e.App, tc.key); got != tc.want {
			t.Fatalf("%s=%q: got %q, want %q", tc.key, tc.stored, got, tc.want)
		}
		if out := e.MustRun("config", "get", tc.key); !strings.Contains(out, tc.key+": "+tc.want) {
			t.Fatal(out)
		}
	}
	if _, code := e.Run("config", "set", "ui.icons", "auto"); code != cmds.ExitUsage {
		t.Fatal("accepted obsolete icon mode")
	}
}
