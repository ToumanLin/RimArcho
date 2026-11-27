package view

import (
	"encoding/json"
	"strings"
	"testing"

	rimarcho "github.com/ToumanLin/RimArcho"
)

const sampleChangelog = "# Changelog\r\n\r\nIntro.\r\n\r\n## Unreleased\r\n\r\n- new\r\n\r\n## 1.1.4 — 2026-11-25\r\n\r\n- four\r\n\r\n## 1.1.3 — 2026-11-24\r\n\r\n- three\r\n\r\n## 1.1.2 — 2026-11-24\r\n- two\r\n## 1.1.1 — 2026-11-23\r\n- one\r\n## 1.1.0 — 2026-11-23\r\n- zero\r\n## 0.9.0 — plans and launching (development milestone, in 1.0.0)\r\n\r\n### Breaking\r\n- old\r\n"

// 16 §2.14 About › Changelog: one entry per "## " heading, newest first, with the rest of the
// heading as its note; the running version is open (Unreleased in a development build) and
// carries its text, the others are read when opened; the first few are shown.
func TestChangelogView(t *testing.T) {
	parts := parseChangelog(sampleChangelog)
	var keys []string
	for _, p := range parts {
		keys = append(keys, p.key)
	}
	if got := strings.Join(keys, " "); got != "Unreleased 1.1.4 1.1.3 1.1.2 1.1.1 1.1.0 0.9.0" {
		t.Fatalf("versions %s", got)
	}
	if parts[6].note != "plans and launching (development milestone, in 1.0.0)" || parts[6].text != "### Breaking\n- old" {
		t.Errorf("milestone %+v", parts[6])
	}
	dev := changelogView(parts, "dev")
	if !dev.Versions[0].Current || dev.Versions[0].Text != "- new" || dev.Versions[1].Text != "" || dev.Shown != ChangelogShown {
		t.Errorf("dev %+v", dev)
	}
	if dev.Versions[1].Note != "2026-11-25" || !strings.HasPrefix(dev.Source, "https://github.com/") {
		t.Errorf("dev %+v", dev)
	}
	rel := changelogView(parts, "1.1.3")
	if rel.Versions[0].Current || !rel.Versions[2].Current || rel.Versions[2].Text != "- three" {
		t.Errorf("release %+v", rel.Versions)
	}
	// a running version further down is still shown, and an unknown one opens the newest
	if old := changelogView(parts, "0.9.0"); old.Shown != 7 || !old.Versions[6].Current {
		t.Errorf("old %+v", old)
	}
	if v := changelogView(parts, "9.9.9"); !v.Versions[0].Current {
		t.Errorf("unknown %+v", v.Versions[0])
	}
	// an empty Unreleased right after a release is left out
	if p := parseChangelog("## Unreleased\n\n## 1.0.0 — x\n- a\n"); len(p) != 1 || p[0].key != "1.0.0" {
		t.Errorf("empty unreleased %+v", p)
	}
}

// The built-in CHANGELOG.md is the repository's, and Settings carries only the open version's
// text (16 §10: the rest is read when a fold opens).
func TestChangelogEmbedded(t *testing.T) {
	g := ChangelogGroup()
	c := g.Changelog
	if c == nil || len(c.Versions) < 5 || c.Versions[0].Key != "Unreleased" || !c.Versions[0].Current {
		t.Fatalf("changelog %+v", c)
	}
	b, _ := json.Marshal(g)
	if len(b) > len(rimarcho.Changelog)/2 {
		t.Errorf("the section carries %d bytes of a %d-byte changelog", len(b), len(rimarcho.Changelog))
	}
	if ChangelogText(c.Versions[1].Key) == "" || ChangelogText("no such version") != "" {
		t.Errorf("text of %s", c.Versions[1].Key)
	}
}

// 16 §2.14 About › Author: the links open web pages; no email address is shown.
func TestAuthorCard(t *testing.T) {
	a := AuthorGroup().Author
	if a == nil || a.Name == "" || a.Seeking == "" || len(a.Links) != 3 {
		t.Fatalf("author %+v", a)
	}
	b, _ := json.Marshal(a)
	if strings.Contains(string(b), "@") || strings.Contains(string(b), "mailto") {
		t.Errorf("an email address in %s", b)
	}
	for _, l := range a.Links {
		if !strings.HasPrefix(l.URL, "https://") {
			t.Errorf("link %+v", l)
		}
	}
}
