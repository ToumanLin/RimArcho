package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 排序规则 (06 §2.5, §9.1, 04 §6.1): the editor's rules card as a screen of its own, on the
// same view.BuildRules / SaveRule / RemoveRule / RestoreRule. The window folds it into the
// editor's left column; the terminal's third column is too narrow for its sentences. Rules
// are global; the screen shows the ones with a mod enabled in the open list (只看当前列表,
// the default) or all of them (a). r on a mod in the editor opens it with the form for
// that mod; enter changes the rule under the cursor, Del removes it (u right after puts it
// back), o shows its mod in the list, esc goes back to the editor. Rules are not list
// edits: they are not in the list's undo stack (u here is only the removal's undo).

// rulesScopeKey keeps 只看当前列表 / 全部排序规则 across starts, as the window keeps it.
const rulesScopeKey = "ui.tui.rules.scope"

type rulesState struct {
	data    *view.Rules
	all     bool
	cursor  int
	off     int
	form    *ruleForm
	removed *store.UserRule // the rule Del took off; u right after puts it back
}

// ruleForm adds a rule (id 0) or changes one in place.
type ruleForm struct {
	id     int64
	kind   string
	a      view.RuleMod
	b      *view.RuleMod
	typed  string // the other mod's box, until a candidate is taken
	note   string
	target string // an ignore rule keeps its constraint: only the note changes
	text   string
	field  int // 0 the kind, 1 the other mod, 2 the note
	cands  []view.RuleMod
	candAt int
}

// openRules shows 排序规则; mod (when not nil) opens the form for a new rule about it.
func (m *model) openRules(mod *item) tea.Cmd {
	m.panel, m.insp, m.full = nil, nil, false
	r := &m.rules
	r.all = m.app.Store.Meta(rulesScopeKey) == "all"
	r.form, r.removed = nil, nil
	if !m.loadRules() {
		return nil
	}
	m.tab = tabRules
	if mod != nil {
		r.form = &ruleForm{kind: "after", a: view.RuleMod{Ref: mod.ref, ID: mod.id, Name: mod.name}, field: 1}
	}
	return nil
}

func (m *model) loadRules() bool {
	d, err := view.BuildRules(m.app)
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return false
	}
	m.rules.data = d
	return true
}

// shown are the rules the screen lists: a rule being changed stays until its form closes.
func (r *rulesState) shown() []view.RuleRow {
	if r.data == nil {
		return nil
	}
	var out []view.RuleRow
	for _, row := range r.data.Rows {
		if r.all || row.InList || r.form != nil && r.form.id == row.ID {
			out = append(out, row)
		}
	}
	return out
}

func (r *rulesState) kind(k string) view.RuleKind {
	if r.data != nil {
		for _, x := range r.data.Kinds {
			if x.Kind == k {
				return x
			}
		}
	}
	return view.RuleKind{Kind: k, Label: k}
}

// kindLabel is a kind as the form names it: the sentence without the first mod, "…" for
// the second (the window's kindLabel).
func kindLabel(k view.RuleKind) string {
	return strings.TrimSpace(strings.Replace(strings.Replace(k.Label, "%s", "", 1), "%s", "…", 1))
}

// ---- keys ----

// rulesTyping: the form is open and has the keys.
func (m *model) rulesTyping() bool {
	return m.tab == tabRules && m.rules.form != nil // the form has every key, letters too
}

