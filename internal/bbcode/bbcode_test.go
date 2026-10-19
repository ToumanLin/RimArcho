package bbcode

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	src := "[h1]Vehicle Framework[/h1]\r\n[img]https://i.imgur.com/a.png[/img]\r\n" +
		"Adds [b]vehicles[/b] to the game.\nSecond line, [url=https://x.org/wiki]the wiki[/url].\n\n\n" +
		"[h2]Features[/h2]\n[list]\n[*] Cars\n[*] Boats [strike]planes[/strike]\n[list][*]nested[/list]\n[/list]\n" +
		"[olist][*]one[*]two[/olist]\n" +
		"[table][tr][th]Mod[/th][th]Status[/th][/tr][tr][td]CE[/td][td][b]ok[/b][/td][/tr][/table]\n" +
		"[hr][/hr][quote=someone]said this[/quote][code]x := 1\n  y[/code][noparse][b]raw[/b][/noparse][unknown]kept[/unknown]"
	got := Plain(Parse(src))
	want := `Vehicle Framework
[image: https://i.imgur.com/a.png]
Adds vehicles to the game.
Second line, the wiki.

Features
• Cars
• Boats planes
  • nested

1. one
2. two

Mod | Status
CE | ok

  said this
  x := 1
    y
[b]raw[/b]kept
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	bs := Parse(src)
	var bold, strike, link bool
	for _, b := range bs {
		for _, s := range b.Spans {
			bold = bold || (s.Bold && s.Text == "vehicles")
			strike = strike || (s.Strike && s.Text == "planes")
			link = link || (s.Link == "https://x.org/wiki" && s.Text == "the wiki")
		}
	}
	if !bold || !strike || !link {
		t.Fatalf("styles: bold %v strike %v link %v", bold, strike, link)
	}
	if bs[0].Kind != Heading || bs[0].Level != 1 {
		t.Fatalf("first block %+v", bs[0])
	}
}

func TestTolerant(t *testing.T) {
	for _, src := range []string{"[b]unclosed", "[/list]stray[/b]", "[img]https://a/b.png", "[table][td]x", "[url]https://bare.link[/url]", ""} {
		_ = Plain(Parse(src)) // must not panic
	}
	if got := Plain(Parse("[url]https://bare.link[/url]")); !strings.Contains(got, "https://bare.link") {
		t.Fatalf("bare url: %q", got)
	}
	if n := TextLen(Parse("[h1]Title[/h1][img]x[/img] body text")); n != len("Title")+len("body text") {
		t.Fatalf("TextLen %d", n)
	}
}

// Every real description parses (RIMARCHO_WORKSHOP_CACHE: the JSON written by
// spikes/infodesc).
func TestRealDescriptions(t *testing.T) {
	path := os.Getenv("RIMARCHO_WORKSHOP_CACHE")
	if path == "" {
		t.Skip("set RIMARCHO_WORKSHOP_CACHE")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var ds []struct{ Description string }
	json.Unmarshal(b, &ds)
	t0 := time.Now()
	blocks, imgs := 0, 0
	for _, d := range ds {
		bs := Parse(d.Description)
		blocks += len(bs)
		for _, x := range bs {
			if x.Kind == Image {
				imgs++
			}
		}
		_ = Plain(bs)
	}
	t.Logf("%d descriptions, %d blocks, %d images, %v", len(ds), blocks, imgs, time.Since(t0))
}
