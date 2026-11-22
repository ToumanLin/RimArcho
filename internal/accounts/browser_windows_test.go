package accounts

import (
	"net/http"
	"testing"
)

// The login window lets the top frame reach only the site over plain https; frames may
// also reach the Cloudflare challenge.
func TestLoginNavigationFilter(t *testing.T) {
	for _, tc := range []struct {
		raw          string
		page, iframe bool // blocked
	}{
		{"https://www.loverslab.com/login/", false, false},
		{"https://LoversLab.com/files", false, false},
		{"https://challenges.cloudflare.com/turnstile", true, false},
		{"http://www.loverslab.com/", true, true},
		{"https://user@www.loverslab.com/", true, true},
		{"https://www.loverslab.com:443/", true, true},
		{"https://example.com/", true, true},
		{"https://evil.loverslab.com.example/", true, true},
		{"%zz", true, true},
		{"", true, true},
	} {
		if got := blockedNavigation(tc.raw, false); got != tc.page {
			t.Errorf("page %q blocked %v, want %v", tc.raw, got, tc.page)
		}
		if got := blockedNavigation(tc.raw, true); got != tc.iframe {
			t.Errorf("frame %q blocked %v, want %v", tc.raw, got, tc.iframe)
		}
	}
}

// WebView2's COREWEBVIEW2_COOKIE_SAME_SITE_KIND: 0 None, 1 Lax, 2 Strict.
func TestWebViewSameSite(t *testing.T) {
	for code, want := range map[int32]http.SameSite{0: http.SameSiteNoneMode, 1: http.SameSiteLaxMode, 2: http.SameSiteStrictMode, 3: 0, -1: 0} {
		if got := cookieSameSite(code); got != want {
			t.Errorf("code %d: %v, want %v", code, got, want)
		}
	}
	for mode, want := range map[http.SameSite]uintptr{http.SameSiteNoneMode: 0, http.SameSiteLaxMode: 1, http.SameSiteStrictMode: 2, http.SameSiteDefaultMode: 1, 0: 1} {
		if got := webViewSameSite(mode); got != want {
			t.Errorf("mode %v: %d, want %d", mode, got, want)
		}
	}
}
