package updatefeed

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)

func syntheticZip(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, name := range []string{"archo.exe", "RimArcho.exe"} {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte("synthetic executable; never run")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func manifest(data []byte, target string) Manifest {
	sum := sha256.Sum256(data)
	return Manifest{Format: 1, Sequence: 7, Issued: testNow.Add(-time.Hour), Expires: testNow.Add(24 * time.Hour), Version: "1.2.3", Artifact: Artifact{Name: "archo-1.2.3-windows-amd64.zip", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}, Sources: []Source{{Name: "synthetic", Hosts: []string{"example.com"}, Download: target}}}
}
func signed(t *testing.T, payload []byte) ([]byte, map[string]ed25519.PublicKey) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(key, append([]byte(signatureDomain+"test\n"), payload...))
	b, err := json.Marshal(Envelope{KeyID: "test", Payload: base64.StdEncoding.EncodeToString(payload), Signature: base64.StdEncoding.EncodeToString(sig)})
	if err != nil {
		t.Fatal(err)
	}
	return b, map[string]ed25519.PublicKey{"test": pub}
}
func parseManifest(t *testing.T, m Manifest) *Feed {
	t.Helper()
	p, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	b, keys := signed(t, p)
	f, err := Parse(bytes.NewReader(b), keys, testNow, 0)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestAuthenticationAndFreshness(t *testing.T) {
	m := manifest([]byte("archive"), "https://example.com/archive")
	p, _ := json.Marshal(m)
	b, keys := signed(t, p)
	f, err := Parse(bytes.NewReader(b), keys, testNow, 7)
	if err != nil || f.Version() != "1.2.3" || f.Sequence() != 7 {
		t.Fatalf("valid feed: %v", err)
	}
	for _, tc := range []struct {
		name   string
		modify func(*Manifest)
		now    time.Time
		floor  uint64
	}{
		{name: "format", modify: func(m *Manifest) { m.Format = 2 }},
		{name: "expired", now: testNow.Add(48 * time.Hour)},
		{name: "future", now: testNow.Add(-time.Hour - time.Minute*6)},
		{name: "replay", floor: 8},
		{name: "zero sequence", modify: func(m *Manifest) { m.Sequence = 0 }},
		{name: "validity", modify: func(m *Manifest) { m.Expires = testNow.Add(91 * 24 * time.Hour) }},
		{name: "prerelease", modify: func(m *Manifest) { m.Version = "1.2.3-rc1" }},
		{name: "path", modify: func(m *Manifest) { m.Artifact.Name = "../archo.exe" }},
		{name: "oversize", modify: func(m *Manifest) { m.Artifact.Size = MaxArchive + 1 }},
		{name: "uppercase digest", modify: func(m *Manifest) { m.Artifact.SHA256 = strings.ToUpper(m.Artifact.SHA256) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := m
			if tc.modify != nil {
				tc.modify(&copy)
			}
			p, _ := json.Marshal(copy)
			b, k := signed(t, p)
			now := tc.now
			if now.IsZero() {
				now = testNow
			}
			if _, err := Parse(bytes.NewReader(b), k, now, tc.floor); err == nil {
				t.Fatal("invalid feed accepted")
			}
		})
	}
	var e Envelope
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatal(err)
	}
	e.Payload = base64.StdEncoding.EncodeToString(append(p, ' '))
	tampered, _ := json.Marshal(e)
	if _, err := Parse(bytes.NewReader(tampered), keys, testNow, 0); err == nil {
		t.Fatal("tampered payload accepted")
	}
	if _, err := Parse(bytes.NewReader(b), map[string]ed25519.PublicKey{"other": keys["test"]}, testNow, 0); err == nil {
		t.Fatal("unknown key accepted")
	}
	if _, err := Parse(strings.NewReader(strings.Repeat("x", MaxEnvelope+1)), keys, testNow, 0); err == nil {
		t.Fatal("oversize envelope accepted")
	}
	for _, invalid := range []string{`{"format":1,"format":1}`, `{"format":1,"unknown":true}`, string(p) + " {}", `{"a":{"b":1,"b":2}}`} {
		b, k := signed(t, []byte(invalid))
		if _, err := Parse(bytes.NewReader(b), k, testNow, 0); err == nil {
			t.Fatalf("invalid JSON accepted: %s", invalid)
		}
	}
}

func TestRecipeDownload(t *testing.T) {
	data := syntheticZip(t)
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		switch r.URL.Path {
		case "/folder":
			if _, err := r.Cookie("acw_sc__v2"); err != nil {
				w.Header().Set("Content-Type", "text/html")
				io.WriteString(w, `<script>var arg1='0123456789ABCDEF0123456789ABCDEF01234567';'acw_sc__v2'</script>`)
				return
			}
			io.WriteString(w, `<form token='synthetic'>`)
		case "/list":
			if r.Method != "POST" || r.FormValue("password") != "synthetic +&=" || r.FormValue("token") != "synthetic" {
				t.Error("form encoding or extraction failed")
				http.Error(w, "bad form", 400)
				return
			}
			if r.Header.Get("Referer") != "synthetic referer" {
				t.Error("missing referer")
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"zt":1,"files":[{"name":"archo-1.2.3-windows-amd64.zip","id":"/redirect"},{"name":"other.zip","id":"/wrong"}]}`)
		case "/redirect":
			w.Header().Set("Location", "/archive")
			w.WriteHeader(http.StatusFound)
		case "/archive":
			w.Header().Set("Content-Type", "application/zip")
			w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	m := manifest(data, "${download}")
	m.Sources[0].Challenge = "acw-v2"
	m.Sources[0].Steps = []Step{
		{Op: "request", Into: "page", Method: "GET", URL: server.URL + "/folder"},
		{Op: "regex", Into: "token", Input: "page", Pattern: `token='([^']+)'`},
		{Op: "request", Into: "listing", Method: "POST", URL: server.URL + "/list", Headers: map[string]string{"Referer": "synthetic referer"}, Form: map[string]string{"token": "${token}", "password": "synthetic +&="}},
		{Op: "json", Into: "status", Input: "listing", Path: "/zt"},
		{Op: "assert", Into: "checked", Input: "${status}", Equal: "1"},
		{Op: "select", Into: "id", Input: "listing", Path: "/files", Match: "name", Equal: "${artifact}", Field: "id"},
		{Op: "url", Into: "download", Base: "${page_url}", Value: "${id}"},
	}
	f := parseManifest(t, m)
	dir := t.TempDir()
	archive, err := f.download(context.Background(), server.Client(), 0, dir, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(archive)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("archive bytes: %v", err)
	}
	if archive.Size != int64(len(data)) || archive.SHA256 != m.Artifact.SHA256 || !strings.HasSuffix(archive.URL, "/archive") {
		t.Fatal("wrong verified result")
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("temporary archive not removed")
	}
	if hits != 5 {
		t.Fatalf("requests: got %d", hits)
	}
}

