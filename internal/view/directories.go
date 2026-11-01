package view

import (
	"github.com/ToumanLin/RimArcho/internal/app"
)

func DirectoryLabel(d app.ManagedDirectory) string {
	switch d.Kind {
	case "game":
		return Tf("game folder (%s)", d.Label)
	case "workshop":
		return T("Workshop folder")
	case "steamcmd":
		return Tf("SteamCMD folder (%s)", d.Label)
	case "extra":
		return Tf("other mod folder (%s)", d.Label)
	case "lock":
		return T("lock library")
	case "source":
		return T("source library")
	case "packs":
		return T("where packs are kept")
	case "monitor":
		return T("Archo Monitor folder")
	case "tool", "steamcmd-tool":
		return T("SteamCMD program folder")
	case app.SubscribedLabel:
		return T("subscription downloads")
	default:
		return d.Label
	}
}

func DirectoryNeedsLocation(d app.ManagedDirectory) bool {
	return d.Status != "ok" && d.Status != "unused"
}

// DirectoryGroup lists actual resolved paths, including the defaults before first use.
func DirectoryGroup(a *app.App) SettingGroup {
	g := SettingGroup{Title: T("managed directories")}
	dirs, err := a.ManagedDirectories()
	if err != nil {
		g.Items = append(g.Items, SettingItem{Label: T("directory check failed"), Note: err.Error()})
		return g
	}
	// One note for the whole group; a row only says what is different about it.
	g.Note = T("locate it after moving it yourself, or move it to a new location")
	for _, d := range dirs {
		note := ""
		if DirectoryNeedsLocation(d) {
			note = T("directory unavailable; enter its new location, or reconnect the drive")
		} else if d.Status == "unused" {
			note = T("not used yet")
		}
		item := SettingItem{Label: DirectoryLabel(d), Value: d.Path, ValuePath: true, Edit: "directory", Directory: d.ID,
			Argv: []string{"folder", "set", d.ID, "{}"}, MoveArgv: []string{"folder", "move", d.ID, "{}"}, Note: note}
		if d.ID == "source" {
			item.ValueNote = SizeText(a.Store.SourceLibraryBytes())
			item.Actions = []SettingAction{
				{Label: T("remove unused"), Argv: []string{"source", "clean", "--yes"}, Confirm: T("remove unused folders from the source library")},
				{Label: T("decompile the current list's mods"), Argv: []string{"source", "decompile", "--active"}},
			}
		}
		if d.Kind == "steamcmd" || d.Kind == "extra" {
			item.Actions = []SettingAction{{Label: T("remove"), Argv: []string{"folder", "remove", "src:" + d.Label},
				Confirm: T("RimArcho forgets this folder; the files stay where they are")}}
		}
		g.Items = append(g.Items, item)
	}
	return g
}

func directoryNeeds(a *app.App) []Need {
	dirs, err := a.ManagedDirectories()
	if err != nil {
		return []Need{{Group: GroupConfirm, Title: T("directory check failed"), Text: err.Error(), Why: err.Error(), Status: StatusWarn, Counted: true}}
	}
	var out []Need
	for _, d := range dirs {
		if !DirectoryNeedsLocation(d) {
			continue
		}
		title := Tf("locate %s", DirectoryLabel(d))
		why := T("the saved directory is unavailable or has changed; enter its new location in settings. You can keep playing")
		out = append(out, Need{Group: GroupConfirm, Title: title, Text: title + "  " + d.Path, Why: why + "  " + d.Path,
			Status: StatusWarn, Counted: true, Actions: []Action{{Kind: ActLocateDirectory, Key: "o", Label: T("locate directory"), Refs: []string{d.ID}, Lines: []string{d.Path}},
				{Kind: ActView, Key: "v", Label: T("directory details"), Argv: []string{"folder", "check"}}}})
	}
	return out
}
