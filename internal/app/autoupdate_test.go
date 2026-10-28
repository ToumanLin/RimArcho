package app_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestAutomaticLoopStartupIntervalAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		go app.AutomaticLoop(ctx, func(context.Context) { calls++ })
		time.Sleep(19 * time.Second)
		synctest.Wait()
		if calls != 0 {
			t.Fatal("startup check blocked first paint")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if calls != 1 {
			t.Fatal(calls)
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if calls != 2 {
			t.Fatal(calls)
		}
		cancel()
		synctest.Wait()
		time.Sleep(time.Hour)
		synctest.Wait()
		if calls != 2 {
			t.Fatal("resident checks after close", calls)
		}
	})
}

func TestAutomaticSchedulePersistsFailuresAndCoalescesWindows(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	ctx := context.Background()
	calls := 0
	check := func(context.Context) (int, string, error) {
		calls++
		return 2, "check unavailable offline", errors.New("offline fixture")
	}
	ran, err := e.App.RunAutomatic(ctx, "mods", check)
	if !ran || err == nil || calls != 1 {
		t.Fatal(ran, err, calls)
	}
	other, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if ran, err := other.RunAutomatic(ctx, "mods", check); ran || err != nil {
		t.Fatal("restart retried failure", ran, err)
	}
	if got := other.AutomaticStatus("mods"); got.Pending != 2 || !got.Checked.Equal(e.Now()) || got.Completed.IsZero() {
		t.Fatal(got)
	}
	e.Advance(24*time.Hour - time.Second)
	if e.App.AutomaticDue("mods") {
		t.Fatal("early check")
	}
	e.Advance(time.Second)
	// A nested second window must see the committed claim before the first starts I/O.
	ran, err = e.App.RunAutomatic(ctx, "mods", func(context.Context) (int, string, error) {
		if ran, err := other.RunAutomatic(ctx, "mods", check); ran || err != nil {
			t.Fatal("duplicate check", ran, err)
		}
		return 0, "check complete", nil
	})
	if !ran || err != nil || calls != 1 {
		t.Fatal(ran, err, calls)
	}
	e.MustRun("config", "set", "updates.auto", "off")
	e.Advance(30 * 24 * time.Hour)
	for _, kind := range app.AutomaticKinds {
		if e.App.AutomaticDue(kind) {
			t.Fatal("disabled", kind)
		}
	}
	e.MustRun("config", "set", "updates.auto", "on")
	e.MustRun("config", "set", "updates.interval", "frequent")
	if got := e.App.AutomaticInterval("tools"); got != 84*time.Hour {
		t.Fatal(got)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if ran, err := e.App.RunAutomatic(cancelled, "tools", check); ran || !errors.Is(err, context.Canceled) {
		t.Fatal(ran, err)
	}
}

func TestAutomaticCommunityDataConditionalChecksAndFailure(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	conditional := false
	fail := false
	calls := 0
	modified := "Wed, 21 Oct 2026 07:28:00 GMT"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if fail {
			http.Error(w, "offline", 503)
			return
		}
		if conditional {
			if r.Header.Get("If-None-Match") != `"fixture"` || r.Header.Get("If-Modified-Since") != modified {
				t.Error("conditional headers missing", r.Header)
			}
			w.WriteHeader(304)
			return
		}
		w.Header().Set("ETag", `"fixture"`)
		w.Header().Set("Last-Modified", modified)
		switch r.URL.Path {
		case "/rules":
			io.WriteString(w, `{"timestamp":1,"rules":{}}`)
		case "/steamdb":
			io.WriteString(w, `{"version":1,"database":{}}`)
		case "/replacements":
			io.WriteString(w, `{"version":"one","rules":[]}`)
		}
	}))
	defer srv.Close()
	for _, ds := range app.Datasets {
		e.MustRun("config", "set", "data."+ds.Key+".url", srv.URL+"/"+ds.Key)
	}
	if ran, err := e.App.RunAutomatic(context.Background(), "data", nil); !ran || err != nil {
		t.Fatal(ran, err)
	}
	first := e.App.DataStatus()["rules"]
	original, err := os.ReadFile(filepath.Join(e.Home, "cache", "communityRules.json"))
	if err != nil {
		t.Fatal(err)
	}
	e.Advance(24 * time.Hour)
	conditional = true
	if ran, err := e.App.RunAutomatic(context.Background(), "data", nil); !ran || err != nil {
		t.Fatal(ran, err)
	}
	second := e.App.DataStatus()["rules"]
	if !second.Checked.After(first.Checked) || !second.Fetched.Equal(first.Fetched) || calls != 6 {
		t.Fatal("304 sidecar", first, second, calls)
	}
	if e.App.AutomaticDue("data") {
		t.Fatal("304 remains due")
	}
	e.Advance(24 * time.Hour)
	fail = true
	if _, err := e.App.RunAutomatic(context.Background(), "data", nil); err == nil {
		t.Fatal("failure hidden")
	}
	got, _ := os.ReadFile(filepath.Join(e.Home, "cache", "communityRules.json"))
	if string(got) != string(original) || !e.App.DataStatus()["rules"].Checked.Equal(second.Checked) {
		t.Fatal("failure changed stored data")
	}
	if e.App.AutomaticDue("data") {
		t.Fatal("failed fetch retried immediately")
	}
	if s := e.App.AutomaticStatus("data"); s.State != "stored data kept" || s.Pending != 0 {
		t.Fatal(s)
	}
}

