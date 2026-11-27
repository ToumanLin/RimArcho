package view_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 16 §2.14 "Layout": the seven Settings groups, in order, each a row of the GUI's list with a
// key, a summary line of one to three facts and one of the three attentions; the former groups
// are sections with their own keys, so a key saved before the merge and deep links still find
// their group.
func TestSettingsGroupsHaveSummaries(t *testing.T) {
	e := testkit.New(t)
	groups := view.SettingsPage(e.App, nil)
	var keys []string
	for _, g := range groups {
		keys = append(keys, g.Key)
		if n := len(g.Summary); n == 0 || n > 3 || strings.TrimSpace(strings.Join(g.Summary, "")) == "" {
			t.Errorf("group %q (%s) summary %q", g.Title, g.Key, g.Summary)
		}
		switch g.Attention {
		case view.AttentionNone, view.AttentionAccent, view.AttentionAmber:
		default:
			t.Errorf("group %q: attention %q", g.Title, g.Attention)
		}
		if len(g.Sections) > 0 && (len(g.Items) > 0 || g.Note != "" || g.Sections[0].Key != g.Key) {
			t.Errorf("group %q: a group of sections has rows or a note of its own, or another key", g.Title)
		}
	}
	want := []string{view.GroupAbout, view.GroupInterface, view.GroupUpdates, view.GroupAccounts, view.GroupFolders, view.GroupTools, view.GroupUninstall}
	if strings.Join(keys, " ") != strings.Join(want, " ") {
		t.Errorf("groups %v, want %v", keys, want)
	}
	seen := map[string]bool{}
	var order []string
	for _, s := range view.SettingSections(groups) {
		if s.Key == "" || seen[s.Key] {
			t.Errorf("section %q: key %q missing or repeated", s.Title, s.Key)
		}
		seen[s.Key] = true
		order = append(order, s.Key)
	}
	sections := strings.Join([]string{view.GroupAbout, view.GroupInterface, view.GroupUpdates, view.GroupData, view.GroupAccounts, view.GroupNetwork,
		view.GroupFolders, view.GroupStorage, view.GroupTools, view.GroupLog}, " ")
	if got := strings.Join(order, " "); got != sections+" "+view.GroupUninstall && got != sections+" "+view.GroupAgent+" "+view.GroupUninstall {
		t.Errorf("sections %s", got)
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
