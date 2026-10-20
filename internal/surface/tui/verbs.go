package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Verbs (06 §13.6): each acts at once on the object, the selection or else the cursor's
// row, and is one undo step. None opens a mode; m opens the position input, a popup.

// objIDs: the instances of the rows (missing entries have none).
func objIDs(its []*item) []string {
	var out []string
	for _, it := range its {
		if it.id != "" {
			out = append(out, it.id)
		}
	}
	return out
}

// placedText: "placed at number 352 · m changes it" for the mods now at positions ps.
func placedText(ps []int) string {
	if len(ps) == 0 {
		return ""
	}
	sort.Ints(ps)
	where := fmt.Sprint(ps[0])
	if len(ps) > 1 {
		where = fmt.Sprintf("%d–%d", ps[0], ps[len(ps)-1])
	}
	return Tf("placed at position %s · m changes it", where)
}

func (m *model) positions(ids []string) []int {
	var ps []int
	for _, id := range ids {
		if p := m.st.Position(id); p > 0 {
			ps = append(ps, p)
		}
	}
	return ps
}

// space enables the object where the load order wants it (Disabled), disables it
// (Enabled), or puts it back where it was (Staging).
func (m *model) space() tea.Cmd {
	sel := m.selection()
	if len(sel) == 0 {
		return nil
	}
	ids, refs, what := objIDs(sel), refsOf(sel), named(sel)
	keep := m.col().curID()
	switch m.focus {
	case cInactive:
		at, err := m.app.SuggestAt(ids)
		if err == nil {
			_, err = m.app.Enable(refs, app.Placement{At: at})
		}
		if !m.applied(err) {
			return nil
		}
		m.say(sayOK, placedText(m.positions(ids)))
		m.placed = ids
	case cActive:
		m.act(Tf("disabled %s", what), append([]string{"disable"}, refs...)...)
	case cStaging:
		if !m.applied(m.restore(refs)) {
			return nil
		}
		m.say(sayOK, placedText(m.positions(ids)))
		m.placed = ids
	}
	m.clearSel()
	m.refilterCol(m.focus, keep)
	return m.requestDetail()
}

// restore puts staged mods back where they were (one undo step).
func (m *model) restore(refs []string) error {
	_, err := m.app.Restore(refs, app.Placement{})
	return err
}

// applied reloads after an edit and reports a failure on the status line.
func (m *model) applied(err error) bool {
	m.reload()
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
		return false
	}
	return true
}

// stageKey: t stages the object; in Staging it puts it back where it was (06 §13.6).
func (m *model) stageKey() tea.Cmd {
	sel := m.selection()
	ids := objIDs(sel)
	if len(ids) == 0 {
		return nil
	}
	refs, what := refsOf(sel), named(sel)
	keep := m.col().curID()
	if m.focus == cStaging {
		if !m.applied(m.restore(refs)) {
			return nil
		}
		m.say(sayOK, placedText(m.positions(ids)))
		m.placed = ids
	} else {
		_, err := m.app.Stage(refs)
		if !m.applied(err) {
			return nil
		}
		m.say(sayOK, Tf("staged %s", what))
	}
	m.clearSel()
	m.refilterCol(m.focus, keep)
	return m.requestDetail()
}

// moveBy: J K move the object d places within Enabled as one block (a selection that is
// not contiguous first gathers at its first mod's place).
func (m *model) moveBy(d int) tea.Cmd {
	sel := m.selection()
	ids := objIDs(sel)
	if m.focus != cActive || len(ids) == 0 {
		m.say(sayInfo, T("only enabled mods can be moved"))
		return nil
	}
	ps := m.positions(ids)
	sort.Ints(ps)
	rest := len(m.st.List) - len(ids)
	at := max(1, min(ps[0]+d, rest+1))
	return m.moveTo(ids, named(sel), at)
}

// moveTo moves enabled mods to a 1-based place in the list without them, as one block.
func (m *model) moveTo(ids []string, what string, at int) tea.Cmd {
	keep := m.cols[cActive].curID()
	_, err := m.app.MoveBlock(ids, at)
	if !m.applied(err) {
		return nil
	}
	m.say(sayOK, m.nowAt(what, ids))
	m.refilterCol(cActive, keep)
	return m.requestDetail()
}

// nowAt: "Alpha is now number 3" / "3 mods are now at 3–5".
func (m *model) nowAt(what string, ids []string) string {
	ps := m.positions(ids)
	sort.Ints(ps)
	if len(ps) == 0 {
		return ""
	}
	if len(ps) == 1 {
		return Tf("%s is now number %d", what, ps[0])
	}
	return Tf("%s are now at %d–%d", what, ps[0], ps[len(ps)-1])
}

// ---- the position input (06 §13.7) ----

type posOpt struct {
	label string // "after  Harmony"
	note  string // "(number 1)"
	at    int    // 1-based, in the list without the moving mods
	edge  bool   // first / last: drawn on one line
}

type posInput struct {
	ids    []string
	refs   []string
	what   string
	from   colKind // where they are: Enabled moves, the others are enabled at the place
	text   string
	before bool // the name candidates put the mods before the match (tab)
	opts   []posOpt
	sel    int
}

// openPos opens the position input for the object, or for the mods space just placed.
func (m *model) openPos(placed []string) tea.Cmd {
	p := &posInput{from: m.focus}
	if len(placed) > 0 {
		p.ids, p.from = placed, cActive
		if len(placed) == 1 {
			p.what = m.st.Ix.ByID[placed[0]].DisplayName()
		} else {
			p.what = plural(len(placed), "mod")
		}
	} else {
		sel := m.selection()
		p.ids, p.what = objIDs(sel), named(sel)
	}
	if len(p.ids) == 0 {
		return nil
	}
	for _, id := range p.ids {
		p.refs = append(p.refs, m.st.Ix.Ref(id))
	}
	m.pos = p
	m.posOptions()
	return nil
}

