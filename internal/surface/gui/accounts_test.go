package gui

import (
	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestSettingsStaleSessionVerifiesOnceIncludingFailure(t *testing.T) {
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
			api.ll = subscribe.NewLoversLab(session, accountTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" || r.URL.Query().Get("controller") != "settings" {
					t.Error("not a single verification GET", r.URL)
				}
				body := `<a href="?do=logout">logout</a>`
				status := 200
				if fail {
					status = 403
					body = "cf-chl-fixture"
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			}))
			first := api.Settings()
			if len(first) == 0 || calls != 1 {
				t.Fatal("stale session was not checked once", calls)
			}
			for range 3 {
				api.Settings()
			}
			if calls != 1 {
				t.Fatal("verification loop", calls)
			}
			if len(api.tasks.List()) != 0 {
				t.Fatal("silent check created a task")
			}
			clock = clock.Add(time.Hour)
			api.Settings()
			if calls != 1 {
				t.Fatal("checked too soon")
			}
		})
	}
}
