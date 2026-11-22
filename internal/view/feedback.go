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

// Settings > Feedback (16 §2.14): a GitHub issue written in RimArcho, sent through the feedback
// relay (no GitHub account needed) or opened in the browser. RimArcho, the game and the
// system always go with it; the enabled mods, the paths, the game log's errors and the recent
// operations (13 §1.3) only when the player ticks them, and the page shows the whole text first.

// FeedbackOptions are the attachments the player ticked, and what the report's template
// attaches by itself: the kind (its own basic lines), the diagnosis and the Mod it is about.
type FeedbackOptions struct {
	Mods     bool   `json:"mods"`
	Paths    bool   `json:"paths"`
	Log      bool   `json:"log"`
	Ops      bool   `json:"ops"`
	Problems bool   `json:"problems"`
	Kind     string `json:"kind,omitempty"`
	Finding  string `json:"finding,omitempty"`
	Mod      string `json:"mod,omitempty"`
}

// FeedbackPart is one optional attachment: its name, what it holds, and whether there is any.
type FeedbackPart struct {
	Key   string `json:"key"` // mods, paths, log, ops, problems
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
// the template's own sections (the diagnosis, the Mod) and the sections the options ask for.
// n is Can it run as last built, for the problems and the diagnosis (nil: not built yet).
// Paths in sections show the home folder as %USERPROFILE%.
func FeedbackAttachments(a *app.App, st *app.LoadoutState, o FeedbackOptions, n *Needs) *FeedbackAttach {
	out := &FeedbackAttach{Send: feedback.RelayURL() != ""}
	if o.Kind != "" {
		out.Basic = append(out.Basic, Tf("report: %s", T(feedback.TemplateOf(o.Kind).Label)))
	}
	out.Basic = append(out.Basic, "RimArcho "+buildinfo.String())
	if st != nil && st.Ix != nil && st.Ix.Env != nil && st.Ix.Env.GameVersion != "" {
		out.Basic = append(out.Basic, "RimWorld "+st.Ix.Env.GameVersion)
	}
	out.Basic = append(out.Basic, feedback.System())
	if o.Kind == feedback.KindData {
		out.Basic = append(out.Basic, feedbackDataLines(a)...)
	}
	home, _ := os.UserHomeDir()
	scrub := func(lines []string) []string {
		for i := range lines {
			lines[i] = feedback.Scrub(lines[i], home)
		}
		return lines
	}
	loaded := st != nil && st.Ix != nil && st.Ix.Env != nil
	if f := strings.TrimSpace(o.Finding); f != "" {
		out.Sections = append(out.Sections, feedback.Section{Key: "finding", Heading: Tf("the diagnosis %s", f), Lines: scrub(feedbackFinding(n, f))})
	}
	if m := strings.TrimSpace(o.Mod); m != "" {
		out.Sections = append(out.Sections, feedback.Section{Key: "mod", Heading: Tf("the Mod %s", m), Lines: scrub(feedbackMod(a, st, m))})
	}

	mods := FeedbackPart{Key: "mods", Label: T("enabled mods"), Note: T("no Game Config yet"), None: true}
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
	ops := FeedbackPart{Key: "ops", Label: T("recent operations"), Note: T("nothing recorded yet"), None: true}
	if n := min(a.Ops.Count(), FeedbackOps); n > 0 {
		ops.Note, ops.None = Tf("the newest %d of what RimArcho did to your files", n), false
		if o.Ops {
			out.Sections = append(out.Sections, feedback.Section{Key: "ops", Heading: Tf("recent operations (the newest %d)", n), Lines: scrub(OpsLines(a, FeedbackOps))})
		}
	}
	probs := FeedbackPart{Key: "problems", Label: T("problems"), Note: T("open Can it run first"), None: true}
	if n != nil {
		lines := feedbackProblems(n)
		probs.Note, probs.None = Tf("Can it run: %d items", len(lines)), len(lines) == 0
		if len(lines) == 0 {
			probs.Note = T("Can it run shows nothing")
		}
		if o.Problems && len(lines) > 0 {
			out.Sections = append(out.Sections, feedback.Section{Key: "problems", Heading: Tf("Can it run (%d)", len(lines)), Lines: scrub(lines)})
		}
	}
	out.Parts = []FeedbackPart{mods, paths, logPart, ops, probs}
	return out
}

// feedbackDataLines are the versions of the data a report about outdated data is about: the
// shared-log data in use and the replacement library.
func feedbackDataLines(a *app.App) []string {
	out := []string{corpusChip(a).Tip}
	if di := a.DataStatus()["replacements"]; di != nil {
		out = append(out, Tf("replacement library: %d entries, %s", di.Entries, ShortDate(di.Fetched)))
	} else {
		out = append(out, T("replacement library: not downloaded"))
	}
	return out
}

// feedbackProblems are Can it run's items, one line each: the ID and the sentence.
func feedbackProblems(n *Needs) []string {
	var out []string
	for _, it := range n.Items {
		if !it.Counted {
			continue
		}
		out = append(out, it.ID+"  "+truncateRunes(it.Text, 200))
	}
	return out
}

// feedbackFinding is the chosen diagnosis as Can it run shows it; an ID not there (one
// RimArcho missed, or gone since) is said as it is.
func feedbackFinding(n *Needs, id string) []string {
	if n != nil {
		for _, it := range n.Items {
			if it.ID != id {
				continue
			}
			out := []string{"id: " + it.ID}
			if it.Kind != "" {
				out = append(out, "kind: "+it.Kind)
			}
			if it.Title != "" {
				out = append(out, it.Title)
			}
			out = append(out, it.Text)
			if it.Why != "" && it.Why != it.Text {
				out = append(out, it.Why)
			}
			if len(it.Mods) > 0 {
				out = append(out, "mods: "+strings.Join(it.Mods, ", "))
			}
			return out
		}
	}
	return []string{T("not on Can it run now")}
}

// feedbackMod is the installed Mod a report about outdated data names, by packageId or
// Workshop ID: its name, ids, where it comes from and its folder.
func feedbackMod(a *app.App, st *app.LoadoutState, id string) []string {
	if st != nil && st.Ix != nil {
		for _, m := range st.Ix.Mods {
			if !strings.EqualFold(m.PackageID, id) && m.WorkshopID() != id {
				continue
			}
			out := []string{m.DisplayName(), "packageId: " + m.PackageID}
			if w := m.WorkshopID(); w != "" {
				out = append(out, "Workshop: "+w)
			}
			out = append(out, "source: "+string(m.Kind))
			if r := a.ReplacementFor(m.WorkshopID(), m.PackageID); r != nil {
				out = append(out, strings.TrimSpace(fmt.Sprintf("replacement library: %s  %s  %s", r.NewName, r.NewPackageID, r.NewWorkshopID)))
			} else {
				out = append(out, "replacement library: none")
			}
			return out
		}
	}
	return []string{T("not installed here")}
}

// FeedbackOps is how many operation-log rows the feedback attachment holds (16 §2.14).
const FeedbackOps = 30

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
	case "one_at_a_time":
		return errors.New(T("the feedback relay takes one problem at a time for now; send them one by one"))
	}
	return errors.New(Tf("the feedback relay could not file it (%s); open GitHub instead", re.Code))
}

