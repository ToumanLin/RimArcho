package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/feedback"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Feedback in the terminal (16 §2.14): the basics only until m / p / l tick a part; enter opens
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
	if !strings.Contains(text, "[x] m enabled mods") || !strings.Contains(text, "\n  2. Alpha  tester.a  1001\n") {
		t.Fatalf("m ticks the mods:\n%s", text)
	}
	// the preview is plain text; the Markdown goes only into the link
	for _, mark := range []string{"<details>", "<summary>", "```", "---"} {
		if strings.Contains(text, mark) {
			t.Fatalf("the preview shows %q:\n%s", mark, text)
		}
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

// s in Feedback (16 §2.14): only with a relay; asks for the title, what happened and the
// contact on the input line, then sends in the background and says the issue's number.
func TestFeedbackSendTUI(t *testing.T) {
	view.SetLang("en")
	e, _ := fixture(t)
	var got feedback.Report
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		fmt.Fprint(w, `{"number":7,"url":"https://github.com/ToumanLin/RimArcho/issues/7"}`)
	}))
	defer srv.Close()

	old := feedback.Relay
	defer func() { feedback.Relay = old }()
	feedback.Relay = ""
	t.Setenv("RIMARCHO_FEEDBACK_RELAY", "")
	m := New(Options{App: e.App}).(*model)
	m.Init()
	m.panel = nil
	m.palRun("feedback")(m)
	if strings.Contains(strings.Join(m.panel.lines, "\n"), "s: send") || m.panel.keys["s"] != nil {
		t.Fatal("no relay, no s")
	}

	t.Setenv("RIMARCHO_FEEDBACK_RELAY", srv.URL+"/report")
	m.panel = nil
	m.palRun("feedback")(m)
	if !strings.Contains(strings.Join(m.panel.lines, "\n"), "s: answer the questions here") {
		t.Fatalf("s offered:\n%s", strings.Join(m.panel.lines, "\n"))
	}
	m.panelKey("5") // other / help: one question
	m.panelKey("s")
	for _, typed := range []string{"on load", "Crash"} {
		if m.prompt == nil {
			t.Fatal("asks on the input line")
		}
		m.prompt.submit(m, typed)
	}
	cmd := m.prompt.submit(m, "me#1")
	if cmd == nil {
		t.Fatal("sends in the background")
	}
	m.update(cmd())
	if got.Title != "Crash" || got.Contact != "me#1" || got.Kind != "other" || !strings.HasPrefix(got.Body, "on load\n\n---\nreport: other / help\n") || !strings.Contains(m.status, "#7") {
		t.Fatalf("sent %+v; status %q", got, m.status)
	}
}

// Shared settings order and full links survive the terminal's text presentation.
func TestSettingsAboutFirst(t *testing.T) {
	e, _ := fixture(t)
	tool := filepath.Join(e.Root, "tools", deps.SteamCMDExecutable())
	e.WriteFile(tool, "synthetic tool")
	e.MustRun("config", "set", "steamcmd.path", tool)
	m := New(Options{App: e.App}).(*model)
	m.Init()
	defer view.SetLang("en")
	for _, lang := range []string{"en", "zh"} {
		view.SetLang(lang)
		m.palRun("settings")(m)
		if m.panel == nil {
			t.Fatal("no settings panel")
		}
		text := strings.Join(m.panel.lines, "\n")
		if !strings.HasPrefix(text, view.T("about")+"\n") || strings.Index(text, view.T("Game Config")) < strings.Index(text, view.T("about")) {
			t.Fatal(text)
		}
		for _, value := range []string{"config set updates.auto {}", "config set updates.interval {}", `config set steamcmd.path ""`} {
			if strings.Count(text, value) != 1 {
				t.Fatalf("want one %q in settings: %s", value, text)
			}
		}
		// the community links moved to the feedback panel (16 §2.14); the QQ group only in Chinese
		if strings.Contains(text, "https://discord.gg/HF2GVyurQs") {
			t.Fatal("community still in settings", text)
		}
		m.palRun("feedback")(m)
		fb := strings.Join(m.panel.lines, "\n")
		for _, value := range []string{"https://discord.gg/HF2GVyurQs", "https://github.com/ToumanLin/RimArcho"} {
			if strings.Count(fb, value) != 1 {
				t.Fatalf("want one %q in feedback: %s", value, fb)
			}
		}
		if strings.Contains(fb, "1042873484") != (lang == "zh") {
			t.Fatalf("QQ group in %s: %s", lang, fb)
		}
		if !strings.Contains(text, "env set") {
			t.Fatal("lost text path commands", text)
		}
	}
}

// W on a Can it run item (16 §2.4): the feedback panel on wrong diagnosis with the item's ID;
// an item that rests on the replacement library also fills outdated data (4) with its Mod.
func TestFeedbackFromNeedW(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "fan.port", Name: "Unofficial port"})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "fan.port")
	e.WriteFile(filepath.Join(e.Home, "cache", "replacements.json.gz"), `{"rules":[{"oldPackageId":"fan.port","newPackageId":"author.official","newName":"Official","newWorkshopId":"2002","newVersions":["1.6"]}]}`)
	e.AddEnv(false)
	e.MustRun("scan")
	m := direct(t, e, 140, 40)
	m.tab = tabProblems
	m.loadProblems()
	m.panel = nil
	at := -1
	for i, it := range m.prob.items {
		if strings.Contains(it.text, "maintained replacement") {
			at = i
		}
	}
	if at < 0 {
		t.Fatalf("no replacement item in %+v", m.prob.items)
	}
	m.prob.cursor = at
	if _, ok := m.needItemKey("W"); !ok || m.panel == nil || m.panel.title != "feedback" {
		t.Fatal("W did not open the feedback panel")
	}
	text := strings.Join(m.panel.lines, "\n")
	if !strings.Contains(text, "> 3 wrong diagnosis") || !strings.Contains(text, "the diagnosis need:") || !strings.Contains(text, "kind: mod.replaced") {
		t.Fatalf("W: wrong diagnosis filled in:\n%s", text)
	}
	m.panelKey("4")
	text = strings.Join(m.panel.lines, "\n")
	if !strings.Contains(text, "> 4 outdated data") || !strings.Contains(text, "the Mod fan.port") || !strings.Contains(text, "the diagnosis need:") {
		t.Fatalf("4: outdated data filled in:\n%s", text)
	}
	// the command panel's feedback starts empty again
	m.palRun("feedback")(m)
	if text = strings.Join(m.panel.lines, "\n"); strings.Contains(text, "fan.port") {
		t.Fatalf("a fresh panel kept the item:\n%s", text)
	}
}
