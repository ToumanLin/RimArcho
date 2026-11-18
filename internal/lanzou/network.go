package lanzou

import (
	"compress/gzip"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const maxBody = 1 << 20
const maxRequests = 24
const maxRedirects = 6
const maxValue = 8 << 10

// A test origin is injected only by same-package tests, never by public configuration.
type runner struct {
	client                             *http.Client
	hosts                              []string
	cookies                            map[string][]*http.Cookie
	requests, redirects, metadataBytes int
	testOrigin                         string
	observe                            func(ProbeEvent)
}

func (r *runner) report(stage, host string, status int) {
	if r.observe != nil {
		r.observe(ProbeEvent{Stage: stage, Host: host, Status: status})
	}
}

func newRunner(client *http.Client, hosts []string, testOrigin string) (*runner, error) {
	// netcfg replaces DefaultTransport. Lanzou must keep a direct transport of
	// its own so every destination is checked before dialing (17 §2.2, §5.1).
	var base http.RoundTripper = &http.Transport{ForceAttemptHTTP2: true,
		MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second, ExpectContinueTimeout: time.Second}
	if client != nil && client.Transport != nil {
		base = client.Transport
	}
	transport, ok := base.(*http.Transport)
	if !ok {
		return nil, errors.New("lanzou requires a standard http.Transport for destination enforcement")
	}
	t := transport.Clone()
	// A proxy can resolve a public name to a private address after local validation.
	t.Proxy = nil
	t.DialTLSContext = nil
	t.DialTLS = nil
	t.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if testOrigin != "" {
			u, _ := url.Parse(testOrigin)
			if address == u.Host {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, address)
			}
		}
		return dialPublic(ctx, network, address, net.DefaultResolver.LookupNetIP, (&net.Dialer{Timeout: 5 * time.Second}).DialContext)
	}
	t.TLSHandshakeTimeout = 5 * time.Second
	t.ResponseHeaderTimeout = 10 * time.Second
	t.MaxResponseHeaderBytes = 16 << 10
	t.MaxConnsPerHost = 2
	t.DisableCompression = true
	t.DisableKeepAlives = true
	c := &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &runner{client: c, hosts: hosts, cookies: map[string][]*http.Cookie{}, testOrigin: testOrigin}, nil
}

// Validate every DNS answer, then connect to a checked literal IP. The HTTP
// transport retains the original hostname for TLS SNI/certificate verification.
func dialPublic(ctx context.Context, network, address string, lookup func(context.Context, string, string) ([]netip.Addr, error), dial func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := lookup(ctx, "ip", host)
	if err != nil {
		return nil, errors.New("update host DNS lookup failed")
	}
	if len(ips) == 0 {
		return nil, errors.New("update host has no addresses")
	}
	for _, ip := range ips {
		if !publicIP(ip) {
			return nil, errors.New("update host resolves to a non-public address")
		}
	}
	var last error
	for _, ip := range ips {
		c, e := dial(ctx, network, net.JoinHostPort(ip.String(), port))
		if e == nil {
			return c, nil
		}
		last = e
	}
	return nil, last
}

var nonPublic = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/3", "2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20"} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, p := range nonPublic {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

