package tui

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf16"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/feedback"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// copyText puts text on the clipboard (Windows clip.exe, given UTF-16 so any language
// survives). A variable so tests never touch the real clipboard (noopen_test.go).
var copyText = func(s string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("no clipboard")
	}
	var b bytes.Buffer
	b.Write([]byte{0xFF, 0xFE})
	for _, u := range utf16.Encode([]rune(s)) {
		b.Write([]byte{byte(u), byte(u >> 8)})
	}
	c := exec.Command("clip")
	c.Stdin = &b
	return c.Run()
}

// feedbackPanel is Feedback in the terminal (16 §2.14, §9): 1–5 pick the report's template,
// m / p / l / o / b tick the attachments (the template ticks its own first), the whole text
// below. s asks the template's questions on the input line and sends it through the feedback
// relay (only when RimArcho has one); enter opens GitHub with the questions as headings to
// answer there. One problem per send: the window's "add another problem" splits.
func (m *model) feedbackPanel(p view.FeedbackProblem) tea.Cmd {
	if p.Kind == "" {
		p.Kind = feedback.Templates[0].Kind
		p.Opts = templateOpts(p.Kind)
	}
	if p.Answers == nil {
		p.Answers = map[string]string{}
	}
	o := p.Options()
	at := view.FeedbackAttachments(m.app, m.st, o, m.feedbackNeeds())
	res := view.ComposeFeedback(p.TitleOrFirst(), view.FeedbackSkeleton(p), at)
	var b strings.Builder
	tpls := view.FeedbackTemplates()
	for i, t := range tpls {
		mark := " "
		if t.Kind == p.Kind {
			mark = ">"
		}
		fmt.Fprintf(&b, "%s %d %s  %s\n", mark, i+1, t.Label, t.Lead)
	}
	if seed := m.fbSeed[p.Kind]; len(seed) > 0 {
		// filled in from the item W reported: the rest of the questions are the player's
		fmt.Fprintf(&b, "  %s\n", strings.TrimSpace(seed["mod"]+"  "+seed["finding"]))
	}
	if t := feedback.TemplateOf(p.Kind); t.Note != "" {
		b.WriteString("\n" + T(t.Note) + "\n")
	}
	on := map[string]bool{"mods": o.Mods, "paths": o.Paths, "log": o.Log, "ops": o.Ops, "problems": o.Problems}
	key := map[string]string{"mods": "m", "paths": "p", "log": "l", "ops": "o", "problems": "b"}
	fmt.Fprintf(&b, "\n[x]   %s  %s\n", T("basic information"), strings.Join(at.Basic, "  "))
	for _, part := range at.Parts {
		mark := "[ ]"
		if on[part.Key] {
			mark = "[x]"
		}
		if part.None {
			mark, key[part.Key] = "   ", " "
		}
		note := part.Note
		if part.Key == "log" && o.Log && at.Pending {
			note = T("the game log is still being analysed…") + "  " + T("r: again")
		}
		fmt.Fprintf(&b, "%s %s %s  %s\n", mark, key[part.Key], part.Label, note)
	}
	// what the player reads; the Markdown sent or copied stays out of sight (res.Body)
	b.WriteString("\n" + T("preview") + "  " + T("public on GitHub") + "\n\n" + view.FeedbackPreview(p.Text(), at) + "\n")
	if at.Send {
		b.WriteString(T("s: answer the questions here and send it, no GitHub account needed") + "\n")
	}
	if res.Full {
		b.WriteString(T("enter: open GitHub with this text; answer the questions there") + "\n")
	} else {
		b.WriteString(T("enter: copy the text and open GitHub; paste it into the issue") + "\n")
	}
	// where players talk, in full (moved from Settings › about, 16 §2.14)
	b.WriteString("\n" + T("community") + "\n")
	for _, l := range view.CommunityLinks() {
		if l.URL != "" {
			fmt.Fprintf(&b, "  %s  %s\n", l.Label, l.URL)
		} else {
			fmt.Fprintf(&b, "  %s\n", l.Label) // the QQ group: its number is in the label
		}
	}
	m.openPanel(T("feedback"), b.String())
	again := func(f func()) func(*model) tea.Cmd {
		return func(m *model) tea.Cmd { f(); return m.feedbackPanel(p) }
	}
	m.panel.keys = map[string]func(*model) tea.Cmd{"r": again(func() {})}
	for i, t := range tpls {
		kind := t.Kind
		m.panel.keys[fmt.Sprint(i+1)] = again(func() {
			if p.Kind != kind {
				p = view.FeedbackProblem{Kind: kind, Title: p.Title, Answers: seedOf(m.fbSeed, kind), Opts: templateOpts(kind)}
			}
		})
	}
	for _, part := range at.Parts {
		if part.None {
			continue
		}
		switch part.Key {
		case "mods":
			m.panel.keys["m"] = again(func() { p.Opts.Mods = !p.Opts.Mods })
		case "paths":
			m.panel.keys["p"] = again(func() { p.Opts.Paths = !p.Opts.Paths })
		case "log":
			m.panel.keys["l"] = again(func() { p.Opts.Log = !p.Opts.Log })
		case "ops":
			m.panel.keys["o"] = again(func() { p.Opts.Ops = !p.Opts.Ops })
		case "problems":
			m.panel.keys["b"] = again(func() { p.Opts.Problems = !p.Opts.Problems })
		}
	}
	if at.Send {
		m.panel.keys["s"] = func(m *model) tea.Cmd { m.panel = nil; return m.askFeedback(p, 0) }
	}
	m.panel.onEnter = func(m *model) tea.Cmd {
		if !res.Full {
			if err := copyText(res.Body); err != nil {
				m.say(sayFail, err.Error())
				return nil
			}
		}
		if err := openURL(res.URL); err != nil {
			m.say(sayFail, err.Error())
			return nil
		}
		if res.Full {
			m.say(sayInfo, T("opened GitHub in the browser"))
		} else {
			m.say(sayInfo, T("opened GitHub; paste the copied text into the issue"))
		}
		return nil
	}
	return nil
}