func TestDownloadFailureLeavesNoFile(t *testing.T) {
	data := syntheticZip(t)
	for _, kind := range []string{"hash", "short", "long", "html", "redirect-loop", "gzip-bomb", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "redirect-loop":
					w.Header().Set("Location", "/archive")
					w.WriteHeader(302)
				case "html":
					w.Header().Set("Content-Type", "text/html")
					io.WriteString(w, "not an archive")
				case "gzip-bomb":
					w.Header().Set("Content-Type", "text/html")
					w.Header().Set("Content-Encoding", "gzip")
					z := gzip.NewWriter(w)
					io.WriteString(z, strings.Repeat("x", maxBody+1))
					z.Close()
				case "short":
					w.Write(data[:len(data)-1])
				case "long":
					w.Write(append(data, 'x'))
				case "hash":
					copy := bytes.Clone(data)
					copy[0] ^= 1
					w.Write(copy)
				case "cancel":
					w.(http.Flusher).Flush()
					<-r.Context().Done()
				}
			}))
			defer server.Close()
			m := manifest(data, server.URL+"/archive")
			f := parseManifest(t, m)
			dir := t.TempDir()
			ctx := context.Background()
			if kind == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}
			if archive, err := f.download(ctx, server.Client(), 0, dir, server.URL); err == nil {
				archive.Close()
				t.Fatal("bad download accepted")
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 0 {
				t.Fatal("unverified archive left behind")
			}
		})
	}
}

func TestDestinationPolicy(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "::1", "::ffff:127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.100.100.200", "0.0.0.0", "192.0.2.1", "198.19.0.1", "224.0.0.1", "fc00::1", "fe80::1", "2002:7f00:1::", "64:ff9b::7f00:1"} {
		if publicIP(netip.MustParseAddr(address)) {
			t.Errorf("non-public IP accepted: %s", address)
		}
	}
	for _, address := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if !publicIP(netip.MustParseAddr(address)) {
			t.Errorf("public IP rejected: %s", address)
		}
	}
	r, err := newRunner(nil, Source{Hosts: []string{"example.com", "127.0.0.1"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"http://example.com/x", "https://example.com:661/x", "https://localhost/x", "https://127.0.0.1/x", "https://example.com.evil.test/x", "https://user:pass@example.com/x", "https://example.com/x#fragment", "file:///tmp/x"} {
		if _, err := r.checkURL(u); err == nil {
			t.Errorf("unsafe URL accepted: %s", u)
		}
	}
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer server.Close()
	f := parseManifest(t, manifest([]byte("archive"), server.URL+"/archive"))
	if _, err := f.Download(context.Background(), server.Client(), 0, t.TempDir()); err == nil || hits != 0 {
		t.Fatal("public API allowed a loopback request")
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", server.URL+"/private")
		w.WriteHeader(302)
	}))
	defer redirect.Close()
	f = parseManifest(t, manifest([]byte("archive"), redirect.URL))
	if _, err := f.download(context.Background(), redirect.Client(), 0, t.TempDir(), redirect.URL); err == nil || hits != 0 {
		t.Fatal("redirect reached an unauthorized destination")
	}
}

