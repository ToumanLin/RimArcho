package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"
)

// DependencyGroup shows the selected copies and commands for managing downloaded tools.
func DependencyGroup(a *app.App) SettingGroup {
	g := SettingGroup{Key: GroupTools, Title: T("external tools"), Attention: AttentionNone, Note: lastChecked(a, a.AutomaticStatus("tools").Checked)}
	var parts []string
	labels := map[string]string{deps.Todds: "todds", deps.SteamCMD: "SteamCMD"}
	for _, i := range a.Deps().All() {
		item := SettingItem{Key: "tool." + i.Name, Label: labels[i.Name], Value: T("not installed"), Note: i.Path, NotePaths: []string{i.Path}}
		if i.Version != "" {
			item.Value = i.Version
		} else if i.OK {
			item.Value = T("installed")
		}
		if i.Err != "" && (i.Path != "" || !i.Installable) {
			item.Note += "  " + Humanize(nil, T(i.Err))
		}
		item.Note = strings.TrimSpace(item.Note)
		update := a.ToolUpdate(i.Name)
		// the list row (16 §2.14 "Layout"): "todds 7.0.2  SteamCMD not installed"; a tool found
		// but unusable is amber, an update the accent
		parts = append(parts, labels[i.Name]+" "+item.Value)
		if i.Err != "" && i.Path != "" {
			g.Attention = AttentionAmber
		} else if update.Pending && i.Own && g.Attention == AttentionNone {
			g.Attention = AttentionAccent
		}
		if update.Pending && i.Own {
			item.ValueNote = Tf("new version available: %s", update.Latest)
			if !update.Compatible {
				item.Note += "  " + Tf("RimArcho supports todds %s; the new version has not been verified.", i.Version)
			}
		}
		if i.ConfigKey != "" {
			item.Edit, item.Argv = "path", []string{"config", "set", i.ConfigKey, "{}"}
			value := a.Store.Meta("cfg:" + i.ConfigKey)
			item.EditValue, item.File = &value, true
		}
		if i.Installable {
			label := T("install tool")
			if i.Path != "" {
				label = T("update tool")
			}
			item.Actions = append(item.Actions, SettingAction{Label: label, Argv: []string{"deps", "install", i.Name},
				Confirm: Tf("download %s into RimArcho's own folder?", labels[i.Name])})
			if update.Pending && !update.Compatible && i.Own {
				item.Actions = nil
			}
		}
		if i.Own && a.CheckDepRemoval(i.Name) == nil {
			item.Actions = append(item.Actions, SettingAction{Label: T("remove"), Argv: []string{"deps", "remove", i.Name},
				Confirm: Tf("remove RimArcho's own copy of %s?", labels[i.Name])})
		}
		if item.EditValue != nil && *item.EditValue != "" {
			item.Actions = append(item.Actions, SettingAction{Label: T("use default"), Argv: []string{"config", "set", i.ConfigKey, ""}})
		}
		g.Items = append(g.Items, item)
	}
	g.Summary = facts(parts...)
	return g
}
