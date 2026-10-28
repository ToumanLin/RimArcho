package gui

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestPlayerAccountForgetAndDownloadProvenance(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	api := newAPI(e.App)
	t.Cleanup(api.Close)
	if _, err := api.AccountAction("loverslab", "enable"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(e.App.AccountsDir(), "downloads", "fixture.zip")
	e.WriteFile(path, "offline-archive-fixture")
	e.WriteFile(filepath.Join(e.App.AccountsDir(), "loverslab.session"), "offline-cookie-sentinel")
	e.WriteFile(filepath.Join(e.App.AccountsDir(), "webview-profile", "Cookies"), "offline-profile-sentinel")
	page := "https://www.loverslab.com/files/file/123-fixture/"
	api.llFiles = map[string]loverDownload{"opaque-fixture-selection": {page: page, path: path}}
	other := "https://www.loverslab.com/files/file/456-other/"
	if _, err := api.LoverInstallPreview(other, "opaque-fixture-selection", ""); err == nil {
		t.Fatal("preview accepted another page's download")
	}
	if _, err := api.LoverInstall(other, "opaque-fixture-selection", ""); err == nil {
		t.Fatal("install accepted another page's download")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("invalid request consumed player's file")
	}
	if _, err := api.AccountAction("loverslab", "forget"); err != nil {
		t.Fatal(err)
	}
	_ = endedTask(t, api, view.TaskAccount)
	if _, err := os.Stat(e.App.AccountsDir()); !os.IsNotExist(err) {
		t.Fatal("session/profile/download remains")
	}
	if len(api.llFiles) != 0 || e.App.Store.Meta("account:loverslab:enabled") != "" {
		t.Fatal("forgotten session enabled")
	}
	b, err := os.ReadFile(filepath.Join(e.Home, "tasks.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"offline-cookie-sentinel", "offline-profile-sentinel", "opaque-fixture-selection", page, "fixture.zip"} {
		if strings.Contains(string(b), secret) {
			t.Fatal("account task contains private data")
		}
	}
	if _, err := api.AccountAction("loverslab", "import-cookies"); err == nil {
		t.Fatal("unexpected credential action")
	}
}

type accountTransport func(*http.Request) (*http.Response, error)

func (f accountTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAutomaticSessionVerifiesWithoutOpeningSettings(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "available", true: "challenge"}[fail], func(t *testing.T) {
			e := testkit.New(t)
			clock := time.Now().Add(5 * time.Hour)
			e.App.Now = func() time.Time { return clock }
			if err := e.App.Store.SetMeta("account:loverslab:enabled", "true"); err != nil {
				t.Fatal(err)
			}
			session, err := accounts.Open(e.App.AccountsDir())
			if err != nil {
				t.Fatal(err)
			}
			u, _ := url.Parse(accounts.Site)
			session.Jar().SetCookies(u, []*http.Cookie{{Name: "ips4_login_key", Value: "offline-key", Path: "/", Secure: true, Expires: time.Now().AddDate(0, 6, 0)}})
			if err = session.SetStatus("available", ""); err != nil {
				t.Fatal(err)
			}
			calls := 0
			api := newAPI(e.App)
			t.Cleanup(api.Close)
			e.App.HTTP.Transport = accountTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" || r.URL.Path != "/settings/" {
					t.Error("not a single verification GET", r.URL)
				}
				body := `<a href="?do=logout">logout</a>`
				status := 200
				if fail {
					status = 403
					body = "cf-chl-fixture"
				}
				headers := make(http.Header)
				if !fail {
					headers.Add("Set-Cookie", "ips4_login_key=renewed-offline-key; Path=/; Secure; HttpOnly; Max-Age=20000000")
				}
				return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			for _, kind := range []string{"mods", "tools", "data"} {
				_, err := e.App.RunAutomatic(context.Background(), kind, func(context.Context) (int, string, error) { return 0, "check complete", nil })
				if err != nil {
					t.Fatal(err)
				}
			}
			first := api.Settings()
			if len(first) == 0 || calls != 0 {
				t.Fatal("settings performed network I/O", calls)
			}
			api.automaticCycle(context.Background())
			if calls != 1 {
				t.Fatal("scheduler did not verify", calls)
			}
			reopened, err := accounts.Open(e.App.AccountsDir())
			if err != nil {
				t.Fatal(err)
			}
			if !fail {
				cookies := reopened.Jar().Cookies(u)
				if len(cookies) != 1 || cookies[0].Value != "renewed-offline-key" || !reopened.RenewalExpiry().After(session.RenewalExpiry()) {
					t.Fatal("renewed cookie was not persisted")
				}
			}
			for _, path := range []string{filepath.Join(e.App.AccountsDir(), "loverslab.session"), filepath.Join(e.Home, "tasks.json")} {
				b, err := os.ReadFile(path)
				if err != nil || strings.Contains(string(b), "offline-key") {
					t.Fatal("missing encrypted session or leaked cookie", err)
				}
			}
			if strings.Contains(e.App.Store.Meta("automatic:session"), "offline-key") {
				t.Fatal("cookie leaked into metadata")
			}
			for range 3 {
				api.Settings()
			}
			if calls != 1 {
				t.Fatal("verification loop", calls)
			}
			if p := view.BuildTasks(api.tasks.List(), clock); len(p.Rows) != 1 || p.Unseen != 0 {
				t.Fatal("automatic task is missing or nagged", p)
			}
			clock = clock.Add(time.Hour)
			api.Settings()
			api.automaticCycle(context.Background())
			if calls != 1 {
				t.Fatal("checked too soon")
			}
		})
	}
}

