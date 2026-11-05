package app

// Automatic maintenance runs only while an interactive surface is open (16 §2.16).

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

var AutomaticKinds = []string{"session", "mods", "tools", "data", "self"}

type AutomaticStatus struct {
	Kind      string    `json:"kind"`
	Checked   time.Time `json:"checked"`
	Completed time.Time `json:"completed"`
	Pending   int       `json:"pending"`
	State     string    `json:"state"`
	// Latest is the newest RimArcho release the "self" check found (17 §5.1): a notice is
	// due again when it changes even though Pending stays 1.
	Latest string `json:"latest,omitempty"`
}

func (a *App) AutomaticEnabled() bool { return a.Store.Meta("cfg:updates.auto") != "off" }

// A single interval preset scales the defaults without four independent switches.
func (a *App) AutomaticInterval(kind string) time.Duration {
	base := 24 * time.Hour
	switch kind {
	case "session":
		base = 4 * time.Hour
	case "tools":
		base = 7 * 24 * time.Hour
	}
	switch a.Store.Meta("cfg:updates.interval") {
	case "frequent":
		base /= 2
	case "relaxed":
		base *= 2
	}
	return base
}

func (a *App) AutomaticStatus(kind string) AutomaticStatus {
	s := AutomaticStatus{Kind: kind, State: "not checked yet"}
	_ = json.Unmarshal([]byte(a.Store.Meta("automatic:"+kind)), &s)
	return s
}

func (a *App) AutomaticDue(kind string) bool {
	if !a.AutomaticEnabled() {
		return false
	}
	last := a.AutomaticStatus(kind).Checked
	interval := a.AutomaticInterval(kind)
	if kind == "session" {
		if a.Store.Meta("account:loverslab:enabled") != "true" {
			return false
		}
		s, err := accounts.Open(a.AccountsDir())
		if err != nil {
			return last.IsZero() || a.Now().Sub(last) >= a.AutomaticInterval(kind)
		}
		if !s.HasCookies() && s.Status().State == "not connected" {
			return false
		}
		if s.Status().Verified.After(last) {
			last = s.Status().Verified
		}
		if expiry := s.RenewalExpiry(); !expiry.IsZero() && expiry.After(a.Now()) && expiry.Before(a.Now().Add(2*interval)) {
			interval = min(interval, max(5*time.Minute, expiry.Sub(a.Now())/2))
		}
	}
	if kind == "self" {
		// A development build has no release to compare with; a manual check counts too.
		if _, ok := selfupdate.Compare(a.SelfVersionNow(), a.SelfVersionNow()); !ok {
			return false
		}
		if c := a.SelfUpdateInfo().Checked; c.After(last) {
			last = c
		}
	}
	if kind == "data" {
		// A manual download/check satisfies the automatic schedule too.
		infos := a.DataStatus()
		oldest := a.Now()
		for _, d := range Datasets {
			info := infos[d.Key]
			if info == nil || info.Source != a.DataSource(d) {
				oldest = time.Time{}
				break
			}
			checked := info.Checked
			if checked.IsZero() {
				checked = info.Fetched
			}
			if checked.Before(oldest) {
				oldest = checked
			}
		}
		if oldest.After(last) {
			last = oldest
		}
	}
	return last.IsZero() || a.Now().Sub(last) >= interval
}

// RunAutomatic records attempts before I/O, using compare-and-swap to coalesce
// concurrent windows. Callbacks allow surface task tracking and synthetic tests.
func (a *App) RunAutomatic(ctx context.Context, kind string, run func(context.Context) (int, string, error)) (bool, error) {
	return a.runAutomatic(ctx, kind, run, false)
}

// RunAutomaticNow runs a check the player asked for (`archo update --check`, the window's
// "check for updates"): recorded like the automatic one, without its switch or interval.
func (a *App) RunAutomaticNow(ctx context.Context, kind string) (bool, error) {
	return a.runAutomatic(ctx, kind, nil, true)
}

func (a *App) runAutomatic(ctx context.Context, kind string, run func(context.Context) (int, string, error), now bool) (bool, error) {
	key := "automatic:" + kind
	old := a.Store.Meta(key)
	if ctx.Err() != nil || !now && !a.AutomaticDue(kind) {
		return false, ctx.Err()
	}
	s := a.AutomaticStatus(kind)
	s.Checked, s.State = a.Now().UTC(), "checking"
	b, _ := json.Marshal(s)
	claimed := string(b)
	_, err := a.Store.DB.Exec(`INSERT OR IGNORE INTO meta(key,value) VALUES(?, '')`, key)
	if err != nil {
		return false, err
	}
	result, err := a.Store.DB.Exec(`UPDATE meta SET value=? WHERE key=? AND value=?`, string(b), key, old)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return false, nil
	}
	if run == nil {
		run = func(ctx context.Context) (int, string, error) { return a.automaticCheck(ctx, kind) }
	}
	previousCtx := a.checkContext
	a.checkContext = ctx
	defer func() { a.checkContext = previousCtx }()
	pending, state, checkErr := run(ctx)
	s.Pending, s.State = pending, state
	if kind == "self" {
		s.Latest = ""
		if r := a.SelfUpdateInfo().Release; r != nil {
			s.Latest = r.Version
		}
	}
	if checkErr != nil && s.State == "" {
		s.State = "check unavailable offline"
	}
	s.Completed = a.Now().UTC()
	b, _ = json.Marshal(s)
	// Do not overwrite a newer attempt from another window after a long suspension.
	_, err = a.Store.DB.Exec(`UPDATE meta SET value=? WHERE key=? AND value=?`, string(b), key, claimed)
	return true, errors.Join(checkErr, err)
}

