package view

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/feedback"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// Settings > Feedback (06 §2.14): a GitHub issue written in RimArcho, sent through the feedback
// relay (no GitHub account needed) or opened in the browser. RimArcho, the game and the
// system always go with it; the enabled mods, the paths and the game log's errors only when
// the player ticks them, and the page shows the whole text first.

// FeedbackOptions are the attachments the player ticked.
type FeedbackOptions struct {
	Mods  bool `json:"mods"`
	Paths bool `json:"paths"`
	Log   bool `json:"log"`
}

// FeedbackPart is one optional attachment: its name, what it holds, and whether there is any.
type FeedbackPart struct {
	Key   string `json:"key"` // mods, paths, log
	Label string `json:"label"`
	Note  string `json:"note"`
	None  bool   `json:"none,omitempty"` // nothing to attach (no game, no log yet)
}

// FeedbackAttach is what goes with the report under the given options.
type FeedbackAttach struct {
	Basic    []string           `json:"basic"`
	Parts    []FeedbackPart     `json:"parts"`
	Sections []feedback.Section `json:"sections"`
	// Pending: the game log is still being analysed; ask again.
	Pending bool `json:"pending,omitempty"`
	// Send: RimArcho has a feedback relay, so send is offered (feedback.RelayURL).
	Send bool `json:"send,omitempty"`
}

// FeedbackAttachments reads the attachments: always the basic lines and the parts' notes,
// and the sections the options ask for. Paths in sections show the home folder as
// %USERPROFILE%.
func FeedbackAttachments(a *app.App, st *app.LoadoutState, o FeedbackOptions) *FeedbackAttach {
	out := &FeedbackAttach{Basic: []string{"RimArcho " + buildinfo.String()}, Send: feedback.RelayURL() != ""}
	if st != nil && st.Ix != nil && st.Ix.Env != nil && st.Ix.Env.GameVersion != "" {
		out.Basic = append(out.Basic, "RimWorld "+st.Ix.Env.GameVersion)
	}
	out.Basic = append(out.Basic, feedback.System())
	home, _ := os.UserHomeDir()
	scrub := func(lines []string) []string {
		for i := range lines {
			lines[i] = feedback.Scrub(lines[i], home)
		}
		return lines
	}
	loaded := st != nil && st.Ix != nil && st.Ix.Env != nil

	mods := FeedbackPart{Key: "mods", Label: T("enabled mods"), Note: T("no game yet"), None: true}
	if loaded && st.Plan != nil {
		mods.Note, mods.None = Tf("list “%s”, %d mods", st.Plan.Name, len(st.List)), false
		if o.Mods {
			out.Sections = append(out.Sections, feedback.Section{Key: "mods", Heading: Tf("enabled mods (list “%s”, %d)", st.Plan.Name, len(st.List)), Lines: feedbackMods(st)})
		}
	}
	paths := FeedbackPart{Key: "paths", Label: T("paths"), Note: T("the game, its data, the mod folders and RimArcho's data; your user name becomes %USERPROFILE%")}
	if o.Paths {
		out.Sections = append(out.Sections, feedback.Section{Key: "paths", Heading: T("paths"), Lines: scrub(feedbackPaths(a, st))})
	}
	logPart := FeedbackPart{Key: "log", Label: T("game log"), Note: T("no game log yet"), None: true}
	if loaded {
		if files := LogFiles(a, st); len(files) > 0 {
			logPart.Note, logPart.None = Tf("the errors in Player.log of %s", files[0].When), false
			if o.Log {
				lines, pending := feedbackLog(a, st)
				out.Pending = pending
				if !pending {
					out.Sections = append(out.Sections, feedback.Section{Key: "log", Heading: Tf("game log errors (Player.log of %s)", files[0].When), Lines: scrub(lines)})
				}
			}
		}
	}
	out.Parts = []FeedbackPart{mods, paths, logPart}
	return out
}

// feedbackMods is the current list in load order: name, packageId, Workshop ID.
func feedbackMods(st *app.LoadoutState) []string {
	var out []string
	for i, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; e.Instance != "" && m != nil {
			out = append(out, strings.TrimSpace(fmt.Sprintf("%d. %s  %s  %s", i+1, m.DisplayName(), m.PackageID, m.WorkshopID())))
			continue
		}
		name := e.Name
		if name == "" {
			name = e.Missing
		}
		out = append(out, fmt.Sprintf("%d. %s  (%s)", i+1, name, T("not installed")))
	}
	return out
}

