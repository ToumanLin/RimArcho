package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/l10ndata"
)

func (a *App) AccountsDir() string { return filepath.Join(a.LocalDir(), "accounts") }

type SteamAccountStatus struct {
	Account          string    `json:"account"`
	Path             string    `json:"path"`
	Source           string    `json:"source"`
	Managed          bool      `json:"managed"`
	State            string    `json:"state"`
	Verified         time.Time `json:"verified"`
	LoggedIn         bool      `json:"loggedIn"`
	OwnsRimWorld     bool      `json:"ownsRimWorld"`
	OwnershipChecked time.Time `json:"ownershipChecked"`
}

func steamStateKey(exe, account string) string {
	h := sha256.Sum256([]byte(strings.ToLower(exe) + "\x00" + account))
	return "account:steamcmd:" + hex.EncodeToString(h[:])
}
func (a *App) steamRecordKey(exe, account string) string {
	return steamStateKey(exe, account) + ":" + a.Store.Meta(steamStateKey(exe, "cache-generation"))
}
func (a *App) SteamAccountStatus() SteamAccountStatus {
	i := a.Deps().Status(deps.SteamCMD)
	account, _ := a.SteamCMDAccount("")
	s := SteamAccountStatus{Account: account, Path: i.Path, Source: i.Source, Managed: i.Own, State: "account not checked yet"}
	var saved SteamAccountStatus
	hasRecord := json.Unmarshal([]byte(a.Store.Meta(a.steamRecordKey(i.Path, account))), &saved) == nil
	if hasRecord {
		s.State, s.Verified = saved.State, saved.Verified
		s.LoggedIn, s.OwnsRimWorld, s.OwnershipChecked = saved.LoggedIn, saved.OwnsRimWorld, saved.OwnershipChecked
		if l10ndata.LegacyEnglish(saved.State) == "signed in" {
			s.LoggedIn = true
		}
	}
	if account == "anonymous" {
		s.State = "anonymous"
		s.LoggedIn, s.OwnsRimWorld = false, false
	} else if !hasRecord && i.Own {
		if _, err := os.Stat(filepath.Join(i.Dir, "config", "config.vdf")); err == nil {
			s.State = "possible cached login, not checked yet"
		}
	}
	if i.Path == "" {
		s.State = "SteamCMD is not installed"
	}
	return s
}
func (a *App) steamAccountRecord(exe, account, state string, success bool) {
	var s SteamAccountStatus
	key := a.steamRecordKey(exe, account)
	_ = json.Unmarshal([]byte(a.Store.Meta(key)), &s)
	s.State = state
	s.LoggedIn = success && account != "anonymous"
	if success {
		s.Verified = a.Now().UTC()
	}
	b, _ := json.Marshal(s)
	_ = a.Store.SetMeta(key, string(b))
}

