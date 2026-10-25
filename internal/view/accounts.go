package view

import (
	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/app"
)

func SteamAccountLine(s app.SteamAccountStatus) string {
	if !s.Managed && s.Path != "" {
		return T("external SteamCMD manages its own login")
	}
	if !s.LoggedIn || s.Account == "anonymous" {
		return T("mod downloads available, not logged in")
	}
	if s.OwnershipChecked.IsZero() {
		return Tf("mod downloads available, game downloads: not checked, account: %s", s.Account)
	}
	if s.OwnsRimWorld {
		return Tf("mod downloads available, game downloads available, account: %s", s.Account)
	}
	return Tf("mod downloads available, game downloads unavailable, account: %s", s.Account)
}

func LoversLabLine(a *app.App) (string, bool) {
	if a.Store.Meta("account:loverslab:enabled") == "true" {
		if session, err := accounts.Open(a.AccountsDir()); err == nil {
			st := session.Status()
			if session.HasCookies() && st.State == "available" {
				expiry := "N/A"
				if !st.Expiry.IsZero() {
					expiry = a.FormatTime(st.Expiry)
				}
				return Tf("available, Cookie expiry: %s", expiry), true
			}
		}
	}
	return T("unavailable, not logged in"), false
}

func AccountGroup(a *app.App) SettingGroup {
	s := a.SteamAccountStatus()
	action := func(service, verb, label, confirm string) SettingAction {
		return SettingAction{Label: T(label), Argv: []string{"account", service, verb}, Confirm: T(confirm)}
	}
	steam := SettingItem{Label: "SteamCMD", Note: SteamAccountLine(s)}
	if s.Managed || s.Path == "" {
		label := "log in"
		if s.LoggedIn {
			label = "log in again"
		}
		login := action("steamcmd", "login", label, "")
		name := s.Account
		if name == "anonymous" {
			name = ""
		}
		login.Input = &name
		steam.Actions = []SettingAction{login}
		if s.LoggedIn {
			steam.Actions = append(steam.Actions, action("steamcmd", "logout", "log out", "Sign out of SteamCMD. If Steam cannot confirm logout, including when offline, RimArcho automatically forgets this managed instance's local login cache for all accounts. Other devices are unaffected."))
		}
	}
	line, signedIn := LoversLabLine(a)
	label := "log in"
	if signedIn {
		label = "log in again"
	}
	confirm := ""
	if a.Store.Meta("account:loverslab:consent") != "true" && a.Store.Meta("account:loverslab:enabled") != "true" {
		confirm = "LoversLab contains adult content and has its own access conditions and rules. Use your own account and follow the site's rules and file licenses. Login cookies are encrypted for your Windows user on this device; passwords stay on the site."
	}
	ll := SettingItem{Label: "LoversLab", Note: line, Actions: []SettingAction{action("loverslab", "login", label, confirm)}}
	if signedIn {
		ll.Actions = append(ll.Actions, action("loverslab", "forget", "log out", "Forget the local LoversLab session. Other devices are unaffected."))
	}
	return SettingGroup{Title: T("accounts"), Items: []SettingItem{steam, ll}}
}
