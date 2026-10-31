package tui

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf16"

	tea "github.com/charmbracelet/bubbletea"

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

// feedbackPanel is 设置 › 反馈 in the terminal (06 §2.14, §9): the same attachments as the
// window, m / p / l tick them, the whole text below, enter opens GitHub with it filled in
// (the title and what happened are written there).
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
	b.WriteString("\n" + res.Body + "\n")
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

func init() {
	view.Register(map[string]string{
		"r: again": "r 再看一次",
		"enter: open GitHub with this text; write the title and what happened there": "enter 用这段文字打开 GitHub，标题和说明在那里写",
		"enter: copy the text and open GitHub; paste it into the issue":              "enter 复制全文并打开 GitHub，粘贴到正文里",
	})
}
