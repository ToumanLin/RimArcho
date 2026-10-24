package gui

import (
	"encoding/json"
	"io/fs"
	"os/exec"
	"regexp"
	"slices"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestAccentRuntime(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is needed for accent behavior tests")
	}
	if out, err := exec.Command(node, "--experimental-vm-modules", "--test", "testdata/accent.mjs").CombinedOutput(); err != nil {
		t.Fatalf("accent: %v\n%s", err, out)
	}
}

func TestAccentScales(t *testing.T) {
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
	e := testkit.New(t)
	api := NewAPI(e.App, nil)
	t.Cleanup(api.Close)
	if got := api.Init().Accent; got != "default" {
		t.Fatalf("startup accent: %q", got)
	}
	for _, value := range []string{"blue", "amber", "default"} {
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
