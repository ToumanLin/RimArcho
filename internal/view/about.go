package view

import (
	"strings"
	"sync"

	rimarcho "github.com/ToumanLin/RimArcho"
	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/feedback"
)

// Settings › About's Author and Changelog sections (16 §2.14).

// Section keys of About after its first, RimArcho (GroupAbout).
const (
	GroupAuthor    = "author"
	GroupChangelog = "changelog"
)

// AuthorCard is who made RimArcho: the name with the GitHub handle after it, a line on their
// study, one on the work they are looking for, and links that open in the browser. No email.
type AuthorCard struct {
	Name    string          `json:"name"`
	Handle  string          `json:"handle"`
	Lines   []string        `json:"lines"`
	Seeking string          `json:"seeking"`
	Links   []CommunityLink `json:"links"`
}

// AuthorGroup is About's Author section.
func AuthorGroup() SettingGroup {
	return SettingGroup{Key: GroupAuthor, Title: T("author"), Attention: AttentionNone, Author: &AuthorCard{
		Name:    "Zixian (Jason) Zhang",
		Handle:  "ToumanLin",
		Lines:   []string{T("Aerospace Engineering, UCLA, class of 2027")},
		Seeking: T("Looking for full-time aerospace roles from spring 2027"),
		Links: []CommunityLink{
			{Label: T("Website"), URL: "https://www.zixianz.com"},
			{Label: "LinkedIn", URL: "https://www.linkedin.com/in/zixianz"},
			{Label: "GitHub", URL: "https://github.com/ToumanLin"},
		},
	}}
}

// Changelog is CHANGELOG.md's versions, newest first. Only the current one carries its text;
// the GUI asks for another's (API.ChangelogText) when it is opened, so the page stays small.
// Shown is how many rows are visible before "older versions".
type Changelog struct {
	Versions []ChangelogVersion `json:"versions"`
	Shown    int                `json:"shown"`
	// Source is where the text's relative links point: the repository on GitHub.
	Source string `json:"source"`
}

// ChangelogVersion is one "## " heading of CHANGELOG.md: Key is its version ("1.1.4") or
// "Unreleased", Title what the interfaces show, Note the rest of the heading (its date).
type ChangelogVersion struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Note    string `json:"note,omitempty"`
	Current bool   `json:"current,omitempty"`
	Text    string `json:"text,omitempty"`
}

// ChangelogShown is how many versions the Changelog shows before "older versions" (16 §10:
// the rest are drawn only when asked for).
const ChangelogShown = 5

type changelogPart struct{ key, note, text string }

var changelogParts = sync.OnceValue(func() []changelogPart { return parseChangelog(rimarcho.Changelog) })

// parseChangelog splits CHANGELOG.md at its "## " headings; a version with no text (an empty
// Unreleased right after a release) is left out.
func parseChangelog(md string) []changelogPart {
	var out []changelogPart
	var cur *changelogPart
	var body []string
	flush := func() {
		if cur != nil {
			cur.text = strings.TrimSpace(strings.Join(body, "\n"))
			if cur.text != "" {
				out = append(out, *cur)
			}
		}
		body = nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		if h, ok := strings.CutPrefix(line, "## "); ok {
			flush()
			key, note, _ := strings.Cut(strings.TrimSpace(h), " — ")
			cur = &changelogPart{key: strings.TrimSpace(key), note: strings.TrimSpace(note)}
			continue
		}
		if cur != nil {
			body = append(body, line)
		}
	}
	flush()
	return out
}

// changelogCurrent is the index of the running version: Unreleased in a development build,
// else the release's own heading, else the newest.
func changelogCurrent(parts []changelogPart, version string) int {
	version = strings.TrimPrefix(version, "v")
	for i, p := range parts {
		if p.key == version || version == "dev" && p.key == "Unreleased" {
			return i
		}
	}
	return 0
}

// ChangelogGroup is About's Changelog section.
func ChangelogGroup() SettingGroup {
	return SettingGroup{Key: GroupChangelog, Title: T("changelog"), Attention: AttentionNone, Changelog: changelogView(changelogParts(), buildinfo.Version)}
}

func changelogView(parts []changelogPart, version string) *Changelog {
	c := &Changelog{Shown: min(ChangelogShown, len(parts)), Source: feedback.Repo + "/blob/master/"}
	if len(parts) == 0 {
		return c
	}
	at := changelogCurrent(parts, version)
	c.Shown = max(c.Shown, at+1)
	for i, p := range parts {
		v := ChangelogVersion{Key: p.key, Title: p.key, Note: p.note, Current: i == at}
		if p.key == "Unreleased" {
			v.Title = T("unreleased")
		}
		if v.Current {
			v.Text = p.text
		}
		c.Versions = append(c.Versions, v)
	}
	return c
}

// ChangelogText is one version's Markdown from CHANGELOG.md, or "".
func ChangelogText(key string) string {
	for _, p := range changelogParts() {
		if p.key == key {
			return p.text
		}
	}
	return ""
}
