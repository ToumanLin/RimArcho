package cmds

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

func TestParseUninstallRequest(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		argv     []string
		run, bad bool
	}{
		{[]string{"uninstall"}, false, false},
		{[]string{"uninstall", "--dry-run"}, false, false},
		{[]string{"uninstall", "--yes", "--keep-locked"}, true, false},
		{[]string{"uninstall", "--saves-to", "--yes"}, false, false},
		{[]string{"uninstall", "--yes", "--dry-run"}, false, true},
		{[]string{"uninstall", "--yes=true"}, false, true},
	} {
		_, run, err := ParseUninstallRequest(tc.argv)
		if run != tc.run || (err != nil) != tc.bad {
			t.Errorf("%v: run=%v err=%v", tc.argv, run, err)
		}
	}
}

// The Archo Agent key item names its action, and the accounts step mentions it.
func TestUninstallAgentKeyText(t *testing.T) {
	t.Parallel()
	it := &app.UninstallItem{Do: "forget-agent-key", Path: "Windows Credential Manager: RimArcho/agent-api-key"}
	if got := UninstallItemText(it); !strings.HasPrefix(got, "forget ") {
		t.Fatalf("item text %q", got)
	}
	p := &app.UninstallPlan{}
	s := &app.UninstallStep{Key: app.UnAccounts, Items: []*app.UninstallItem{{Do: "forget-accounts"}, it}}
	if got := UninstallStepText(p, s); !strings.Contains(got, "Archo Agent API key") {
		t.Fatalf("step text %q", got)
	}
}
