package tui

import (
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The ops of a row, the same on the editor and Mod 库存 (06 §3.5, §8.5, view.OpTable): each
// op code has one runner here, so a key, the row menu and Mod 库存's detail all run the same.

// library is the library as of the board's build (the editor's rows ask it for their copy's
// version and disk ops).
func (m *model) library() *app.Library {
	if m.libCache != nil && m.libAt == m.boardSeq {
		return m.libCache
	}
	lib, err := m.app.Library()
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return nil
	}
	m.libCache, m.libAt = lib, m.boardSeq
	return lib
}

// editorOps are the op codes of the editor's cursor row.
func (m *model) editorOps() []string {
	it := m.current()
	if it == nil {
		return nil
	}
	if it.id == "" {
		if m.focus == cActive {
			return []string{view.OpDisable} // a list entry with no installed mod
		}
		return nil
	}
	return view.RowOps(m.library(), it.id, view.Column(m.focus))
}

// runEditorOp runs an op on the editor's object (the selection, else the cursor's row).
func (m *model) runEditorOp(code string) tea.Cmd {
	sel := m.selection()
	if len(sel) == 0 {
		return nil
	}
	switch code {
	case view.OpEnable, view.OpDisable, view.OpPutBack:
		return m.space()
	case view.OpEnableAt, view.OpMoveTo:
		return m.openPos(nil)
	case view.OpStage:
		return m.stageKey()
	case view.OpSwap:
		if it := m.current(); it != nil && it.id != "" {
			m.act(Tf("switched the version of %s", it.name), "swap", it.ref)
		}
		return m.requestDetail()
	case view.OpTags:
		return m.tagKey()
	case view.OpNote:
		return m.noteKey()
	case view.OpFolder:
		if it := m.current(); it != nil {
			return m.openFolderOf(it.id)
		}
		return nil
	case view.OpWorkshop, view.OpSteam:
		if it := m.current(); it != nil {
			return m.openWorkshopPage(m.workshopOf(it.id), code == view.OpSteam)
		}
		return nil
	}
	return m.libOp(code, refsOf(sel), named(sel))
}

// libOp runs an op of the version and disk groups (and the plan ops of Mod 库存) on copies:
// one that changes files shows its dry run first and runs on enter (06 §6).
func (m *model) libOp(code string, refs []string, what string) tea.Cmd {
	o := view.OpOf(code)
	if o.Code == "" {
		m.say(sayFail, "unknown op "+code)
		return nil
	}
	if !o.Confirm {
		return m.libDo(code, refs)
	}
	p, err := view.LibPreview(m.app, code, refs)
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return nil
	}
	pn := &panel{title: o.Label + "  " + what, lines: append([]string{}, p.Lines...)}
	if p.Apply {
		pn.lines = append(pn.lines, "", Tf("enter: %s, esc: cancel", o.Label))
		pn.onEnter = func(m *model) tea.Cmd { return m.libDo(code, refs) }
	} else {
		pn.lines = append(pn.lines, "", T("this cannot be done now, esc goes back"))
	}
	m.panel = pn
	return nil
}

// libDo runs a library action and says what it did; Steam out of reach opens the pages.
func (m *model) libDo(code string, refs []string) tea.Cmd {
	t, err := view.LibDo(m.app, code, refs)
	var pages *view.OpenPages
	if errors.As(err, &pages) {
		for _, u := range pages.URLs {
			m.open(u)
		}
		t, err = pages.Msg, nil
	}
	m.reload()
	m.clearSel()
	if m.tab == tabLibrary {
		m.loadLibrary()
	}
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return nil
	}
	m.say(sayOK, t)
	return m.requestDetail()
}

// delKey is Del: unsubscribe or delete, whichever the row has (06 §3.5).
func (m *model) delKey(ops []string, run func(code string) tea.Cmd) tea.Cmd {
	for _, c := range ops {
		if c == view.OpUnsub || c == view.OpDelete {
			return run(c)
		}
	}
	m.say(sayInfo, T("nothing to unsubscribe or recycle here"))
	return nil
}

