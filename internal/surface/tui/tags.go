package tui

import (
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Tag editing (06 §5.6): # on the selection; "word" or "+word" adds, "-word" removes;
// tab completes the current word from the tags in use.

// tagEdit is the prompt's completion state.
type tagEdit struct {
	known []tagCount // tags in use, most used first
	mods  []string   // refs
	names string     // "Harmony" / "3 mods"
	has   []string   // tags all selected mods already have
}

type tagCount struct {
	tag string
	n   int
}

func (m *model) tagKey() tea.Cmd {
	sel := m.selection()
	if len(sel) == 0 || sel[0].id == "" {
		return nil
	}
	te := &tagEdit{mods: refsOf(sel), names: named(sel)}
	if m.st != nil {
		if ts, err := m.app.Store.TagCounts(m.st.Ix.Env.ID); err == nil {
			for _, t := range ts {
				n, _ := strconv.Atoi(t[1])
				te.known = append(te.known, tagCount{t[0], n})
			}
		}
		sort.SliceStable(te.known, func(i, j int) bool { return te.known[i].n > te.known[j].n })
		// tags every selected mod has
		count := map[string]int{}
		for _, it := range sel {
			for _, t := range it.tags {
				count[t]++
			}
		}
		for t, n := range count {
			if n == len(sel) {
				te.has = append(te.has, t)
			}
		}
		sort.Strings(te.has)
	}
	m.tagging = te
	m.prompt = &prompt{label: Tf("tags for %s (word adds, -word removes): ", te.names), submit: func(m *model, text string) tea.Cmd {
		te := m.tagging
		m.tagging = nil
		var add, rm []string
		for _, w := range strings.Fields(text) {
			switch {
			case strings.HasPrefix(w, "-") && len(w) > 1:
				rm = append(rm, w[1:])
			case strings.HasPrefix(w, "+") && len(w) > 1:
				add = append(add, w[1:])
			case w != "+" && w != "-":
				add = append(add, w)
			}
		}
		if len(add) == 0 && len(rm) == 0 {
			return nil
		}
		ok := true
		for _, op := range []struct {
			verb string
			tags []string
		}{{"add", add}, {"remove", rm}} {
			if len(op.tags) == 0 {
				continue
			}
			argv := append([]string{"tag", op.verb, te.mods[0]}, op.tags...)
			for _, r := range te.mods[1:] {
				argv = append(argv, "--mod", r)
			}
			ok = m.act("", argv...) && ok
		}
		if ok {
			var parts []string
			if len(add) > 0 {
				parts = append(parts, Tf("added %s", strings.Join(add, ", ")))
			}
			if len(rm) > 0 {
				parts = append(parts, Tf("removed %s", strings.Join(rm, ", ")))
			}
			m.say(sayOK, Tf("tags of %s: %s", te.names, strings.Join(parts, "; ")))
		}
		m.marked = map[string]bool{}
		m.refilterCol(m.focus, "")
		return m.requestDetail()
	}}
	return nil
}

// tagSuggestions: tags in use that start with the word being typed (without + or -),
// not already typed, most used first.
func (te *tagEdit) suggestions(text string) (prefix string, words []string, out []tagCount) {
	words = strings.Fields(text)
	cur := ""
	if len(words) > 0 && !strings.HasSuffix(text, " ") {
		cur = words[len(words)-1]
		words = words[:len(words)-1]
	}
	sign := ""
	if strings.HasPrefix(cur, "-") || strings.HasPrefix(cur, "+") {
		sign, cur = cur[:1], cur[1:]
	}
	typed := map[string]bool{}
	for _, w := range words {
		typed[strings.ToLower(strings.TrimLeft(w, "+-"))] = true
	}
	for _, t := range te.known {
		if typed[strings.ToLower(t.tag)] {
			continue
		}
		if strings.HasPrefix(strings.ToLower(t.tag), strings.ToLower(cur)) {
			out = append(out, t)
		}
	}
	return sign, words, out
}

// complete replaces the word being typed by the first suggestion.
func (te *tagEdit) complete(text string) string {
	sign, words, sug := te.suggestions(text)
	if len(sug) == 0 {
		return text
	}
	return strings.Join(append(words, sign+sug[0].tag), " ") + " "
}

// tagStatus is the status line while tagging: the current tags, then the completions.
func (m *model) tagStatus() string {
	te := m.tagging
	_, _, sug := te.suggestions(m.prompt.text)
	var parts []string
	if len(te.has) > 0 {
		parts = append(parts, T("has")+" "+strings.Join(te.has, " "))
	}
	var s []string
	for i, t := range sug {
		if i == 8 {
			s = append(s, "…")
			break
		}
		s = append(s, t.tag+" "+strconv.Itoa(t.n))
	}
	if len(s) > 0 {
		parts = append(parts, T("tab completes")+" "+strings.Join(s, "  "))
	} else if len(te.known) == 0 {
		parts = append(parts, T("no tags yet: type one"))
	}
	return strings.Join(parts, "   ")
}
