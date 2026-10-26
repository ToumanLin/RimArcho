package app_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

type accountProc struct{ wait func() error }

func (p accountProc) Wait() error { return p.wait() }

func TestSteamAccountWindowVerificationAndManagedForget(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.App.SteamCMDNoPath = true
	dir := filepath.Join(e.App.LocalDir(), "steamcmd")
	exe := filepath.Join(dir, "steamcmd.exe")
	e.WriteFile(exe, "fake executable marker")
	e.MustRun("config", "set", "steamcmd.account", "fixture-player")
	log := filepath.Join(dir, "logs", "console_log.txt")
	e.MkdirAll(filepath.Dir(log))
	var args []string
	line := "Logging in user 'fixture-player' to Steam Public...OK\nWaiting for user info...OK\n"
	failed := false
	e.App.SteamCMDStart = func(got string, argv []string, console bool) (app.SteamCMDProc, error) {
		if got != exe {
			t.Fatal("wrong instance/window")
		}
		args = append([]string{}, argv...)
		return accountProc{func() error {
			f, err := os.OpenFile(log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = f.WriteString(line)
			if failed {
				return errors.New("offline fixture failure")
			}
			return err
		}}, nil
	}
	if err := e.App.SteamAccountAction("login"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args, []string{"+login", "fixture-player", "+licenses_print", "+quit"}) {
		t.Fatal(args)
	}
	if s := e.App.SteamAccountStatus(); s.Verified.IsZero() || !s.LoggedIn {
		t.Fatal(s)
	}
	if err := e.App.SteamAccountAction("verify"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args, []string{"+login", "fixture-player", "+licenses_print", "+quit"}) {
		t.Fatal(args)
	}
	line = "Logging in user 'different-player' to Steam Public...OK\nWaiting for user info...OK\n"
	if err := e.App.SteamAccountAction("verify"); err == nil {
		t.Fatal("wrong identity accepted")
	}
	failed = true
	if err := e.App.SteamAccountAction("logout"); err != nil {
		t.Fatal("local logout fallback failed", err)
	}
	if !reflect.DeepEqual(args, []string{"+logout", "+quit"}) {
		t.Fatal(args)
	}
	for _, p := range []string{"config/config.vdf", "ssfn123", "legacy.sentry"} {
		e.WriteFile(filepath.Join(dir, filepath.FromSlash(p)), "offline-auth-fixture")
	}
	mod := filepath.Join(dir, "steamapps", "workshop", "content", "294100", "1", "About", "About.xml")
	e.WriteFile(mod, "player mod fixture")
	if err := e.App.SteamAccountAction("local-forget"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"config/config.vdf", "ssfn123", "legacy.sentry"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); !os.IsNotExist(err) {
			t.Fatal("cache retained")
		}
	}
	if _, err := os.Stat(mod); err != nil {
		t.Fatal("mod changed")
	}
	external := filepath.Join(e.Root, "external", "steamcmd.exe")
	e.WriteFile(external, "external")
	externalCache := filepath.Join(filepath.Dir(external), "config", "config.vdf")
	e.WriteFile(externalCache, "external-auth-fixture")
	e.MustRun("config", "set", "steamcmd.path", external)
	if s := e.App.SteamAccountStatus(); s.Managed || !s.Verified.IsZero() {
		t.Fatal("path reused state", s)
	}
	if err := e.App.SteamAccountAction("local-forget"); err == nil {
		t.Fatal("external cache takeover")
	}
	if err := e.App.ForgetAccounts(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(externalCache); err != nil {
		t.Fatal("external cache removed")
	}
}

func TestFakeSteamCMDScript(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("Windows console stub")
	}
	e := testkit.New(t)
	e.App.SteamCMDNoPath = true
	dir := filepath.Join(e.App.LocalDir(), "steamcmd")
	e.WriteFile(filepath.Join(dir, "steamcmd.exe"), "stub marker")
	script := filepath.Join(dir, "fake.cmd")
	e.WriteFile(script, "@echo off\r\nif not exist logs mkdir logs\r\necho Logging in user 'fixture-player' to Steam Public...OK>>logs\\console_log.txt\r\necho Waiting for user info...OK>>logs\\console_log.txt\r\n")
	e.MustRun("config", "set", "steamcmd.account", "fixture-player")
	e.App.SteamCMDStart = func(_ string, args []string, console bool) (app.SteamCMDProc, error) {
		cmd := exec.Command("cmd.exe", "/c", script)
		cmd.Dir = dir
		if cmd.Stdin != nil {
			t.Fatal("stdin captured")
		}
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return cmd, nil
	}
	if err := e.App.SteamAccountAction("verify"); err != nil {
		t.Fatal(err)
	}
}

