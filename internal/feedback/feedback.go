// Package feedback writes a GitHub issue from RimArcho (06 §2.14 反馈): the body, and a
// prefilled issues/new link the system browser opens. Nothing is sent from RimArcho; the
// player submits the issue on GitHub. No token is needed.
package feedback

import (
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The project's public pages, shown on the settings page (06 §2.14).
const (
	Repo = "https://github.com/ToumanLin/RimArcho"
	// Discord is the community server's invite.
	Discord = "https://discord.gg/HF2GVyurQs"
)

// MaxURL is the longest link RimArcho opens. Windows hands a link to the browser through
// ShellExecute, which takes at most 2083 characters (INTERNET_MAX_URL_LENGTH; GitHub itself
// accepts about 8 KB); a longer body goes through the clipboard instead (Compose).
const MaxURL = 2000

// Section is one attachment: a heading and its lines, shown folded on GitHub.
type Section struct {
	Heading string   `json:"heading"`
	Lines   []string `json:"lines"`
}

// Draft is what the player wrote and what RimArcho adds.
type Draft struct {
	Title string `json:"title"`
	Text  string `json:"text"`
	// Basic is always attached: RimArcho, the game and the system, one per line.
	Basic    []string  `json:"basic"`
	Sections []Section `json:"sections"`
	// Paste is said in the link's body when the full text did not fit and was copied
	// instead ("paste the copied text here"), in the player's language.
	Paste string `json:"paste"`
}

// Result is the issue: the full body, and the link to open.
type Result struct {
	Body string `json:"body"`
	URL  string `json:"url"`
	// Full: the link carries the whole body. Otherwise it carries the title, what fits of
	// the player's text and the basic lines, and the body is to be copied and pasted.
	Full bool `json:"full"`
}

// Body is the issue text: the player's words, a rule, the basic lines, then each section
// folded in <details> with its lines as a code block.
func (d Draft) Body() string {
	var b strings.Builder
	if t := strings.TrimSpace(d.Text); t != "" {
		b.WriteString(t + "\n\n")
	}
	b.WriteString("---\n")
	b.WriteString(strings.Join(d.Basic, "\n") + "\n")
	for _, s := range d.Sections {
		b.WriteString("\n<details><summary>" + escapeHTML(s.Heading) + "</summary>\n\n```\n")
		for _, l := range s.Lines {
			b.WriteString(strings.ReplaceAll(l, "```", "'''") + "\n")
		}
		b.WriteString("```\n\n</details>\n")
	}
	return b.String()
}

// Compose builds the body and the link: the whole body when the link stays within MaxURL;
// else the title, the player's text and the basic lines with Paste; else (the text alone is
// too long) only Paste and the basic lines. The title is shortened as a last resort.
func Compose(d Draft) Result {
	body := d.Body()
	if u := IssueURL(d.Title, body); len(u) <= MaxURL {
		return Result{Body: body, URL: u, Full: true}
	}
	basic := "---\n" + strings.Join(d.Basic, "\n") + "\n"
	paste := strings.TrimSpace(d.Paste) + "\n\n"
	if t := strings.TrimSpace(d.Text); t != "" {
		if u := IssueURL(d.Title, paste+t+"\n\n"+basic); len(u) <= MaxURL {
			return Result{Body: body, URL: u}
		}
	}
	title := d.Title
	for {
		u := IssueURL(title, paste+basic)
		if len(u) <= MaxURL || title == "" {
			return Result{Body: body, URL: u}
		}
		_, n := utf8.DecodeLastRuneInString(title)
		title = title[:len(title)-n]
	}
}

// IssueURL is the repository's new-issue link with the title and body filled in.
func IssueURL(title, body string) string {
	q := "title=" + escape(strings.TrimSpace(title)) + "&body=" + escape(body)
	return Repo + "/issues/new?" + q
}

// escape is query escaping with %20 for spaces (GitHub reads "+" as a space too, but a
// literal "+" in the text must survive).
func escape(s string) string { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }

func escapeHTML(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// Scrub replaces the user's home folder in attached text by %USERPROFILE%, so a path does
// not give away the Windows user name. home is matched without regard to case.
func Scrub(s, home string) string {
	home = strings.TrimRight(home, `\/`)
	if len(home) < 3 {
		return s
	}
	return regexp.MustCompile("(?i)"+regexp.QuoteMeta(home)).ReplaceAllLiteralString(s, "%USERPROFILE%")
}
