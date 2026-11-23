package netcfg

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// counter is a loopback server that answers with a fixed status and body and counts hits.
type counter struct {
	*httptest.Server
	mu   sync.Mutex
	hits []string
}

func (c *counter) n() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.hits) }

func newCounter(t *testing.T, status int, body string) *counter {
	c := &counter{}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.hits = append(c.hits, r.Method+" "+r.RequestURI)
		c.mu.Unlock()
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(c.Close)
	return c
}

func addr(s *httptest.Server) string { return strings.TrimPrefix(s.URL, "http://") }

// closedAddr is a loopback port nothing listens on.
func closedAddr(t *testing.T) string {
	s := httptest.NewServer(http.NotFoundHandler())
	a := addr(s)
	s.Close()
	return a
}

// routed is a client whose dials to the named hosts reach loopback addresses or are
// refused ("refuse") or hang ("hang"); any other non-loopback dial fails the test.
func routed(t *testing.T, hosts map[string]string) *http.Client {
	base := http.DefaultTransport.(*http.Transport).Clone()
	d := &net.Dialer{Timeout: 2 * time.Second}
	base.DialContext = func(ctx context.Context, network, a string) (net.Conn, error) {
		h, _, _ := net.SplitHostPort(a)
		if to, ok := hosts[h]; ok {
			switch to {
			case "refuse":
				return nil, &net.OpError{Op: "dial", Net: network, Err: syscall.ECONNREFUSED}
			case "hang":
				<-ctx.Done()
				return nil, &net.OpError{Op: "dial", Net: network, Err: ctx.Err()}
			}
			a = to
		} else if !Loopback(h) {
			t.Errorf("dial outside the machine: %s", a)
			return nil, errors.New("outside")
		}
		return d.DialContext(ctx, network, a)
	}
	return &http.Client{Transport: New(base), Timeout: 10 * time.Second}
}

func setup(t *testing.T, mirrors ...string) {
	saved := []any{AutoMirrors, LocalProxyPorts, AttemptLimit, ChainLimit, MinAttempt}
	AutoMirrors, LocalProxyPorts = mirrors, nil
	readSystem = func() systemProxySettings { return systemProxySettings{} }
	sysCache.at = time.Time{}
	t.Cleanup(func() {
		AutoMirrors, LocalProxyPorts = saved[0].([]string), saved[1].([]string)
		AttemptLimit, ChainLimit, MinAttempt = saved[2].(time.Duration), saved[3].(time.Duration), saved[4].(time.Duration)
		readSystem, sysCache.at = readSystemProxy, time.Time{}
		OnFallback = nil
		Configure(Settings{})
	})
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if os.Getenv(k) != "" {
			t.Skip("the environment sets a proxy")
		}
	}
	Configure(Settings{})
}

