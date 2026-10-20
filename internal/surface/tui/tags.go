package tui

import (
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/cmds"
)

// The tag bar (06 §12.3): # opens one line of candidates for the picked mods (or the one
// under the cursor). ←→ choose, enter applies, esc closes. Left of the first candidate is
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

type tagCand struct {
	tag  string
	n    int // mods of the environment that have it
	have int // of the picked mods
}

func (m *model) tagKey() tea.Cmd {
	sel := m.selection()
	if len(sel) == 0 || sel[0].id == "" {
		return nil
	}
	tb := &tagBar{mods: refsOf(sel), names: named(sel)}
	have := map[string]int{}
	for _, it := range sel {
		for _, t := range it.tags {
			have[strings.ToLower(t)]++
		}
	}
	seen := map[string]bool{}
	add := func(tag string, n int) {
		k := strings.ToLower(tag)
		if tag == "" || seen[k] {
			return
		}
		seen[k] = true
		tb.cands = append(tb.cands, tagCand{tag: tag, n: n, have: min(have[k], len(sel))})
	}
	counts := map[string]int{}
	var used []tagCand
	if m.st != nil {
		if ts, err := m.app.Store.TagCounts(m.st.Ix.Env.ID); err == nil {
			for _, t := range ts {
				n, _ := strconv.Atoi(t[1])
				counts[strings.ToLower(t[0])] = n
				used = append(used, tagCand{tag: t[0], n: n})
			}
		}
	}
	for _, t := range strings.FieldsFunc(cmds.Setting(m.app, "tags.pinned"), func(r rune) bool { return r == ',' || r == '，' }) {
		t = strings.TrimSpace(t)
		add(t, counts[strings.ToLower(t)])
	}
	sort.SliceStable(used, func(i, j int) bool { return used[i].n > used[j].n })
	for _, t := range used {
		add(t.tag, t.n)
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
	case tea.KeyLeft:
		tb.sel = max(tb.sel-1, -1)
		return nil
	case tea.KeyRight:
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
	verb, msg := "add", Tf("added %s", tag)
	if remove {
		verb, msg = "remove", Tf("removed %s", tag)
	}
	argv := []string{"tag", verb, tb.mods[0], tag}
	for _, r := range tb.mods[1:] {
		argv = append(argv, "--mod", r)
	}
	if m.act("", argv...) {
		m.say(sayOK, Tf("tags of %s: %s", tb.names, msg))
	}
	if !m.picking() {
		m.marked = map[string]bool{}
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
