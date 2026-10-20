package tui

import (
	"fmt"
	"strings"

	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// The command palette (06 §8): ctrl+k, a "❯" input line over a result list in the style of
// magpie's picker. It finds commands (by their Chinese name or English alias), mods (enter
// jumps to the mod) and the other environments. Ranking: prefix > substring > subsequence
// > description; ties go to commands, then the shorter name.

type palKind int

const (
	palCommand palKind = view.PalCommand
	palEnv     palKind = view.PalEnv
	palMod     palKind = view.PalMod
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

// palRun: what each shared command (view.Commands) does here, with the key that also does it.
func (m *model) palRun(id string) func(m *model) tea.Cmd {
	list := func(key string) func(m *model) tea.Cmd {
		return func(m *model) tea.Cmd { m.tab = tabList; return m.listKey(key) }
	}
	switch id {
	case "sort":
		return func(m *model) tea.Cmd { m.tab = tabList; return m.sortPreview() }
	case "save":
		return func(m *model) tea.Cmd { m.tab = tabList; return m.savePreview() }
	case "launch":
		return list("L")
	case "undo":
		return list("u")
	case "redo":
		return list("ctrl+r")
	case "needs":
		return func(m *model) tea.Cmd { m.tab = tabProblems; return m.ensureProblems() }
	case "changes":
		return func(m *model) tea.Cmd { m.tab = tabChanges; m.loadChanges(); return nil }
	case "rescan":
		return func(m *model) tea.Cmd { return m.startScan() }
	case "analyze":
		return func(m *model) tea.Cmd { return m.startAnalyze() }
	case "log":
		return func(m *model) tea.Cmd { return m.chooseLog() }
	case "env":
		return func(m *model) tea.Cmd { return m.chooseEnv() }
	case "restore-all":
		return list("T")
	case "keep-official":
		return func(m *model) tea.Cmd {
			if m.verb(view.KeepOfficial(m.app)) {
				m.clearSel()
			}
			return m.requestDetail()
		}
	case "filter":
		return list("/")
	case "details":
		return list("enter")
	case "tech":
		return list("i")
	case "tags":
		return list("#")
	case "cli":
		return func(m *model) tea.Cmd {
			m.tab = tabList
			return m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
		}
	case "packs":
		// The packs as text (10 §8); build and play them with the commands shown.
		return func(m *model) tea.Cmd {
			var b strings.Builder
			rows, others, err := view.PackRows(m.app)
			if err != nil {
				b.WriteString(err.Error() + "\n")
			}
			if others > 0 {
				b.WriteString(Tf("%d packs belong to other environments (archo pack list)", others) + "\n\n")
			}
			for _, r := range rows {
				fmt.Fprintf(&b, "%s\n  %s · %s\n  : archo pack launch %q\n\n", r.Name, r.Facts, r.Played, r.Name)
			}
			if len(rows) == 0 {
				b.WriteString(T("no packs yet: a pack fixes the current list — every mod as it is now, without the files of other game versions — and plays with its own saves") + "\n\n")
			}
			b.WriteString(": archo pack build <name>\n")
			m.openPanel(T("packs"), b.String(), nil)
			return nil
		}
	case "settings":
		// The settings page as text (10 §7); each value says the command that changes it.
		return func(m *model) tea.Cmd {
			var b strings.Builder
			for _, g := range append(view.EnvGroups(m.app, m.st), view.SettingsPage(m.app, m.st)...) {
				b.WriteString(g.Title + "\n")
				for _, it := range g.Items {
					fmt.Fprintf(&b, "  %s: %s\n", it.Label, it.Value)
					if len(it.Argv) > 0 {
						fmt.Fprintf(&b, "    : archo %s\n", strings.Join(it.Argv, " "))
					}
					for _, a := range it.Actions {
						fmt.Fprintf(&b, "    %s  : archo %s\n", a.Label, strings.Join(a.Argv, " "))
					}
				}
				b.WriteString("\n")
			}
			m.openPanel(T("settings"), b.String(), nil)
			return nil
		}
	case "help":
		return func(m *model) tea.Cmd { m.openPanel(T("help"), T(helpText), nil); return nil }
	case "quit":
		return func(m *model) tea.Cmd { m.quitting = true; return tea.Quit }
	}
	return func(m *model) tea.Cmd { return nil }
}

// palCommands: every command of the main screen.
func (m *model) palCommands() []palItem {
	var out []palItem
	for _, c := range view.Commands {
		out = append(out, palItem{kind: palCommand, name: c.Name(), alias: c.En, desc: c.Desc, run: m.palRun(c.ID)})
	}
	return out
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
			out = append(out, palItem{kind: palMod, name: it.name, desc: it.pkg + " " + it.v.Author, run: func(m *model) tea.Cmd { return m.jumpTo(id) }})
		}
	}
	return out
}

// palScore ranks one candidate for q (view.PalScore).
func palScore(q string, it palItem) int {
	return view.PalScore(q, view.PalEntry{Name: it.name, Alias: it.alias, Desc: it.desc})
}

func (m *model) palFilter() {
	p := m.pal
	all := m.palAll()
	entries := make([]view.PalEntry, len(all))
	for i, it := range all {
		entries[i] = view.PalEntry{Kind: int(it.kind), ID: strconv.Itoa(i), Name: it.name, Alias: it.alias, Desc: it.desc}
	}
	p.results = p.results[:0]
	for _, e := range view.PalRank(p.query, entries) {
		n, _ := strconv.Atoi(e.ID)
		it := all[n]
		it.score = e.Score
		p.results = append(p.results, it)
	}
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
	case tea.KeyUp, tea.KeyLeft, tea.KeyCtrlP: // 06 §13.4: in a popup both pairs of arrows choose
		p.sel = max(p.sel-1, 0)
		return nil
	case tea.KeyDown, tea.KeyRight, tea.KeyCtrlN, tea.KeyTab:
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