// feedbackPaths: the game, its data folder, every mod folder and RimArcho's data.
func feedbackPaths(a *app.App, st *app.LoadoutState) []string {
	var out []string
	if st != nil && st.Ix != nil && st.Ix.Env != nil {
		e := st.Ix.Env
		out = append(out, "game: "+e.GameRoot, "game data: "+e.UserDataRoot)
		for _, s := range st.Ix.Sources {
			if s.Status != source.StatusRetired {
				out = append(out, string(s.Kind)+": "+s.Root)
			}
		}
	}
	return append(out, "RimArcho: "+a.Home)
}

// feedbackLog is the newest Player.log's error groups as the Game log page lists them, at most
// 40, each one line.
func feedbackLog(a *app.App, st *app.LoadoutState) ([]string, bool) {
	p, err := BuildLogPage(a, st, "", nil)
	if err != nil {
		return []string{err.Error()}, false
	}
	if p.Pending != "" {
		return nil, true
	}
	var out []string
	for _, r := range p.Rows {
		if r.Info {
			continue
		}
		if len(out) == 40 {
			out = append(out, Tf("… %d more error groups", len(p.Rows)-40))
			break
		}
		line := fmt.Sprintf("%s ×%d  %s", r.ID, r.Count, truncateRunes(r.Text, 200))
		if r.Mods != "" {
			line += "  [" + r.Mods + "]"
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		out = append(out, T("no errors"))
	}
	return out, false
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ComposeFeedback is the issue for what the player wrote and the attachments read last.
func ComposeFeedback(title, text string, at *FeedbackAttach) feedback.Result {
	d := feedback.Draft{Title: title, Text: text, Paste: T("The full text was too long for the link; RimArcho copied it. Paste it here.")}
	if at != nil {
		d.Basic, d.Sections = at.Basic, at.Sections
	}
	return feedback.Compose(d)
}

// FeedbackPreview is the report as the player reads it (the TUI's panel; the window builds
// the same from FeedbackAttach): what happened, then the basic lines and each attachment
// under its heading, indented. No Markdown or HTML: that is only in the body sent.
func FeedbackPreview(text string, at *FeedbackAttach) string {
	var parts []string
	if t := strings.TrimSpace(text); t != "" {
		parts = append(parts, t)
	}
	part := func(heading string, lines []string) {
		p := heading
		for _, l := range lines {
			p += "\n  " + l
		}
		parts = append(parts, p)
	}
	if at != nil {
		part(T("basic information"), at.Basic)
		for _, s := range at.Sections {
			part(s.Heading, s.Lines)
		}
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// SendFeedback files the report through the feedback relay: the title (or the first line of
// what happened), the whole body cut to the relay's limit, and the contact, which the relay
// keeps for the maintainer and never puts in the public issue.
func SendFeedback(ctx context.Context, title, text, contact string, at *FeedbackAttach) (feedback.Sent, error) {
	relay := feedback.RelayURL()
	if relay == "" {
		return feedback.Sent{}, errors.New(T("RimArcho has no feedback relay; open GitHub instead"))
	}
	title = strings.TrimSpace(title)
	if title == "" {
		first, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
		title = truncateRunes(strings.TrimSpace(first), 60)
	}
	if title == "" {
		return feedback.Sent{}, errors.New(T("write a title or what happened first"))
	}
	d := feedback.Draft{Title: title, Text: text, More: T("… %d more lines")}
	if at != nil {
		d.Basic, d.Sections = at.Basic, at.Sections
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	r := feedback.Report{Title: title, Body: d.SendBody(feedback.MaxBody), Contact: strings.TrimSpace(contact)}
	s, err := feedback.Send(ctx, relay, "RimArcho/"+buildinfo.Version, r)
	if err != nil {
		return s, feedbackSendError(err)
	}
	return s, nil
}

// feedbackSendError says a refusal in the player's words; the caps point to GitHub.
func feedbackSendError(err error) error {
	var re *feedback.RelayError
	if !errors.As(err, &re) {
		return errors.New(Tf("could not reach the feedback relay: %v", err))
	}
	switch re.Code {
	case "slow_down":
		return errors.New(T("one report a minute; send it again in a moment"))
	case "ip_daily":
		return errors.New(T("this computer has sent enough reports today; send it tomorrow or open GitHub"))
	case "daily":
		return errors.New(T("RimArcho has had too many reports today; send it tomorrow or open GitHub"))
	case "too_long":
		return errors.New(T("the report is too long; untick an attachment or shorten the contact"))
	}
	return errors.New(Tf("the feedback relay could not file it (%s); open GitHub instead", re.Code))
}

// CommunityLink is one of RimArcho's public pages (the settings page's head, 06 §2.14).
type CommunityLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// CommunityLinks: the GitHub repository and the Discord server.
func CommunityLinks() []CommunityLink {
	return []CommunityLink{{Label: "Discord", URL: feedback.Discord}, {Label: "GitHub", URL: feedback.Repo}}
}
