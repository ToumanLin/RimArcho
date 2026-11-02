package tui

import (
	"context"
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"sync"
	"time"
)

// Search files shares the window's three fields and bounded streaming model.
const fsTick = 300 * time.Millisecond

type fsTickMsg struct{ seq int }
type fsRow struct {
	mod, file, line int
	cursor          bool
	dirPath, modID  string
}
type fsState struct {
	text, files, mods string
	regex, cs         bool
	initialized       bool
	results           bool
	editing           bool
	row, caret        int
	page              *view.FileSearchPage
	at                view.FileSearchReq
	rows              []fsRow
	cursor, off       int
	expanded          map[string]bool
	running           bool
	seq, since        int
	cancel            context.CancelFunc
	run               *view.FileSearchRun
	status            view.FileSearchUpdate
	lib               *app.Library
	gate              *sync.Mutex
}

func (m *model) openSearch() tea.Cmd {
	m.panel, m.insp, m.full = nil, nil, false
	s := &m.fs
	m.tab = tabSearch
	if !s.initialized {
		lib, err := m.app.FileSearchLibrary()
		if err != nil {
			m.say(sayFail, m.humanize(err.Error()))
			return nil
		}
		info, err := view.BuildFileSearchInfo(lib, "", true)
		if err != nil {
			m.say(sayFail, m.humanize(err.Error()))
			return nil
		}
		s.files, s.mods, s.lib = info.Files, info.Mods, lib
		s.initialized = true
		if s.gate == nil {
			s.gate = &sync.Mutex{}
		}
	}
	s.results, s.editing, s.row = false, true, 0
	s.caret = len([]rune(s.text))
	return nil
}
func (m *model) fsLeft() {
	if m.tab != tabSearch {
		m.fsStop(false)
		m.fs.page = nil
		m.fs.rows = nil
		m.fs.run = nil
		m.fs.lib = nil
		m.fs.initialized = false
	}
}
func (s *fsState) field() *string {
	switch s.row {
	case 1:
		return &s.files
	case 2:
		return &s.mods
	default:
		return &s.text
	}
}
func (s *fsState) request(_ int) view.FileSearchReq {
	return view.FileSearchReq{Pattern: s.text, Files: s.files, ModsQuery: s.mods, Case: s.cs, Regex: s.regex}
}
func (m *model) fsRun(_ int) tea.Cmd {
	s := &m.fs
	q := s.request(1)
	if _, _, err := view.ParseFileSearchKinds(q.Files); err != nil {
		m.say(sayFail, err.Error())
		return nil
	}
	if strings.TrimSpace(q.Pattern) == "" {
		m.say(sayFail, T("type what to look for"))
		return nil
	}
	if _, err := view.SearchExpression(q); err != nil {
		m.say(sayFail, err.Error())
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.seq++
	seq := s.seq
	s.running, s.cancel, s.run, s.since = true, cancel, view.NewFileSearchRun(), 0
	s.page = &view.FileSearchPage{Mods: []view.FileSearchMod{}}
	s.at = q
	s.rows = nil
	s.cursor = 0
	s.off = 0
	s.expanded = map[string]bool{}
	run := s.run
	if s.gate == nil {
		s.gate = &sync.Mutex{}
	}
	gate := s.gate
	search := m.bg(func(a *app.App) func(m *model) tea.Cmd {
		gate.Lock()
		run.Search(ctx, a, q)
		gate.Unlock()
		cancel()
		return func(m *model) tea.Cmd {
			if m.fs.seq == seq {
				m.fsPoll()
			}
			return nil
		}
	})
	return tea.Batch(search, fsNext(seq))
}
func fsNext(seq int) tea.Cmd {
	return tea.Tick(fsTick, func(time.Time) tea.Msg { return fsTickMsg{seq} })
}
func (m *model) fsPoll() {
	s := &m.fs
	if s.run == nil {
		return
	}
	u := s.run.Update(s.since)
	s.status = u
	s.since = u.Next
	if s.page == nil {
		s.page = &view.FileSearchPage{}
	}
	for _, d := range u.Deltas {
		mi := -1
		for i := range s.page.Mods {
			if s.page.Mods[i].ID == d.ID {
				mi = i
				break
			}
		}
		if mi < 0 {
			s.page.Mods = append(s.page.Mods, view.FileSearchMod{ID: d.ID, Name: d.Name, Copy: d.Copy})
			mi = len(s.page.Mods) - 1
		}
		mod := &s.page.Mods[mi]
		mod.Files = append(mod.Files, d.File)
		mod.Count += d.File.Count
	}
	s.page.Summary, s.page.Hint, s.page.Notes = u.Summary, u.Hint, u.Notes
	if len(u.Deltas) > 0 {
		s.buildRows()
	}
	if u.Done {
		s.running = false
		s.cancel = nil
		if u.Error != "" {
			m.say(sayFail, m.humanize(u.Error))
		}
	}
}
func (m *model) fsStop(announce bool) {
	s := &m.fs
	if s.cancel != nil {
		s.cancel()
	}
	if s.run != nil {
		m.fsPoll()
	}
	if s.running && s.page != nil {
		s.page.Summary = Tf("stopped: %d matches in %d of %d mods", s.status.Matches, s.status.Searched, s.status.Total)
	}
	s.seq++
	s.running = false
	s.cancel = nil
	if announce {
		m.say(sayOK, T("stopped"))
	}
}
func fsFileKey(mod string, file view.FileSearchFile) string { return mod + "\n" + file.Path }
func (s *fsState) buildRows() {
	var selected fsRow
	if s.cursor < len(s.rows) {
		selected = s.rows[s.cursor]
	}
	s.rows = nil
	if s.page == nil {
		return
	}
	for mi, mod := range s.page.Mods {
		s.rows = append(s.rows, fsRow{mod: mi, file: -1, line: -1, modID: mod.ID})
		for fi, file := range mod.Files {
			s.rows = append(s.rows, fsRow{mod: mi, file: fi, line: -1, cursor: true, dirPath: file.Path, modID: mod.ID})
			if len(file.Lines) == 1 || s.expanded[fsFileKey(mod.ID, file)] {
				for li, hit := range file.Lines {
					if hit.Line > 0 {
						s.rows = append(s.rows, fsRow{mod: mi, file: fi, line: li, cursor: true, dirPath: file.Path, modID: mod.ID})
					}
				}
			}
		}
	}
	s.cursor = s.firstCursor()
	for i, r := range s.rows {
		if r.cursor && r.modID == selected.modID && r.dirPath == selected.dirPath && r.line == selected.line {
			s.cursor = i
			break
		}
	}
}
func (s *fsState) firstCursor() int {
	for i, r := range s.rows {
		if r.cursor {
			return i
		}
	}
	return 0
}
func (s *fsState) step(d int) {
	if len(s.rows) == 0 {
		return
	}
	i, dir := s.cursor, 1
	if d < 0 {
		dir, d = -1, -d
	}
	for ; d > 0; d-- {
		j := i + dir
		for j >= 0 && j < len(s.rows) && !s.rows[j].cursor {
			j += dir
		}
		if j < 0 || j >= len(s.rows) {
			break
		}
		i = j
	}
	s.cursor = i
}
func (m *model) fsOpenFolder(r fsRow) {
	ix, err := m.app.Index()
	if err == nil {
		var dir string
		if dir, err = view.SearchFolder(ix, m.app.SourceDir(), r.modID, r.dirPath); err == nil {
			open := m.opts.OpenDir
			if open == nil {
				open = openDir
			}
			err = open(dir)
		}
	}
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
	} else {
		m.say(sayOK, T("opened the folder"))
	}
}
func (m *model) fsLines(n int) []string {
	s := &m.fs
	w := max(m.w-4, 20)
	out := []string{}
	for i, label := range []string{T("search"), T("files"), T("mods")} {
		text := []string{s.text, s.files, s.mods}[i]
		current := !s.results && s.row == i
		if current && s.editing {
			chars := []rune(text)
			at := max(0, min(s.caret, len(chars)))
			text = string(chars[:at]) + "_" + string(chars[at:])
		}
		st := sSecond
		if current {
			st = sText
		}
		prefix := "  "
		if i == 0 {
			prefix = sDimS.Render("1 ")
		}
		line := prefix + sDimS.Render(fit(label, 7)) + st.Render(text)
		if current && s.editing {
			line += sDimS.Render(m.fsComplete())
		}
		if i == 0 {
			flag := func(label string, on bool) string {
				if on {
					return sSel.Render(label)
				}
				return sDimS.Render(label)
			}
			line += "  " + flag("Aa alt+c", s.cs) + "  " + flag(".* alt+r", s.regex)
		}
		out = append(out, width.Truncate(line, w, "…"))
	}
	if _, _, err := view.ParseFileSearchKinds(s.files); err != nil {
		out = append(out, "  "+sWarnS.Render(err.Error()))
	}
	out = append(out, "")
	title := sDimS.Render("2 " + T("results"))
	if s.results {
		title = sAccent.Render("2 " + T("results"))
	}
	if s.page != nil {
		title += "  " + sSecond.Render(s.page.Summary)
	}
	if s.running {
		title += sDimS.Render("  esc " + T("stop"))
	}
	out = append(out, width.Truncate(title, w, "…"))
	body := m.fsResultLines(w)
	rows := max(n-len(out)-1, 1)
	if s.cursor < s.off {
		s.off = s.cursor
	}
	if s.cursor >= s.off+rows {
		s.off = s.cursor - rows + 1
	}
	s.off = max(0, min(s.off, len(body)-rows))
	return append(out, window(body, s.off, rows)...)
}
func (m *model) fsResultLines(w int) []string {
	s := &m.fs
	if s.page == nil {
		return nil
	}
	var out []string
	for i, row := range s.rows {
		mod := s.page.Mods[row.mod]
		cur := s.results && i == s.cursor
		mark := "  "
		if cur {
			mark = sAccent.Render(markSel) + " "
		}
		switch {
		case row.file < 0:
			out = append(out, width.Truncate("  "+sBold.Render(mod.Name)+"  "+sDimS.Render(mod.Copy+"  "+itoa(mod.Count)), w, "…"))
		case row.line < 0:
			file := mod.Files[row.file]
			chev := " "
			if len(file.Lines) > 1 {
				chev = "›"
				if s.expanded[fsFileKey(mod.ID, file)] {
					chev = "⌄"
				}
			}
			st := sText
			if cur {
				st = sSel
			}
			out = append(out, width.Truncate("    "+mark+sDimS.Render(chev+" ")+st.Render(file.Rel)+"  "+sDimS.Render(itoa(file.Count)), w, "…"))
		default:
			hit := mod.Files[row.file].Lines[row.line]
			text := ""
			for _, part := range hit.Parts {
				if part.Hit {
					text += sWarnS.Bold(true).Render(part.Text)
				} else {
					text += sSecond.Render(part.Text)
				}
			}
			if len(hit.Parts) == 0 {
				text = sSecond.Render(hit.Text)
			}
			out = append(out, width.Truncate("      "+mark+sDimS.Render(itoa(hit.Line)+"  ")+text, w, "…"))
		}
	}
	if len(s.rows) == 0 && !s.running {
		out = append(out, "  "+sSecond.Render(T("not found")))
		if s.page.Hint != "" {
			out = append(out, "  "+sDimS.Render(width.Truncate(s.page.Hint, w-2, "…")))
		}
	}
	for _, note := range s.page.Notes {
		out = append(out, "  "+sDimS.Render(width.Truncate(note, w-2, "…")))
	}
	return out
}
func (m *model) fsPairs() [][2]string {
	s := &m.fs
	pairs := [][2]string{{"1 /", T("search")}, {"2", T("results")}}
	if !s.results {
		pairs = append(pairs, [2]string{"↑↓", T("fields")}, [2]string{"enter", T("search")}, [2]string{"tab", T("complete")}, [2]string{"alt+c alt+r", T("case, regex")})
	} else if len(s.rows) > 0 {
		pairs = append(pairs, [2]string{"↑↓", T("move the cursor")}, [2]string{"←→", T("collapse / expand")}, [2]string{"enter ctrl+o", T("open the folder")})
	}
	if s.running {
		pairs = append(pairs, [2]string{"esc", T("stop")})
	}
	return append(pairs, [2]string{"?", T("all keys")})
}
