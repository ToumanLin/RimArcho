package view_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 16 §2.14 "Layout": every Settings group is a row of the GUI's list, with a key, a summary
// line and one of the three attentions; keys are unique, so the last group viewed and deep
// links find their group.
func TestSettingsGroupsHaveSummaries(t *testing.T) {
	e := testkit.New(t)
	seen := map[string]bool{}
	groups := view.SettingsPage(e.App, nil)
	for _, g := range groups {
		if g.Key == "" || seen[g.Key] {
			t.Errorf("group %q: key %q missing or repeated", g.Title, g.Key)
		}
		seen[g.Key] = true
		if len(g.Summary) == 0 || strings.TrimSpace(strings.Join(g.Summary, "")) == "" {
			t.Errorf("group %q (%s) has no summary", g.Title, g.Key)
		}
		switch g.Attention {
		case view.AttentionNone, view.AttentionAccent, view.AttentionAmber:
		default:
			t.Errorf("group %q: attention %q", g.Title, g.Attention)
		}
	}
	for _, k := range []string{view.GroupAbout, view.GroupInterface, view.GroupAccounts, view.GroupUpdates, view.GroupFolders, view.GroupLog,
		view.GroupNetwork, view.GroupTools, view.GroupData, view.GroupStorage, view.GroupUninstall} {
		if !seen[k] {
			t.Errorf("no %s group", k)
		}
	}
	if last := groups[len(groups)-1]; last.Key != view.GroupUninstall {
		t.Errorf("uninstall is not last: %s", last.Key)
	}
}

// An unavailable Mod folder makes Folders amber and says how many (16 §2.14 "Layout").
func TestSettingsFoldersAttention(t *testing.T) {
	e := testkit.New(t)
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.MkdirAll(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	e.MustRun("folder", "add", "extra", gone)
	folders := func() view.SettingGroup {
		for _, g := range view.SettingsPage(e.App, nil) {
			if g.Key == view.GroupFolders {
				return g
			}
		}
		t.Fatal("no folders group")
		return view.SettingGroup{}
	}
	if g := folders(); g.Attention == view.AttentionAmber && strings.Contains(strings.Join(g.Summary, "  "), "unavailable") {
		t.Fatalf("a present folder is amber: %+v", g.Summary)
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	if g := folders(); g.Attention != view.AttentionAmber || !strings.Contains(strings.Join(g.Summary, "  "), "1 unavailable") {
		t.Fatalf("missing folder: attention %q, summary %q", g.Attention, g.Summary)
	}
}
