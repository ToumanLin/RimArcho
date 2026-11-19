package netcfg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// AutoMirrors are the GitHub mirrors network.github_mirror auto tries, in order, after
// GitHub itself failed (17 §2.2). Public mirrors come and go: the list is checked before
// each release and changed in code, never from downloaded data.
var AutoMirrors = []string{"https://gh-proxy.com/", "https://ghfast.top/", "https://ghproxy.net/"}

// LocalProxyPorts are the loopback ports tried once a request failed without a known
// proxy: Clash's mixed port, Clash Verge Rev's, v2rayN's HTTP and SOCKS ports.
var LocalProxyPorts = []string{"http://127.0.0.1:7890", "http://127.0.0.1:7897", "http://127.0.0.1:10809", "socks5://127.0.0.1:10808"}

// Time limits of a request that has somewhere to fall back to: a route gets AttemptLimit
// to answer with headers, and no route starts after ChainLimit (the last one gets what
// is left, at least MinAttempt). A request with one route keeps the plain client's limits.
var (
	AttemptLimit = 10 * time.Second
	ChainLimit   = 30 * time.Second
	MinAttempt   = 5 * time.Second
	// RememberFor keeps the route that answered for a host first; DeadFor skips a system
	// proxy whose port refused; probeTimeout bounds one loopback port check.
	RememberFor  = 10 * time.Minute
	DeadFor      = time.Minute
	probeTimeout = 300 * time.Millisecond
)

type routeKind int

const (
	direct routeKind = iota
	viaProxy
	viaLocal // a proxy found on a loopback port at attempt time
	viaMirror
)

// route is one way to make a request.
type route struct {
	kind   routeKind
	proxy  *url.URL // viaProxy; for viaMirror the proxy that reaches the mirror (nil: direct)
	mirror string   // viaMirror
	local  bool     // a viaProxy found on a loopback port
}

// key names the route for the per-host memory; a found local proxy stays "local".
func (r route) key() string {
	switch r.kind {
	case viaProxy:
		if r.local {
			return "local"
		}
		return "proxy " + r.proxy.Host
	case viaLocal:
		return "local"
	case viaMirror:
		return "mirror " + MirrorHost(r.mirror)
	}
	return "direct"
}

// String is the route as Settings and logs show it: direct, proxy host:port, mirror host.
func (r route) String() string {
	if r.kind == viaProxy {
		return "proxy " + r.proxy.Host
	}
	return r.key()
}

// MirrorHost is a mirror setting's host, for display.
func MirrorHost(m string) string {
	if u, err := url.Parse(strings.Replace(m, "{url}", "x", 1)); err == nil && u.Host != "" {
		return u.Host
	}
	return m
}

type routeKey struct{}

// routeProxy is the base transport's proxy function: the attempt's route, or ProxyFor.
func routeProxy(r *http.Request) (*url.URL, error) {
	if rt, ok := r.Context().Value(routeKey{}).(*route); ok {
		return rt.proxy, nil
	}
	return ProxyFor(r)
}

type noAutoMirrorKey struct{}

// WithoutAutoMirror marks requests that must not go through an automatic mirror: RimArcho's
// own update metadata and checksums (17 §5.1). A mirror the player set still applies.
func WithoutAutoMirror(ctx context.Context) context.Context {
	return context.WithValue(ctx, noAutoMirrorKey{}, true)
}

