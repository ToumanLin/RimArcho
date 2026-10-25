package view_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSteamCMDSettingCanReturnToAnonymous(t *testing.T) {
	e, _, _ := shellEnv(t)
	for _, value := range []string{"", "player", "anonymous"} {
		e.MustRun("config", "set", "steamcmd.account", value)
		found := false
		for _, group := range view.SettingsPage(e.App, nil) {
			for _, item := range group.Items {
				if strings.Join(item.Argv, " ") != "config set steamcmd.account {}" {
					continue
				}
				found = true
				want := value
				if want == "anonymous" {
					want = ""
				}
				if item.EditValue == nil || *item.EditValue != want || item.Edit != "text" || view.SettingCommand(item.Argv) != nil {
					t.Fatalf("%+v", item)
				}
			}
		}
		if !found {
			t.Fatal("missing account setting")
		}
	}
}
