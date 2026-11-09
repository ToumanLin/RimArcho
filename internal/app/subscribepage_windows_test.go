//go:build windows

package app_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

type accountSiteTransport struct {
	base      *url.URL
	transport http.RoundTripper
}

func (tr accountSiteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c := r.Clone(r.Context())
	u := *r.URL
	u.Host, u.Scheme = tr.base.Host, tr.base.Scheme
	c.URL = &u
	resp, err := tr.transport.RoundTrip(c)
	if resp != nil {
		resp.Request = r
	}
	return resp, err
}

func TestLoversLabArchiveInstallUpdateWithBlockedPublicPage(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	archive := pageZip(t, "Fixture/", "<Defs><ThingDef><defName>First</defName></ThingDef></Defs>", false)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") == "" {
			w.WriteHeader(403)
			return
		}
		if r.Header.Get("Cookie") != "fixture=offline-session" {
			t.Error("unexpected cookie")
		}
		if r.URL.Query().Get("do") == "download" {
			w.Header().Set("Content-Type", "application/zip")
			b, _ := os.ReadFile(archive)
			_, _ = w.Write(b)
			return
		}
		fmt.Fprint(w, `<a href="?do=download">fixture.zip</a>`)
	}))
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	tr := accountSiteTransport{base, srv.Client().Transport}
	e.App.HTTP = &http.Client{Transport: tr}
	session, err := accounts.Open(e.App.AccountsDir())
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(accounts.Site)
	session.Jar().SetCookies(u, []*http.Cookie{{Name: "fixture", Value: "offline-session", Domain: "loverslab.com", Path: "/", Secure: true, Expires: time.Now().Add(time.Hour)}})
	ll := subscribe.NewLoversLab(session, tr)
	page := accounts.Site + "files/file/123-fixture/"
	get := func() string {
		t.Helper()
		p, err := ll.Files(context.Background(), page)
		if err != nil {
			t.Fatal(err)
		}
		_, path, err := ll.Follow(context.Background(), page, p.Files[0].ID, false, t.TempDir())
		if err != nil || path == "" {
			t.Fatal("download", err)
		}
		return path
	}
	op, err := e.App.InstallLoversLabFile(page, get(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	it := op.Items[0]
	archive = pageZip(t, "Fixture/", "<Defs><ThingDef><defName>Updated</defName></ThingDef></Defs>", false)
	if _, err = e.App.InstallLoversLabFile(accounts.Site+"files/file/456-other/", archive, it.Ref, true); err == nil {
		t.Fatal("wrong update provenance accepted")
	}
	if _, err = e.App.InstallLoversLabFile(page, get(), it.Ref, false); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(it.Folder, "Defs", "Test.xml"))
	if err != nil || !strings.Contains(string(b), "Updated") {
		t.Fatal("update not installed", err)
	}
	record, err := subscribe.Read(it.Folder)
	if err != nil || record.URL != page || strings.Contains(fmt.Sprint(record), "offline-session") {
		t.Fatal("unsafe subscription metadata", err)
	}
	// The same archive path remains available after session loss, with public metadata blocked.
	if err = accounts.Forget(e.App.AccountsDir()); err != nil {
		t.Fatal(err)
	}
	if _, err = ll.Files(context.Background(), page); err == nil {
		t.Fatal("forgotten session reused")
	}
	if _, err = e.App.InstallLoversLabFile(page, archive, it.Ref, false); err != nil {
		t.Fatal("manual fallback", err)
	}
}
