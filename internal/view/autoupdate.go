package view

import (
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
)

func AutomaticName(kind string) string {
	return T(map[string]string{"mods": "mods", "tools": "external tools", "session": "LoversLab session", "data": "community data"}[kind])
}

func AutomaticNotice(before, after app.AutomaticStatus) string {
	if after.Pending <= 0 || after.Pending == before.Pending {
		return ""
	}
	switch after.Kind {
	case "mods":
		return Tf("%d mod updates available", after.Pending)
	case "tools":
		return Tf("%d tool updates available; see Settings", after.Pending)
	case "session":
		return T("LoversLab needs you to log in again; see Settings → accounts")
	}
	return ""
}

func AutomaticGroup(a *app.App) SettingGroup {
	g := SettingGroup{Title: T("automatic checks"), Note: Tf("While the window is open: mods and data every %d hours, tools every %d hours, LoversLab every %d hours. Mod and tool updates wait for your click.", int(a.AutomaticInterval("mods").Hours()), int(a.AutomaticInterval("tools").Hours()), int(a.AutomaticInterval("session").Hours()))}
	g.Items = []SettingItem{
		{Label: T("automatic checks"), Value: cmds.Setting(a, "updates.auto"), Edit: "choice", Choices: []string{"on", "off"}, ChoiceLabels: []string{T("on"), T("off")}, Argv: []string{"config", "set", "updates.auto", "{}"}},
		{Label: T("check interval"), Value: cmds.Setting(a, "updates.interval"), Edit: "choice", Choices: []string{"normal", "frequent", "relaxed"}, ChoiceLabels: []string{T("standard intervals"), T("half the interval"), T("twice the interval")}, Argv: []string{"config", "set", "updates.interval", "{}"}},
	}
	for _, kind := range app.AutomaticKinds {
		s := a.AutomaticStatus(kind)
		value := Text(s.State)
		note := ""
		if !s.Checked.IsZero() {
			note = Tf("last checked %s", a.FormatTime(s.Checked))
		}
		if s.Pending > 0 {
			value += "  " + Tf("%d pending", s.Pending)
		}
		g.Items = append(g.Items, SettingItem{Label: AutomaticName(kind), Value: value, Note: note})
	}
	return g
}
