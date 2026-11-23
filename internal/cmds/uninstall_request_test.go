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
	p := &app.UninstallPlan{}
	if got := UninstallItemText(p, it); !strings.HasPrefix(got, "forget ") {
		t.Fatalf("item text %q", got)
	}
	s := &app.UninstallStep{Key: app.UnAccounts, Items: []*app.UninstallItem{{Do: "forget-accounts"}, it}}
	if got := UninstallStepText(p, s); !strings.Contains(got, "Archo Agent API key") {
		t.Fatalf("step text %q", got)
	}
}

// Another home's link sharing the game is left alone as a link, not as "a folder named
// like one but not a link" (VM 2026-11-23: the throwaway home's _Archo_monitor_ junction);
// a plain folder with the prefix still reads as one (02 §2).
func TestUninstallLinksLeftAloneByKind(t *testing.T) {
	t.Parallel()
	p := &app.UninstallPlan{}
	s := &app.UninstallStep{Key: app.UnLinks, Items: []*app.UninstallItem{
		{Do: app.DoUnlink, Path: `Mods\_Archo_workshop_1`},
		{Do: app.DoLeave, Path: `Mods\_Archo_monitor_ArchoMonitor-0.12.0`, Note: "ownership or link health is unproven: left alone"},
	}}
	got := UninstallStepText(p, s)
	if !strings.Contains(got, "1 link not proven to be this home's") || strings.Contains(got, "not a link") {
		t.Fatalf("another home's link: %q", got)
	}
	s.Items = append(s.Items, &app.UninstallItem{Do: app.DoLeave, Path: `Mods\_Archo_x`, Note: app.NotALinkNote})
	if got := UninstallStepText(p, s); !strings.Contains(got, "1 folder named like one but not a link") || !strings.Contains(got, "1 link not proven") {
		t.Fatalf("both kinds: %q", got)
	}
}
