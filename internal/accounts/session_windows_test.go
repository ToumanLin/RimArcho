//go:build windows

package accounts

import (
	"bytes"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDPAPIRoundTripAndSessionRestriction(t *testing.T) {
	plain := []byte("offline-fixture-session-value")
	encrypted, err := protect(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, plain) {
		t.Fatal("plaintext stored")
	}
	got, err := unprotect(encrypted)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatal("round trip", err)
	}
	if _, err = unprotect([]byte("corrupt")); err == nil {
		t.Fatal("corrupt DPAPI accepted")
	}
	if _, err = unprotect(nil); err == nil {
		t.Fatal("empty DPAPI accepted")
	}
	dir := filepath.Join(t.TempDir(), "accounts")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err = s.capture([]*http.Cookie{{Name: "fixture", Value: string(plain), Domain: ".loverslab.com", Path: "/files", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, Expires: expiry}, {Name: "foreign", Value: "not-retained", Domain: "example.com", Secure: true, Expires: expiry}, {Name: "transient", Value: "session-only", Domain: "loverslab.com", Secure: true}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "loverslab.session"))
	if err != nil || bytes.Contains(data, plain) || bytes.Contains(data, []byte("session-only")) {
		t.Fatal("secret at rest", err)
	}
	restored, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.cookies) != 1 || !restored.cookies[0].HttpOnly || restored.cookies[0].SameSite != http.SameSiteStrictMode || !restored.cookies[0].Expires.Equal(expiry) {
		t.Fatal("cookie attributes not preserved")
	}
	jar := restored.Jar()
	for _, raw := range []string{"http://www.loverslab.com/files/", "https://evil.loverslab.com/files/", "https://loverslab.com.evil.test/files/", "https://example.com/files/", "https://www.loverslab.com/other"} {
		u, _ := url.Parse(raw)
		if len(jar.Cookies(u)) != 0 {
			t.Fatal("cookie leaked", raw)
		}
	}
	u, _ := url.Parse("https://www.loverslab.com/files/file/1/")
	if len(jar.Cookies(u)) != 1 {
		t.Fatal("service cookie absent")
	}
	jar.SetCookies(u, []*http.Cookie{{Name: "poison", Value: "fixture", Domain: "com", Secure: true}})
	if len(restored.cookies) != 1 {
		t.Fatal("domain poison retained")
	}
	if err = restored.Forget(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("profile/session not forgotten")
	}
}

func TestSessionExpiryDeletionAndForgetInvalidatesClients(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "accounts")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(Site)
	jar := s.Jar()
	jar.SetCookies(u, []*http.Cookie{{Name: "fixture", Value: "offline", Path: "/", Secure: true, Expires: time.Now().Add(time.Hour)}})
	if err = s.SetStatus("可用", "Fixture"); err != nil {
		t.Fatal(err)
	}
	other, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	apex, _ := url.Parse("https://loverslab.com/")
	if len(other.Jar().Cookies(apex)) != 0 {
		t.Fatal("host-only cookie widened")
	}
	if len(other.Jar().Cookies(u)) != 1 {
		t.Fatal("host-only cookie lost")
	}
	jar.SetCookies(u, []*http.Cookie{{Name: "fixture", Path: "/", Secure: true, MaxAge: -1}})
	if s.HasCookies() {
		t.Fatal("deleted cookie is usable")
	}
	jar.SetCookies(u, []*http.Cookie{{Name: "expired", Path: "/", Secure: true, Expires: time.Now().Add(-time.Hour)}})
	if s.HasCookies() {
		t.Fatal("expired cookie is usable")
	}
	if err = other.Forget(); err != nil {
		t.Fatal(err)
	}
	if s.HasCookies() || len(jar.Cookies(u)) != 0 {
		t.Fatal("forgotten session remains usable")
	}
	if err = s.SetStatus("可用", ""); err == nil {
		t.Fatal("forgotten session recreated")
	}
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("forget resurrected files")
	}
}
