package wezterm

import (
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {
	c := Config(`C:\Program Files\RimArcho\fonts`)
	for _, want := range []string{
		`config.font_dirs = { [[C:\Program Files\RimArcho\fonts]] }`,
		`config.exit_behavior = 'CloseOnCleanExit'`,
		`config.treat_east_asian_ambiguous_width_as_wide = false`,
		`'Maple Mono NF CN'`,
		`background = '#0E0F12'`,
	} {
		if !strings.Contains(c, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(c, "ConfigDirsOnly") {
		t.Error("font_locator restricted")
	}
	if got := luaString(`a]]b]=]c`); got != `[==[a]]b]=]c]==]` {
		t.Errorf("long string level: %s", got)
	}
}
