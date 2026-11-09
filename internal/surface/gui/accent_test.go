package gui

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"slices"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestAccentScales(t *testing.T) {
	t.Parallel()
	b, err := fs.ReadFile(assets, "assets/js/accent.js")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?s)export const scales = (\{.*?\n\});`).FindSubmatch(b)
	if m == nil {
		t.Fatal("no palette table in the embedded accent module")
	}
	var scales map[string]map[string][]string
	if err := json.Unmarshal(m[1], &scales); err != nil {
		t.Fatal(err)
	}
	choices := cmds.SettingChoices("ui.accent")
	if len(choices) == 0 || choices[0] != "default" || len(scales) != len(choices)-1 {
		t.Fatalf("config choices %v do not match the %d scales", choices, len(scales))
	}
	for _, required := range []string{"blue", "green", "orange", "red", "purple", "teal", "pink", "amber"} {
		if !slices.Contains(choices, required) {
			t.Errorf("missing required scale %s", required)
		}
	}
	hex := regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	for _, name := range choices[1:] {
		for _, mode := range []string{"light", "dark"} {
			steps := scales[name][mode]
			if len(steps) != 12 {
				t.Errorf("%s %s: %d steps, want 12", name, mode, len(steps))
			}
			for i, colour := range steps {
				if !hex.MatchString(colour) {
					t.Errorf("%s %s step %d: invalid hex %q", name, mode, i+1, colour)
				}
			}
		}
	}
}

func TestAccentStartupAndSettings(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	api := NewAPI(e.App, nil)
	t.Cleanup(api.Close)
	if got := api.Init().Accent; got != "default" {
		t.Fatalf("startup accent: %q", got)
	}
	for _, value := range []string{"blue", "amber", "#3a7bd5", "default"} {
		if _, err := api.ApplySetting([]string{"config", "set", "ui.accent", "{}"}, value); err != nil {
			t.Fatal(err)
		}
		if got := api.Init().Accent; got != value {
			t.Fatalf("startup data after saving %s: %q", value, got)
		}
	}
	if _, err := api.ApplySetting([]string{"config", "set", "ui.accent", "{}"}, "invalid"); err == nil {
		t.Fatal("GUI accepted an invalid accent")
	}
	if got := api.Init().Accent; got != "default" {
		t.Fatalf("rejected setting changed startup accent: %q", got)
	}
}

func TestThemeStartupAndSettings(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	api := NewAPI(e.App, nil)
	t.Cleanup(api.Close)
	if api.Init().Theme != "auto" {
		t.Fatal("default theme is not auto")
	}
	for _, value := range cmds.SettingChoices("ui.theme") {
		if _, err := api.ApplySetting([]string{"config", "set", "ui.theme", "{}"}, value); err != nil {
			t.Fatal(err)
		}
		if got := api.Init().Theme; got != value {
			t.Fatalf("saved theme %q, got %q", value, got)
		}
	}
	if _, err := api.ApplySetting([]string{"config", "set", "ui.theme", "{}"}, "invalid"); err == nil {
		t.Fatal("GUI accepted invalid theme")
	}
	if api.Init().Theme != "dark" {
		t.Fatal("invalid theme overwrote saved value")
	}
}