func (m *model) rulesKey(k tea.KeyMsg) tea.Cmd {
	r := &m.rules
	if r.form != nil {
		return m.ruleFormKey(k)
	}
	rows := r.shown()
	undo := r.removed
	r.removed = nil // u puts a removed rule back only right after
	switch k.String() {
	case "esc":
		m.tab = tabList
		return m.requestDetail()
	case "up", "k":
		r.cursor = max(r.cursor-1, 0)
	case "down", "j":
		r.cursor = min(r.cursor+1, max(len(rows)-1, 0))
	case "pgup":
		r.cursor = max(r.cursor-max(m.h-8, 1), 0)
	case "pgdown":
		r.cursor = min(r.cursor+max(m.h-8, 1), max(len(rows)-1, 0))
	case "home":
		r.cursor = 0
	case "end":
		r.cursor = max(len(rows)-1, 0)
	case "a":
		r.all = !r.all
		v := ""
		if r.all {
			v = "all"
		}
		_ = m.app.Store.SetMeta(rulesScopeKey, v)
		r.cursor, r.off = 0, 0
	case "r":
		it := m.current()
		if it == nil || it.id == "" {
			m.say(sayInfo, T("choose a mod in the lists first"))
			return nil
		}
		r.form = &ruleForm{kind: "after", a: view.RuleMod{Ref: it.ref, ID: it.id, Name: it.name}, field: 1}
	case "enter":
		if r.cursor < len(rows) {
			row := rows[r.cursor]
			f := &ruleForm{id: row.ID, kind: row.Kind, b: row.B, note: row.Note, target: row.Target, text: row.Text, field: 2}
			if row.A != nil {
				f.a = *row.A
			}
			if row.B != nil {
				f.typed = row.B.Name
			}
			r.form = f
		}
	case "delete":
		if r.cursor < len(rows) {
			text, removed, err := view.RemoveRule(m.app, rows[r.cursor].ID)
			if err != nil {
				m.say(sayFail, m.humanize(err.Error()))
				return nil
			}
			r.removed = removed
			m.rulesChanged()
			m.say(sayOK, text+"  "+Tf("u %s", T("undo")))
		}
	case "u", "ctrl+z":
		if undo == nil {
			m.say(sayInfo, T("nothing to undo"))
			return nil
		}
		text, err := view.RestoreRule(m.app, undo)
		if err != nil {
			m.say(sayFail, m.humanize(err.Error()))
			return nil
		}
		m.rulesChanged()
		m.say(sayOK, text)
	case "o":
		// the rule's mod in the list, as a click on its name in the window
		if r.cursor < len(rows) {
			for _, mod := range []*view.RuleMod{rows[r.cursor].A, rows[r.cursor].B} {
				if mod != nil && mod.ID != "" {
					return m.jumpTo(mod.ID)
				}
			}
			m.say(sayInfo, T("a mod that is no longer installed"))
		}
	}
	return nil
}

// ruleFormKey: tab and shift+tab go between the kind, the other mod and the note; ← → on
// the kind change it; the other mod's box offers candidates (↑↓, enter or tab takes one);
// enter saves; esc closes the candidates, then the form.
func (m *model) ruleFormKey(k tea.KeyMsg) tea.Cmd {
	r := &m.rules
	f := r.form
	two := f.kind != "ignore" && r.kind(f.kind).Two
	fields := []int{0, 1, 2}
	switch {
	case f.kind == "ignore":
		fields = []int{2}
	case !two:
		fields = []int{0, 2}
	}
	at := 0
	for i, x := range fields {
		if x == f.field {
			at = i
		}
	}
	step := func(d int) { f.field, f.cands = fields[(at+d+len(fields))%len(fields)], nil }
	switch k.Type {
	case tea.KeyEsc:
		if len(f.cands) > 0 {
			f.cands = nil
			return nil
		}
		r.form = nil
		return nil
	case tea.KeyShiftTab:
		step(-1)
		return nil
	case tea.KeyTab:
		if f.field == 1 && len(f.cands) > 0 {
			m.rulePick()
			return nil
		}
		step(1)
		return nil
	case tea.KeyEnter:
		if f.field == 1 && len(f.cands) > 0 {
			m.rulePick()
			return nil
		}
		return m.ruleSave()
	case tea.KeyUp, tea.KeyDown:
		if f.field == 1 && len(f.cands) > 0 {
			d := 1
			if k.Type == tea.KeyUp {
				d = -1
			}
			f.candAt = (f.candAt + d + len(f.cands)) % len(f.cands)
			return nil
		}
		if k.Type == tea.KeyUp {
			step(-1)
		} else {
			step(1)
		}
		return nil
	case tea.KeyLeft, tea.KeyRight:
		if f.field == 0 && r.data != nil && len(r.data.Kinds) > 0 {
			i := 0
			for j, x := range r.data.Kinds {
				if x.Kind == f.kind {
					i = j
				}
			}
			d := 1
			if k.Type == tea.KeyLeft {
				d = -1
			}
			n := len(r.data.Kinds)
			f.kind = r.data.Kinds[(i+d+n)%n].Kind
		}
		return nil
	}
	edit := func(fn func(string) string) {
		switch f.field {
		case 1:
			f.typed, f.b = fn(f.typed), nil
			list, err := view.RuleCandidates(m.app, f.typed, f.a.Ref)
			if err != nil {
				m.say(sayFail, m.humanize(err.Error()))
			}
			f.cands, f.candAt = list, 0
		case 2:
			f.note = fn(f.note)
		}
	}
	switch k.Type {
	case tea.KeyRunes, tea.KeySpace:
		s := string(k.Runes)
		if k.Type == tea.KeySpace {
			s = " "
		}
		edit(func(v string) string { return v + s })
	case tea.KeyBackspace:
		edit(func(v string) string {
			if r := []rune(v); len(r) > 0 {
				return string(r[:len(r)-1])
			}
			return v
		})
	case tea.KeyCtrlU:
		edit(func(string) string { return "" })
	}
	return nil
}

