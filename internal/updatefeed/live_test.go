package updatefeed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// This opt-in experiment uses only HTTP and t.TempDir, never a game or Steam state.
// There is no published signed feed yet: the first download supplies an observation,
// not an authenticity assertion. A throwaway-signed manifest then verifies a repeat.
func TestLiveLanzou(t *testing.T) {
	if os.Getenv("RIMARCHO_LIVE_LANZOU") != "1" {
		t.Skip("set RIMARCHO_LIVE_LANZOU=1 to probe the maintainer's share")
	}
	name := os.Getenv("RIMARCHO_LIVE_LANZOU_VERSION")
	if name == "" {
		name = "1.0.10"
	}
	artifact := "archo-" + name + "-windows-amd64.zip"
	s := liveLanzouSource()
	r, err := newRunner(&http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}, s, "")
	if err != nil {
		t.Fatal(err)
	}
	defer r.client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	target, err := r.resolve(ctx, artifact)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(target)
	t.Logf("resolved download host: %s", u.Hostname())
	resp, err := r.send(ctx, target, "GET", nil, http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		b, e := r.metadata(resp)
		if e != nil {
			t.Fatal(e)
		}
		if !r.challenge(resp, b) {
			t.Fatalf("download HTML: network_validation=%v", strings.Contains(string(b), "down_r"))
		}
		resp, err = r.send(ctx, target, "GET", nil, http.Header{})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
			b, e := r.metadata(resp)
			t.Fatalf("download HTML after challenge: network_validation=%v err=%v", strings.Contains(string(b), "down_r"), e)
		}
	}
	prefix := make([]byte, 4)
	if _, err := io.ReadFull(resp.Body, prefix); err != nil || string(prefix) != "PK\x03\x04" {
		resp.Body.Close()
		t.Fatal("resolved response is not a ZIP")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(io.MultiReader(strings.NewReader(string(prefix)), resp.Body), MaxArchive+1))
	resp.Body.Close()
	if err != nil || n <= 0 || n > MaxArchive {
		t.Fatalf("observed archive size %d: %v", n, err)
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if expected := os.Getenv("RIMARCHO_LIVE_LANZOU_SHA256"); expected != "" && expected != digest {
		t.Fatal("observed archive differs from supplied SHA-256")
	}
	t.Logf("observed %s: bytes=%d sha256=%s final_host=%s", artifact, n, digest, resp.Request.URL.Hostname())
	now := time.Now().UTC()
	m := Manifest{Format: 1, Sequence: 1, Issued: now.Add(-time.Minute), Expires: now.Add(time.Hour), Version: name, Artifact: Artifact{Name: artifact, Size: n, SHA256: digest}, Sources: []Source{s}}
	payload, _ := json.Marshal(m)
	b, keys := signed(t, payload)
	feed, err := Parse(strings.NewReader(string(b)), keys, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Run the full recipe again in a new cookie jar and DNS/connection state. Do not
	// reuse a token as persisted metadata: resolving is deliberately inexpensive.
	archive, err := feed.Download(ctx, &http.Client{}, 0, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	independent := sha256.New()
	got, err := io.Copy(independent, archive)
	if err != nil || got != n || hex.EncodeToString(independent.Sum(nil)) != digest || archive.SHA256 != digest {
		t.Fatalf("verified archive mismatch: %v", err)
	}
	t.Logf("fresh signed-feed download verified: bytes=%d sha256=%s host=%s", got, archive.SHA256, hostOnly(archive.URL))
}
func hostOnly(raw string) string { u, _ := url.Parse(raw); return u.Hostname() }

// Exact, dated page patterns, independently written from live HTTP responses.
// This fixture is a recipe example, not a built-in provider in the application.
func liveLanzouSource() Source {
	root := "https://rimarcho.lanzn.com"
	folder := root + "/b01i608qja"
	headers := func(ref, origin string) map[string]string {
		return map[string]string{"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36", "Referer": ref, "Origin": origin, "X-Requested-With": "XMLHttpRequest"}
	}
	s := Source{UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36", Name: "lanzou", Challenge: "acw-v2", Hosts: []string{"rimarcho.lanzn.com", "apifile.woozooo.com", "developer4.lanrar.com"}, Download: "${nodeurl}"}
	s.Steps = append(s.Steps, Step{Op: "request", Into: "folder", Method: "GET", URL: folder, Headers: map[string]string{"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36"}})
	regex := func(into, input, pattern string) {
		s.Steps = append(s.Steps, Step{Op: "regex", Into: into, Input: input, Pattern: pattern})
	}
	regex("endpoint", "folder", `url\s*:\s*'([^']*filemore[^']*)'`)
	for _, field := range []string{"lx", "fid", "uid", "puid", "rep", "up", "ls"} {
		// Fields observed with integer or quoted literal values. Capture either without JS.
		regex(field, "folder", fmt.Sprintf(`'%s'\s*:\s*'?([A-Za-z0-9]+)'?`, field))
	}
	regex("timestamp", "folder", `var\s+\w+\s*=\s*'(\d{10})'`)
	regex("key", "folder", `var\s+\w+\s*=\s*'([a-f0-9]{32})'`)
	s.Steps = append(s.Steps, Step{Op: "request", Into: "listing", Method: "POST", URL: root + "${endpoint}", Headers: headers(folder, root), Form: map[string]string{"lx": "${lx}", "fid": "${fid}", "uid": "${uid}", "puid": "${puid}", "rep": "${rep}", "up": "${up}", "ls": "${ls}", "t": "${timestamp}", "k": "${key}", "pg": "1", "pwd": "ctot"}},
		Step{Op: "select", Into: "id", Input: "listing", Path: "/text", Match: "name_all", Equal: "${artifact}", Field: "id"},
		Step{Op: "url", Into: "fileurl", Base: root + "/", Value: "${id}"},
		Step{Op: "request", Into: "filepage", Method: "GET", URL: "${fileurl}", Headers: headers(folder, root)})
	regex("iframe", "filepage", `<iframe[^>]+src="([^"]+)"`)
	s.Steps = append(s.Steps, Step{Op: "url", Into: "frameurl", Base: "${filepage_url}", Value: "${iframe}"}, Step{Op: "request", Into: "frame", Method: "GET", URL: "${frameurl}", Headers: headers("${fileurl}", root)})
	for _, v := range []struct{ into, name string }{{"api", "domain1"}, {"websign", "ajaxdata"}, {"sign", "wp_sign"}} {
		regex(v.into, "frame", fmt.Sprintf(`var\s+%s\s*=\s*'([^']*)'`, v.name))
	}
	s.Steps = append(s.Steps, Step{Op: "request", Into: "apiresult", Method: "POST", URL: "${api}", Headers: headers("${frameurl}", root), Form: map[string]string{"action": "downprocess", "websignkey": "${websign}", "signs": "${websign}", "sign": "${sign}", "websign": "2", "kd": "1", "ves": "1"}},
		Step{Op: "json", Into: "dom", Input: "apiresult", Path: "/dom"}, Step{Op: "json", Into: "token", Input: "apiresult", Path: "/url"},
		Step{Op: "value", Into: "nodeurl", Value: "${dom}/file/${token}"})
	return s
}
