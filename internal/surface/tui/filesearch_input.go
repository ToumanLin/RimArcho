package tui

import (
	"github.com/ToumanLin/RimArcho/internal/view"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
)

// Field editing and result navigation for the Search files page.
func (m *model) fsTyping() bool { return m.tab == tabSearch && !m.fs.results && m.fs.editing }
func (m *model) fsOption(k string) (tea.Cmd, bool) {
	s := &m.fs
	switch k {
	case "alt+c":
		s.cs = !s.cs
	case "alt+r":
		s.regex = !s.regex
	default:
		return nil, false
	}
	if strings.TrimSpace(s.text) != "" {
		return m.fsRun(1), true
	}
	return nil, true
}
func (m *model) fsComplete() string {
	s := &m.fs
	text := *s.field()
	if s.caret != len([]rune(text)) {
		return ""
	}
	if s.row == 2 && s.lib != nil {
		return view.CompleteLibrary(text, s.lib)
	}
	if s.row != 1 || strings.HasSuffix(text, " ") {
		return ""
	}
	at := strings.LastIndex(text, " ") + 1
	last := text[at:]
	prefix := ""
	if strings.HasPrefix(last, "-") {
		prefix = "-"
		last = last[1:]
	}
	for _, kind := range []string{"C#", "Defs", "patches", "About", "translations", "textures", "sounds"} {
		if len(last) > 0 && strings.HasPrefix(strings.ToLower(kind), strings.ToLower(last)) && len(kind) > len(last) {
			return (prefix + kind)[len(prefix)+len(last):]
		}
	}
	return ""
}
func (m *model) fsTypeKey(k tea.KeyMsg) tea.Cmd {
	s := &m.fs
	if cmd, ok := m.fsOption(k.String()); ok {
		return cmd
	}
	text := s.field()
	runes := []rune(*text)
	s.caret = max(0, min(s.caret, len(runes)))
	switch k.Type {
	case tea.KeyRunes, tea.KeySpace:
		add := string(k.Runes)
		if k.Type == tea.KeySpace {
			add = " "
		}
		add = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(add)
		ins := []rune(add)
		runes = append(append(append([]rune{}, runes[:s.caret]...), ins...), runes[s.caret:]...)
		s.caret += len(ins)
	case tea.KeyBackspace:
		if s.caret > 0 {
			runes = append(runes[:s.caret-1], runes[s.caret:]...)
			s.caret--
		}
	case tea.KeyDelete:
		if s.caret < len(runes) {
			runes = append(runes[:s.caret], runes[s.caret+1:]...)
		}
	case tea.KeyCtrlU:
		runes = nil
		s.caret = 0
	case tea.KeyLeft:
		s.caret = max(0, s.caret-1)
	case tea.KeyRight:
		s.caret = min(len(runes), s.caret+1)
	case tea.KeyHome:
		s.caret = 0
	case tea.KeyEnd:
		s.caret = len(runes)
	case tea.KeyEnter:
		return m.fsRun(1)
	case tea.KeyUp:
		if s.row > 0 {
			s.row--
			s.caret = len([]rune(*s.field()))
		}
		return nil
	case tea.KeyDown:
		if s.row < 2 {
			s.row++
			s.caret = len([]rune(*s.field()))
		} else {
			s.results = true
			s.editing = false
		}
		return nil
	case tea.KeyEsc:
		if s.running {
			m.fsStop(true)
		} else {
			s.editing = false
		}
		return nil
	case tea.KeyTab:
		if suffix := m.fsComplete(); suffix != "" {
			*text += suffix
			s.caret = len([]rune(*text))
			return nil
		}
		return m.subPage(1)
	case tea.KeyShiftTab:
		return m.subPage(-1)
	}
	*text = string(runes)
	return nil
}
func (m *model) fsKey(k string) tea.Cmd {
	s := &m.fs
	if cmd, ok := m.fsOption(k); ok {
		return cmd
	}
	switch k {
	case "esc":
		if s.running {
			m.fsStop(true)
		} else if !s.results {
			s.results = true
		} else {
			m.tab = tabList
			return m.requestDetail()
		}
		return nil
	case "/", "ctrl+f", "1":
		s.results, s.editing = false, true
		s.caret = len([]rune(*s.field()))
		return nil
	case "2":
		s.results, s.editing = true, false
		return nil
	}
	if !s.results {
		switch k {
		case "up", "k":
			s.row = max(0, s.row-1)
		case "down", "j":
			if s.row < 2 {
				s.row++
			} else {
				s.results = true
			}
		case "right", "l":
			s.results = true
		case "enter", " ":
			s.editing = true
			s.caret = len([]rune(*s.field()))
		}
		return nil
	}
	n := max(m.h-10, 1)
	switch k {
	case "up", "k":
		s.step(-1)
	case "down", "j":
		s.step(1)
	case "pgup":
		s.step(-n)
	case "pgdown":
		s.step(n)
	case "home":
		s.cursor = s.firstCursor()
	case "end":
		s.step(len(s.rows))
	case "enter", "ctrl+o":
		if s.cursor < len(s.rows) && s.rows[s.cursor].cursor {
			m.fsOpenFolder(s.rows[s.cursor])
		}
	case "left", "right":
		if s.cursor >= len(s.rows) || s.page == nil {
			return nil
		}
		row := s.rows[s.cursor]
		if row.file < 0 {
			return nil
		}
		file := s.page.Mods[row.mod].Files[row.file]
		id := fsFileKey(row.modID, file)
		s.expanded[id] = k == "right"
		s.buildRows()
	}
	return nil
}
