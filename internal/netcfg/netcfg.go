// Package netcfg routes RimArcho's own HTTP requests (17 §2.2): through a proxy (the
// environment's or the Windows system proxy by default) and, for GitHub, through a mirror
// first. Install wraps http.DefaultTransport once per process, so every client without a
// transport of its own follows the settings; Configure changes them at any time.
package netcfg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Settings are the two network settings (config network.proxy, network.github_mirror).
type Settings struct {
	// Proxy: "system" (default: HTTPS_PROXY / HTTP_PROXY, then the Windows system proxy),
	// "off" (direct), or a proxy URL such as http://127.0.0.1:7890 or socks5://127.0.0.1:7891.
	Proxy string
	// Mirror: "" (direct) or a GitHub mirror, either a prefix the full URL is appended to
	// (https://gh-proxy.org/) or a template with {url}.
	Mirror string
}

var cur atomic.Pointer[Settings]

// Configure sets the settings for every request that follows.
func Configure(s Settings) { cur.Store(&s) }

func current() Settings {
	if s := cur.Load(); s != nil {
		return *s
	}
	return Settings{}
}

var once sync.Once

// Install wraps http.DefaultTransport. Only the programs call it; tests use their own transports.
func Install() {
	once.Do(func() {
		if t, ok := http.DefaultTransport.(*http.Transport); ok {
			http.DefaultTransport = New(t)
		}
	})
}

// New wraps base: its proxy follows the settings, and GitHub requests try the mirror first.
func New(base *http.Transport) http.RoundTripper {
	b := base.Clone()
	b.Proxy = ProxyFor
	m := b.Clone()
	// A mirror that does not answer must not hold up the direct request for long.
	m.ResponseHeaderTimeout = 15 * time.Second
	return &transport{base: b, mirror: m}
}

type transport struct{ base, mirror *http.Transport }

func (t *transport) RoundTrip(r *http.Request) (*http.Response, error) {
	s := current()
	if s.Mirror != "" && (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
		(r.Body == nil || r.Body == http.NoBody) && GitHubHost(r.URL.Hostname()) {
		if u, err := url.Parse(Mirrored(s.Mirror, r.URL.String())); err == nil && u.Host != "" {
			mr := r.Clone(r.Context())
			mr.URL, mr.Host = u, ""
			mr.Header.Del("Authorization")
			mr.Header.Del("Cookie")
			resp, err := t.mirror.RoundTrip(mr)
			if err == nil && resp.StatusCode < 400 {
				return resp, nil
			}
			if resp != nil {
				io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
				resp.Body.Close()
			}
			if r.Context().Err() != nil {
				return nil, r.Context().Err()
			}
		}
	}
	resp, err := t.base.RoundTrip(r)
	if err != nil && !errors.Is(err, context.Canceled) && !Loopback(r.URL.Hostname()) {
		be := &blockedError{err: err, github: GitHubHost(r.URL.Hostname())}
		if p, _ := ProxyFor(r); p != nil {
			be.proxy = p.Host
		}
		err = be
	}
	return resp, err
}

// blockedError adds what the player can set when a site cannot be reached.
type blockedError struct {
	err    error
	github bool
	proxy  string // the proxy the request went through, if any
}

func (e *blockedError) Error() string {
	hint := "if this site is blocked on your network, set a proxy in Settings › network (archo config set network.proxy http://127.0.0.1:<port>)"
	if e.proxy != "" {
		hint = "through the proxy " + e.proxy + "; check that it runs, or change it in Settings › network (network.proxy)"
	}
	if e.github {
		hint += " or a GitHub mirror (network.github_mirror https://gh-proxy.org/)"
	}
	return fmt.Sprintf("%v (%s)", e.err, hint)
}
func (e *blockedError) Unwrap() error { return e.err }

// Timeout keeps url.Error's Timeout answer.
func (e *blockedError) Timeout() bool {
	var t interface{ Timeout() bool }
	return errors.As(e.err, &t) && t.Timeout()
}

// githubHosts are the GitHub hosts mirrors serve: pages and releases, raw files, release
// downloads, archives and the API (a mirror that refuses it falls back to direct).
var githubHosts = map[string]bool{
	"github.com": true, "api.github.com": true, "codeload.github.com": true,
	"raw.githubusercontent.com": true, "objects.githubusercontent.com": true,
	"release-assets.githubusercontent.com": true, "gist.githubusercontent.com": true,
}

// GitHubHost reports whether a mirror is tried for host.
func GitHubHost(host string) bool { return githubHosts[strings.ToLower(host)] }

// Mirrored is target through mirror m: {url} replaced, or the target appended to the prefix.
func Mirrored(m, target string) string {
	if strings.Contains(m, "{url}") {
		return strings.Replace(m, "{url}", target, 1)
	}
	if !strings.HasSuffix(m, "/") {
		m += "/"
	}
	return m + target
}

// CheckMirror checks a network.github_mirror value.
func CheckMirror(v string) error {
	if v == "" {
		return nil
	}
	u, err := url.Parse(strings.Replace(v, "{url}", "x", 1))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("network.github_mirror must be empty (direct) or an http(s) prefix such as https://gh-proxy.org/ or a template with {url}")
	}
	return nil
}