func (r *runner) checkURL(raw string) (*url.URL, error) {
	if len(raw) > maxValue || strings.ContainsAny(raw, "\r\n\t ") {
		return nil, errors.New("invalid update URL")
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Opaque != "" || u.Fragment != "" || u.Hostname() == "" {
		return nil, errors.New("invalid update URL")
	}
	if r.testOrigin != "" && u.Scheme+"://"+u.Host == r.testOrigin {
		return u, nil
	}
	if u.Scheme != "https" || (u.Port() != "" && u.Port() != "443") {
		return nil, errors.New("update URL must use HTTPS port 443")
	}
	allowed := false
	for _, host := range r.hosts {
		if strings.EqualFold(u.Hostname(), host) {
			allowed = true
		}
	}
	if !allowed {
		return nil, fmt.Errorf("update URL host %q is outside the configured allowlist", u.Hostname())
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !publicIP(ip) {
		return nil, errors.New("update URL contains a non-public address")
	}
	return u, nil
}

// send follows redirects explicitly, validating each destination before any request.
func (r *runner) send(ctx context.Context, raw, method string, form url.Values, headers http.Header) (*http.Response, error) {
	for {
		u, err := r.checkURL(raw)
		if err != nil {
			return nil, err
		}
		r.requests++
		if r.requests > maxRequests {
			return nil, errors.New("source exceeds 24 HTTP requests")
		}
		var body io.Reader
		if method == "POST" {
			body = strings.NewReader(form.Encode())
		}
		req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
		if err != nil {
			return nil, errors.New("cannot create update request")
		}
		req.Header = headers.Clone()
		req.Header.Set("Accept-Encoding", "identity")
		if req.Header.Get("User-Agent") == "" {
			ua := userAgent
			if ua == "" {
				ua = "RimArcho-lanzou/1"
			}
			req.Header.Set("User-Agent", ua)
		}
		if method == "POST" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		for _, c := range r.cookies[u.Hostname()] {
			req.AddCookie(c)
		}
		headerBytes := 0
		for key, values := range req.Header {
			for _, value := range values {
				headerBytes += len(key) + len(value) + 4
			}
		}
		if headerBytes > 16<<10 {
			return nil, errors.New("request headers exceed 16 KiB")
		}
		resp, err := r.client.Do(req)
		if err != nil {
			r.report("http-failed", u.Hostname(), 0)
			// url.Error includes bearer query tokens; callers only need the failure class.
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, errors.New("update HTTP request failed")
		}
		r.report("http", u.Hostname(), resp.StatusCode)
		if err := r.remember(u, resp.Cookies()); err != nil {
			resp.Body.Close()
			return nil, err
		}
		switch resp.StatusCode {
		case 301, 302, 303, 307, 308:
			resp.Body.Close()
			r.redirects++
			if r.redirects > maxRedirects {
				return nil, errors.New("source exceeds six redirects")
			}
			if method != "GET" {
				return nil, errors.New("POST redirects are unsupported")
			}
			next, err := u.Parse(resp.Header.Get("Location"))
			if err != nil {
				return nil, errors.New("invalid update redirect")
			}
			if resp.Header.Get("Location") == "" {
				return nil, errors.New("empty update redirect")
			}
			if !strings.EqualFold(next.Host, u.Host) {
				headers = http.Header{"User-Agent": headers.Values("User-Agent")}
			}
			raw = next.String()
			continue
		case http.StatusOK:
			return resp, nil
		default:
			resp.Body.Close()
			return nil, fmt.Errorf("update source returned HTTP %d", resp.StatusCode)
		}
	}
}

func (r *runner) remember(u *url.URL, cookies []*http.Cookie) error {
	existing := r.cookies[u.Hostname()]
	for _, c := range cookies {
		// Host-only cookies prevent a download node poisoning another source's state.
		c.Domain = ""
		c.Path = "/"
		c.Secure = true
		found := false
		for i, old := range existing {
			if old.Name == c.Name {
				existing[i] = c
				found = true
				break
			}
		}
		if !found {
			existing = append(existing, c)
		}
	}
	r.cookies[u.Hostname()] = existing
	count, size := 0, 0
	for _, cs := range r.cookies {
		for _, c := range cs {
			count++
			size += len(c.String())
		}
	}
	if count > 16 || size > 8<<10 {
		return errors.New("source cookies exceed limit")
	}
	return nil
}

func (r *runner) metadata(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	wire := io.LimitReader(resp.Body, maxBody+1)
	var reader io.Reader = wire
	switch resp.Header.Get("Content-Encoding") {
	case "", "identity":
	case "gzip":
		z, err := gzip.NewReader(wire)
		if err != nil {
			return nil, errors.New("invalid gzip metadata")
		}
		defer z.Close()
		reader = z
	default:
		return nil, errors.New("unsupported metadata content encoding")
	}
	b, err := limited(reader, maxBody)
	if err != nil {
		return nil, err
	}
	r.metadataBytes += len(b)
	if r.metadataBytes > 4<<20 {
		return nil, errors.New("source metadata exceeds 4 MiB")
	}
	return b, nil
}

// acwCookie implements only the fixed permutation/XOR observed in the live page.
// Page JavaScript is never evaluated.
var acwArg = regexp.MustCompile(`\barg1\s*=\s*['"]([0-9a-fA-F]{40})['"]`)

func acwCookie(body []byte) (string, bool) {
	if !strings.Contains(string(body), "acw_sc__v2") {
		return "", false
	}
	m := acwArg.FindSubmatch(body)
	if m == nil {
		return "", false
	}
	order := []int{15, 35, 29, 24, 33, 16, 1, 38, 10, 9, 19, 31, 40, 27, 22, 23, 25, 13, 6, 11, 39, 18, 20, 8, 14, 21, 32, 26, 2, 30, 7, 4, 17, 5, 3, 28, 34, 37, 12, 36}
	mixed := make([]byte, 40)
	for i, n := range order {
		mixed[i] = m[1][n-1]
	}
	a, _ := hex.DecodeString(string(mixed))
	key, _ := hex.DecodeString("3000176000856006061501533003690027800375")
	for i := range a {
		a[i] ^= key[i]
	}
	return hex.EncodeToString(a), true
}

func (r *runner) challenge(resp *http.Response, b []byte) bool {
	value, ok := acwCookie(b)
	if !ok {
		return false
	}
	ok = r.remember(resp.Request.URL, []*http.Cookie{{Name: "acw_sc__v2", Value: value}}) == nil
	if ok {
		r.report("acw-challenge", resp.Request.URL.Hostname(), resp.StatusCode)
	}
	return ok
}
