package view_test

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSteamCMDLoginPrefillsInlineName(t *testing.T) {
	e, _, _ := shellEnv(t)
	e.App.SteamCMDNoPath = true
	for _, value := range []string{"", "player", "anonymous"} {
		e.MustRun("config", "set", "steamcmd.account", value)
		row := view.AccountGroup(e.App).Items[0]
		want := value
		if want == "anonymous" {
			want = ""
		}
		if row.Edit != "" || len(row.Argv) != 0 || len(row.Actions) != 1 || row.Actions[0].Input == nil || *row.Actions[0].Input != want {
			t.Fatalf("account %q: %+v", value, row)
		}
	}
}
