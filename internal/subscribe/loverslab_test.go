package subscribe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/accounts"
)

type siteTransport struct {
	base      *url.URL
	transport http.RoundTripper
}

func (t siteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	u := *r.URL
	u.Host = t.base.Host
	u.Scheme = t.base.Scheme
	copy.URL = &u
	resp, e := t.transport.RoundTrip(copy)
	if resp != nil {
		resp.Request = r
	}
	return resp, e
}
func TestLoversLabFileAgreementDownloadAndFallback(t *testing.T) {
	mode := "files"
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Cookie") != "fixture=offline-session" {
			t.Error("missing session")
		}
		switch mode {
		case "403":
			w.WriteHeader(403)
			io.WriteString(w, "denied")
			return
		case "login":
			http.Redirect(w, r, "/login/", 302)
			return
		case "foreign":
			http.Redirect(w, r, "https://unobserved-cdn.test/file.zip?token=offline-secret", 302)
			return
		case "challenge":
			w.WriteHeader(403)
			io.WriteString(w, `<title>Just a moment</title>cf-chl-fixture`)
			return
		case "limit":
			w.Header().Set("Retry-After", "20")
			w.WriteHeader(429)
			return
		}
		if r.URL.Query().Get("confirm") == "1" {
			if r.Header.Get("Origin") != "https://www.loverslab.com" || !strings.Contains(r.Referer(), "do=download") {
				t.Error("missing browser form context")
			}
			if r.Method != "POST" {
				t.Error("agreement method")
			}
			r.ParseForm()
			if r.Form.Get("csrfKey") != "offline-csrf" || r.Form.Get("agree") != "yes" {
				t.Error("agreement values")
			}
			w.Header().Set("Content-Type", "application/zip")
			io.WriteString(w, "PK\x03\x04fixture-archive")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Query().Get("do") == "download" {
			io.WriteString(w, `<form method="post" action="?do=download&confirm=1"><p>I agree to the author rules.</p><input type="hidden" name="csrfKey" value="offline-csrf"><input type="checkbox" name="agree" value="yes"><button type="submit">I agree</button></form>`)
			return
		}
		if r.URL.Query().Get("controller") == "settings" {
			io.WriteString(w, `<a id="elUserLink">Fixture Player</a><a href="?do=logout">Sign out</a>`)
			return
		}
		io.WriteString(w, `<a href="?do=download&r=1">one.zip</a><a href="?do=download&r=2">two.zip</a><a href="https://evil.test/?do=download">evil</a>`)
	}))
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	session, err := accounts.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(accounts.Site)
	session.Jar().SetCookies(u, []*http.Cookie{{Name: "fixture", Value: "offline-session", Domain: "loverslab.com", Path: "/", Secure: true, Expires: time.Now().Add(time.Hour)}})
	l := NewLoversLab(session, siteTransport{base, srv.Client().Transport})
	ctx := context.Background()
	page := accounts.Site + "files/file/123-fixture/"
	p, err := l.Files(ctx, page)
	if err != nil || len(p.Files) != 2 {
		t.Fatalf("files: %+v %v", p, err)
	}
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), "offline-session") || strings.Contains(string(b), "do=download") {
		t.Fatal("secret/link exposed")
	}
	l.next = time.Time{}
	p, path, err := l.Follow(ctx, page, p.Files[1].ID, false, t.TempDir())
	if err != nil || path != "" || !strings.Contains(p.Agreement, "I agree") {
		t.Fatalf("agreement: %+v %v", p, err)
	}
	before := calls
	p, _, err = l.Follow(ctx, page, p.Files[0].ID, false, t.TempDir())
	if err != nil || calls != before {
		t.Fatal("agreement bypass")
	}
	b, _ = json.Marshal(p)
	if strings.Contains(string(b), "offline-csrf") {
		t.Fatal("CSRF exposed")
	}
	l.next = time.Time{}
	_, path, err = l.Follow(ctx, page, p.Files[0].ID, true, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "PK") {
		t.Fatal("not downloaded")
	}
	l.next = time.Time{}
	before = calls
	if err = l.Verify(ctx); err != nil || calls != before+1 || session.Status().Name != "Fixture Player" {
		t.Fatal("single GET verification", err, calls-before)
	}
	for _, value := range []string{"403", "login", "challenge", "foreign", "limit"} {
		mode = value
		l.next = time.Time{}
		_, err = l.Files(ctx, page)
		if err == nil || !strings.Contains(err.Error(), "浏览器下载后选择 zip") {
			t.Fatal(value, err)
		}
		if strings.Contains(err.Error(), "offline-") {
			t.Fatal("unsafe error")
		}
		if value == "limit" {
			before := calls
			if _, err = l.Files(ctx, page); err == nil || calls != before {
				t.Fatal("Retry-After ignored")
			}
		}
	}
	mode = "files"
	l.next = time.Time{}
	before = calls
	for _, raw := range []string{"https://evil.test/files/file/1/", "https://loverslab.com.evil.test/files/file/1/", "https://.loverslab.com/files/file/1/", "https://www.loverslab.com/login/", "http://www.loverslab.com/files/file/1/"} {
		if _, err = l.Files(ctx, raw); err == nil {
			t.Fatal("domain accepted")
		}
	}
	if calls != before {
		t.Fatal("unsafe network call")
	}
	empty, _ := accounts.Open(t.TempDir())
	noSession := NewLoversLab(empty, siteTransport{base, srv.Client().Transport})
	if _, err = noSession.Files(ctx, page); err == nil || !strings.Contains(err.Error(), "需玩家登录") {
		t.Fatal("missing session fallback")
	}
}