// posOptions: a number is "number N"; a name lists the enabled mods it matches ("after X",
// tab makes them "before X"); then the suggested place, then first and last.
func (m *model) posOptions() {
	p := m.pos
	moving := map[string]bool{}
	for _, id := range p.ids {
		moving[id] = true
	}
	type ent struct {
		name string
		pos  int // shown: its number now
		at   int // its place in the list without the moving mods, 1-based
	}
	var rest []ent
	for i, e := range m.st.List {
		if moving[e.Instance] && e.Instance != "" {
			continue
		}
		name := e.Missing
		if mod := m.st.Ix.ByID[e.Instance]; mod != nil {
			name = mod.DisplayName()
		}
		rest = append(rest, ent{name, i + 1, len(rest) + 1})
	}
	n := len(rest)
	p.opts = nil
	q := strings.ToLower(strings.TrimSpace(p.text))
	if v, err := strconv.Atoi(q); err == nil && v > 0 {
		v = min(v, n+1)
		p.opts = append(p.opts, posOpt{label: Tf("position %d", v), at: v})
	} else if q != "" {
		type hit struct {
			e     ent
			score int
		}
		var hits []hit
		for _, e := range rest {
			l := strings.ToLower(e.name)
			s := 0
			switch {
			case strings.HasPrefix(l, q):
				s = 3
			case strings.Contains(l, q):
				s = 2
			case view.Subsequence(q, l):
				s = 1
			}
			if s > 0 {
				hits = append(hits, hit{e, s})
			}
		}
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
		for i, h := range hits {
			if i == 5 {
				break
			}
			o := posOpt{label: T("after") + "  " + h.e.name, note: Tf("(position %d)", h.e.pos), at: h.e.at + 1}
			if p.before {
				o.label, o.at = T("before")+"  "+h.e.name, h.e.at
			}
			p.opts = append(p.opts, o)
		}
	}
	if at, err := m.app.SuggestAt(p.ids); err == nil {
		p.opts = append(p.opts, posOpt{label: Tf("position %d", at), note: T("(suggested)"), at: at})
	}
	p.opts = append(p.opts, posOpt{label: T("first"), at: 1, edge: true}, posOpt{label: T("last"), at: n + 1, edge: true})
	p.sel = 0
}

// posKey: typing filters; ↑↓ ←→ choose (06 §13.4: the arrows move among the options);
// tab switches after / before; enter moves; esc closes.
func (m *model) posKey(k tea.KeyMsg) tea.Cmd {
	p := m.pos
	switch k.Type {
	case tea.KeyEsc:
		m.pos = nil
		return nil
	case tea.KeyUp, tea.KeyLeft, tea.KeyShiftTab:
		p.sel = max(p.sel-1, 0)
		return nil
	case tea.KeyDown, tea.KeyRight:
		p.sel = min(p.sel+1, len(p.opts)-1)
		return nil
	case tea.KeyTab:
		p.before = !p.before
		sel := p.sel
		m.posOptions()
		p.sel = sel
		return nil
	case tea.KeyBackspace:
		if r := []rune(p.text); len(r) > 0 {
			p.text = string(r[:len(r)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		p.text += string(k.Runes)
	case tea.KeyEnter:
		m.pos = nil
		if p.sel < len(p.opts) {
			return m.placeAt(p, p.opts[p.sel].at)
		}
		return nil
	default:
		return nil
	}
	m.posOptions()
	return nil
}

// placeAt applies the position input: enabled mods move, the others are enabled there
// (and leave staging). One undo step.
func (m *model) placeAt(p *posInput, at int) tea.Cmd {
	if p.from == cActive {
		return m.moveTo(p.ids, p.what, at)
	}
	keep := m.col().curID()
	_, err := m.app.Enable(p.refs, app.Placement{At: at})
	if err == nil && p.from == cStaging {
		_, err = m.app.Unstage(p.refs)
	}
	if !m.applied(err) {
		return nil
	}
	m.say(sayOK, m.nowAt(p.what, p.ids))
	m.clearSel()
	m.refilterCol(m.focus, keep)
	return m.requestDetail()
}

// posLines draws the position input, w wide, for the bottom of the body.
func (m *model) posLines(w int) []string {
	p := m.pos
	head := " " + Tf("move %s to", p.what) + " "
	hw := min(width.StringWidth(head), w/2)
	out := []string{sSecond.Render(fit(head, hw)) + sAccent.Render(markPrompt+" ") + sText.Render(fit(p.text+"▌", max(w-hw-2, 1)))}
	var edges []string
	for i, o := range p.opts {
		if o.edge {
			if i == p.sel {
				edges = append(edges, sAccent.Render(markSel+" ")+sSel.Render(o.label))
			} else {
				edges = append(edges, "  "+sText.Render(o.label))
			}
			continue
		}
		mark, st := "   ", sText
		if i == p.sel {
			mark, st = " "+sAccent.Render(markSel)+" ", sSel
		}
		lw := min(width.StringWidth(o.label), max(w-3-width.StringWidth(o.note)-2, 4))
		line := mark + st.Render(fit(o.label, lw))
		if o.note != "" {
			line += "  " + sDimS.Render(o.note)
		}
		out = append(out, line)
	}
	if len(edges) > 0 {
		out = append(out, " "+strings.Join(edges, sDimS.Render("  "+markDot)))
	}
	return out
}