// noteKey is n in the editor: the mod's own note (the Lineage's, 06 §4.1).
func (m *model) noteKey() tea.Cmd {
	it := m.current()
	if it == nil || it.id == "" || m.st == nil {
		return nil
	}
	mod := m.st.Ix.ByID[it.id]
	if mod == nil {
		return nil
	}
	ref, name := it.ref, it.name
	m.prompt = &prompt{label: Tf("note on %s", name) + ": ", text: mod.Note, submit: func(m *model, text string) tea.Cmd {
		msg, err := view.SetModNote(m.app, ref, text)
		m.reload()
		if err != nil {
			m.say(sayFail, m.humanize(err.Error()))
			return nil
		}
		m.say(sayOK, msg)
		return m.requestDetail()
	}}
	return nil
}

// selectAll is ctrl+a: every row the focused list shows.
func (m *model) selectAll() tea.Cmd {
	c := m.col()
	for _, vi := range c.view {
		if id := c.items[vi].id; id != "" {
			m.marked[id] = true
		}
	}
	m.sel = selRange{}
	return nil
}

// ---- pages ----

// navKeys: shift+1…8 are the sidebar's entries (06 §3.5); a terminal sends them as the
// characters of the US layout.
var navKeys = map[string]int{"!": 1, "@": 2, "#": 3, "$": 4, "%": 5, "^": 6, "&": 7, "*": 8}

// goPage opens the n-th sidebar entry: 启动 能不能跑 游戏日志 Mod 库存 方案 整合包 贴图优化 环境.
func (m *model) goPage(n int) tea.Cmd {
	m.full, m.insp, m.panel = false, nil, nil
	switch n {
	case 1:
		m.say(sayInfo, T("the start page is in the window only, ctrl+k goes anywhere"))
		return nil
	case 2:
		m.tab = tabProblems
		return m.ensureProblems()
	case 3:
		return m.palRun("log")(m)
	case 4:
		return m.openLibrary()
	case 5:
		m.tab = tabList
		return m.requestDetail()
	case 6:
		return m.palRun("packs")(m)
	case 7:
		return m.texturesPanel()
	case 8:
		return m.chooseEnv()
	}
	return nil
}

// subPage is tab / shift+tab: the next page under the same sidebar entry (Mod 库存 › 全部 ↔
// 变化; the plans under 方案, which makes that plan current).
func (m *model) subPage(d int) tea.Cmd {
	switch m.tab {
	case tabLibrary:
		return m.openChangesOf("", "")
	case tabChanges:
		return m.openLibrary()
	case tabList:
		rows, err := view.PlanRows(m.app)
		if err != nil || len(rows) < 2 {
			m.say(sayInfo, T("one list only, P makes another"))
			return nil
		}
		at := 0
		for i, r := range rows {
			if r.Open {
				at = i
			}
		}
		return m.usePlan(rows[(at+d+len(rows))%len(rows)].ID)
	}
	return nil
}

// helpPage is `?`: the keys of the page shown (view.KeyHelp).
func (m *model) helpPage() tea.Cmd {
	page := "other"
	switch m.tab {
	case tabList:
		page = view.EditorPage
	case tabLibrary:
		page = view.LibraryPage
	case tabChanges:
		page = view.ChangesPage
	}
	rows := view.KeyHelp(page, true)
	w := 0
	for _, r := range rows {
		w = max(w, width.StringWidth(r.Keys))
	}
	var b strings.Builder
	b.WriteString(T("the arrows only move; keys that change things act at once, u undoes") + "\n\n")
	for _, r := range rows {
		b.WriteString(width.FillRight(r.Keys, w+3) + r.What + "\n")
	}
	m.openPanel(T("all keys"), b.String())
	return nil
}