// CheckProxy checks a network.proxy value.
func CheckProxy(v string) error {
	if v == "system" || v == "off" {
		return nil
	}
	if _, err := proxyURL(v); err != nil {
		return fmt.Errorf("network.proxy must be system, off or a proxy URL such as http://127.0.0.1:7890 or socks5://127.0.0.1:7891")
	}
	return nil
}

func proxyURL(v string) (*url.URL, error) {
	if !strings.Contains(v, "://") {
		v = "http://" + v
	}
	u, err := url.Parse(v)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q", u.Scheme)
	}
	if u.Hostname() == "" || u.Port() == "" {
		return nil, fmt.Errorf("proxy needs a host and port")
	}
	return u, nil
}

// Loopback reports whether host is this machine; it never goes through a proxy.
func Loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ProxyFor is the transport's proxy function.
func ProxyFor(r *http.Request) (*url.URL, error) {
	if Loopback(r.URL.Hostname()) {
		return nil, nil
	}
	switch p := current().Proxy; p {
	case "off":
		return nil, nil
	case "", "system":
		if u, err := http.ProxyFromEnvironment(r); u != nil || err != nil {
			return u, err
		}
		return systemProxy(r.URL)
	default:
		return proxyURL(p)
	}
}

// Describe says which proxy a request to target would use, for the settings page.
func Describe(target string) string {
	u, _ := url.Parse(target)
	if u == nil {
		return ""
	}
	p, err := ProxyFor(&http.Request{URL: u, Header: http.Header{}})
	switch {
	case err != nil:
		return err.Error()
	case p == nil:
		return "direct"
	}
	return p.Host
}

// systemProxySettings is the Windows per-user proxy (Internet Settings): what Clash, v2rayN
// and similar tools set in system-proxy mode. PAC scripts are not read.
type systemProxySettings struct {
	Enabled  bool
	Server   string // "host:port" or "http=host:port;https=host:port;socks=host:port"
	Override string // "localhost;127.*;<local>"
}

var sysCache struct {
	sync.Mutex
	at time.Time
	s  systemProxySettings
}

// readSystem is replaced by tests.
var readSystem = readSystemProxy

func systemProxy(u *url.URL) (*url.URL, error) {
	sysCache.Lock()
	if time.Since(sysCache.at) > 5*time.Second {
		sysCache.s, sysCache.at = readSystem(), time.Now()
	}
	s := sysCache.s
	sysCache.Unlock()
	return pickSystem(s, u)
}

func pickSystem(s systemProxySettings, u *url.URL) (*url.URL, error) {
	if !s.Enabled || strings.TrimSpace(s.Server) == "" || bypassed(s.Override, u.Hostname()) {
		return nil, nil
	}
	server := strings.TrimSpace(s.Server)
	if strings.Contains(server, "=") {
		per := map[string]string{}
		for _, part := range strings.Split(server, ";") {
			if k, v, ok := strings.Cut(strings.TrimSpace(part), "="); ok && v != "" {
				per[strings.ToLower(k)] = strings.TrimSpace(v)
			}
		}
		switch {
		case per[u.Scheme] != "":
			server = per[u.Scheme]
		case per["http"] != "":
			server = per["http"]
		case per["socks"] != "":
			server = "socks5://" + strings.TrimPrefix(per["socks"], "socks://")
		default:
			return nil, nil
		}
	}
	p, err := proxyURL(server)
	if err != nil {
		return nil, nil // an unusable system setting: go direct, as before
	}
	return p, nil
}

// bypassed matches host against the ProxyOverride list: ';'-separated, '*' wildcards,
// <local> for names without a dot.
func bypassed(list, host string) bool {
	host = strings.ToLower(host)
	for _, pat := range strings.Split(strings.ToLower(list), ";") {
		pat = strings.TrimSpace(pat)
		switch {
		case pat == "":
		case pat == "<local>":
			if !strings.Contains(host, ".") {
				return true
			}
		case wildcard(pat, host):
			return true
		}
	}
	return false
}

func wildcard(pat, s string) bool {
	if !strings.Contains(pat, "*") {
		return pat == s
	}
	parts := strings.Split(pat, "*")
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for i, p := range parts[1:] {
		if i == len(parts)-2 {
			return strings.HasSuffix(s, p)
		}
		j := strings.Index(s, p)
		if j < 0 {
			return false
		}
		s = s[j+len(p):]
	}
	return true
}