// rulePick takes the candidate under the cursor as the other mod and goes on to the note.
func (m *model) rulePick() {
	f := m.rules.form
	c := f.cands[f.candAt]
	f.b, f.typed, f.cands, f.field = &c, c.Name, nil, 2
}

// ruleSave adds or changes the rule; the editor reloads (the order problems, the row icons
// and 能不能跑's number follow), and nothing is sorted (06 §2.5).
func (m *model) ruleSave() tea.Cmd {
	r := &m.rules
	f := r.form
	spec := app.RuleSpec{Kind: f.kind, A: f.a.Ref, Note: f.note}
	if f.kind == "ignore" {
		spec = app.RuleSpec{Kind: "ignore", Target: f.target, Note: f.note}
	} else if r.kind(f.kind).Two {
		if f.b == nil {
			m.say(sayFail, T("choose the other mod"))
			f.field = 1
			return nil
		}
		spec.B = f.b.Ref
	}
	text, err := view.SaveRule(m.app, f.id, spec)
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return nil
	}
	r.form = nil
	m.rulesChanged()
	m.say(sayOK, text)
	return nil
}

// rulesChanged reads the rules again, and the editor with them.
func (m *model) rulesChanged() {
	m.reload()
	m.prob.loaded = false
	m.loadRules()
	m.rules.cursor = max(0, min(m.rules.cursor, len(m.rules.shown())-1))
}

// ---- drawing ----

func (m *model) rulesLines(n int) []string {
	r := &m.rules
	w := max(m.w-4, 20)
	seg := func(on bool, s string) string {
		if on {
			return sSel.Render(s)
		}
		return sDimS.Render(s)
	}
	out := []string{"  " + seg(!r.all, T("only the current list")) + sDimS.Render("   ") + seg(r.all, T("all load-order rules")) + sDimS.Render("   a"), ""}
	if r.form != nil && r.form.id == 0 {
		out = append(out, m.ruleFormLines(w)...)
		out = append(out, "")
	}
	rows := r.shown()
	var body []string
	at := 0 // the body line of the cursor's rule
	for i, row := range rows {
		if r.form != nil && r.form.id == row.ID {
			body = append(body, m.ruleFormLines(w)...)
			continue
		}
		cur := r.form == nil && i == r.cursor
		if cur {
			at = len(body)
		}
		lead, st := "    ", sText
		if cur {
			lead, st = "  "+sAccent.Render(markSel)+" ", sSel
		}
		text := row.Text
		if row.A != nil && row.A.ID == "" || row.B != nil && row.B.ID == "" {
			st = sSecond // a mod that is gone (the window's faint name)
			if cur {
				st = sSel
			}
		}
		body = append(body, lead+st.Render(width.Truncate(text, w-2, "…")))
		if row.Note != "" {
			body = append(body, "      "+sDimS.Render(width.Truncate(row.Note, w-4, "…")))
		}
	}
	if len(rows) == 0 && (r.form == nil || r.form.id != 0) {
		msg := T("no load-order rules yet")
		if !r.all && r.data != nil && len(r.data.Rows) > 0 {
			msg = Tf("no rule names a mod of this list; a shows all %d", len(r.data.Rows))
		}
		body = append(body, "    "+sDimS.Render(msg))
	}
	room := max(n-len(out), 1)
	if at < r.off {
		r.off = at
	}
	if at >= r.off+room-1 {
		r.off = at - room + 2
	}
	r.off = max(0, min(r.off, max(len(body)-room, 0)))
	return append(out, window(body, r.off, room)...)
}