func TestAutomaticSteamCMDModsCheckOnlyAndKeepsPendingOffline(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(true)
	folder := filepath.Join(e.SteamCMD, "1002")
	e.WriteMod(folder, testkit.Sample("tester.cmd", "CMD fixture"))
	if err := subscribe.Write(folder, &subscribe.Record{Kind: "steamcmd", WorkshopID: "1002", InstalledAt: e.Now()}); err != nil {
		t.Fatal(err)
	}
	e.MustRun("scan")
	calls := 0
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if fail {
			http.Error(w, "offline", 503)
			return
		}
		fmt.Fprintf(w, `{"response":{"publishedfiledetails":[{"publishedfileid":"1002","result":1,"time_updated":%d}]}}`, e.Now().Add(time.Hour).Unix())
	}))
	defer server.Close()
	e.App.SteamAPI = server.URL
	downloads := 0
	e.App.SteamCMDRun = func(string, []string) (string, error) { downloads++; return "", errors.New("must not download") }
	// A game running must not prevent a metadata-only check.
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42} }
	if ran, err := e.App.RunAutomatic(context.Background(), "mods", nil); !ran || err != nil {
		t.Fatal(ran, err)
	}
	if s := e.App.AutomaticStatus("mods"); s.Pending != 1 {
		t.Fatal(s)
	}
	e.Advance(24 * time.Hour)
	fail = true
	if _, err := e.App.RunAutomatic(context.Background(), "mods", nil); err == nil {
		t.Fatal("offline check succeeded")
	}
	// The failed request is sent once more.
	if s := e.App.AutomaticStatus("mods"); s.Pending != 1 || calls != 3 || downloads != 0 {
		t.Fatal(s, calls, downloads)
	}
	if _, err := e.App.UpdateSubscriptions(nil, false); err == nil || !strings.Contains(err.Error(), "game is running") {
		t.Fatal("update allowed while game runs", err)
	}
}

type automaticTestTransport func(*http.Request) (*http.Response, error)

func (f automaticTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAutomaticToolVersionsAndOfflineRetention(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.App.SteamCMDNoPath, e.App.ToddsNoPath = true, true
	e.App.TextureEncoder = &texEnc{}
	steam := filepath.Join(e.App.LocalDir(), "steamcmd")
	e.WriteFile(filepath.Join(steam, "steamcmd.exe"), "fixture executable")
	e.WriteFile(filepath.Join(steam, "package", "steam_cmd_win32.installed"), `"steam_cmd_win32" { "version" "100" }`)
	e.WriteFile(filepath.Join(e.App.LocalDir(), "todds", "todds.exe"), "fixture encoder")
	failed := false
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if failed {
			http.Error(w, "offline", 503)
			return
		}
		switch r.URL.Path {
		case "/steam_cmd_win32":
			io.WriteString(w, `"steam_cmd_win32" { "version" "101" }`)
		case "/repos/todds-encoder/todds/releases/latest":
			io.WriteString(w, `{"tag_name":"0.5.0"}`)
		default:
			t.Error("unexpected tool download", r.URL)
			http.Error(w, "unexpected", 500)
		}
	}))
	defer srv.Close()
	e.App.WezTermAPI = srv.URL
	base := e.App.HTTP.Transport
	e.App.HTTP.Transport = automaticTestTransport(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "steamstatic.com") {
			req, err := http.NewRequestWithContext(r.Context(), r.Method, srv.URL+r.URL.Path, nil)
			if err != nil {
				return nil, err
			}
			return base.RoundTrip(req)
		}
		return base.RoundTrip(r)
	})
	if ran, err := e.App.RunAutomatic(context.Background(), "tools", nil); !ran || err != nil {
		t.Fatal(ran, err)
	}
	if s := e.App.ToolUpdate("steamcmd"); !s.Pending || s.Installed != "100" || s.Latest != "101" || !s.Compatible {
		t.Fatal(s)
	}
	if s := e.App.ToolUpdate("todds"); !s.Pending || s.Compatible || s.Latest != "0.5.0" {
		t.Fatal(s)
	}
	if s := e.App.AutomaticStatus("tools"); s.Pending != 2 || calls != 2 {
		t.Fatal(s, calls)
	}
	failed = true
	e.Advance(7 * 24 * time.Hour)
	if _, err := e.App.RunAutomatic(context.Background(), "tools", nil); err == nil {
		t.Fatal("offline success")
	}
	if s := e.App.ToolUpdate("steamcmd"); !s.Pending || s.Latest != "101" {
		t.Fatal("discarded pending update", s)
	}
	if s := e.App.ToolUpdate("todds"); !s.Pending || s.Compatible {
		t.Fatal("offline check discarded compatibility restriction", s)
	}
	b, _ := os.ReadFile(filepath.Join(steam, "steamcmd.exe"))
	if string(b) != "fixture executable" {
		t.Fatal("check changed tool")
	}
}

