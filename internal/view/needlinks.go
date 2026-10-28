package view

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/identity"
)

// NeedPart is literal text, or one named mod's navigation target. Strings stay available
// for terminals and machine clients; GUI renderers never have to recover names from them.
type NeedPart struct {
	Text string `json:"text"`
	ID   string `json:"id,omitempty"`
	URL  string `json:"url,omitempty"`
}

type NeedContent struct {
	Title []NeedPart   `json:"title,omitempty"`
	Text  []NeedPart   `json:"text,omitempty"`
	Why   []NeedPart   `json:"why,omitempty"`
	Parts [][]NeedPart `json:"parts,omitempty"`
}

// One transient name trie per snapshot: no per-item walk of the mod library, and longest
// names win ("Vanilla Expanded" must not swallow "Vanilla Expanded Framework").
type needNames struct {
	next map[byte]*needNames
	mods []namedMod
}

type namedMod struct {
	part  NeedPart
	owner string
}

func (r *needNames) add(p NeedPart) {
	r.addFor(p, p.ID)
}

func (r *needNames) addFor(p NeedPart, owner string) {
	if p.Text == "" {
		return
	}
	for i := 0; i < len(p.Text); i++ {
		if r.next == nil {
			r.next = map[byte]*needNames{}
		}
		if r.next[p.Text[i]] == nil {
			r.next[p.Text[i]] = &needNames{}
		}
		r = r.next[p.Text[i]]
	}
	for _, m := range r.mods {
		if m.part == p && m.owner == owner {
			return
		}
	}
	r.mods = append(r.mods, namedMod{part: p, owner: owner})
}

func newNeedNames(a *app.App, st *app.LoadoutState, ps *app.ProblemSet, missing []app.MissingMod) *needNames {
	r := &needNames{}
	installed := map[string]bool{}
	for _, m := range st.Ix.Mods {
		if m.Status != identity.Live {
			p := NeedPart{Text: m.DisplayName()}
			if ws := m.WorkshopID(); ws != "" {
				p.URL = "https://steamcommunity.com/sharedfiles/filedetails/?id=" + ws
			}
			r.addFor(p, m.ID)
			continue
		}
		r.add(NeedPart{Text: m.DisplayName(), ID: m.ID})
		installed[strings.ToLower(m.PackageID)] = true
	}
	for _, m := range missing {
		if m.CanEnable && m.Instance != "" {
			r.add(NeedPart{Text: m.Name, ID: m.Instance})
		} else if m.WorkshopID != "" && !installed[strings.ToLower(m.PackageID)] {
			url := "https://steamcommunity.com/sharedfiles/filedetails/?id=" + m.WorkshopID
			r.add(NeedPart{Text: m.Name, URL: url})
			r.add(NeedPart{Text: m.PackageID, URL: url})
		}
	}
	for _, p := range ps.List {
		var url string
		for _, s := range p.Suggestions {
			if u, ok := strings.CutPrefix(s, "subscribe: "); ok {
				url = u
				break
			}
		}
		for _, s := range subjects(p) {
			if pkg, ok := strings.CutPrefix(s, "pkg:"); ok && !installed[strings.ToLower(pkg)] {
				u := url
				if a != nil {
					if w, ok := a.WorkshopFor(pkg); ok {
						if u == "" && w.WorkshopID != "" {
							u = "https://steamcommunity.com/sharedfiles/filedetails/?id=" + w.WorkshopID
						}
						if u != "" {
							r.add(NeedPart{Text: w.Name, URL: u})
						}
					}
				}
				if u == "" {
					continue
				}
				r.add(NeedPart{Text: pkg, URL: u})
				if p.Code == "dep.missing" {
					r.add(NeedPart{Text: depName(p.Claim, pkg), URL: u})
				}
			}
		}
		if p.Code == "mod.replaced" && url != "" {
			if m := replacedRe.FindStringSubmatch(p.Claim); m != nil {
				r.add(NeedPart{Text: m[1], URL: url})
			}
		}
	}
	return r
}

func nameWord(r rune) bool {
	return r < utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')
}

func (r *needNames) parts(text string, preferred map[string]int) []NeedPart {
	var out []NeedPart
	start := 0
	used := map[string]bool{}
	for i := 0; i < len(text); {
		node, end := r, i
		var mods []namedMod
		for j := i; j < len(text); j++ {
			node = node.next[text[j]]
			if node == nil {
				break
			}
			if len(node.mods) > 0 {
				first, _ := utf8.DecodeRuneInString(text[i:])
				prev, _ := utf8.DecodeLastRuneInString(text[:i])
				last, _ := utf8.DecodeLastRuneInString(text[i : j+1])
				next, _ := utf8.DecodeRuneInString(text[j+1:])
				if !(nameWord(first) && nameWord(prev)) && !(nameWord(last) && nameWord(next)) {
					end, mods = j+1, node.mods
				}
			}
		}
		if end == i {
			_, size := utf8.DecodeRuneInString(text[i:])
			i += size
			continue
		}
		pick := mods[0]
		rank := int(^uint(0) >> 1)
		for _, m := range mods {
			if k := preferred[m.owner]; k > 0 && !used[m.owner] && k < rank {
				pick = m
				rank = k
			}
		}
		used[pick.owner] = true
		p := pick.part
		if i > start {
			out = append(out, NeedPart{Text: text[start:i]})
		}
		p.Text = text[i:end]
		out = append(out, p)
		i, start = end, end
	}
	if len(out) == 0 {
		return nil
	}
	if start < len(text) {
		out = append(out, NeedPart{Text: text[start:]})
	}
	return out
}

func (r *needNames) fill(n *Need) {
	preferred := map[string]int{}
	prefer := func(id string) {
		if id != "" && preferred[id] == 0 {
			preferred[id] = len(preferred) + 1
		}
	}
	for _, id := range n.Mods {
		prefer(id)
	}
	for _, o := range n.RequiredBy {
		prefer(o.ID)
	}
	// Names in a dependency's title can refer to a different installed mod than its owners.
	for _, a := range n.Actions {
		for _, ref := range a.Refs {
			prefer(finding.InstID(ref))
		}
	}
	c := &NeedContent{Title: r.parts(n.Title, preferred), Text: r.parts(n.Text, preferred), Why: r.parts(n.Why, preferred)}
	for _, p := range n.Parts {
		c.Parts = append(c.Parts, r.parts(p, preferred))
	}
	if len(c.Title)+len(c.Text)+len(c.Why)+len(c.Parts) > 0 {
		n.Content = c
	}
}

func reasonContent(n Need) *NeedContent {
	if n.Content == nil {
		return nil
	}
	text := n.Content.Text
	if n.Title != "" {
		text = n.Content.Title
	}
	return &NeedContent{Text: text, Why: n.Content.Why}
}
