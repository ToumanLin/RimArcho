package netcfg

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// client returns a client whose github.com dials reach origin; everything else dials as asked.
// Nothing leaves the machine.
func client(t *testing.T, origin string) *http.Client {
	base := http.DefaultTransport.(*http.Transport).Clone()
	d := &net.Dialer{Timeout: 2 * time.Second}
	base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if strings.HasPrefix(addr, "github.com:") {
			addr = origin
		} else if h, _, _ := net.SplitHostPort(addr); !Loopback(h) {
			t.Fatalf("dial outside the machine: %s", addr)
		}
		return d.DialContext(ctx, network, addr)
	}
	return &http.Client{Transport: New(base), Timeout: 5 * time.Second}
}

func serve(t *testing.T, status int, body string, hits *[]string) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits = append(*hits, r.Method+" "+r.URL.RequestURI())
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(s.Close)
	return s
}

func get(t *testing.T, c *http.Client, method, u string) string {
	req, _ := http.NewRequest(method, u, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestMirrorFirstThenDirect(t *testing.T) {
	t.Cleanup(func() { Configure(Settings{}) })
	var originHits, mirrorHits []string
	origin := serve(t, 200, "origin", &originHits)
	c := client(t, strings.TrimPrefix(origin.URL, "http://"))
	target := "http://github.com/todds-encoder/todds/releases/download/0.4.1/todds.zip"

	// No mirror: direct.
	Configure(Settings{Proxy: "off"})
	if got := get(t, c, "GET", target); got != "origin" {
		t.Fatalf("direct: %q", got)
	}

	// A working mirror answers; GitHub is not asked.
	mirror := serve(t, 200, "mirror", &mirrorHits)
	Configure(Settings{Proxy: "off", Mirror: mirror.URL})
	originHits = nil
	if got := get(t, c, "GET", target); got != "mirror" || len(originHits) != 0 {
		t.Fatalf("mirror: %q, origin hits %v", got, originHits)
	}
	if want := "GET /" + target; mirrorHits[0] != want {
		t.Fatalf("mirror asked %q, want %q", mirrorHits[0], want)
	}
	// A template with {url}.
	Configure(Settings{Proxy: "off", Mirror: mirror.URL + "/fetch?u={url}"})
	mirrorHits = nil
	if got := get(t, c, "GET", target); got != "mirror" || !strings.HasPrefix(mirrorHits[0], "GET /fetch?u=http://github.com/") {
		t.Fatalf("template: %q %v", got, mirrorHits)
	}

	// A failing mirror falls back to GitHub.
	var badHits []string
	bad := serve(t, 502, "bad gateway", &badHits)
	Configure(Settings{Proxy: "off", Mirror: bad.URL})
	if got := get(t, c, "GET", target); got != "origin" || len(badHits) != 1 {
		t.Fatalf("fallback: %q, mirror hits %v", got, badHits)
	}
	// So does one that is not there.
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	Configure(Settings{Proxy: "off", Mirror: dead.URL})
	if got := get(t, c, "GET", target); got != "origin" {
		t.Fatalf("dead mirror: %q", got)
	}

	// Only GitHub's GET and HEAD go through the mirror.
	Configure(Settings{Proxy: "off", Mirror: mirror.URL})
	mirrorHits, originHits = nil, nil
	get(t, c, "POST", target)
	get(t, c, "GET", origin.URL+"/other")
	if len(mirrorHits) != 0 || len(originHits) != 2 {
		t.Fatalf("mirrored a POST or another host: mirror %v origin %v", mirrorHits, originHits)
	}
}

func TestBlockedErrorNamesTheSettings(t *testing.T) {
	t.Cleanup(func() { Configure(Settings{}) })
	Configure(Settings{Proxy: "off"})
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return nil, &net.OpError{Op: "dial", Net: network, Err: os.ErrDeadlineExceeded}
	}
	c := &http.Client{Transport: New(base)}
	_, err := c.Get("http://raw.githubusercontent.com/x")
	if err == nil || !strings.Contains(err.Error(), "network.proxy") || !strings.Contains(err.Error(), "network.github_mirror") {
		t.Fatalf("GitHub error: %v", err)
	}
	_, err = c.Get("http://api.steampowered.com/x")
	if err == nil || !strings.Contains(err.Error(), "network.proxy") || strings.Contains(err.Error(), "github_mirror") {
		t.Fatalf("Steam error: %v", err)
	}
	if !os.IsTimeout(err) {
		t.Fatalf("the cause is kept: %v", err)
	}
	// A request through a proxy names the proxy instead.
	Configure(Settings{Proxy: "http://127.0.0.1:9"})
	_, err = c.Get("http://api.steampowered.com/x")
	if err == nil || !strings.Contains(err.Error(), "through the proxy 127.0.0.1:9") {
		t.Fatalf("proxy error: %v", err)
	}
}

func TestProxyChoice(t *testing.T) {
	t.Cleanup(func() { Configure(Settings{}); readSystem = readSystemProxy; sysCache.at = time.Time{} })
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if os.Getenv(k) != "" {
			t.Skip("the environment sets a proxy")
		}
	}
	at := func(u string) string {
		r, _ := http.NewRequest("GET", u, nil)
		p, err := ProxyFor(r)
		if err != nil {
			return "error"
		}
		if p == nil {
			return "direct"
		}
		return p.String()
	}
	sys := systemProxySettings{Enabled: true, Server: "127.0.0.1:7890", Override: "localhost;127.*;10.*;*.lan;<local>"}
	readSystem = func() systemProxySettings { return sys }
	sysCache.at = time.Time{}

	Configure(Settings{})
	for u, want := range map[string]string{
		"https://github.com/x":    "http://127.0.0.1:7890",
		"https://nas.lan/x":       "direct",
		"http://intranet/x":       "direct",
		"http://10.0.0.5/x":       "direct",
		"http://127.0.0.1:8080/x": "direct",
		"http://localhost:1/x":    "direct",
	} {
		if got := at(u); got != want {
			t.Errorf("system %s: %s, want %s", u, got, want)
		}
	}
	Configure(Settings{Proxy: "off"})
	if got := at("https://github.com/x"); got != "direct" {
		t.Errorf("off: %s", got)
	}
	Configure(Settings{Proxy: "socks5://127.0.0.1:7891"})
	if got := at("https://github.com/x"); got != "socks5://127.0.0.1:7891" {
		t.Errorf("explicit: %s", got)
	}
	if got := at("http://127.0.0.1:9/x"); got != "direct" {
		t.Errorf("explicit, loopback: %s", got)
	}

	// Per-scheme system settings, and a switched-off system proxy.
	Configure(Settings{Proxy: "system"})
	for _, c := range []struct{ server, u, want string }{
		{"http=127.0.0.1:1;https=127.0.0.1:2", "https://github.com/", "http://127.0.0.1:2"},
		{"http=127.0.0.1:1;https=127.0.0.1:2", "http://github.com/", "http://127.0.0.1:1"},
		{"socks=127.0.0.1:3", "https://github.com/", "socks5://127.0.0.1:3"},
		{"not a proxy", "https://github.com/", "direct"},
	} {
		s := systemProxySettings{Enabled: true, Server: c.server}
		if p, _ := pickSystem(s, mustURL(c.u)); (p == nil && c.want != "direct") || (p != nil && p.String() != c.want) {
			t.Errorf("%s for %s: %v, want %s", c.server, c.u, p, c.want)
		}
	}
	if p, _ := pickSystem(systemProxySettings{Server: "127.0.0.1:7890"}, mustURL("https://github.com/")); p != nil {
		t.Errorf("disabled system proxy used: %v", p)
	}
}

func TestChecks(t *testing.T) {
	for v, ok := range map[string]bool{"system": true, "off": true, "http://127.0.0.1:7890": true, "127.0.0.1:7890": true,
		"socks5://127.0.0.1:7891": true, "ftp://x:1": false, "http://nohost": false, "on": false} {
		if (CheckProxy(v) == nil) != ok {
			t.Errorf("CheckProxy(%q) ok=%v", v, !ok)
		}
	}
	for v, ok := range map[string]bool{"": true, "https://gh-proxy.org/": true, "https://m.example/{url}": true, "gh-proxy.org": false, "file:///x": false} {
		if (CheckMirror(v) == nil) != ok {
			t.Errorf("CheckMirror(%q) ok=%v", v, !ok)
		}
	}
	if got := Mirrored("https://gh-proxy.org", "https://github.com/a"); got != "https://gh-proxy.org/https://github.com/a" {
		t.Errorf("prefix: %s", got)
	}
}

func mustURL(s string) *url.URL { u, _ := url.Parse(s); return u }