func TestAutomaticGitAndAuthenticatedPageChecksNeverInstall(t *testing.T) {
	// The repository fixture disables PATH; it must not run in parallel.
	e := setup(t, false)
	g := testkit.NewGitRepo(t, "")
	op, err := e.App.SubscribeGit(g.Bare, "branch", "main", false)
	if err != nil {
		t.Fatal(err)
	}
	gitFolder := op.Items[0].Folder
	latest := g.Commit(t, "Defs/new.xml", "<Defs/>")
	page := accounts.Site + "files/file/123-fixture/"
	folder := filepath.Join(e.App.SubscribedDir(), "page-fixture")
	e.WriteMod(folder, testkit.Sample("tester.page", "Page fixture"))
	if err := subscribe.Write(folder, &subscribe.Record{Kind: "page", URL: page, Installed: "1.0", InstalledAt: e.Now()}); err != nil {
		t.Fatal(err)
	}
	e.MustRun("scan")
	session, err := accounts.Open(e.App.AccountsDir())
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(accounts.Site)
	session.Jar().SetCookies(u, []*http.Cookie{{Name: "ips4_login_key", Value: "synthetic-login", Secure: true, Path: "/", Expires: time.Now().Add(48 * time.Hour)}})
	if err := session.SetStatus("available", ""); err != nil {
		t.Fatal(err)
	}
	if err := e.App.Store.SetMeta("account:loverslab:enabled", "true"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	e.App.HTTP.Transport = automaticTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != page || r.Method != "GET" || !strings.Contains(r.Header.Get("Cookie"), "synthetic-login") {
			t.Error("expected authenticated metadata GET only", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Request: r, Body: io.NopCloser(strings.NewReader(`<script type="application/ld+json">{"@type":"SoftwareApplication","name":"Page fixture","softwareVersion":"2.0"}</script>`))}, nil
	})
	if ran, err := e.App.RunAutomatic(context.Background(), "mods", nil); !ran || err != nil {
		t.Fatal(ran, err)
	}
	if s := e.App.AutomaticStatus("mods"); s.Pending != 2 || calls != 1 {
		t.Fatal(s, calls)
	}
	if gitHead(t, gitFolder) != g.First {
		t.Fatal("check installed the Git update", latest)
	}
	r, err := subscribe.Read(folder)
	if err != nil || r.Installed != "1.0" || r.Latest != "2.0" {
		t.Fatal("page check changed installed version or missed update", r, err)
	}
}

func TestAutomaticWezTermVersionWithoutExecutingTool(t *testing.T) {
	// Force the real finder down its managed-copy path without host discovery.
	e := testkit.New(t)
	t.Setenv("PATH", "")
	for _, key := range []string{"ProgramW6432", "ProgramFiles", "LOCALAPPDATA"} {
		t.Setenv(key, e.Root)
	}
	path := filepath.Join(e.Home, "wezterm", "WezTerm-windows-20240101-000000-fixture", "wezterm-gui.exe")
	e.WriteFile(path, "fixture, never execute")
	e.App.SteamCMDNoPath = true
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/repos/wezterm/wezterm/releases/latest" {
			t.Error("unexpected tool lookup", r.URL)
		}
		io.WriteString(w, `{"tag_name":"20240203-110809-fixture"}`)
	}))
	defer srv.Close()
	e.App.WezTermAPI = srv.URL
	if ran, err := e.App.RunAutomatic(context.Background(), "tools", nil); !ran || err != nil {
		t.Fatal(ran, err)
	}
	if s := e.App.ToolUpdate("wezterm"); !s.Pending || !s.Compatible || s.Installed != "20240101-000000-fixture" || calls != 1 {
		t.Fatal(s, calls)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "fixture, never execute" {
		t.Fatal("check changed tool")
	}
}