func TestUninstallForgetsSessionsBeforeKeptSteamCMD(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.App.SteamCMDNoPath = true
	e.App.ArchoProbe = func() []int { return nil }
	e.App.AfterExitFunc = func(app.AfterExit) error { return nil }
	dir := filepath.Join(e.App.LocalDir(), "steamcmd")
	e.WriteFile(filepath.Join(dir, "steamcmd.exe"), "stub")
	cache := filepath.Join(dir, "config", "config.vdf")
	e.WriteFile(cache, "offline-auth-fixture")
	mod := filepath.Join(dir, "steamapps", "workshop", "content", "294100", "1", "About", "About.xml")
	e.WriteFile(mod, "<ModMetaData/>")
	session := filepath.Join(e.App.AccountsDir(), "loverslab.session")
	e.WriteFile(session, "encrypted-fixture-marker")
	e.WriteFile(filepath.Join(e.App.AccountsDir(), "webview-profile", "cache"), "profile fixture")
	p, err := e.App.PlanUninstall(app.UninstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Steps[0].Key != app.UnAccounts {
		t.Fatal("forget is not first")
	}
	var steps []string
	_, err = e.App.Uninstall(app.UninstallOptions{}, func(p app.UninstallProgress) { steps = append(steps, p.Step) })
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) == 0 || steps[0] != app.UnAccounts {
		t.Fatal(steps)
	}
	for _, path := range []string{cache, session} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("auth retained")
		}
	}
	if _, err := os.Stat(mod); err != nil {
		t.Fatal("kept mod removed")
	}
}

func TestUninstallForgetFailurePreservesToolsAndData(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.App.SteamCMDNoPath = true
	e.App.ArchoProbe = func() []int { return nil }
	called := false
	e.App.AfterExitFunc = func(app.AfterExit) error { called = true; return nil }
	dir := filepath.Join(e.App.LocalDir(), "steamcmd")
	exe := filepath.Join(dir, "steamcmd.exe")
	e.WriteFile(exe, "stub")
	// An unknown auth-cache layout must stop uninstall before deleting tools or DB.
	e.MkdirAll(filepath.Join(dir, "config", "config.vdf"))
	_, err := e.App.Uninstall(app.UninstallOptions{}, nil)
	if err == nil || called || e.App.Uninstalled {
		t.Fatal("forget failure did not stop uninstall", err)
	}
	if _, err = os.Stat(exe); err != nil {
		t.Fatal("tool removed after forget failure")
	}
	if _, err = e.App.Store.Envs(); err != nil {
		t.Fatal("database removed after forget failure", err)
	}
}

func TestSteamLoginRequiresNameButVerifyAllowsAnonymous(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.App.SteamCMDNoPath = true
	e.WriteFile(filepath.Join(e.App.LocalDir(), "steamcmd", "steamcmd.exe"), "offline fixture")
	calls := 0
	e.App.SteamCMDStart = func(_ string, args []string, _ bool) (app.SteamCMDProc, error) {
		calls++
		if !reflect.DeepEqual(args, []string{"+login", "anonymous", "+licenses_print", "+quit"}) {
			t.Fatal(args)
		}
		return accountProc{func() error { return nil }}, nil
	}
	for _, name := range []string{"", "anonymous", "ANONYMOUS"} {
		e.MustRun("config", "set", "steamcmd.account", name)
		if err := e.App.SteamAccountAction("login"); err == nil || err.Error() != "enter a Steam account name first" {
			t.Fatal(err)
		}
	}
	if calls != 0 {
		t.Fatal("anonymous login launched")
	}
	_ = e.App.SteamAccountAction("verify")
	if calls != 1 {
		t.Fatal("anonymous verification was blocked")
	}
}

