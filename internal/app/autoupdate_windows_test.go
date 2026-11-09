//go:build windows

package app_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

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
