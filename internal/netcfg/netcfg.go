// Package netcfg routes RimArcho's own HTTP requests (17 §2.2): through a proxy (the
// environment's or the Windows system proxy by default), with fallback to direct, to a
// local proxy and, for GitHub, to mirrors. Install wraps http.DefaultTransport once per
// process, so every client without a transport of its own follows the settings.
package netcfg

import (
	"fmt"
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
	// Proxy: "system" (default: HTTPS_PROXY / HTTP_PROXY, then the Windows system proxy,
	// with fallback to direct and to a local proxy), "off" (direct only), or a proxy URL
	// such as http://127.0.0.1:7890 or socks5://127.0.0.1:7891 (that proxy only).
	Proxy string
	// Mirror: "" or "auto" (default: GitHub first, then the built-in mirrors), "off" (never a
	// mirror), or one mirror tried before GitHub: a prefix the full URL is appended to
	// (https://gh-proxy.org/) or a template with {url}.
	Mirror string
}

// mirrors is the mirror list for these settings and whether the player chose it.
func (s Settings) mirrors() (list []string, explicit bool) {
	switch m := strings.TrimSpace(s.Mirror); m {
	case "", "auto":
		return AutoMirrors, false
	case "off":
		return nil, false
	default:
		return []string{m}, true
	}
}

var cur atomic.Pointer[Settings]

// Configure sets the settings for every request that follows. A change forgets the
// routes remembered under the old settings.
func Configure(s Settings) {
	cur.Store(&s)
	resetMemory()
}

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

// New wraps base: its proxy follows the settings, and a failed request falls back to the
// next route (fallback.go).
func New(base *http.Transport) http.RoundTripper {
	b := base.Clone()
	b.Proxy = routeProxy
	return &transport{base: b}
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

// steamDomains are Steam's own hosts and their subdomains: the Community and Workshop
// pages, the Web API, the store and the CDNs for previews and downloads. Game
// accelerators (UU, Xiaoheihe, Watt Toolkit) speed these up below HTTP, so they
// go direct first (17 §2.2).
var steamDomains = []string{"steamcommunity.com", "steampowered.com", "steamstatic.com",
	"steamcontent.com", "steamusercontent.com", "steamserver.net", "steamgames.com"}

// steamAkamai are Steam's CDN names on akamaihd.net (not the whole domain).
var steamAkamai = map[string]bool{"steamcdn-a.akamaihd.net": true, "steamuserimages-a.akamaihd.net": true,
	"steamcommunity-a.akamaihd.net": true, "steamstore-a.akamaihd.net": true}

// SteamHost reports whether host is one of Steam's: direct first, then a proxy.
func SteamHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if steamAkamai[host] {
		return true
	}
	for _, d := range steamDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

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
	if v == "" || v == "auto" || v == "off" {
		return nil
	}
	u, err := url.Parse(strings.Replace(v, "{url}", "x", 1))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("network.github_mirror must be auto, off, an http(s) prefix such as https://gh-proxy.org/ or a template with {url}")
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

// ProxyFor is the proxy a request starts with: the setting's, or for system the
// environment's or the Windows system proxy unless that one was just found not running.
func ProxyFor(r *http.Request) (*url.URL, error) {
	if Loopback(r.URL.Hostname()) {
		return nil, nil
	}
	switch p := current().Proxy; p {
	case "off":
		return nil, nil
	case "", "system":
		u, err := http.ProxyFromEnvironment(r)
		if u == nil && err == nil {
			u, err = systemProxy(r.URL)
		}
		if u != nil && proxyDead(u) {
			return nil, nil
		}
		return u, err
	default:
		return proxyURL(p)
	}
}

// Describe says which proxy a request to target would start with, for the settings page.
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
