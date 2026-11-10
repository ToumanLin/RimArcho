package view_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestArchoMotionSettingsRow(t *testing.T) {
	t.Setenv("ARCHO_LANG", "")
	t.Cleanup(func() { view.SetLang("en") })
	e := testkit.New(t)
	for _, lang := range []string{"en", "zh"} {
		view.SetLang(lang)
		for _, value := range []string{"live", "still", "off"} {
			e.MustRun("config", "set", "ui.archo.motion", value)
			found := 0
			for _, group := range view.SettingsPage(e.App, nil) {
				for i, item := range group.Items {
					if strings.Join(item.Argv, " ") != "config set ui.archo.motion {}" {
						continue
					}
					found++
					labels := []string{view.T("off"), view.T("still"), view.T("animated")}
					hover := view.T("Archo's figure while pages load and in the first-run guide; still uses no GPU")
					if group.Title != view.T("interface") || item.Label != view.T("Archo animation") ||
						item.Value != value || item.Edit != "segments" || item.Hover != hover ||
						!slices.Equal(item.Choices, []string{"off", "still", "live"}) ||
						!slices.Equal(item.ChoiceLabels, labels) || view.SettingCommand(item.Argv) != nil {
						t.Fatalf("%s motion row: %+v in %s", lang, item, group.Title)
					}
					if i == 0 || strings.Join(group.Items[i-1].Argv, " ") != "config set ui.gpu {}" {
						t.Fatal("Archo motion must follow GPU acceleration")
					}
					if lang == "zh" {
						for _, key := range []string{"Archo animation", "still", "animated", "Archo's figure while pages load and in the first-run guide; still uses no GPU"} {
							if view.T(key) == key {
								t.Errorf("missing translation: %s", key)
							}
						}
					}
				}
			}
			if found != 1 {
				t.Fatalf("%s: found %d motion rows", lang, found)
			}
		}
	}
}

// Settings › motion (16 §2.14): follow system or reduced, after Archo animation, translated.
func TestMotionSettingsRow(t *testing.T) {
	t.Setenv("ARCHO_LANG", "")
	t.Cleanup(func() { view.SetLang("en") })
	e := testkit.New(t)
	for _, lang := range []string{"en", "zh"} {
		view.SetLang(lang)
		for _, value := range []string{"auto", "reduced"} {
			e.MustRun("config", "set", "ui.motion", value)
			found := 0
			for _, group := range view.SettingsPage(e.App, nil) {
				for i, item := range group.Items {
					if strings.Join(item.Argv, " ") != "config set ui.motion {}" {
						continue
					}
					found++
					if group.Title != view.T("interface") || item.Label != view.T("motion") || item.Value != value || item.Edit != "segments" ||
						!slices.Equal(item.Choices, []string{"auto", "reduced"}) ||
						!slices.Equal(item.ChoiceLabels, []string{view.T("follow system"), view.T("reduced")}) || item.Hover == "" {
						t.Fatalf("%s motion row: %+v", lang, item)
					}
					if i == 0 || strings.Join(group.Items[i-1].Argv, " ") != "config set ui.archo.motion {}" {
						t.Fatal("motion must follow Archo animation")
					}
					if lang == "zh" {
						for _, key := range []string{"motion", "reduced"} {
							if view.T(key) == key {
								t.Errorf("missing translation: %s", key)
							}
						}
						if strings.Contains(item.Hover, "keeps the window still") {
							t.Error("missing translation of the motion hover")
						}
					}
				}
			}
			if found != 1 {
				t.Fatalf("%s: found %d motion rows", lang, found)
			}
		}
	}
	if _, code := e.Run("config", "set", "ui.motion", "fast"); code == 0 {
		t.Fatal("ui.motion accepted an unknown value")
	}
}
