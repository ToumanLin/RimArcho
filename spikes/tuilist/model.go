package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type row struct {
	idx               int
	name, pkg, status string
}

var (
	names = []string{
		"原版扩展：机械族 Vanilla Expanded – Mechanoids",
		"Harmony",
		"中文汉化补丁（简体）Chinese Simplified",
		"Combat Extended",
		"更好的殖民者 Better Pawns",
		"Fishing 钓鱼",
		"Dubs Bad Hygiene 卫生与排水",
		"RimHUD",
		"【整合包】大型殖民地优化 Performance Fish + Rocketman",
		"Allow Tool",
		"ＲｉｍＷｏｒｌｄ全角Ａ１２３ Fullwidth",
		"Realistic Planets Slimmed – 星球生成器",
	}
	statuses = []string{"OK", "MISSING", "OUTDATED", "CONFLICT"}
	pkgs     = []string{"ludeon.rimworld", "brrainz.harmony", "vanillaexpanded.vfecore", "ceteam.combatextended", "zh.cn.patch", "fish.test.mod"}
)

func genRows(n int) []row {
	rs := make([]row, n)
	for i := range rs {
		rs[i] = row{i + 1,
			fmt.Sprintf("%s #%d", names[i%len(names)], i+1),
			fmt.Sprintf("%s.%d", pkgs[i%len(pkgs)], i),
			statuses[i%len(statuses)]}
	}
	return rs
}

type model struct {
	all       []row
	view      []row
	cursor    int
	off       int
	w, h      int
	query     string
	searching bool
}

func newModel(n int) model {
	rs := genRows(n)
	return model{all: rs, view: rs, w: 120, h: 40}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) page() int {
	if p := m.h - 2; p > 1 {
		return p
	}
	return 1
}

func (m *model) refilter() {
	q := strings.ToLower(m.query)
	if q == "" {
		m.view = m.all
	} else {
		out := make([]row, 0, len(m.all))
		for _, r := range m.all {
			if strings.Contains(strings.ToLower(r.name), q) {
				out = append(out, r)
			}
		}
		m.view = out
	}
	m.cursor, m.off = 0, 0
}

func (m *model) move(d int) {
	m.cursor += d
	if m.cursor >= len(m.view) {
		m.cursor = len(m.view) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	p := m.page()
	if m.cursor < m.off {
		m.off = m.cursor
	}
	if m.cursor >= m.off+p {
		m.off = m.cursor - p + 1
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.move(0)
	case tea.KeyMsg:
		if m.searching {
			switch msg.Type {
			case tea.KeyEnter:
				m.searching = false
			case tea.KeyEsc:
				m.searching, m.query = false, ""
				m.refilter()
			case tea.KeyBackspace:
				if r := []rune(m.query); len(r) > 0 {
					m.query = string(r[:len(r)-1])
					m.refilter()
				}
			case tea.KeyRunes, tea.KeySpace:
				m.query += string(msg.Runes)
				m.refilter()
			case tea.KeyUp:
				m.move(-1)
			case tea.KeyDown:
				m.move(1)
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "/":
			m.searching = true
		case "j", "down":
			m.move(1)
		case "k", "up":
			m.move(-1)
		case "pgdown":
			m.move(m.page())
		case "pgup":
			m.move(-m.page())
		case "home", "g":
			m.move(-len(m.view))
		case "end", "G":
			m.move(len(m.view))
		}
	}
	return m, nil
}

// fit truncates/pads s to exactly w display cells.
func fit(s string, w int) string {
	return runewidth.FillRight(runewidth.Truncate(s, w, "…"), w)
}

var (
	selStyle = lipgloss.NewStyle().Reverse(true)
	hdrStyle = lipgloss.NewStyle().Bold(true)
)

func (m model) line(r row, sel bool) string {
	const wi, wp, ws = 5, 36, 9
	wn := m.w - wi - wp - ws - 3
	if wn < 8 {
		wn = 8
	}
	s := fit(fmt.Sprint(r.idx), wi) + " " + fit(r.name, wn) + " " + fit(r.pkg, wp) + " " + fit(r.status, ws)
	if sel {
		return selStyle.Render(s)
	}
	return s
}

// View renders header + (h-2) list rows + footer = h lines total.
func (m model) View() string {
	var b strings.Builder
	b.WriteString(hdrStyle.Render(m.line(row{0, "Name 名称", "packageId", "Status"}, false)))
	b.WriteByte('\n')
	p := m.page()
	end := m.off + p
	if end > len(m.view) {
		end = len(m.view)
	}
	n := 0
	for i := m.off; i < end; i++ {
		b.WriteString(m.line(m.view[i], i == m.cursor))
		b.WriteByte('\n')
		n++
	}
	for ; n < p; n++ {
		b.WriteString(strings.Repeat(" ", m.w) + "\n")
	}
	q := m.query
	if m.searching {
		q += "_"
	}
	fmt.Fprintf(&b, "pos %d/%d  search[/]: %s", m.cursor+1, len(m.view), q)
	return b.String()
}