// FeedbackProblem is one problem of a report: its template, the title, the answers by
// question key and the attachments. Sent, each problem becomes its own issue ("one report,
// one topic", 16 §2.14).
type FeedbackProblem struct {
	Kind    string            `json:"kind"`
	Title   string            `json:"title"`
	Answers map[string]string `json:"answers"`
	Opts    FeedbackOptions   `json:"opts"`
}

// FeedbackTemplate is a report kind in the player's language.
type FeedbackTemplate = feedback.Template

// FeedbackTemplates are the report kinds translated, in the page's order.
func FeedbackTemplates() []FeedbackTemplate {
	out := make([]FeedbackTemplate, len(feedback.Templates))
	for i, t := range feedback.Templates {
		t.Label, t.Lead = T(t.Label), T(t.Lead)
		if t.Note != "" {
			t.Note = T(t.Note)
		}
		qs := make([]feedback.Question, len(t.Questions))
		for j, q := range t.Questions {
			q.Label = T(q.Label)
			if q.Hint != "" {
				q.Hint = T(q.Hint)
			}
			cs := make([]string, len(q.Choices))
			for k, c := range q.Choices {
				cs[k] = T(c)
			}
			q.Choices = cs
			qs[j] = q
		}
		t.Questions = qs
		out[i] = t
	}
	return out
}

// Options are the problem's attachments with what its template adds by itself: the kind,
// the diagnosis or Mod it names, and the recent operations when files were changed or lost.
func (p FeedbackProblem) Options() FeedbackOptions {
	o := p.Opts
	o.Kind = feedback.TemplateOf(p.Kind).Kind
	o.Finding, o.Mod = "", ""
	switch o.Kind {
	case feedback.KindDiagnosis:
		o.Finding = strings.TrimSpace(p.Answers["finding"])
	case feedback.KindData:
		o.Mod = strings.TrimSpace(p.Answers["mod"])
	}
	if feedback.Priority(o.Kind, p.Answers) {
		o.Ops = true
	}
	return o
}