func TestRecipeLimitsAndFailure(t *testing.T) {
	for _, step := range []Step{
		{Op: "exec", Into: "x"}, {Op: "request", Into: "x", Method: "PUT"},
		{Op: "request", Into: "x", Method: "GET", Headers: map[string]string{"Authorization": "secret"}},
		{Op: "regex", Into: "x", Pattern: "(a)(b)"}, {Op: "regex", Into: "x", Pattern: "("},
	} {
		s := manifest([]byte("a"), "https://example.com/a").Sources[0]
		s.Steps = []Step{step}
		if validateSource(s) == nil {
			t.Fatalf("invalid step accepted: %+v", step)
		}
	}
	s := manifest([]byte("a"), "https://example.com/a").Sources[0]
	s.Steps = make([]Step, 49)
	if validateSource(s) == nil {
		t.Fatal("too many steps accepted")
	}
	// A signed recipe can still be stale. Missing fields and ambiguous names must fail.
	for _, steps := range [][]Step{
		{{Op: "value", Into: "x", Value: "${missing}"}},
		{{Op: "value", Into: "page", Value: "changed page"}, {Op: "regex", Into: "x", Input: "page", Pattern: "token=(.+)"}},
		{{Op: "value", Into: "page", Value: `[{"name":"x","id":"a"},{"name":"x","id":"b"}]`}, {Op: "select", Into: "x", Input: "page", Match: "name", Equal: "x", Field: "id"}},
	} {
		s.Steps = steps
		r, err := newRunner(nil, s, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.resolve(context.Background(), "archive"); err == nil {
			t.Fatal("broken recipe succeeded")
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	defer server.Close()
	s.Steps = nil
	for i := 0; i < 25; i++ {
		s.Steps = append(s.Steps, Step{Op: "request", Into: "page", Method: "GET", URL: server.URL})
	}
	r, err := newRunner(server.Client(), s, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.resolve(context.Background(), "archive"); err == nil {
		t.Fatal("HTTP budget not enforced")
	}
}

func TestDNSPinning(t *testing.T) {
	calls := 0
	var destination string
	dial := func(_ context.Context, _, address string) (net.Conn, error) {
		calls++
		destination = address
		return nil, errors.New("synthetic dial")
	}
	for _, answers := range [][]netip.Addr{
		{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("127.0.0.1")},
		{netip.MustParseAddr("169.254.169.254")},
	} {
		lookup := func(context.Context, string, string) ([]netip.Addr, error) { return answers, nil }
		_, err := dialPublic(context.Background(), "tcp", "example.com:443", lookup, dial)
		if err == nil || calls != 0 {
			t.Fatal("non-public DNS answer reached the dialer")
		}
	}
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("1.1.1.1")}, nil
	}
	_, _ = dialPublic(context.Background(), "tcp", "example.com:443", lookup, dial)
	if calls != 1 || destination != "1.1.1.1:443" {
		t.Fatal("dial did not pin the checked address")
	}
}

func TestExpansionIsBoundedAndSinglePass(t *testing.T) {
	vars := map[string]string{"page": strings.Repeat("x", maxBody), "literal": "${page}"}
	if _, err := expand(strings.Repeat("${page}", 2000), vars); err == nil {
		t.Fatal("large replacement was accepted")
	}
	got, err := expand("${literal}", vars)
	if err != nil || got != "${page}" {
		t.Fatal("replacement was recursively expanded")
	}
	if _, err := expand(strings.Repeat("x", maxValue+1), vars); err == nil {
		t.Fatal("large literal was accepted")
	}
	var value any
	if strictJSON([]byte(`{"key":1,"KEY":2}`), &value) == nil {
		t.Fatal("case-colliding keys were accepted")
	}
	if strictJSON([]byte{'"', 0xff, '"'}, &value) == nil {
		t.Fatal("invalid UTF-8 was accepted")
	}
}
