package view

import (
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
)

func AutomaticName(kind string) string {
	return T(map[string]string{"mods": "mods", "tools": "external tools", "session": "LoversLab session", "data": "community data", "self": "RimArcho updates"}[kind])
}

func AutomaticNotice(a *app.App, before, after app.AutomaticStatus) string {
	if after.Kind == "self" {
		return selfNotice(a, before, after)
	}
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

// selfNotice: once per new version, not on every check that still finds it.
func selfNotice(a *app.App, before, after app.AutomaticStatus) string {
	if after.Pending <= 0 || after.Latest == "" || before.Pending > 0 && before.Latest == after.Latest {
		return ""
	}
	if !app.SelfUpdateSupported() {
		return SelfCheckResult(a)
	}
	return Tf("RimArcho %s can be updated; see Settings", after.Latest)
}

// AboutGroup keeps RimArcho's version and checks together (16 §2.14).
func AboutGroup(a *app.App) SettingGroup {
	return SettingGroup{Title: T("about"), Items: []SettingItem{
		SelfUpdateItem(a),
		{Label: T("automatically check for updates"), Value: cmds.Setting(a, "updates.auto"), Edit: "choice", Choices: []string{"on", "off"}, ChoiceLabels: []string{T("on"), T("off")}, Argv: []string{"config", "set", "updates.auto", "{}"},
			Note: T("Checks RimArcho, mods, external tools and community data while RimArcho is open.")},
		{Label: T("feedback"), Actions: []SettingAction{{Label: T("bugs and suggestions…"), Argv: []string{"feedback"}}}},
		{Key: "community", Label: T("community"), Links: CommunityLinks()},
	}}
}

func AutomaticGroup(a *app.App) SettingGroup {
	note := Tf("While RimArcho is open: check mods, community data and RimArcho %s, tools %s.", checkInterval(a.AutomaticInterval("mods")), checkInterval(a.AutomaticInterval("tools")))
	if a.Store.Meta("account:loverslab:enabled") == "true" {
		note += " " + Tf("Check the LoversLab login %s.", checkInterval(a.AutomaticInterval("session")))
	}
	g := SettingGroup{Title: T("updates"), Note: note + " " + T("Mod, tool and RimArcho updates wait for your click.")}
	// Version and automatic checks live in AboutGroup.
	g.Items = []SettingItem{
		{Label: T("update source"), Value: cmds.Setting(a, "updates.source"), Edit: "choice", Choices: cmds.SettingChoices("updates.source"), ChoiceLabels: []string{T("automatic"), "GitHub"}, Argv: []string{"config", "set", "updates.source", "{}"}},
		{Label: T("check interval"), Value: cmds.Setting(a, "updates.interval"), Edit: "choice", Choices: []string{"normal", "frequent", "relaxed"}, ChoiceLabels: []string{T("standard intervals"), T("half the interval"), T("twice the interval")}, Argv: []string{"config", "set", "updates.interval", "{}"}},
	}
	return g
}

func checkInterval(d time.Duration) string {
	switch {
	case d == 24*time.Hour:
		return T("daily")
	case d == 7*24*time.Hour:
		return T("weekly")
	case d == 7*12*time.Hour:
		return T("twice a week")
	case d%(7*24*time.Hour) == 0:
		return Tf("every %d weeks", int(d/(7*24*time.Hour)))
	case d%(24*time.Hour) == 0:
		return Tf("every %d days", int(d/(24*time.Hour)))
	default:
		return Tf("every %d hours", int(d/time.Hour))
	}
}

func lastChecked(a *app.App, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return Tf("last checked %s", a.FormatTime(t))
}

// UpdateCheckResult also renders persisted checks made by earlier releases.
func UpdateCheckResult(pending int, state string) string {
	if state == "check complete" || state == "up to date" {
		if pending == 1 {
			return T("1 update available")
		}
		if pending > 0 {
			return Tf("%d updates available", pending)
		}
		return T("everything is up to date")
	}
	return Text(state)
}

func AutomaticResult(s app.AutomaticStatus) string { return UpdateCheckResult(s.Pending, s.State) }