// Text is what the player wrote, each answer under its question in the player's language
// (feedback.AnswersText); a ticked box comes first.
func (p FeedbackProblem) Text() string {
	t := feedback.TemplateOf(p.Kind)
	var as []feedback.Answer
	var checks []string
	for _, q := range t.Questions {
		v := strings.TrimSpace(p.Answers[q.Key])
		if v == "" {
			continue
		}
		switch q.Form {
		case feedback.FormCheck:
			if feedback.Priority(t.Kind, map[string]string{q.Key: v}) || v == "yes" {
				checks = append(checks, T(q.Label))
			}
			continue
		}
		label := T(q.Label)
		if t.Kind == feedback.KindOther {
			label = "" // one question: the words alone
		}
		as = append(as, feedback.Answer{Label: label, Text: v})
	}
	return feedback.AnswersText(as, checks)
}

// FeedbackSkeleton is Text with each unanswered question (not a box) as a bold heading over
// an empty line: the link to GitHub carries it for the player to answer there.
func FeedbackSkeleton(p FeedbackProblem) string {
	var parts []string
	if t := p.Text(); t != "" {
		parts = append(parts, t)
	}
	if feedback.TemplateOf(p.Kind).Kind == feedback.KindOther && len(parts) > 0 {
		return parts[0]
	}
	for _, q := range feedback.TemplateOf(p.Kind).Questions {
		if q.Form == feedback.FormCheck || strings.TrimSpace(p.Answers[q.Key]) != "" {
			continue
		}
		parts = append(parts, "**"+T(q.Label)+"**\n")
	}
	return strings.Join(parts, "\n\n")
}

// TitleOrFirst is the title, or the first line of the first answer, up to 60 characters.
func (p FeedbackProblem) TitleOrFirst() string {
	if t := strings.TrimSpace(p.Title); t != "" {
		return t
	}
	for _, q := range feedback.TemplateOf(p.Kind).Questions {
		if q.Form == feedback.FormCheck || q.Form == feedback.FormChoice {
			continue
		}
		if v := strings.TrimSpace(p.Answers[q.Key]); v != "" {
			first, _, _ := strings.Cut(v, "\n")
			return truncateRunes(strings.TrimSpace(first), 60)
		}
	}
	return ""
}

// Empty: nothing written yet.
func (p FeedbackProblem) Empty() bool { return p.TitleOrFirst() == "" }

// ComposeProblem is the issue for one problem with its attachments (Compose: body and link).
func ComposeProblem(p FeedbackProblem, at *FeedbackAttach) feedback.Result {
	return ComposeFeedback(p.Title, p.Text(), at)
}

// SendFeedbackProblems files each problem as its own issue through the relay, in one request
// (the relay creates them one by one). ats[i] are problem i's attachments. Issues filed
// before a failure come back with the error; the caller keeps the rest of the draft.
func SendFeedbackProblems(ctx context.Context, ps []FeedbackProblem, ats []*FeedbackAttach, contact string) ([]feedback.Sent, error) {
	relay := feedback.RelayURL()
	if relay == "" {
		return nil, errors.New(T("RimArcho has no feedback relay; open GitHub instead"))
	}
	if len(ps) == 0 {
		return nil, errors.New(T("write a title or what happened first"))
	}
	if len(ps) > feedback.MaxSplit {
		return nil, errors.New(Tf("at most %d problems at once", feedback.MaxSplit))
	}
	var rs []feedback.Report
	for i, p := range ps {
		title := p.TitleOrFirst()
		if title == "" {
			return nil, errors.New(T("write a title or what happened first"))
		}
		d := feedback.Draft{Title: title, Text: p.Text(), More: T("… %d more lines")}
		if i < len(ats) && ats[i] != nil {
			d.Basic, d.Sections = ats[i].Basic, ats[i].Sections
		}
		rs = append(rs, feedback.Report{Title: title, Body: d.SendBody(feedback.MaxBody), Kind: feedback.TemplateOf(p.Kind).Kind, Priority: feedback.Priority(p.Kind, p.Answers)})
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second*time.Duration(len(rs)))
	defer cancel()
	sent, err := feedback.SendAll(ctx, relay, "RimArcho/"+buildinfo.Version, rs, strings.TrimSpace(contact))
	if err != nil {
		return sent, feedbackSendError(err)
	}
	return sent, nil
}

// CommunityLink is one of RimArcho's public pages (the settings page's head, 16 §2.14).
type CommunityLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// CommunityLinks: the GitHub repository and the Discord server.
func CommunityLinks() []CommunityLink {
	return []CommunityLink{{Label: "Discord", URL: feedback.Discord}, {Label: "GitHub", URL: feedback.Repo}}
}