func mirrorable(r *http.Request) bool {
	return (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
		(r.Body == nil || r.Body == http.NoBody) && GitHubHost(r.URL.Hostname())
}

// plan is the ordered routes for r, and whether a mirror the player set comes first.
func plan(r *http.Request) (routes []route, explicitMirror bool) {
	host := strings.ToLower(r.URL.Hostname())
	if Loopback(host) {
		return []route{{kind: direct}}, false
	}
	s := current()
	switch p := s.Proxy; p {
	case "off":
		routes = []route{{kind: direct}}
	case "", "system":
		sp, _ := ProxyFor(r)
		switch {
		case sp == nil:
			routes = []route{{kind: direct}, {kind: viaLocal}}
		case SteamHost(host):
			// Accelerators work below HTTP: Steam goes direct first (17 §2.2).
			routes = []route{{kind: direct}, {kind: viaProxy, proxy: sp}}
		default:
			routes = []route{{kind: viaProxy, proxy: sp}, {kind: direct}}
		}
	default:
		u, err := proxyURL(p)
		if err != nil {
			return []route{{kind: direct}}, false
		}
		routes = []route{{kind: viaProxy, proxy: u}}
	}
	mirrors, explicit := s.mirrors()
	if len(mirrors) > 0 && mirrorable(r) && (explicit || AutoMirrorAllowed(r.Context())) {
		var ms []route
		for _, m := range mirrors {
			ms = append(ms, route{kind: viaMirror, mirror: m, proxy: routes[0].proxy})
		}
		if explicit {
			return append(ms, routes...), true
		}
		routes = append(routes, ms...)
	}
	if k := remembered(host); k != "" {
		for i, rt := range routes {
			if i > 0 && rt.key() == k {
				routes = append([]route{rt}, append(routes[:i:i], routes[i+1:]...)...)
				break
			}
		}
	}
	return routes, false
}

type transport struct{ base *http.Transport }

func (t *transport) RoundTrip(r *http.Request) (*http.Response, error) {
	host := strings.ToLower(r.URL.Hostname())
	if Loopback(host) {
		return t.base.RoundTrip(r) // this machine: no routes, nothing remembered or shown
	}
	routes, explicitMirror := plan(r)
	if r.Body != nil && r.Body != http.NoBody && r.GetBody == nil {
		routes = routes[:1] // the body cannot be sent twice
	}
	start := time.Now()
	var tried []string
	var last error
	for i, rt := range routes {
		final := i == len(routes)-1
		var limit time.Duration
		if i > 0 {
			if r.Context().Err() != nil || !retryAfter(r, last) {
				break
			}
			left := ChainLimit - time.Since(start)
			if left <= 0 {
				break
			}
			limit = min(AttemptLimit, max(left, MinAttempt))
		} else if !final {
			limit = AttemptLimit
		}
		if rt.kind == viaLocal {
			p := localProxy()
			if p == nil {
				continue
			}
			rt = route{kind: viaProxy, proxy: p, local: true}
		}
		resp, err := t.try(r, rt, limit, i > 0)
		if err == nil && (final || !fallbackStatus(rt, resp.StatusCode)) {
			if !explicitMirror {
				remember(host, rt.key())
			}
			used(host, rt, i > 0, tried)
			return resp, nil
		}
		if err == nil {
			err = &statusError{code: resp.StatusCode}
			io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
		}
		if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
			return nil, err
		}
		if rt.kind == viaProxy && proxyRefused(err) {
			if rt.local {
				forgetLocal()
			} else {
				markDead(rt.proxy)
			}
		}
		tried = append(tried, rt.String())
		last = err
	}
	if r.Context().Err() != nil {
		return nil, last
	}
	be := &blockedError{err: last, github: GitHubHost(host), steam: SteamHost(host)}
	if len(tried) > 1 {
		be.tried = tried
	} else if len(tried) == 1 && strings.HasPrefix(tried[0], "proxy ") {
		be.proxy = strings.TrimPrefix(tried[0], "proxy ")
	}
	return nil, be
}

