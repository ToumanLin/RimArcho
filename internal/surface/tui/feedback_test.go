package tui

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/feedback"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 反馈 in the terminal (06 §2.14): the basics only until m / p / l tick a part; enter opens
// the prefilled link, copying the text first when it is too long for the link.
func TestFeedbackPanel(t *testing.T) {
	view.SetLang("en")
	e, _ := fixture(t)
	m := New(Options{App: e.App}).(*model)
	m.Init()
	m.panel = nil
	var opened, copied string
	oldOpen, oldCopy := openURL, copyText
	openURL = func(u string) error { opened = u; return nil }
	copyText = func(s string) error { copied = s; return nil }
	t.Cleanup(func() { openURL, copyText = oldOpen, oldCopy })

	m.palRun("feedback")(m)
	if m.panel == nil || m.panel.title != "feedback" {
		t.Fatal("no feedback panel")
	}
	text := strings.Join(m.panel.lines, "\n")
	if !strings.Contains(text, "RimWorld 1.6.4633 rev1261") || strings.Contains(text, "tester.a") || !strings.Contains(text, "[ ] m enabled mods") {
		t.Fatalf("basics only:\n%s", text)
	}
	m.panelKey("m")
	text = strings.Join(m.panel.lines, "\n")
	if !strings.Contains(text, "[x] m enabled mods") || !strings.Contains(text, "tester.a  1001") {
		t.Fatalf("m ticks the mods:\n%s", text)
	}
	m.panelKey("enter")
	u, err := url.Parse(opened)
	if err != nil || !strings.HasPrefix(opened, feedback.Repo+"/issues/new?") || !strings.Contains(u.Query().Get("body"), "tester.a") || copied != "" {
		t.Fatalf("enter opens the whole text: %q (copied %q)", opened, copied)
	}

	// A long list: the link carries the basics and the text goes through the clipboard.
	big := testkit.New(t)
	active := []string{"ludeon.rimworld"}
	for i := range 40 {
		id := fmt.Sprintf("tester.long%02d", i)
		big.AddWorkshop(fmt.Sprint(3000+i), testkit.Sample(id, "A mod with a fairly long name "+id))
		active = append(active, id)
	}
	big.WriteModsConfig("1.6.4633 rev1261", active...)
	big.AddEnv(false)
	big.MustRun("scan")
	m = New(Options{App: big.App}).(*model)
	m.Init()
	m.panel, opened = nil, ""
	m.palRun("feedback")(m)
	m.panelKey("m")
	m.panelKey("enter")
	if !strings.Contains(copied, "tester.long39") || opened == "" || len(opened) > feedback.MaxURL || strings.Contains(opened, "long39") {
		t.Fatalf("too long: opened %d chars, copied %d", len(opened), len(copied))
	}
}
