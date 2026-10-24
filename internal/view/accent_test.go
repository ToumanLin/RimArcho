package view_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestAccentSettingsRow(t *testing.T) {
	t.Setenv("ARCHO_LANG", "")
	t.Cleanup(func() { view.SetLang("en") })
	e := testkit.New(t)
	choices := cmds.SettingChoices("ui.accent")
	zhLabels := []string{"默认", "靛蓝", "蓝", "青", "水鸭", "绿", "草绿", "橙", "琥珀", "红", "绯红", "粉", "梅", "紫", "紫罗兰", "鸢尾"}
	for _, lang := range []string{"en", "zh"} {
		view.SetLang(lang)
		for _, value := range []string{"default", "amber"} {
			e.MustRun("config", "set", "ui.accent", value)
			found := 0
			for _, group := range view.SettingsPage(e.App, nil) {
				for _, item := range group.Items {
					if strings.Join(item.Argv, " ") != "config set ui.accent {}" {
						continue
					}
					found++
					label, labels := "accent colour", choices
					if lang == "zh" {
						label, labels = "强调色", zhLabels
					}
					if group.Title != view.T("interface") || item.Label != label || item.Edit != "choice" || item.Value != value ||
						!slices.Equal(item.Choices, choices) || !slices.Equal(item.ChoiceLabels, labels) || view.SettingCommand(item.Argv) != nil {
						t.Fatalf("%s accent row: %+v in %s", lang, item, group.Title)
					}
				}
			}
			if found != 1 {
				t.Fatalf("%s: found %d accent rows", lang, found)
			}
		}
	}
}
