package subscribe_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

func TestReadPage(t *testing.T) {
	const fixture = `<meta property="og:title" content="Fallback title"><meta property="og:updated_time" content="2026-10-23T15:16:59Z">
<script type="application/ld+json">[{"@graph":[{"@type":"Organization","name":"Example Hosting","dateModified":"2020-01-01"},{"@type":"SoftwareApplication","name":"Example Colony Tweaks","softwareVersion":"1.4.1","dateModified":"2025-09-05T05:06:09+0000"}]}]</script>
<a href="https://gitgud.io/Example/colony">repo</a><a href="https://gitgud.io/Example/colony/-/tree/v1">tree</a><a href="https://gitgud.io/Example/colony/-/blob/main/About/About.xml">file</a>
<a href="https://github.com/Example/SharedFramework/releases">dependency</a><a href="https://gitlab.com/Example/colony.git/tree/main">other</a><a href="https://github.com/Example">profile</a><a href="file:///tmp/local.git">local</a><a href="?do=download">download</a>`
	calls := 0
	body := fixture
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.RawQuery != "" || r.Header.Get("Cookie") != "" || !strings.Contains(r.UserAgent(), "Mozilla") {
			t.Errorf("unexpected request: %s %v", r.URL, r.Header)
		}
		fmt.Fprint(w, body)
	}))
	defer srv.Close()
	client := srv.Client()
	client.Jar, _ = cookiejar.New(nil)
	p, err := subscribe.ReadPage(context.Background(), client, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || p.Name != "Example Colony Tweaks" || p.Stamp() != "1.4.1 @ 2025-09-05T05:06:09Z" || strings.Join(p.Repos, ",") != "https://gitgud.io/Example/colony,https://github.com/Example/SharedFramework,https://gitlab.com/Example/colony" {
		t.Fatalf("page: %+v calls %d", p, calls)
	}
	if p.Main != "https://gitgud.io/Example/colony" {
		t.Fatalf("main repository: %q", p.Main)
	}
	body = `<meta property="og:title" content="Example fallback"><meta property="og:updated_time" content="2026-10-23T15:16:59Z">`
	p, err = subscribe.ReadPage(context.Background(), client, srv.URL)
	if err != nil || p.Name != "Example fallback" || p.Stamp() != "2026-10-23T15:16:59Z" {
		t.Fatalf("fallback: %+v %v", p, err)
	}
	body = `<script type="application/ld+json">{"name":"Example title only"}</script><meta property="og:updated_time" content="2026-10-23T15:16:59Z">`
	p, err = subscribe.ReadPage(context.Background(), client, srv.URL)
	if err != nil || p.Name != "Example title only" {
		t.Fatalf("JSON-LD name: %+v %v", p, err)
	}
	body = strings.Repeat("x", (4<<20)+1)
	if _, err := subscribe.ReadPage(context.Background(), client, srv.URL); err == nil {
		t.Fatal("oversize page accepted")
	}
	if _, err := subscribe.ReadPage(context.Background(), client, "http://127.0.0.1/page"); err == nil {
		t.Fatal("HTTP accepted")
	}
}

func TestPageNoRedirectOrRetry(t *testing.T) {
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Redirect(w, r, "/?do=download", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := subscribe.ReadPage(context.Background(), srv.Client(), srv.URL); err == nil || calls != 1 {
		t.Fatalf("redirect followed: %v calls %d", err, calls)
	}
}