func TestSteamAccountVerificationLogFormats(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		sample, account string
		success         bool
	}{
		{"anonymous", "", true}, {"user", "fixture-player", true},
		{"failed", "fixture-player", false}, {"one-line", "fixture-player", true},
	} {
		t.Run(tc.sample, func(t *testing.T) {
			e := testkit.New(t)
			e.App.SteamCMDNoPath = true
			exe := filepath.Join(e.App.LocalDir(), "steamcmd", "steamcmd.exe")
			e.WriteFile(exe, "offline fixture")
			e.MustRun("config", "set", "steamcmd.account", tc.account)
			sample, err := os.ReadFile(filepath.Join("testdata", "steam-login", tc.sample+".log"))
			if err != nil {
				t.Fatal(err)
			}
			e.App.SteamCMDStart = func(_ string, _ []string, _ bool) (app.SteamCMDProc, error) {
				return accountProc{func() error {
					log := filepath.Join(filepath.Dir(exe), "logs", "console_log.txt")
					e.WriteFile(log, string(sample))
					return nil
				}}, nil
			}
			err = e.App.SteamAccountAction("verify")
			if (err == nil) != tc.success {
				t.Fatalf("verify: %v", err)
			}
			if verified := !e.App.SteamAccountStatus().Verified.IsZero(); verified != tc.success {
				t.Fatalf("verified=%v", verified)
			}
		})
	}
}

func TestSteamLoginAutomaticallyVerifiesOwnershipAndLogoutFallsBack(t *testing.T) {
	t.Parallel()
	for _, owned := range []bool{false, true} {
		t.Run(map[bool]string{false: "no game", true: "owns game"}[owned], func(t *testing.T) {
			e := testkit.New(t)
			e.App.SteamCMDNoPath = true
			dir := filepath.Join(e.App.LocalDir(), "steamcmd")
			exe := filepath.Join(dir, "steamcmd.exe")
			e.WriteFile(exe, "offline fixture")
			e.MustRun("config", "set", "steamcmd.account", "dummyPlayer")
			calls := 0
			e.App.SteamCMDStart = func(_ string, args []string, console bool) (app.SteamCMDProc, error) {
				calls++
				if calls == 1 && (!console || !reflect.DeepEqual(args, []string{"+login", "dummyPlayer"})) {
					t.Fatal(args, console)
				}
				if calls == 2 && (console || !reflect.DeepEqual(args, []string{"+login", "dummyPlayer", "+licenses_print", "+quit"})) {
					t.Fatal(args, console)
				}
				if calls > 2 {
					return nil, errors.New("offline")
				}
				return accountProc{func() error {
					text := "Logging in user 'dummyPlayer'...\nOK\nWaiting for user info...\nOK\nLicense packageID 12345:\n Apps: 123\n"
					if owned {
						text += "License packageID 54321:\n Apps: 294100 (RimWorld)\n"
					}
					path := filepath.Join(dir, "logs", "console_log.txt")
					e.MkdirAll(filepath.Dir(path))
					f, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
					if e != nil {
						return e
					}
					defer f.Close()
					_, e = f.WriteString(text)
					if e == nil && calls == 1 {
						return errors.New("console closed with X")
					}
					return e
				}}, nil
			}
			if err := e.App.SteamAccountAction("login"); err != nil {
				t.Fatal(err)
			}
			status := e.App.SteamAccountStatus()
			if calls != 2 || !status.LoggedIn || status.OwnsRimWorld != owned || status.OwnershipChecked.IsZero() {
				t.Fatal(calls, status)
			}
			for _, p := range []string{"config/config.vdf", "ssfn123"} {
				e.WriteFile(filepath.Join(dir, filepath.FromSlash(p)), "offline auth fixture")
			}
			mod := filepath.Join(dir, "steamapps", "workshop", "content", "294100", "1", "About", "About.xml")
			e.WriteFile(mod, "mod fixture")
			if err := e.App.SteamAccountAction("logout"); err != nil {
				t.Fatal(err)
			}
			if e.App.SteamAccountStatus().LoggedIn {
				t.Fatal("still logged in")
			}
			if _, err := os.Stat(filepath.Join(dir, "config", "config.vdf")); !os.IsNotExist(err) {
				t.Fatal("cache retained")
			}
			if _, err := os.Stat(mod); err != nil {
				t.Fatal("mod touched", err)
			}
		})
	}
}
