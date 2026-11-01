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

// feedbackPanel is Settings > Feedback in the terminal (06 §2.14, §9): the same attachments as the
// window, m / p / l tick them, the whole text below. s sends it through the feedback relay
// (asked for the title, what happened and a contact on the input line; only when RimArcho
// has a relay); enter opens GitHub with it filled in (the title and text are written there).
func (m *model) feedbackPanel(o view.FeedbackOptions) tea.Cmd {
	at := view.FeedbackAttachments(m.app, m.st, o)
	res := view.ComposeFeedback("", "", at)
	on := map[string]bool{"mods": o.Mods, "paths": o.Paths, "log": o.Log}
	key := map[string]string{"mods": "m", "paths": "p", "log": "l"}
	var b strings.Builder
	fmt.Fprintf(&b, "[x]   %s  %s\n", T("basic information"), strings.Join(at.Basic, "  "))
	for _, p := range at.Parts {
		mark := "[ ]"
		if on[p.Key] {
			mark = "[x]"
		}
		if p.None {
			mark, key[p.Key] = "   ", " "
		}
		note := p.Note
		if p.Key == "log" && o.Log && at.Pending {
			note = T("the game log is still being analysed…") + "  " + T("r: again")
		}
		fmt.Fprintf(&b, "%s %s %s  %s\n", mark, key[p.Key], p.Label, note)
	}
	// what the player reads; the Markdown sent or copied stays out of sight (res.Body)
	b.WriteString("\n" + T("preview") + "  " + T("public on GitHub") + "\n\n" + view.FeedbackPreview("", at) + "\n")
	if at.Send {
		b.WriteString(T("s: send it here, no GitHub account needed") + "\n")
	}
	if res.Full {
		b.WriteString(T("enter: open GitHub with this text; write the title and what happened there") + "\n")
	} else {
		b.WriteString(T("enter: copy the text and open GitHub; paste it into the issue") + "\n")
	}
	m.openPanel(T("feedback"), b.String())
	toggle := func(f func()) func(*model) tea.Cmd {
		return func(m *model) tea.Cmd { f(); return m.feedbackPanel(o) }
	}
	m.panel.keys = map[string]func(*model) tea.Cmd{"r": toggle(func() {})}
	for _, p := range at.Parts {
		if p.None {
			continue
		}
		switch p.Key {
		case "mods":
			m.panel.keys["m"] = toggle(func() { o.Mods = !o.Mods })
		case "paths":
			m.panel.keys["p"] = toggle(func() { o.Paths = !o.Paths })
		case "log":
			m.panel.keys["l"] = toggle(func() { o.Log = !o.Log })
		}
	}
	if at.Send {
		m.panel.keys["s"] = func(m *model) tea.Cmd { m.panel = nil; return m.askFeedback(at) }
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

// askFeedback asks for the title, what happened and the contact, one input line each (esc
// stops), then sends in the background; feedbackSentMsg says how it went.
func (m *model) askFeedback(at *view.FeedbackAttach) tea.Cmd {
	m.prompt = &prompt{label: T("title") + ": ", submit: func(m *model, title string) tea.Cmd {
		m.prompt = &prompt{label: T("what happened") + ": ", submit: func(m *model, text string) tea.Cmd {
			if title == "" && text == "" {
				m.say(sayFail, T("write a title or what happened first"))
				return nil
			}
			m.prompt = &prompt{label: T("contact (optional, not public)") + ": ", submit: func(m *model, contact string) tea.Cmd {
				m.say(sayBusy, T("sending…"))
				return func() tea.Msg {
					s, err := view.SendFeedback(context.Background(), title, text, contact, at)
					return feedbackSentMsg{s, err}
				}
			}}
			return nil
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
