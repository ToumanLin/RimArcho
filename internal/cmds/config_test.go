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
	e.MustRun("config", "set", "tags.pinned", "a,b")
	if out := e.MustRun("config", "get"); !strings.Contains(out, "tags.pinned") {
		t.Fatalf("config get:\n%s", out)
	}
	if v := cmds.Setting(e.App, "tags.pinned"); v != "a,b" {
		t.Fatalf("tags.pinned: %q", v)
	}
}

// The tags.pinned preset follows the display language (16 §3.8): English players got the
// Chinese preset before.
func TestPinnedTagPresetFollowsLanguage(t *testing.T) {
	t.Setenv("ARCHO_LANG", "")
	e := testkit.New(t)
	for _, tc := range []struct{ lang, want string }{
		{"en", "RimWorld,framework,visuals,"},
		{"zh", "RimWorld,框架,美化,"},
	} {
		e.MustRun("config", "set", "ui.lang", tc.lang)
		if v := cmds.Setting(e.App, "tags.pinned"); !strings.HasPrefix(v, tc.want) {
			t.Fatalf("ui.lang %s: tags.pinned default %q", tc.lang, v)
		}
	}
	t.Setenv("ARCHO_LANG", "en") // overrides ui.lang, as for the interface
	if v := cmds.Setting(e.App, "tags.pinned"); !strings.HasPrefix(v, "RimWorld,framework,") {
		t.Fatalf("ARCHO_LANG en: tags.pinned default %q", v)
	}
	e.MustRun("config", "set", "tags.pinned", "a,b") // a stored list stays in any language
	if v := cmds.Setting(e.App, "tags.pinned"); v != "a,b" {
		t.Fatalf("stored tags.pinned: %q", v)
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
		{"ui.archo.motion", "obsolete", "live"},
		{"ui.archo.motion", "still", "still"},
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