// SteamAccountAction is player-only; it must run outside the GUI mutex.
func (a *App) SteamAccountAction(action string) error {
	i := a.Deps().Status(deps.SteamCMD)
	if action == "anonymous" {
		return a.Store.SetMeta("cfg:steamcmd.account", "")
	}
	if i.Path == "" {
		return ErrNoSteamCMD
	}
	un := accounts.Lock(i.Path)
	defer un()
	account, err := a.SteamCMDAccount("")
	if err != nil {
		return err
	}
	if !i.Own {
		return errors.New("external SteamCMD manages its own login")
	}
	if action == "local-forget" {
		if err := a.forgetManagedSteam(); err != nil {
			return err
		}
		a.steamAccountRecord(i.Path, account, "login required", false)
		return a.Store.SetMeta("cfg:steamcmd.account", "")
	}
	if action != "login" && action != "verify" && action != "logout" {
		return errors.New("unknown account action")
	}
	if action == "login" && account == "anonymous" {
		return errors.New("enter a Steam account name first")
	}
	run := func(args []string, console bool) ([]string, error) {
		log := filepath.Join(filepath.Dir(i.Path), "logs", "console_log.txt")
		var off int64
		if st, e := os.Stat(log); e == nil {
			off = st.Size()
		}
		p, e := a.steamCMD().start(i.Path, args, console)
		if e != nil {
			return nil, e
		}
		e = p.Wait()
		tail := &logTail{path: log, off: off}
		lines := tail.lines()
		if tail.partial != "" {
			lines = append(lines, tail.partial)
		}
		return lines, e
	}
	if action == "logout" {
		lines, runErr := run([]string{"+logout", "+quit"}, false)
		confirmed := false
		for _, line := range lines {
			low := strings.ToLower(line)
			confirmed = confirmed || strings.Contains(low, "logged out") || strings.Contains(low, "logging off")
		}
		if runErr != nil || !confirmed {
			if err := a.forgetManagedSteam(); err != nil {
				return err
			}
		}
		a.steamAccountRecord(i.Path, account, "signed out", false)
		return a.Store.SetMeta("cfg:steamcmd.account", "")
	}
	if action == "login" {
		p, err := a.steamCMD().start(i.Path, []string{"+login", account}, true)
		if err != nil {
			a.steamAccountRecord(i.Path, account, "login required", false)
			return errors.New("SteamCMD window did not complete; try signing in again")
		}
		// Closing the console with X can return a nonzero exit code even after
		// authentication. The cached check below decides whether login succeeded.
		_ = p.Wait()
	}
	// The cached check never receives a password and cannot prompt in the GUI process.
	lines, runErr := run([]string{"+login", account, "+licenses_print", "+quit"}, false)
	login := steamLogin{account: account}
	for _, line := range lines {
		if !login.ready {
			login.read(line)
		}
	}
	if login.ready && login.err == nil && runErr == nil {
		a.steamAccountRecord(i.Path, account, "signed in", true)
		key := a.steamRecordKey(i.Path, account)
		var saved SteamAccountStatus
		_ = json.Unmarshal([]byte(a.Store.Meta(key)), &saved)
		saved.OwnsRimWorld = account != "anonymous" && steamOwnsRimWorld(strings.Join(lines, "\n"))
		saved.OwnershipChecked = a.Now().UTC()
		b, _ := json.Marshal(saved)
		return a.Store.SetMeta(key, string(b))
	}
	state := "login required"
	if login.err != nil && login.err.Kind == "network" {
		state = "network error; login status unknown"
	}
	a.steamAccountRecord(i.Path, account, state, false)
	return errors.New(state + "; sign in again to complete password and Steam Guard verification")
}

// Never select config/PATH instances, read token contents or leave auth backups.
func (a *App) forgetManagedSteam() (result error) {
	dir := filepath.Join(a.LocalDir(), "steamcmd")
	defer func() {
		if result == nil {
			result = a.Store.SetMeta(steamStateKey(filepath.Join(dir, "steamcmd.exe"), "cache-generation"), time.Now().UTC().Format(time.RFC3339Nano))
		}
	}()
	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || !samePath(resolved, dir) {
		return errors.New("managed SteamCMD path was redirected; restore it before signing out")
	}
	var paths []string
	for _, pattern := range []string{"config/config.vdf", "ssfn*", "*.sentry", "config/*.sentry", "config/ssfn*"} {
		ms, e := filepath.Glob(filepath.Join(dir, filepath.FromSlash(pattern)))
		if e != nil {
			return errors.New("authentication cache cleanup failed")
		}
		paths = append(paths, ms...)
	}
	for _, p := range paths {
		r, e := filepath.EvalSymlinks(p)
		if e != nil || !samePath(r, p) {
			return errors.New("authentication cache path was redirected; nothing was removed")
		}
		fi, e := os.Lstat(p)
		if e != nil || fi.IsDir() {
			return errors.New("unknown authentication cache layout; sign out in SteamCMD and retry")
		}
	}
	for _, p := range paths {
		if os.Remove(p) != nil {
			return errors.New("authentication cache cleanup failed; close SteamCMD and retry")
		}
	}
	return nil
}
func (a *App) ForgetAccounts() error {
	un := accounts.Lock(filepath.Join(a.LocalDir(), "steamcmd", "steamcmd.exe"))
	defer un()
	unLL := accounts.Lock(a.AccountsDir())
	defer unLL()
	if err := accounts.Forget(a.AccountsDir()); err != nil {
		return err
	}
	if err := a.forgetManagedSteam(); err != nil {
		return err
	}
	return a.Store.SetMeta("cfg:steamcmd.account", "")
}
