package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/l10ndata"
)

// The GUI's Settings list (16 §2.14 "Layout"): each group is a row with its name, one muted
// summary line and, when there is something to do or to look at, a dot.

// Group keys: a group's identity whatever the interface language, for the GUI's last group
// viewed and deep links.
const (
	GroupAbout     = "about"
	GroupInterface = "interface"
	GroupAccounts  = "accounts"
	GroupUpdates   = "updates"
	GroupFolders   = "folders"
	GroupLog       = "log"
	GroupNetwork   = "network"
	GroupAgent     = "agent"
	GroupTools     = "tools"
	GroupData      = "data"
	GroupStorage   = "storage"
	GroupUninstall = "uninstall"
)

// Attention on a group's row: none, accent (something can be done, an update is available) or
// amber (something needs a look: a folder is unavailable, a drive is low, a tool cannot run).
const (
	AttentionNone   = "none"
	AttentionAccent = "accent"
	AttentionAmber  = "amber"
)

// facts are a summary's parallel facts, empty ones left out; the GUI spaces them apart
// (16 §7.3 rule 5: no middle dots).
func facts(parts ...string) []string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// choiceText is the words a choice item shows for its value.
func choiceText(it SettingItem) string {
	for i, c := range it.Choices {
		if c == it.Value && i < len(it.ChoiceLabels) {
			return it.ChoiceLabels[i]
		}
	}
	return it.Value
}

// itemByArgvKey is the group's item that sets key (config set key {}).
func itemByArgvKey(g SettingGroup, key string) (SettingItem, bool) {
	for _, it := range g.Items {
		if len(it.Argv) >= 3 && it.Argv[0] == "config" && it.Argv[2] == key {
			return it, true
		}
	}
	return SettingItem{}, false
}

// languageInUse is the name of the language the interface shows now (auto resolved).
func languageInUse() string {
	if l := Lang(); l != "en" {
		return l10ndata.LanguageName(l)
	}
	return "English"
}

// interfaceSummary: "English  dark  110%".
func interfaceSummary(g SettingGroup) []string {
	theme, _ := itemByArgvKey(g, "ui.theme")
	zoom, _ := itemByArgvKey(g, "ui.zoom")
	z := zoom.Value
	if z != "" {
		z += "%"
	}
	return facts(languageInUse(), choiceText(theme), z)
}

// choicesSummary joins the shown words of the items that set keys.
func choicesSummary(g SettingGroup, keys ...string) []string {
	var parts []string
	for _, k := range keys {
		if it, ok := itemByArgvKey(g, k); ok {
			parts = append(parts, choiceText(it))
		}
	}
	return facts(parts...)
}

// lowestDisk is the meter of the drive with the least free space among the items', or nil.
func lowestDisk(items []SettingItem) *DiskMeter {
	var low *DiskMeter
	for _, it := range items {
		if it.Disk != nil && (low == nil || it.Disk.Free < low.Free) {
			low = it.Disk
		}
	}
	return low
}

// folderCount is "1 folder" or "N folders".
func folderCount(n int) string {
	if n == 1 {
		return T("1 folder")
	}
	return Tf("%d folders", n)
}
