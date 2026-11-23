//go:build windows || cgo

package gui

import (
	"io/fs"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// 16 §3.5, §3 rule 5: a layer's backdrop dims everything on the page, Archo and the sidebar
// included. Every page (Archo's spots among them) is drawn inside #body, a stacking context
// whose z-index is below #popup's, and #popup is a sibling of #app, outside it; so no
// z-index inside a page can lift anything above the backdrop. The command panel's backdrop
// covers the whole window, the bottom line too.
func TestBackdropCoversThePage(t *testing.T) {
	t.Parallel()
	css, err := fs.ReadFile(assets, "assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	html, err := fs.ReadFile(assets, "assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	rule := func(sel string) string {
		m := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(sel) + ` \{([^}]*)\}`).FindSubmatch(css)
		if m == nil {
			t.Fatalf("no %s rule", sel)
		}
		return string(m[1])
	}
	// z: the z-index of the first top-level rule whose selector list names sel alone
	z := func(sel string) int {
		for _, m := range regexp.MustCompile(`(?m)^([^{}@]+)\{([^}]*)\}`).FindAllSubmatch(css, -1) {
			named := false
			for _, part := range strings.Split(string(m[1]), ",") {
				named = named || strings.TrimSpace(part) == sel
			}
			if zi := regexp.MustCompile(`z-index:\s*(\d+)`).FindSubmatch(m[2]); named && zi != nil {
				n, _ := strconv.Atoi(string(zi[1]))
				return n
			}
		}
		t.Fatalf("%s has no z-index", sel)
		return 0
	}
	if b := rule("#body"); !strings.Contains(b, "position: relative") {
		t.Errorf("#body must be positioned to be a stacking context: %s", b)
	}
	if body, pop := z("#body"), z("#popup"); body >= pop {
		t.Errorf("#body z-index %d is not below #popup's %d: a page could draw over the backdrop", body, pop)
	}
	if d := rule("#popup.command-dock"); regexp.MustCompile(`\b(bottom|top|left|right|inset)\s*:`).MatchString(d) {
		t.Errorf("the command panel's backdrop must cover the whole window: %s", d)
	}
	h := string(html)
	app, body, popup := strings.Index(h, `id="app"`), strings.Index(h, `id="body"`), strings.Index(h, `id="popup"`)
	if app < 0 || body < 0 || popup < 0 {
		t.Fatal("index.html lacks #app, #body or #popup")
	}
	// #popup comes after #app's last child, the bottom line, so it is not inside #body
	if foot := strings.Index(h, `</footer>`); foot < 0 || popup < foot || strings.Index(h, `id="screen"`) < body {
		t.Error("#popup must follow #app's content, outside #body; the pages must be inside #body")
	}
	for _, sel := range []string{".archo-slot", ".archo-spot", ".archo-anchor"} {
		for _, m := range regexp.MustCompile(`(?m)^[^{}]*`+regexp.QuoteMeta(sel)+`[^{}]*\{[^}]*position:\s*fixed`).FindAll(css, -1) {
			t.Errorf("Archo must stay in the page's flow, not fixed: %s", m)
		}
	}
}
