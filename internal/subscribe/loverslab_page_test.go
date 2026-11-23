//go:build windows

package subscribe

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/accounts"
)

// parseFixture parses body as a file page served at page.
func parseFixture(t *testing.T, l *LoversLab, page, body string) (*FilePage, error) {
	t.Helper()
	u, err := url.Parse(page)
	if err != nil {
		t.Fatal(err)
	}
	resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Request: &http.Request{URL: u}}
	return l.parsePage(resp, siteAction{page: page})
}

// A download page's links and forms: a GET form carries its fields in the query, an
// agreement too long to show is left out with everything inside it, a nameless link gets a
// generic name, which goes once a file has a real one, and a page with none is an error.
func TestLoversLabParsePageForms(t *testing.T) {
	session, err := accounts.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	l := NewLoversLab(session, nil)
	page := accounts.Site + "files/file/123-fixture/"

	p, err := parseFixture(t, l, page, `<form action="?do=download&x=1">Agree <input type="hidden" name="k" value="v"><input type="submit" name="go"></form>`)
	if err != nil || len(p.Files) != 1 || p.Files[0].Name != "confirm download" || p.Agreement != "Agree" {
		t.Fatalf("GET form: %+v %v", p, err)
	}
	a := l.actions[p.Files[0].ID]
	if a.method != "GET" || a.values.Get("k") != "v" || a.values.Get("go") != "1" {
		t.Fatalf("GET form action: %+v", a)
	}
	if q, _ := url.Parse(a.target); q.Query().Get("k") != "v" || q.Query().Get("go") != "1" || q.Query().Get("x") != "1" || q.Query().Get("do") != "download" {
		t.Fatalf("GET form query: %s", a.target)
	}

	long := strings.Repeat("x", 12001)
	p, err = parseFixture(t, l, page, `<form method="post" action="?do=download"><p>`+long+`</p><a href="?do=download&r=9">inner.zip</a></form><a href="?do=download&r=1">outer.zip</a>`)
	if err != nil || len(p.Files) != 1 || p.Files[0].Name != "outer.zip" || p.Agreement != "" {
		t.Fatalf("long agreement: %+v %v", p, err)
	}

	p, err = parseFixture(t, l, page, `<a href="?do=download&r=1"></a>`)
	if err != nil || len(p.Files) != 1 || p.Files[0].Name != "download file" {
		t.Fatalf("nameless link: %+v %v", p, err)
	}
	p, err = parseFixture(t, l, page, `<a href="?do=download&r=1"></a><a href="?do=download&r=2">real.zip</a>`)
	if err != nil || len(p.Files) != 1 || p.Files[0].Name != "real.zip" {
		t.Fatalf("generic beside a named file: %+v %v", p, err)
	}

	if _, err = parseFixture(t, l, page, `<a href="/elsewhere/">nothing</a>`); err == nil || !strings.Contains(err.Error(), "no downloadable files recognized") {
		t.Fatalf("no files: %v", err)
	}
}
