package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// The command palette (06 §8): ctrl+k, a "❯" input line over a result list in the style of
// magpie's picker. It finds commands (by their Chinese name or English alias), mods (enter
// jumps to the mod) and the other environments. Ranking: prefix > substring > subsequence
// > description; ties go to commands, then the shorter name.

type palKind int

const (
	palCommand palKind = iota
	palMod
	palEnv
)

type palItem struct {
	kind  palKind
	name  string // shown
	alias string // English alias of a command
	desc  string // matched last: package id and author of a mod, what a command does
	run   func(m *model) tea.Cmd
	score int
}

type palette struct {
	query   string
	sel     int
	results []palItem
}

// palCommands: every command of the main screen, with the key that also does it.
func (m *model) palCommands() []palItem {
	list := func(key string) func(m *model) tea.Cmd {
		return func(m *model) tea.Cmd { m.tab = tabList; return m.listKey(key) }
	}
	cmd := func(zh, en, desc string, run func(m *model) tea.Cmd) palItem {
		name := en
		if lang == "zh" {
			name = zh
		}
		return palItem{kind: palCommand, name: name, alias: en, desc: desc, run: run}
	}
	return []palItem{
		cmd("自动排序", "sort", "S", func(m *model) tea.Cmd { m.tab = tabList; return m.sortPreview() }),
		cmd("保存到游戏", "save", "w ModsConfig.xml", func(m *model) tea.Cmd { m.tab = tabList; return m.savePreview() }),
		cmd("启动游戏", "launch", "L RimWorld", list("L")),
		cmd("撤销", "undo", "u", list("u")),
		cmd("重做", "redo", "ctrl+r", list("ctrl+r")),
		cmd("需要处理", "needs attention", "! problems", func(m *model) tea.Cmd { m.tab = tabProblems; return m.ensureProblems() }),
		cmd("最近变化", "recent changes", "c history", func(m *model) tea.Cmd { m.tab = tabChanges; m.loadChanges(); return nil }),
		cmd("重新扫描", "rescan", "R scan", func(m *model) tea.Cmd { return m.startScan() }),
		cmd("重新检查文件", "check the files again", "A analyze", func(m *model) tea.Cmd { return m.startAnalyze() }),
		cmd("选择日志", "choose a log", "Player.log", func(m *model) tea.Cmd { return m.chooseLog() }),
		cmd("切换环境", "switch environment", "E env", func(m *model) tea.Cmd { return m.chooseEnv() }),
		cmd("全部从暂存放回", "put all back", "T staging", list("T")),
		cmd("筛选这一列", "filter", "/ search", list("/")),
		cmd("详情", "details", "enter", list("enter")),
		cmd("技术信息", "technical information", "i", list("i")),
		cmd("编辑标签", "tags", "#", list("#")),
		cmd("运行 archo 命令", "command line", ": archo", func(m *model) tea.Cmd {
			m.tab = tabList
			return m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
		}),
		cmd("帮助", "help", "?", func(m *model) tea.Cmd { m.openPanel(T("help"), T(helpText), nil); return nil }),
		cmd("退出", "quit", "q", func(m *model) tea.Cmd { m.quitting = true; return tea.Quit }),
	}
}

// palAll: commands, then the other environments, then every mod.
func (m *model) palAll() []palItem {
	out := m.palCommands()
	if envs, err := m.app.Store.Envs(); err == nil {
		for _, e := range envs {
			if e.Name == m.envName {
				continue
			}
			name := e.Name
			out = append(out, palItem{kind: palEnv, name: Tf("switch to %s", name), alias: name, desc: "env", run: func(m *model) tea.Cmd {
				m.app.EnvName = ""
				if m.act(Tf("switched to %s", name), "env", "use", name) {
					for k := range m.cols {
						m.cols[k].cursor, m.cols[k].off, m.cols[k].filter = 0, 0, ""
						m.refilterCol(colKind(k), "")
					}
					m.detailRef, m.prob.loaded = "", false
				}
				return m.requestDetail()
			}})
		}
	}
	seen := map[string]bool{}
	for k := range m.cols {
		for i := range m.cols[k].items {
			it := &m.cols[k].items[i]
			if it.id == "" || seen[it.id] {
				continue
			}
			seen[it.id] = true
			id := it.id
			out = append(out, palItem{kind: palMod, name: it.name, desc: it.pkg + " " + it.author, run: func(m *model) tea.Cmd { return m.jumpTo(id) }})
		}
	}
	return out
}