// try makes one attempt. A positive limit cancels it when no headers came by then; the
// body, once returned, is not limited.
func (t *transport) try(r *http.Request, rt route, limit time.Duration, again bool) (*http.Response, error) {
	ctx, cancel := context.WithCancel(r.Context())
	ctx = context.WithValue(ctx, routeKey{}, &rt)
	req := r.Clone(ctx)
	if again && r.GetBody != nil {
		b, err := r.GetBody()
		if err != nil {
			cancel()
			return nil, err
		}
		req.Body = b
	}
	if rt.kind == viaMirror {
		u, err := url.Parse(Mirrored(rt.mirror, r.URL.String()))
		if err != nil || u.Host == "" {
			cancel()
			return nil, fmt.Errorf("mirror %q: bad address", rt.mirror)
		}
		req.URL, req.Host = u, ""
		req.Header = r.Header.Clone()
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
	}
	var timer *time.Timer
	if limit > 0 {
		// not scheduled: one request's time limit for its headers
		timer = time.AfterFunc(limit, cancel)
	}
	resp, err := t.base.RoundTrip(req)
	if timer != nil && !timer.Stop() {
		if resp != nil {
			resp.Body.Close()
		}
		cancel()
		return nil, &attemptTimeout{limit: limit}
	}
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = &cancelBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

// fallbackStatus: a mirror's error answer, or GitHub's 5xx, moves on to the next route.
func fallbackStatus(rt route, code int) bool {
	if rt.kind == viaMirror {
		return code >= 400
	}
	return code >= 500
}

// retryAfter: GET and HEAD try the next route after any failure; other methods only when
// nothing was sent (the connection, or the proxy's, was never made).
func retryAfter(r *http.Request, err error) bool {
	switch r.Method {
	case "", http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return notSent(err)
}

func notSent(err error) bool {
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return true
	}
	var op *net.OpError
	return errors.As(err, &op) && (op.Op == "dial" || op.Op == "proxyconnect" || strings.HasPrefix(op.Op, "socks"))
}

// proxyRefused: the proxy itself could not be reached (its program has exited).
func proxyRefused(err error) bool {
	var op *net.OpError
	if !errors.As(err, &op) || (op.Op != "proxyconnect" && !strings.HasPrefix(op.Op, "socks")) {
		return false
	}
	var inner *net.OpError
	return errors.As(op.Err, &inner) && inner.Op == "dial"
}

type statusError struct{ code int }

func (e *statusError) Error() string { return fmt.Sprintf("HTTP %d", e.code) }

type attemptTimeout struct{ limit time.Duration }

func (e *attemptTimeout) Error() string { return fmt.Sprintf("no answer within %s", e.limit) }
func (e *attemptTimeout) Timeout() bool { return true }

// memory: per-host winners, dead system proxies, the local proxy found, and what the
// last requests used.
var mem struct {
	sync.Mutex
	hosts     map[string]memo
	dead      map[string]time.Time
	local     *url.URL
	localAt   time.Time
	last      Used
	lastGH    Used
	fallbacks []Used
}

type memo struct {
	key   string
	until time.Time
}

// Used is a route that answered, for Settings, logs and task results.
type Used struct {
	Host     string    `json:"host"`
	Route    string    `json:"route"`    // direct, proxy host:port, mirror host
	Fallback bool      `json:"fallback"` // an earlier route failed first
	Tried    []string  `json:"tried,omitempty"`
	At       time.Time `json:"at"`
}

func resetMemory() {
	mem.Lock()
	mem.hosts, mem.dead, mem.local, mem.localAt = nil, nil, nil, time.Time{}
	mem.last, mem.lastGH = Used{}, Used{}
	mem.Unlock()
}

func remembered(host string) string {
	mem.Lock()
	defer mem.Unlock()
	if m, ok := mem.hosts[host]; ok && time.Now().Before(m.until) {
		return m.key
	}
	return ""
}

func remember(host, key string) {
	mem.Lock()
	if mem.hosts == nil {
		mem.hosts = map[string]memo{}
	}
	mem.hosts[host] = memo{key: key, until: time.Now().Add(RememberFor)}
	mem.Unlock()
}

func markDead(p *url.URL) {
	mem.Lock()
	if mem.dead == nil {
		mem.dead = map[string]time.Time{}
	}
	mem.dead[p.Host] = time.Now().Add(DeadFor)
	mem.Unlock()
}

func proxyDead(p *url.URL) bool {
	mem.Lock()
	defer mem.Unlock()
	return time.Now().Before(mem.dead[p.Host])
}

// OnFallback, when set, hears every request that answered on a later route (the app logs it).
var OnFallback func(Used)

func used(host string, rt route, fallback bool, tried []string) {
	u := Used{Host: host, Route: rt.String(), Fallback: fallback, Tried: tried, At: time.Now()}
	mem.Lock()
	mem.last = u
	if GitHubHost(host) {
		mem.lastGH = u
	}
	if fallback {
		mem.fallbacks = append(mem.fallbacks, u)
		if len(mem.fallbacks) > 32 {
			mem.fallbacks = mem.fallbacks[len(mem.fallbacks)-32:]
		}
	}
	hook := OnFallback
	mem.Unlock()
	if fallback {
		log.Printf("network: %s reached through %s after %s failed", host, u.Route, strings.Join(tried, ", "))
		if hook != nil {
			hook(u)
		}
	}
}

// LastUsed is the route of the last request that answered: any host, or GitHub's only.
func LastUsed(github bool) (Used, bool) {
	mem.Lock()
	defer mem.Unlock()
	u := mem.last
	if github {
		u = mem.lastGH
	}
	return u, !u.At.IsZero()
}

// FallbacksSince lists the fallbacks after t, one per host and route, oldest first.
func FallbacksSince(t time.Time) []Used {
	mem.Lock()
	defer mem.Unlock()
	var out []Used
	seen := map[string]bool{}
	for _, u := range mem.fallbacks {
		if u.At.Before(t) || seen[u.Host+" "+u.Route] {
			continue
		}
		seen[u.Host+" "+u.Route] = true
		out = append(out, u)
	}
	return out
}

// localProxy finds a proxy program listening on a well-known loopback port. Only a failed
// request asks, and only loopback is dialled; the answer is kept for DeadFor.
func localProxy() *url.URL {
	mem.Lock()
	if time.Since(mem.localAt) < DeadFor {
		p := mem.local
		mem.Unlock()
		return p
	}
	mem.Unlock()
	found := make([]bool, len(LocalProxyPorts))
	var wg sync.WaitGroup
	for i, p := range LocalProxyPorts {
		u, err := url.Parse(p)
		if err != nil || !Loopback(u.Hostname()) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c, err := net.DialTimeout("tcp", u.Host, probeTimeout); err == nil {
				c.Close()
				found[i] = true
			}
		}()
	}
	wg.Wait()
	var p *url.URL
	for i, ok := range found {
		if ok {
			p, _ = url.Parse(LocalProxyPorts[i])
			break
		}
	}
	mem.Lock()
	mem.local, mem.localAt = p, time.Now()
	mem.Unlock()
	return p
}

