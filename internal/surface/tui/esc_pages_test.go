package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// escPage is one state of one of the TUI's pages besides the editor (06 §9.1), set up from
// the editor as a player gets there.
type escPage struct {
	name  string
	page  int // the page the state is on
	setup func(m *model)
}

var ctrlK = tea.KeyMsg{Type: tea.KeyCtrlK}

// runAlias runs a palette command by its alias, as typing it and enter does.
func runAlias(m *model, alias string) {
	m.update(ctrlK)
	typeIn(m, alias)
	press(m, "enter")
}

var escPages = []escPage{
	{"能不能跑", tabProblems, func(m *model) { press(m, "@") }},
	{"能不能跑，help", tabProblems, func(m *model) { pressAll(m, "@", "?") }},
	{"能不能跑，palette", tabProblems, func(m *model) { press(m, "@"); m.update(ctrlK) }},
	{"能不能跑，an item's keys armed", tabProblems, func(m *model) {
		press(m, "@")
		m.arm("x", "press x again", func(*model) tea.Cmd { return nil })
	}},
	{"游戏日志，choose a log", tabList, func(m *model) { press(m, "#") }},
	{"Mod 库存 › 全部", tabLibrary, func(m *model) { press(m, "^") }},
	{"Mod 库存 › 全部，filter open", tabLibrary, func(m *model) { pressAll(m, "^", `\`) }},
	{"Mod 库存 › 全部，search typed", tabLibrary, func(m *model) { pressAll(m, "^", "/"); typeIn(m, "a") }},
	{"Mod 库存 › 全部，filters set, filter open", tabLibrary, func(m *model) { pressAll(m, "^", "/"); typeIn(m, "a"); pressAll(m, "enter", `\`) }},
	{"Mod 库存 › 全部，selection + details over the page", tabLibrary, func(m *model) { pressAll(m, "^", "shift+down", "shift+down", "enter") }},
	{"Mod 库存 › 全部，selection + filters + details", tabLibrary, func(m *model) {
		pressAll(m, "^", "/")
		typeIn(m, "a")
		pressAll(m, "enter", "shift+down", "enter")
	}},
	{"Mod 库存 › 全部，filters set + selection + details + filter open", tabLibrary, func(m *model) {
		pressAll(m, "^", "/")
		typeIn(m, "a")
		pressAll(m, "enter", "shift+down", "enter", `\`)
	}},
	{"Mod 库存 › 全部，focus in a copy's details", tabLibrary, func(m *model) {
		press(m, "^")
		for i := 0; i < 4 && !m.libr.detail; i++ {
			pressAll(m, "right", "right")
			if !m.libr.detail {
				press(m, "down")
			}
		}
	}},
	{"Mod 库存 › 全部，row menu", tabLibrary, func(m *model) { press(m, "^"); pressKey(m, tea.KeyF10) }},
	{"Mod 库存 › 全部，help", tabLibrary, func(m *model) { pressAll(m, "^", "?") }},
	{"Mod 库存 › 订阅与更新", tabChanges, func(m *model) { press(m, "c") }},
	{"Mod 库存 › 订阅与更新，one mod's + help", tabChanges, func(m *model) {
		m.openChangesOf(m.current().id, m.current().name)
		press(m, "?")
	}},
	{"Mod 库存 › 搜索文件", tabSearch, func(m *model) { runAlias(m, "search files") }},
	{"Mod 库存 › 搜索文件，searched + back in the box", tabSearch, func(m *model) {
		runAlias(m, "search files")
		typeIn(m, "Harmony")
		pressAll(m, "enter", "/")
	}},
	{"排序规则", tabRules, func(m *model) { runAlias(m, "load-order rules") }},
	{"排序规则，the form open", tabRules, func(m *model) { press(m, "r") }},
	{"启动", tabHome, func(m *model) { press(m, "!") }},
	{"整合包", tabList, func(m *model) { press(m, "%") }},
	{"贴图优化", tabList, func(m *model) { press(m, "&") }},
	{"设置", tabList, func(m *model) { runAlias(m, "settings") }},
	{"卸载，the typed confirmation", tabList, func(m *model) { runAlias(m, "uninstall"); press(m, "enter") }},
	{"方案，help", tabList, func(m *model) { press(m, "?") }},
}

// atBase: the page shown has nothing over it, nothing selected and no open filter (06 §11
// T20). Filters applied with the bar closed are the page's own state (a further esc clears
// them). esc on a page with nothing over it goes back to the editor (the TUI has no
// sidebar), which is the editor's base.
func atBase(m *model, page int) bool {
	l := &m.libr
	clean := !m.full && !m.detailFocus && m.insp == nil && m.menu == nil && m.popupName() == "" && m.selCount() == 0 &&
		!l.full && !l.detail && len(l.marked) == 0 && !l.bar.open && !l.bar.typing &&
		(m.tab != tabChanges || m.chg.of == "") && !m.fsTyping() && (m.tab != tabRules || m.rules.form == nil)
	return clean && (m.tab == page || m.tab == tabList)
}

// T20 on the pages besides the editor (G5's TUI half): esc reaches the base in at most 3.
func TestT20EscBackPages(t *testing.T) {
	t.Parallel()
	e, _ := v3Fixture(t)
	e.WriteFile(filepath.Join(e.UserData, "Player.log"), "RimWorld 1.6.4633 rev1261\n") // 游戏日志 has a log to choose
	e.WriteFile(filepath.Join(e.Root, "LocalAppData", "RimArcho", "steamcmd", "steamcmd.exe"), "MZ")
	for _, s := range escPages {
		t.Run(s.name, func(t *testing.T) {
			m := direct(t, e, 160, 40)
			s.setup(m)
			if m.app.Uninstalled {
				t.Fatal("uninstalled")
			}
			for i := 0; ; i++ {
				if atBase(m, s.page) {
					t.Logf("%d esc (tab %d)", i, m.tab)
					break
				}
				if i == 3 {
					t.Fatalf("not back after 3 esc (tab %d, popup %q, library full %v detail %v marked %d bar %v):\n%s",
						m.tab, m.popupName(), m.libr.full, m.libr.detail, len(m.libr.marked), m.libr.bar.open,
						strings.Join(strings.Split(ansi.Strip(m.View()), "\n")[:3], "\n"))
				}
				press(m, "esc")
			}
		})
	}
}

// The states really are built: each named overlay is there before esc.
func TestT20PagesBuilt(t *testing.T) {
	t.Parallel()
	e, _ := v3Fixture(t)
	e.WriteFile(filepath.Join(e.UserData, "Player.log"), "RimWorld 1.6.4633 rev1261\n")
	e.WriteFile(filepath.Join(e.Root, "LocalAppData", "RimArcho", "steamcmd", "steamcmd.exe"), "MZ")
	for _, s := range escPages {
		m := direct(t, e, 160, 40)
		s.setup(m)
		built := m.tab == s.page && (s.page != tabList || m.popupName() != "")
		if strings.Contains(s.name, ", ") && atBase(m, s.page) {
			built = false
		}
		if !built {
			t.Errorf("%s: not built (tab %d, popup %q)", s.name, m.tab, m.popupName())
		}
	}
}