// palScore ranks one candidate for q (lower case): 4 prefix, 3 substring, 2 subsequence,
// 1 description, 0 no match.
func palScore(q string, it palItem) int {
	if q == "" {
		return 1
	}
	best := 0
	for _, f := range []string{it.name, it.alias} {
		f = strings.ToLower(f)
		if f == "" {
			continue
		}
		switch {
		case strings.HasPrefix(f, q):
			best = max(best, 4)
		case strings.Contains(f, q):
			best = max(best, 3)
		case subsequence(q, f):
			best = max(best, 2)
		}
	}
	if best == 0 && strings.Contains(strings.ToLower(it.desc), q) {
		best = 1
	}
	return best
}

func (m *model) palFilter() {
	p := m.pal
	q := strings.ToLower(strings.TrimSpace(p.query))
	p.results = p.results[:0]
	for _, it := range m.palAll() {
		if q == "" && it.kind == palMod {
			continue // an empty query lists the commands and environments
		}
		if s := palScore(q, it); s > 0 {
			it.score = s
			p.results = append(p.results, it)
		}
	}
	sort.SliceStable(p.results, func(i, j int) bool {
		a, b := p.results[i], p.results[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		if q != "" && len(a.name) != len(b.name) {
			return len(a.name) < len(b.name)
		}
		return false
	})
	p.sel = max(0, min(p.sel, len(p.results)-1))
}

func (m *model) openPalette() tea.Cmd {
	m.pal = &palette{}
	m.palFilter()
	return nil
}

func (m *model) paletteKey(k tea.KeyMsg) tea.Cmd {
	p := m.pal
	switch k.Type {
	case tea.KeyEsc, tea.KeyCtrlK:
		m.pal = nil
		return nil
	case tea.KeyEnter:
		m.pal = nil
		if p.sel < len(p.results) {
			return p.results[p.sel].run(m)
		}
		return nil
	case tea.KeyUp, tea.KeyCtrlP:
		p.sel = max(p.sel-1, 0)
		return nil
	case tea.KeyDown, tea.KeyCtrlN, tea.KeyTab:
		p.sel = min(p.sel+1, max(len(p.results)-1, 0))
		return nil
	case tea.KeyBackspace:
		if r := []rune(p.query); len(r) > 0 {
			p.query = string(r[:len(r)-1])
		}
	case tea.KeyCtrlU:
		p.query = ""
	case tea.KeySpace:
		p.query += " "
	case tea.KeyRunes:
		p.query += string(k.Runes)
	default:
		return nil
	}
	p.sel = 0
	m.palFilter()
	return nil
}

// paletteLines draws the palette over the body: the input line, then the results.
func (m *model) paletteLines(n int) []string {
	p := m.pal
	w := m.w
	out := []string{"  " + sAccent.Render(markPrompt+" ") + sText.Render(fit(p.query+"_", w-4)), ""}
	kinds := map[palKind]string{palCommand: T("command"), palMod: "Mod", palEnv: T("environment")}
	rows := max(n-len(out), 1)
	off := max(0, p.sel-rows+1)
	for i := off; i < len(p.results) && len(out) < n; i++ {
		it := p.results[i]
		tag := kinds[it.kind]
		tw := width.StringWidth(tag)
		name := it.name
		if it.kind == palCommand && it.alias != "" && it.alias != it.name {
			name += "  " + sDimS.Render(it.alias)
		}
		left := w - 6 - tw - 2
		line := clip(name, left)
		if i == p.sel {
			out = append(out, "  "+sAccent.Render(markSel)+" "+sSel.Render(fit(stripANSIWidth(name), left))+"  "+sDimS.Render(tag))
			continue
		}
		out = append(out, "    "+line+"  "+sDimS.Render(tag))
	}
	if len(p.results) == 0 {
		out = append(out, "    "+sDimS.Render(fit(T("nothing matches"), w-4)))
	}
	return out
}

// jumpTo selects a mod in the column that holds it, clearing that column's search when the
// search hides it.
func (m *model) jumpTo(id string) tea.Cmd {
	m.tab, m.full, m.insp = tabList, false, nil
	for k := range m.cols {
		c := &m.cols[k]
		idx := -1
		for i := range c.items {
			if c.items[i].id == id {
				idx = i
			}
		}
		if idx < 0 {
			continue
		}
		in := false
		for _, v := range c.view {
			if v == idx {
				in = true
			}
		}
		if !in {
			c.filter = ""
			m.refilterCol(colKind(k), id)
		} else {
			m.refilterCol(colKind(k), id)
		}
		m.focus = colKind(k)
		c.scroll(m.rowsShown() - 1)
		m.detailRef = ""
		return m.requestDetail()
	}
	return nil
}