func TestAutomaticSessionWarnsWhenServerDoesNotExtendExpiry(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	clock := time.Now().Add(5 * time.Hour)
	e.App.Now = func() time.Time { return clock }
	if err := e.App.Store.SetMeta("account:loverslab:enabled", "true"); err != nil {
		t.Fatal(err)
	}
	s, err := accounts.Open(e.App.AccountsDir())
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(accounts.Site)
	s.Jar().SetCookies(u, []*http.Cookie{
		{Name: "ips4_login_key", Value: "expiry-fixture", Path: "/", Secure: true, Expires: time.Now().Add(6 * time.Hour)},
		{Name: "ips4_device_key", Value: "device-fixture", Path: "/", Secure: true, Expires: time.Now().AddDate(0, 6, 0)},
		{Name: "ips4_session", Value: "session-fixture", Path: "/", Secure: true, Expires: time.Now().Add(5 * time.Minute)},
	})
	if err := s.SetStatus("available", ""); err != nil {
		t.Fatal(err)
	}
	if expiry := s.RenewalExpiry(); expiry.Before(clock) || expiry.After(clock.Add(2*time.Hour)) {
		t.Fatal("expiry used device or short session cookie", expiry)
	}
	e.App.HTTP.Transport = accountTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Request: r, Body: io.NopCloser(strings.NewReader(`<a href="?do=logout">logout</a>`))}, nil
	})
	if ran, err := e.App.RunAutomatic(context.Background(), "session", nil); !ran || err != nil {
		t.Fatal(ran, err)
	}
	status := e.App.AutomaticStatus("session")
	if status.Pending != 1 || status.State != "session expires soon" {
		t.Fatal("verification hid impending expiry", status)
	}
	if line, _ := view.LoversLabLine(e.App); line != view.T("session needs confirmation; log in again") {
		t.Fatal("account row hid expiry warning", line)
	}
	if e.App.AutomaticDue("session") {
		t.Fatal("immediate renewal loop")
	}
	clock = clock.Add(35 * time.Minute)
	if !e.App.AutomaticDue("session") {
		t.Fatal("renewal was not brought forward before expiry")
	}
}