// reportNeed is W on a Can it run item: the feedback panel on the wrong diagnosis template
// with the item's ID; an item that rests on shared-log data or the replacement library also
// fills outdated data (4) with its Mod and the item.
func (m *model) reportNeed(id, mod string) tea.Cmd {
	m.fbSeed = map[string]map[string]string{feedback.KindDiagnosis: {"finding": id}}
	if mod != "" {
		m.fbSeed[feedback.KindData] = map[string]string{"mod": mod, feedback.DataFinding: id}
	}
	kind := feedback.KindDiagnosis
	return m.feedbackPanel(view.FeedbackProblem{Kind: kind, Answers: seedOf(m.fbSeed, kind), Opts: templateOpts(kind)})
}

// seedOf is a copy of the answers a template starts with.
func seedOf(seed map[string]map[string]string, kind string) map[string]string {
	out := map[string]string{}
	for k, v := range seed[kind] {
		out[k] = v
	}
	return out
}

// templateOpts are the attachments a template ticks when it is chosen.
func templateOpts(kind string) view.FeedbackOptions {
	var o view.FeedbackOptions
	for _, k := range feedback.TemplateOf(kind).Attach {
		switch k {
		case "mods":
			o.Mods = true
		case "paths":
			o.Paths = true
		case "log":
			o.Log = true
		case "ops":
			o.Ops = true
		case "problems":
			o.Problems = true
		}
	}
	return o
}

// feedbackNeeds is Can it run for the problems attachment and the diagnosis: the problems
// RimArcho already knows, without a new check.
func (m *model) feedbackNeeds() *view.Needs {
	ps, err := m.app.Problems(false, nil)
	if err != nil || ps == nil {
		return nil
	}
	return view.BuildNeeds(m.app, m.st, ps)
}

// askFeedback asks the template's questions one input line each (esc stops; an optional one
// may be left empty), then the title and the contact, then sends in the background;
// feedbackSentMsg says how it went.
func (m *model) askFeedback(p view.FeedbackProblem, i int) tea.Cmd {
	qs := view.FeedbackTemplates()
	var t feedback.Template
	for _, x := range qs {
		if x.Kind == p.Kind {
			t = x
		}
	}
	if i < len(t.Questions) {
		q := t.Questions[i]
		label := q.Label
		switch q.Form {
		case feedback.FormCheck:
			label += " (y/n)"
		case feedback.FormChoice:
			label += " (" + strings.Join(q.Choices, " / ") + ")"
		}
		if q.Optional {
			label += " " + T("(optional)")
		}
		m.prompt = &prompt{label: label + ": ", submit: func(m *model, v string) tea.Cmd {
			v = strings.TrimSpace(v)
			if q.Form == feedback.FormCheck {
				if strings.HasPrefix(strings.ToLower(v), "y") {
					v = "yes"
				} else {
					v = ""
				}
			}
			p.Answers[q.Key] = v
			return m.askFeedback(p, i+1)
		}}
		return nil
	}
	m.prompt = &prompt{label: T("title") + ": ", submit: func(m *model, title string) tea.Cmd {
		p.Title = title
		if p.Empty() {
			m.say(sayFail, T("write a title or what happened first"))
			return nil
		}
		m.prompt = &prompt{label: T("contact (optional, not public)") + ": ", submit: func(m *model, contact string) tea.Cmd {
			m.say(sayBusy, T("sending…"))
			a, st := m.app, m.st
			n := m.feedbackNeeds()
			return func() tea.Msg {
				at := view.FeedbackAttachments(a, st, p.Options(), n)
				s, err := view.SendFeedbackProblems(context.Background(), []view.FeedbackProblem{p}, []*view.FeedbackAttach{at}, contact)
				var one feedback.Sent
				if len(s) > 0 {
					one = s[0]
					view.RecordFeedbackSent(a, one, p.TitleOrFirst())
				}
				return feedbackSentMsg{one, err}
			}
		}}
		return nil
	}}
	return nil
}

// feedbackSentMsg is the relay's answer to s in Feedback.
type feedbackSentMsg struct {
	sent feedback.Sent
	err  error
}
