package feedback

import (
	"net/url"
	"strings"
	"testing"
)

func draft() Draft {
	return Draft{Title: "Background + solid colour", Text: "The start page background is only a solid colour.", Basic: []string{"RimArcho 0.9.0", "RimWorld 1.6.4633"}, Paste: "The full text is copied; paste it here."}
}

// read decodes a link back to its title and body.
func read(t *testing.T, link string) (string, string) {
	t.Helper()
	if !strings.HasPrefix(link, Repo+"/issues/new?") {
		t.Fatalf("not a new-issue link: %s", link)
	}
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("title"), u.Query().Get("body")
}

func TestComposeShortCarriesEverything(t *testing.T) {
	d := draft()
	d.Sections = []Section{{Heading: "Mods <2>", Lines: []string{"Harmony  brrainz.harmony  2009463077", "a ``` b"}}}
	r := Compose(d)
	if !r.Full || len(r.URL) > MaxURL {
		t.Fatalf("short draft should fit: full=%v len=%d", r.Full, len(r.URL))
	}
	title, body := read(t, r.URL)
	if title != "Background + solid colour" {
		t.Fatalf("title %q (a + must stay a +)", title)
	}
	if body != r.Body {
		t.Fatalf("link body differs from Body:\n%s\n---\n%s", body, r.Body)
	}
	for _, want := range []string{"The start page background is only a solid colour.\n\n---\nRimArcho 0.9.0\nRimWorld 1.6.4633\n", "<summary>Mods &lt;2&gt;</summary>", "a ''' b"} {
		if !strings.Contains(r.Body, want) {
			t.Fatalf("body lacks %q:\n%s", want, r.Body)
		}
	}
	if strings.Contains(r.URL, "+") {
		t.Fatalf("spaces and pluses must be percent-encoded: %s", r.URL)
	}
}

func TestComposeLongFallsBackToPaste(t *testing.T) {
	d := draft()
	var lines []string
	for range 300 {
		lines = append(lines, "Vanilla Expanded Framework  oskarpotocki.vanillafactionsexpanded.core  2023507013")
	}
	d.Sections = []Section{{Heading: "Mods", Lines: lines}}
	r := Compose(d)
	if r.Full || len(r.URL) > MaxURL {
		t.Fatalf("long draft: full=%v len=%d", r.Full, len(r.URL))
	}
	_, body := read(t, r.URL)
	if !strings.Contains(body, d.Paste) || !strings.Contains(body, d.Text) || !strings.Contains(body, "RimWorld 1.6.4633") {
		t.Fatalf("short link body should keep the paste note, the text and the basic lines:\n%s", body)
	}
	if !strings.Contains(r.Body, "oskarpotocki") {
		t.Fatal("the full body (to copy) keeps every section")
	}
	// The player's own text too long: only the note and the basic lines go in the link.
	d.Sections, d.Text = nil, strings.Repeat("A long description.", 400)
	r = Compose(d)
	_, body = read(t, r.URL)
	if r.Full || len(r.URL) > MaxURL || strings.Contains(body, "A long description") || !strings.Contains(body, d.Paste) {
		t.Fatalf("text too long: full=%v len=%d body=%q", r.Full, len(r.URL), body)
	}
	// Even an absurd title is cut to fit.
	d.Title = strings.Repeat("T", 2000)
	if r = Compose(d); len(r.URL) > MaxURL {
		t.Fatalf("title not shortened: %d", len(r.URL))
	}
}

func TestScrub(t *testing.T) {
	got := Scrub(`C:\Users\Player\AppData\x; c:\users\player\Saves; D:\Games`, `C:\Users\Player\`)
	if got != `%USERPROFILE%\AppData\x; %USERPROFILE%\Saves; D:\Games` {
		t.Fatal(got)
	}
	if Scrub("abc", "") != "abc" {
		t.Fatal("an empty home changes nothing")
	}
}

func TestSystem(t *testing.T) {
	if s := System(); s == "" {
		t.Fatal("no system name")
	}
}