func forgetLocal() {
	mem.Lock()
	mem.local, mem.localAt = nil, time.Now()
	mem.Unlock()
}

// SteamHint starts the advice in a failed Steam request's error; the interfaces replace
// the English advice with their own text (view.Humanize).
const SteamHint = "if you use a game accelerator (UU, Xiaoheihe, Watt Toolkit), turn on its Steam Community acceleration"

// blockedError adds what was tried and what the player can do when a site cannot be reached.
type blockedError struct {
	err    error
	github bool
	steam  bool
	proxy  string   // the one proxy the request went through, if any
	tried  []string // the routes tried, when more than one
}

func (e *blockedError) Error() string {
	hint := "if this site is blocked on your network, set a proxy in Settings › network (archo config set network.proxy http://127.0.0.1:<port>)"
	if e.proxy != "" {
		hint = "through the proxy " + e.proxy + "; check that it runs, or change it in Settings › network (network.proxy)"
	}
	if e.steam {
		hint = SteamHint + "; or " + hint
	}
	if e.github {
		hint += " or a GitHub mirror (network.github_mirror https://gh-proxy.org/)"
	}
	if len(e.tried) > 0 {
		hint = "tried " + strings.Join(e.tried, ", ") + "; " + hint
	}
	return fmt.Sprintf("%v (%s)", e.err, hint)
}
func (e *blockedError) Unwrap() error { return e.err }

// Timeout keeps url.Error's Timeout answer.
func (e *blockedError) Timeout() bool {
	var t interface{ Timeout() bool }
	return errors.As(e.err, &t) && t.Timeout()
}

// SteamBlocked reports whether err is a Steam host that could not be reached by any route;
// the interfaces then suggest an accelerator's Steam Community acceleration or a proxy.
func SteamBlocked(err error) bool {
	var be *blockedError
	return errors.As(err, &be) && be.steam
}

// AutoMirrorAllowed reports whether requests made with ctx may use an automatic mirror.
func AutoMirrorAllowed(ctx context.Context) bool { return ctx.Value(noAutoMirrorKey{}) == nil }
