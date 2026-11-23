package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Steam Web API pacing (11 §2.1): every request to one origin (scheme and host) goes through
// one gate for the whole process, whichever App sends it. Apps with their own HTTP client
// (tests) get gates of their own.

const (
	steamConcurrent  = 2                // requests in flight to one origin
	steamBurst       = 4                // requests sent without waiting
	steamEvery       = time.Second      // then one per this
	steamCooldownMin = time.Minute      // the first cooldown
	steamCooldownMax = 30 * time.Minute // cooldowns double up to this
	steamServerErrs  = 3                // server errors in a row that start a cooldown
)

// SteamBusy: Steam is limiting this computer's requests; none is sent before Until.
type SteamBusy struct{ Until time.Time }

func (e *SteamBusy) Error() string {
	return "Steam is limiting requests; RimArcho asks again after " + e.Until.Local().Format("15:04")
}

type steamGate struct {
	slots      chan struct{}
	mu         sync.Mutex
	tokens     float64
	refilled   time.Time
	until      time.Time     // the cooldown's end
	last       time.Duration // the last cooldown, doubled by the next one
	serverErrs int
}

type gateKey struct {
	origin string
	client *http.Client
}

var steamGates sync.Map // gateKey → *steamGate

func steamOrigin(u *url.URL) string { return u.Scheme + "://" + u.Host }

// steamGateFor is the gate of u's origin; a new gate starts with the cooldown stored in
// this App's cache, so a restart or the CLI does not ask a limiting Steam again.
func (a *App) steamGateFor(u *url.URL) *steamGate {
	origin := steamOrigin(u)
	key := gateKey{origin, a.HTTP}
	if g, ok := steamGates.Load(key); ok {
		return g.(*steamGate)
	}
	g := &steamGate{slots: make(chan struct{}, steamConcurrent), tokens: steamBurst, refilled: time.Now()}
	if until, ok := a.storedCooldowns()[origin]; ok {
		g.until = time.Unix(until, 0)
	}
	actual, _ := steamGates.LoadOrStore(key, g)
	return actual.(*steamGate)
}

// wait takes a slot once a token is free; a cooldown answers SteamBusy without waiting.
func (g *steamGate) wait(ctx context.Context) error {
	g.mu.Lock()
	now := time.Now()
	if now.Before(g.until) {
		g.mu.Unlock()
		return &SteamBusy{Until: g.until}
	}
	g.tokens = min(steamBurst, g.tokens+float64(now.Sub(g.refilled))/float64(steamEvery))
	g.refilled = now
	g.tokens--
	delay := time.Duration(-g.tokens * float64(steamEvery))
	g.mu.Unlock()
	if delay > 0 {
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			g.mu.Lock()
			g.tokens++ // the token was not used
			g.mu.Unlock()
			return ctx.Err()
		case <-t.C:
		}
	}
	select {
	case g.slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	g.mu.Lock()
	until := g.until
	g.mu.Unlock()
	if time.Now().Before(until) { // a request in flight met the limit meanwhile
		<-g.slots
		return &SteamBusy{Until: until}
	}
	return nil
}

func (g *steamGate) release() { <-g.slots }

// limited starts (or lengthens) a cooldown: at least retryAfter, else the last one doubled.
// Requests that were already in flight when it started do not double it again.
func (g *steamGate) limited(retryAfter time.Duration) time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	if now.Before(g.until) && now.Add(retryAfter).Before(g.until) {
		return g.until
	}
	d := min(max(steamCooldownMin, 2*g.last), steamCooldownMax)
	d = min(max(d, retryAfter), steamCooldownMax)
	g.last, g.serverErrs = d, 0
	if until := time.Now().Add(d); until.After(g.until) {
		g.until = until
	}
	return g.until
}

// observe records an answer: a limit starts a cooldown, a success ends the doubling.
func (g *steamGate) observe(resp *http.Response) (until time.Time, started bool) {
	switch code := resp.StatusCode; {
	case code == http.StatusTooManyRequests || code == http.StatusForbidden:
		return g.limited(retryAfter(resp.Header.Get("Retry-After"))), true
	case code >= 500:
		g.mu.Lock()
		g.serverErrs++
		n := g.serverErrs
		g.mu.Unlock()
		if n >= steamServerErrs {
			return g.limited(0), true
		}
	case code == http.StatusOK:
		g.mu.Lock()
		g.serverErrs, g.last = 0, 0
		g.mu.Unlock()
	}
	return time.Time{}, false
}

// retryAfter reads a Retry-After header in seconds; dates and junk count as none.
func retryAfter(v string) time.Duration {
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return 0
}

// steamDo sends one Steam Web API request through its origin's gate (11 §2.1).
func (a *App) steamDo(req *http.Request) (*http.Response, error) {
	g := a.steamGateFor(req.URL)
	if err := g.wait(req.Context()); err != nil {
		return nil, err
	}
	defer g.release()
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	if until, ok := g.observe(resp); ok {
		a.storeCooldown(steamOrigin(req.URL), until)
	}
	return resp, nil
}

// steamLimited: Steam answered an item with "rate limited" or "limit exceeded" (results
// 84 and 25), which starts a cooldown like a 429.
func (a *App) steamLimited(api string) {
	u, err := url.Parse(api)
	if err != nil {
		return
	}
	a.storeCooldown(steamOrigin(u), a.steamGateFor(u).limited(0))
}

func limitedResult(code int) bool { return code == 25 || code == 84 }

func (a *App) cooldownPath() string { return filepath.Join(a.cacheDir("steam"), "cooldown.json") }

// storedCooldowns: origin → the cooldown's end (Unix seconds).
func (a *App) storedCooldowns() map[string]int64 {
	m := map[string]int64{}
	if a.Home == "" {
		return m
	}
	if b, err := os.ReadFile(a.cooldownPath()); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

var cooldownMu sync.Mutex

func (a *App) storeCooldown(origin string, until time.Time) {
	if a.Home == "" {
		return
	}
	cooldownMu.Lock()
	defer cooldownMu.Unlock()
	m := a.storedCooldowns()
	now := time.Now().Unix()
	for k, v := range m {
		if v <= now {
			delete(m, k)
		}
	}
	m[origin] = until.Unix()
	b, _ := json.Marshal(m)
	path := a.cooldownPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if os.WriteFile(tmp, b, 0o644) == nil && os.Rename(tmp, path) != nil {
		os.Remove(tmp)
	}
}

// isSteamBusy: the error is a cooldown, which a retry cannot help.
func isSteamBusy(err error) bool {
	var b *SteamBusy
	return errors.As(err, &b)
}