// ruleFormLines draws the form: the mod and the kind, the other mod with its candidates,
// the note.
func (m *model) ruleFormLines(w int) []string {
	r := &m.rules
	f := r.form
	box := func(on bool, text string) string {
		if on {
			return sAccent.Render(markPrompt+" ") + sText.Render(text+"_")
		}
		return sDimS.Render(markPrompt+" ") + sText.Render(text)
	}
	var out []string
	if f.kind == "ignore" {
		out = append(out, "  "+sAccent.Render("┃ ")+sText.Render(width.Truncate(f.text, w-4, "…")))
	} else {
		kind := kindLabel(r.kind(f.kind))
		ks := sText.Render("‹" + kind + "›")
		if f.field == 0 {
			ks = sSel.Render("‹"+kind+"›") + sDimS.Render("  ← →")
		}
		out = append(out, "  "+sAccent.Render("┃ ")+sBold.Render(f.a.Name)+"  "+ks)
		if r.kind(f.kind).Two {
			out = append(out, "  "+sAccent.Render("┃ ")+sDimS.Render(T("the other mod")+"  ")+box(f.field == 1, f.typed))
			for i, c := range f.cands {
				name := c.Name
				pos := ""
				if c.Pos > 0 {
					pos = "  " + sDimS.Render(itoa(c.Pos))
				}
				if i == f.candAt {
					out = append(out, "  "+sAccent.Render("┃ ")+"    "+sAccent.Render(markSel+" ")+sSel.Render(name)+pos)
				} else {
					out = append(out, "  "+sAccent.Render("┃ ")+"      "+sText.Render(name)+pos)
				}
			}
		}
	}
	out = append(out, "  "+sAccent.Render("┃ ")+sDimS.Render(T("note (optional)")+"  ")+box(f.field == 2, f.note))
	return out
}

// rulesPairs: the hint line of 排序规则.
func (m *model) rulesPairs() [][2]string {
	r := &m.rules
	if f := r.form; f != nil {
		pairs := [][2]string{{"tab", T("next field")}}
		switch {
		case f.field == 0:
			pairs = append(pairs, [2]string{"←→", T("kind")})
		case f.field == 1 && len(f.cands) > 0:
			pairs = append(pairs, [2]string{"↑↓", T("choose")}, [2]string{"enter", T("take it")})
		}
		label := T("add")
		if f.id != 0 {
			label = T("save")
		}
		if f.field != 1 || len(f.cands) == 0 {
			pairs = append(pairs, [2]string{"enter", label})
		}
		return append(pairs, [2]string{"esc", T("cancel")})
	}
	pairs := [][2]string{}
	if len(r.shown()) > 0 {
		pairs = append(pairs, [2]string{"enter", T("edit")}, [2]string{"Del", T("delete")}, [2]string{"o", T("show it in the list")})
	}
	pairs = append(pairs, [2]string{"r", T("add a rule")})
	if r.all {
		pairs = append(pairs, [2]string{"a", T("only the current list")})
	} else {
		pairs = append(pairs, [2]string{"a", T("all load-order rules")})
	}
	return append(pairs, [2]string{"esc", T("back")}, [2]string{"?", T("all keys")})
}

func init() {
	view.Register(map[string]string{
		"no load-order rules yet":                          "还没有排序规则",
		"no rule names a mod of this list; a shows all %d": "没有涉及这个列表的规则；按 a 看全部 %d 条",
		"next field": "下一项",
		"take it":    "选定",
	})
}
