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
	if len(restored.cookies) != 2 || !restored.cookies[0].HttpOnly || restored.cookies[0].SameSite != http.SameSiteStrictMode || !restored.cookies[0].Expires.Equal(expiry) {
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
	if len(restored.cookies) != 2 {
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
	if err = s.SetStatus("available", "Fixture"); err != nil {
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
	if err = s.SetStatus("available", ""); err == nil {
		t.Fatal("forgotten session recreated")
	}
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("forget resurrected files")
	}
}

func TestRememberMeExpiryAndCookieRotationPersist(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(Site)
	jar := s.Jar()
	short := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	long := short.Add(180 * 24 * time.Hour)
	jar.SetCookies(u, []*http.Cookie{
		{Name: "ips4_IPSSessionFront", Value: "short-offline", Path: "/", Secure: true, Expires: short},
		{Name: "ips4_member_id", Value: "123", Path: "/", Expires: long},
		{Name: "ips4_login_key", Value: "old-offline-key", Path: "/", Secure: true, HttpOnly: true, Expires: long},
		{Name: "analytics", Value: "not-auth", Path: "/", Secure: true, Expires: long.AddDate(5, 0, 0)},
	})
	if err = s.SetStatus("available", ""); err != nil {
		t.Fatal(err)
	}
	if !s.Status().Expiry.Equal(long) {
		t.Fatal("wrong auth expiry", s.Status())
	}
	next := long.AddDate(0, 1, 0)
	jar.SetCookies(u, []*http.Cookie{{Name: "ips4_login_key", Value: "rotated-offline-key", Path: "/", Secure: true, HttpOnly: true, Expires: next}, {Name: "ips4_IPSSessionFront", Path: "/", Secure: true, MaxAge: -1}})
	if err = s.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Status().Expiry.Equal(next) {
		t.Fatal(reopened.Status())
	}
	got := reopened.Jar().Cookies(u)
	found := false
	for _, c := range got {
		if c.Name == "ips4_login_key" && c.Value == "rotated-offline-key" {
			found = true
		}
		if c.Name == "ips4_IPSSessionFront" {
			t.Fatal("deleted cookie retained")
		}
	}
	if !found {
		t.Fatal("rotating login key not persisted")
	}
	if err = reopened.capture([]*http.Cookie{{Name: "ips4_IPSSessionFront", Value: "session-offline", Domain: "www.loverslab.com", Path: "/", Secure: true}}); err != nil {
		t.Fatal(err)
	}
	reopened, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.HasCookies() || !reopened.Status().Expiry.IsZero() {
		t.Fatal("session-only cookies need N/A", reopened.Status())
	}
}
