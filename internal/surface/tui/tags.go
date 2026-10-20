package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// The tag bar (06 §12.3): # opens one line of candidates for the selected mods (or the one
// under the cursor; 06 §13.6). ←→ choose, enter applies, esc closes. Left of the first candidate is
// "✎ type…", where typing makes a new tag. A tag every mod has shows as [✓tag] (enter
// removes it), one only some have as [~tag] (enter gives it to all). The pinned tags
// (config tags.pinned) come first, then the tags in use, most used first.

type tagBar struct {
	mods  []string // refs
	names string   // "Harmony" / "3 mods"
	cands []tagCand
	sel   int // -1: the input
	input string
	off   int // the first candidate shown
}

// tagCand is a candidate tag (view.TagCand: tag, uses, how many selected have it).
type tagCand struct {
	tag  string
	n    int
	have int
}

func (m *model) tagKey() tea.Cmd {
	sel := m.selection()
	if len(sel) == 0 || sel[0].id == "" {
		return nil
	}
	tb := &tagBar{mods: refsOf(sel), names: named(sel)}
	var selTags [][]string
	for _, it := range sel {
		selTags = append(selTags, it.tags)
	}
	if m.st != nil {
		for _, c := range view.TagCandidates(m.app, m.st.Ix.Env.ID, selTags) {
			tb.cands = append(tb.cands, tagCand{tag: c.Tag, n: c.N, have: c.Have})
		}
	}
	if len(tb.cands) == 0 {
		tb.sel = -1
	}
	m.tags = tb
	return nil
}

// tagBarKey: the keys while the tag bar is open.
func (m *model) tagBarKey(k tea.KeyMsg) tea.Cmd {
	tb := m.tags
	switch k.Type {
	case tea.KeyEsc:
		m.tags = nil
		return nil
	case tea.KeyLeft, tea.KeyUp: // 06 §13.4: in a popup both pairs of arrows choose
		tb.sel = max(tb.sel-1, -1)
		return nil
	case tea.KeyRight, tea.KeyDown:
		tb.sel = min(tb.sel+1, len(tb.cands)-1)
		return nil
	case tea.KeyHome:
		tb.sel = -1
		return nil
	case tea.KeyEnd:
		tb.sel = len(tb.cands) - 1
		return nil
	case tea.KeyBackspace:
		if r := []rune(tb.input); tb.sel < 0 && len(r) > 0 {
			tb.input = string(r[:len(r)-1])
		}
		return nil
	case tea.KeyRunes, tea.KeySpace:
		tb.sel = -1 // typing always goes to the input
		tb.input += string(k.Runes)
		return nil
	case tea.KeyEnter:
		m.tags = nil
		if tb.sel < 0 {
			t := strings.TrimSpace(tb.input)
			if t == "" {
				return nil
			}
			return m.applyTag(tb, t, false)
		}
		c := tb.cands[tb.sel]
		return m.applyTag(tb, c.tag, c.have == len(tb.mods))
	}
	return nil
}

// applyTag adds a tag to every mod of the bar, or removes it from them.
func (m *model) applyTag(tb *tagBar, tag string, remove bool) tea.Cmd {
	msg := Tf("added %s", tag)
	if remove {
		msg = Tf("removed %s", tag)
	}
	_, err := m.app.SetTags(tb.mods, []string{tag}, remove)
	m.reload()
	if err != nil {
		m.say(sayFail, m.humanize(err.Error()))
	} else {
		m.say(sayOK, Tf("tags of %s: %s", tb.names, msg))
	}
	m.refilterCol(m.focus, m.col().curID())
	return m.requestDetail()
}

// tagBarLine draws the bar in w cells: the input, then the candidates from off, scrolled
// so the chosen one shows.
func (m *model) tagBarLine(w int) string {
	tb := m.tags
	in := " " + "✎" + " " + T("type…") + " "
	if tb.input != "" || tb.sel < 0 {
		in = " " + "✎" + " " + tb.input
		if tb.sel < 0 {
			in += "_"
		}
		if tb.input == "" {
			in += T("type…")
		}
		in += " "
	}
	label := func(c tagCand) string {
		switch {
		case c.have == len(tb.mods):
			return "[✓" + c.tag + "]"
		case c.have > 0:
			return "[~" + c.tag + "]"
		}
		return "[" + c.tag + "]"
	}
	cw := func(i int) int { return ansi.StringWidthWc(label(tb.cands[i])) + 2 }
	avail := w - ansi.StringWidthWc(in) - 2 - 2 // the gap after the input, the "…" at the end
	tb.off = min(tb.off, max(tb.sel, 0))
	for tb.sel >= 0 && tb.off < tb.sel {
		used := 0
		for i := tb.off; i <= tb.sel; i++ {
			used += cw(i)
		}
		if used <= avail {
			break
		}
		tb.off++
	}
	var b strings.Builder
	used := 0
	if tb.sel < 0 {
		b.WriteString(sPill.Render(in))
	} else {
		b.WriteString(sSecond.Render(in))
	}
	b.WriteString("  ")
	used += ansi.StringWidthWc(in) + 2
	if tb.off > 0 {
		b.WriteString(sDimS.Render("…"))
		used++
	}
	for i := tb.off; i < len(tb.cands); i++ {
		l := label(tb.cands[i])
		lw := ansi.StringWidthWc(l)
		if used+lw+2 > w-1 {
			b.WriteString(sDimS.Render(" …"))
			used += 2
			break
		}
		switch {
		case i == tb.sel:
			b.WriteString(sPill.Render(l))
		case tb.cands[i].have > 0:
			b.WriteString(sAccent.Render(l))
		default:
			b.WriteString(sText.Render(l))
		}
		b.WriteString("  ")
		used += lw + 2
	}
	return b.String() + strings.Repeat(" ", max(w-used, 0))
}
