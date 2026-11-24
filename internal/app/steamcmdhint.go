package app

import (
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ToumanLin/RimArcho/internal/l10ndata"
)

// SteamCMD's password prompt (11 §3 "Password prompt"): SteamCMD shows nothing while the
// password is typed, and with an input method on the keystrokes may never reach it; Enter
// then sends an empty or wrong password. RimArcho never asks for the password itself, so
// every surface says the same three things before a signed-in SteamCMD run may prompt.

// SteamCMDPasswordHints are those three things, in English (the interface translates them).
var SteamCMDPasswordHints = []string{
	"the password stays hidden while you type: type it, then press Enter",
	"switch the input method to English first",
	"right-click pastes into that window",
}

// SteamPasswordRefused is the failure text of a refused password: the hints again (#19).
const SteamPasswordRefused = "Steam did not accept the password. SteamCMD shows nothing while you type it: switch the input method to English, type the password (or right-click to paste it), then press Enter"

// SteamCMDSignsIn reports whether SteamCMD signs in as account, so it may ask for the
// password and the Steam Guard code.
func SteamCMDSignsIn(account string) bool {
	account = strings.TrimSpace(account)
	return account != "" && !strings.EqualFold(account, "anonymous")
}

// steamConsoleTitleKey is the console window's title in the locale's supplemental table.
const steamConsoleTitleKey = "@steamcmd.title"

// steamConsoleTitleEN is the title without a translation.
const steamConsoleTitleEN = "SteamCMD: password hidden while typing; English input; right-click pastes"

// consoleTitle is the title the next SteamCMD console window gets (console_windows.go).
var consoleTitle atomic.Pointer[string]

// steamConsoleTitle is the SteamCMD console window's title in the player's language.
func (a *App) steamConsoleTitle() string {
	lang := l10ndata.ResolveLang(a.Store.Meta("cfg:ui.lang"))
	if v := l10ndata.Extra(lang, steamConsoleTitleKey); v != steamConsoleTitleKey {
		return v
	}
	return steamConsoleTitleEN
}

// steamSignIn tells the surface that a SteamCMD run that may ask for the password starts
// (App.SteamCMDSignIn) and sets the console title; the returned func says it ended (once).
func (a *App) steamSignIn(signsIn bool) (end func()) {
	if a.Store != nil {
		t := a.steamConsoleTitle()
		consoleTitle.Store(&t)
	}
	if !signsIn || a.SteamCMDSignIn == nil {
		return func() {}
	}
	f := a.SteamCMDSignIn
	f(true)
	var once sync.Once
	return func() { once.Do(func() { f(false) }) }
}
