package app

import (
	"net/url"
	"strings"
	"testing"
)

func TestChooseAndImageURL(t *testing.T) {
	long := strings.Repeat("A framework for new races. ", 20)
	if got := choose(long, "[img]https://x/banner.png[/img]\n"+long+"[h1]More[/h1]"); got != "workshop" {
		t.Errorf("same start after a banner: %s", got)
	}
	if got := choose(long, "Something else entirely. "+long); got != "both" {
		t.Errorf("different: %s", got)
	}
	for in, want := range map[string]string{
		"https://imgur.com/eQZ72Ib.png": "https://i.imgur.com/eQZ72Ib.png",
		"https://imgur.com/a/album":     "https://imgur.com/a/album",
		"https://i.imgur.com/x.gif":     "https://i.imgur.com/x.gif",
	} {
		u, _ := url.Parse(in)
		if got := imageURL(u); got != want {
			t.Errorf("imageURL(%s) = %s", in, got)
		}
	}
}
