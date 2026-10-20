package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

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

func placedText(ps []int) string { return view.PlacedText(ps) }

func (m *model) positions(ids []string) []int { return view.Positions(m.st, ids) }

// object: the shared verb object of the selection (06 §13.6).
func object(sel []*item) view.Object {
	return view.Object{IDs: objIDs(sel), Refs: refsOf(sel), What: named(sel)}
}

// verb runs a shared verb and shows its result.
func (m *model) verb(r *view.Result, err error) bool {
	if !m.applied(err) {
		return false
	}
	m.say(sayOK, r.Text)
	m.placed = r.Placed
	return true
}

// space enables the object where the load order wants it (Disabled), disables it
// (Enabled), or puts it back where it was (Staging).
func (m *model) space() tea.Cmd {
	sel := m.selection()
	if len(sel) == 0 {
		return nil
	}
	keep := m.col().curID()
	if !m.verb(view.Toggle(m.app, view.Column(m.focus), object(sel))) {
		return nil
	}
	m.clearSel()
	m.refilterCol(m.focus, keep)
	return m.requestDetail()
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
	keep := m.col().curID()
	if !m.verb(view.Stage(m.app, view.Column(m.focus), object(sel))) {
		return nil
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
	keep := m.cols[cActive].curID()
	if !m.verb(view.MoveBy(m.app, object(sel), d)) {
		return nil
	}
	m.refilterCol(cActive, keep)
	return m.requestDetail()
}

// moveTo moves enabled mods to a 1-based place in the list without them, as one block.
func (m *model) moveTo(ids []string, what string, at int) tea.Cmd {
	keep := m.cols[cActive].curID()
	if !m.verb(view.Place(m.app, view.Active, view.Object{IDs: ids, What: what}, at)) {
		return nil
	}
	m.refilterCol(cActive, keep)
	return m.requestDetail()
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

// posOptions: view.PosOptions for the input's text.
func (m *model) posOptions() {
	p := m.pos
	p.opts = nil
	for _, o := range view.PosOptions(m.app, m.st, p.ids, p.text, p.before) {
		p.opts = append(p.opts, posOpt{label: o.Label, note: o.Note, at: o.At, edge: o.Edge})
	}
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
	if !m.verb(view.Place(m.app, view.Column(p.from), view.Object{IDs: p.ids, Refs: p.refs, What: p.what}, at)) {
		return nil
	}
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