func body(t *testing.T, c *http.Client, method, u string) (string, error) {
	t.Helper()
	req, _ := http.NewRequest(method, u, nil)
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

// auto: GitHub first, then the mirrors in order; the route that answered is remembered.
func TestAutoMirrorOrderAndMemory(t *testing.T) {
	bad := newCounter(t, 502, "bad")
	good := newCounter(t, 200, "mirror")
	setup(t, bad.URL+"/", good.URL+"/")
	var heard []Used
	OnFallback = func(u Used) { heard = append(heard, u) }
	c := routed(t, map[string]string{"github.com": "refuse"})
	target := "http://github.com/a/b/releases/download/v1/x.zip"

	if got, err := body(t, c, "GET", target); err != nil || got != "mirror" {
		t.Fatalf("auto: %q %v", got, err)
	}
	if bad.n() != 1 || good.n() != 1 || good.hits[0] != "GET /"+target {
		t.Fatalf("order: bad %v good %v", bad.hits, good.hits)
	}
	u, ok := LastUsed(true)
	if !ok || !u.Fallback || u.Route != "mirror "+addr(good.Server) || len(heard) != 1 {
		t.Fatalf("last route %+v, heard %v", u, heard)
	}
	if f := FallbacksSince(time.Now().Add(-time.Minute)); len(f) != 1 || f[0].Host != "github.com" {
		t.Fatalf("fallbacks: %+v", f)
	}
	// The next request starts with the mirror that answered.
	if got, _ := body(t, c, "GET", target); got != "mirror" || bad.n() != 1 || good.n() != 2 {
		t.Fatalf("memory: %q bad %d good %d", got, bad.n(), good.n())
	}
	// A settings change forgets it.
	Configure(Settings{})
	body(t, c, "GET", target)
	if bad.n() != 2 {
		t.Fatalf("memory survived Configure: bad %d", bad.n())
	}

	// GitHub's own 5xx moves on too; a working GitHub never meets a mirror.
	origin := newCounter(t, 503, "down")
	c = routed(t, map[string]string{"github.com": addr(origin.Server)})
	Configure(Settings{})
	if got, _ := body(t, c, "GET", target); got != "mirror" || origin.n() != 1 {
		t.Fatalf("5xx: %q origin %d", got, origin.n())
	}
	ok200 := newCounter(t, 200, "origin")
	c = routed(t, map[string]string{"github.com": addr(ok200.Server)})
	Configure(Settings{})
	before := good.n()
	if got, _ := body(t, c, "GET", target); got != "origin" || good.n() != before {
		t.Fatalf("direct works: %q, mirror hits %d → %d", got, before, good.n())
	}
	if u, _ := LastUsed(true); u.Route != "direct" || u.Fallback {
		t.Fatalf("last: %+v", u)
	}
}

// off never uses a mirror; an explicit mirror keeps going first; WithoutAutoMirror skips
// only the automatic ones; POST and other hosts never meet a mirror.
func TestMirrorSettingsUnchanged(t *testing.T) {
	m := newCounter(t, 200, "mirror")
	setup(t, m.URL+"/")
	origin := newCounter(t, 200, "origin")
	c := routed(t, map[string]string{"github.com": "refuse", "example.org": addr(origin.Server)})
	target := "http://github.com/x"

	Configure(Settings{Mirror: "off"})
	if _, err := body(t, c, "GET", target); err == nil || m.n() != 0 {
		t.Fatalf("off used a mirror: %v, hits %d", err, m.n())
	}
	Configure(Settings{})
	req, _ := http.NewRequestWithContext(WithoutAutoMirror(context.Background()), "GET", target, nil)
	if _, err := c.Do(req); err == nil || m.n() != 0 {
		t.Fatalf("WithoutAutoMirror used a mirror: %v", err)
	}
	explicit := newCounter(t, 200, "explicit")
	Configure(Settings{Mirror: explicit.URL + "/{url}"})
	req, _ = http.NewRequestWithContext(WithoutAutoMirror(context.Background()), "GET", target, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if explicit.n() != 1 {
		t.Fatalf("a mirror the player set is not used")
	}
	// Mirror first even when GitHub works, every time (no memory).
	gh := newCounter(t, 200, "origin")
	c = routed(t, map[string]string{"github.com": addr(gh.Server)})
	body(t, c, "GET", target)
	body(t, c, "GET", target)
	if explicit.n() != 3 || gh.n() != 0 {
		t.Fatalf("explicit mirror not first: mirror %d github %d", explicit.n(), gh.n())
	}
	Configure(Settings{})
	c = routed(t, map[string]string{"github.com": "refuse", "example.org": addr(origin.Server)})
	body(t, c, "POST", target)
	body(t, c, "GET", "http://example.org/")
	if m.n() != 0 || origin.n() != 1 {
		t.Fatalf("mirrored a POST or another host: mirror %d origin %d", m.n(), origin.n())
	}
}

// A proxy server for http:// targets: it answers itself and records the absolute URLs.
func newProxy(t *testing.T) *counter { return newCounter(t, 200, "proxy") }

// Steam hosts go direct first even with a system proxy set, and use the proxy when direct fails.
func TestSteamDirectFirst(t *testing.T) {
	setup(t)
	p := newProxy(t)
	readSystem = func() systemProxySettings { return systemProxySettings{Enabled: true, Server: addr(p.Server)} }
	sysCache.at = time.Time{}
	origin := newCounter(t, 200, "steam")
	c := routed(t, map[string]string{"steamcommunity.com": addr(origin.Server), "api.steampowered.com": "refuse", "example.org": "refuse"})

	if got, _ := body(t, c, "GET", "http://steamcommunity.com/sharedfiles/filedetails/?id=1"); got != "steam" || p.n() != 0 {
		t.Fatalf("Steam through the proxy first: %q, proxy hits %d", got, p.n())
	}
	if got, _ := body(t, c, "POST", "http://api.steampowered.com/ISteamRemoteStorage/GetPublishedFileDetails/v1/"); got != "proxy" {
		t.Fatalf("Steam direct refused, no proxy: %q", got)
	}
	// Other hosts keep the system proxy first.
	if got, _ := body(t, c, "GET", "http://example.org/"); got != "proxy" || p.n() != 2 {
		t.Fatalf("other host: %q, proxy %d", got, p.n())
	}
	if !SteamHost("cdn.akamai.steamstatic.com") || !SteamHost("steamuserimages-a.akamaihd.net") || SteamHost("evilsteamcommunity.com") || SteamHost("akamaihd.net") {
		t.Fatal("SteamHost")
	}
}

// A system proxy whose program exited: requests go direct, and for a while ProxyFor skips it.
func TestDeadSystemProxyGoesDirect(t *testing.T) {
	setup(t)
	dead := closedAddr(t)
	readSystem = func() systemProxySettings { return systemProxySettings{Enabled: true, Server: dead} }
	sysCache.at = time.Time{}
	origin := newCounter(t, 200, "origin")
	c := routed(t, map[string]string{"example.org": addr(origin.Server)})
	if got, err := body(t, c, "GET", "http://example.org/"); got != "origin" {
		t.Fatalf("dead proxy: %q %v", got, err)
	}
	if got := Describe("http://example.org/"); got != "direct" {
		t.Fatalf("the dead proxy is still described: %s", got)
	}
	if u, _ := LastUsed(false); u.Route != "direct" || !u.Fallback || u.Tried[0] != "proxy "+dead {
		t.Fatalf("last: %+v", u)
	}
	// POST: nothing was sent through the dead proxy, so direct is tried as well.
	Configure(Settings{})
	if got, _ := body(t, c, "POST", "http://example.org/"); got != "origin" {
		t.Fatalf("POST after a dead proxy: %q", got)
	}

	// An explicit proxy is the only route, dead or not.
	Configure(Settings{Proxy: "http://" + dead})
	_, err := body(t, c, "GET", "http://example.org/")
	if err == nil || origin.n() != 2 || !strings.Contains(err.Error(), "through the proxy "+dead) {
		t.Fatalf("explicit proxy fell back: %v, origin %d", err, origin.n())
	}
	// off is direct only: no local proxy probe either.
	lp := newProxy(t)
	LocalProxyPorts = []string{lp.URL}
	Configure(Settings{Proxy: "off"})
	c = routed(t, map[string]string{"example.org": "refuse"})
	if _, err := body(t, c, "GET", "http://example.org/"); err == nil || lp.n() != 0 {
		t.Fatalf("off: %v, local proxy %d", err, lp.n())
	}
}

// Without a known proxy, a failed request tries the well-known loopback proxy ports.
func TestLocalProxyAfterFailure(t *testing.T) {
	setup(t)
	lp := newProxy(t)
	LocalProxyPorts = []string{"http://" + closedAddr(t), lp.URL}
	origin := newCounter(t, 200, "origin")
	c := routed(t, map[string]string{"example.org": "refuse", "good.example": addr(origin.Server)})
	if got, _ := body(t, c, "GET", "http://good.example/"); got != "origin" {
		t.Fatalf("direct: %q", got)
	}
	if got, _ := body(t, c, "GET", "http://example.org/"); got != "proxy" || lp.n() != 1 {
		t.Fatalf("local proxy: %q %d", got, lp.n())
	}
	if u, _ := LastUsed(false); u.Route != "proxy "+addr(lp.Server) || !u.Fallback {
		t.Fatalf("last: %+v", u)
	}
	// Remembered: next time the local proxy goes first.
	body(t, c, "GET", "http://example.org/")
	if u, _ := LastUsed(false); u.Fallback {
		t.Fatalf("memory: %+v", u)
	}
}

// The whole chain has a deadline: a hanging GitHub and hanging mirrors end in bounded time.
func TestChainDeadline(t *testing.T) {
	setup(t, "http://mirror-a.test/", "http://mirror-b.test/", "http://mirror-c.test/")
	AttemptLimit, ChainLimit, MinAttempt = 300*time.Millisecond, 700*time.Millisecond, 100*time.Millisecond
	c := routed(t, map[string]string{"github.com": "hang", "mirror-a.test": "hang", "mirror-b.test": "hang", "mirror-c.test": "hang"})
	start := time.Now()
	_, err := body(t, c, "GET", "http://github.com/x")
	took := time.Since(start)
	if err == nil || took > 1500*time.Millisecond {
		t.Fatalf("chain took %s: %v", took, err)
	}
	if !os.IsTimeout(err) || !strings.Contains(err.Error(), "tried direct, mirror mirror-a.test") {
		t.Fatalf("error: %v", err)
	}
	// A slow answer that starts within the limit is not cut: only the headers are timed.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		time.Sleep(500 * time.Millisecond)
		io.WriteString(w, "late body")
	}))
	t.Cleanup(slow.Close)
	c = routed(t, map[string]string{"github.com": addr(slow)})
	Configure(Settings{})
	if got, err := body(t, c, "GET", "http://github.com/x"); got != "late body" {
		t.Fatalf("body cut: %q %v", got, err)
	}
}