func (a *App) automaticCheck(ctx context.Context, kind string) (int, string, error) {
	switch kind {
	case "mods":
		unlock := accounts.Lock(a.AccountsDir())
		defer unlock()
		var ll *subscribe.LoversLab
		if a.Store.Meta("account:loverslab:enabled") == "true" {
			if s, err := accounts.Open(a.AccountsDir()); err == nil && s.HasCookies() {
				ll = subscribe.NewLoversLab(s, a.httpClient().Transport)
			}
		}
		var failures []error
		for _, source := range []string{LibSteamCMD, LibGit, LibPage} {
			if ctx.Err() != nil {
				return 0, "check interrupted", ctx.Err()
			}
			failures = append(failures, a.CheckSubscriptionSource(source, func(page string) (*subscribe.PageInfo, error) {
				if ll != nil && subscribe.LoversLabPage(page) {
					return ll.PageInfo(ctx, page)
				}
				return subscribe.ReadPage(ctx, a.HTTP, page)
			}))
		}
		pending, err := a.PendingSubscriptionUpdates()
		joined := errors.Join(append(failures, err)...)
		state := "check complete"
		if joined != nil {
			state = "some checks failed"
		}
		return pending, state, joined
	case "tools":
		return a.CheckToolUpdates(ctx)
	case "session":
		unlock := accounts.Lock(a.AccountsDir())
		defer unlock()
		s, err := accounts.Open(a.AccountsDir())
		if err != nil {
			return 1, "login required", nil
		}
		if !s.HasCookies() {
			return 1, "login required", nil
		}
		ll := subscribe.NewLoversLab(s, a.httpClient().Transport)
		verifyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = ll.Verify(verifyCtx)
		cancel()
		st := s.Status()
		if st.State != "available" {
			return 1, "login required", err
		}
		if expiry := s.RenewalExpiry(); !expiry.IsZero() && expiry.Before(a.Now().Add(24*time.Hour)) {
			return 1, "session expires soon", err
		}
		if err != nil {
			return a.AutomaticStatus(kind).Pending, "check unavailable offline", err
		}
		return 0, "available", err
	case "self":
		return a.CheckSelfUpdate(ctx)
	case "data":
		_, err := a.UpdateData(nil, false)
		if err != nil {
			return 0, "stored data kept", err
		}
		return 0, "community data updated", nil
	}
	return 0, "", errors.New("unknown automatic check")
}

// RunAutomaticCycle owns this detached App; never call it on a page's connection.
func (a *App) RunAutomaticCycle(ctx context.Context, observe func(string, func() error)) {
	for _, kind := range AutomaticKinds {
		if ctx.Err() != nil {
			return
		}
		if !a.AutomaticDue(kind) {
			continue
		}
		run := func() error { _, err := a.RunAutomatic(ctx, kind, nil); return err }
		if observe != nil {
			observe(kind, run)
		} else {
			_ = run()
		}
	}
}

func (a *App) subscriptionContext() context.Context {
	if a.checkContext != nil {
		return a.checkContext
	}
	return context.Background()
}

func (a *App) PendingSubscriptionUpdates() (int, error) {
	ss, err := a.Subscriptions()
	if err != nil {
		return 0, err
	}
	latest := a.SubscriptionSourceCheck(LibSteamCMD).Latest
	n := 0
	for _, s := range ss {
		if s.Record.HasUpdate() {
			n++
			continue
		}
		if s.Record.Kind == "steamcmd" {
			installed := max(s.Mod.ACFTimeUpdated, s.Record.InstalledAt.Unix())
			if installed > 0 && latest[s.Mod.ID] > installed {
				n++
			}
		}
	}
	return n, nil
}

func (a *App) recordModUpdatesPending() {
	if a.Store.Meta("automatic:mods") == "" {
		return
	}
	if n, err := a.PendingSubscriptionUpdates(); err == nil {
		s := a.AutomaticStatus("mods")
		s.Pending = n
		b, _ := json.Marshal(s)
		_ = a.Store.SetMeta("automatic:mods", string(b))
	}
}
