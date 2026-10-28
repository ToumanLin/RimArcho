package view

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/store"
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

// Each item gets a small trie of its own subjects and targets. Longest names win
// ("Vanilla Expanded" must not swallow "Vanilla Expanded Framework").
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
	for i, m := range r.mods {
		if m.owner == owner {
			if m.part.ID == "" && m.part.URL == "" {
				r.mods[i].part = p
			}
			return
		}
	}
	r.mods = append(r.mods, namedMod{part: p, owner: owner})
}

// The package lookup is built once per snapshot; unrelated library names never enter
// an item's trie. Instance references use the existing ByID index directly.
type needNameIndex struct {
	a        *app.App
	st       *app.LoadoutState
	packages map[string]*store.Mod
	missing  map[string]app.MissingMod
}

func newNeedNames(a *app.App, st *app.LoadoutState, missing []app.MissingMod) *needNameIndex {
	r := &needNameIndex{a: a, st: st, packages: map[string]*store.Mod{}, missing: map[string]app.MissingMod{}}
	for _, m := range st.Ix.Mods {
		pkg := strings.ToLower(m.PackageID)
		if m.Status == identity.Live && r.packages[pkg] == nil {
			r.packages[pkg] = m
		}
	}
	for _, m := range missing {
		r.missing[strings.ToLower(m.PackageID)] = m
	}
	return r
}

func namedInstance(m *store.Mod) NeedPart {
	p := NeedPart{Text: m.DisplayName()}
	if m.Status == identity.Live {
		p.ID = m.ID
	} else if ws := m.WorkshopID(); ws != "" {
		p.URL = "https://steamcommunity.com/sharedfiles/filedetails/?id=" + ws
	}
	return p
}

// addRef accepts subject tokens, action references, and the dependency target. The
// returned part also supplies the target for a name alias in a declared dependency.
func (r *needNameIndex) addRef(names *needNames, ref, url string, prefer func(string)) NeedPart {
	id := finding.InstID(ref)
	if r.st.Ix.ByID[ref] != nil {
		id = ref
	}
	if id != "" {
		if m := r.st.Ix.ByID[id]; m != nil {
			p := namedInstance(m)
			names.addFor(p, m.ID)
			prefer(m.ID)
			return p
		}
		return NeedPart{}
	}
	if strings.HasPrefix(ref, "#") || strings.HasPrefix(ref, "L#") {
		if m, err := r.st.Ix.ResolveOne(ref); err == nil {
			return r.addRef(names, m.ID, url, prefer)
		}
		return NeedPart{}
	}
	pkg := strings.ToLower(strings.TrimPrefix(ref, "pkg:"))
	if m := r.packages[pkg]; m != nil {
		return r.addRef(names, m.ID, url, prefer)
	}
	if !strings.HasPrefix(ref, "pkg:") && url == "" && r.missing[pkg].PackageID == "" {
		return NeedPart{}
	}
	if m, ok := r.missing[pkg]; ok {
		if m.CanEnable && m.Instance != "" {
			p := r.addRef(names, m.Instance, url, prefer)
			p.Text = m.Name
			names.addFor(p, m.Instance)
			return p
		}
		if url == "" && m.WorkshopID != "" {
			url = "https://steamcommunity.com/sharedfiles/filedetails/?id=" + m.WorkshopID
		}
		names.addFor(NeedPart{Text: m.Name, URL: url}, "pkg:"+pkg)
	}
	if r.a != nil {
		if w, ok := r.a.WorkshopFor(pkg); ok {
			if url == "" && w.WorkshopID != "" {
				url = "https://steamcommunity.com/sharedfiles/filedetails/?id=" + w.WorkshopID
			}
			names.addFor(NeedPart{Text: w.Name, URL: url}, "pkg:"+pkg)
		}
	}
	p := NeedPart{Text: pkg, URL: url}
	// Unknown non-mod action arguments (versions, Workshop numbers, etc.) are not names.
	if strings.HasPrefix(ref, "pkg:") || url != "" || r.missing[pkg].PackageID != "" {
		names.addFor(p, "pkg:"+pkg)
		prefer("pkg:" + pkg)
	}
	return p
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

func (r *needNameIndex) fill(n *Need) {
	names := &needNames{}
	preferred := map[string]int{}
	prefer := func(id string) {
		if id != "" && preferred[id] == 0 {
			preferred[id] = len(preferred) + 1
		}
	}
	for _, id := range n.Mods {
		r.addRef(names, id, "", prefer)
	}
	for _, o := range n.RequiredBy {
		r.addRef(names, o.ID, "", prefer)
	}
	if n.Target != "" {
		r.addRef(names, "pkg:"+n.Target, "", prefer)
	}
	for _, p := range n.linkProblems {
		if p == nil {
			continue
		}
		url := ""
		for _, s := range p.Suggestions {
			if u, ok := strings.CutPrefix(s, "subscribe: "); ok {
				url = u
				break
			}
		}
		for _, ref := range subjects(p) {
			target := r.addRef(names, ref, url, prefer)
			if pkg, ok := strings.CutPrefix(ref, "pkg:"); ok && p.Code == "dep.missing" {
				target.Text = depName(p.Claim, pkg)
				names.addFor(target, ref)
			}
		}
		if p.Code == "mod.replaced" {
			if m := replacedRe.FindStringSubmatch(p.Claim); m != nil {
				names.addFor(NeedPart{Text: m[1], URL: url}, "replacement")
			}
		}
	}
	for _, a := range n.Actions {
		for _, ref := range a.Refs {
			r.addRef(names, ref, "", prefer)
		}
		if a.After != "" {
			r.addRef(names, a.After, "", prefer)
		}
		for _, o := range a.Options {
			for _, ref := range o.Refs {
				r.addRef(names, ref, "", prefer)
			}
		}
	}
	c := &NeedContent{Title: names.parts(n.Title, preferred), Text: names.parts(n.Text, preferred), Why: names.parts(n.Why, preferred)}
	for _, p := range n.Parts {
		c.Parts = append(c.Parts, names.parts(p, preferred))
	}
	if len(c.Title)+len(c.Text)+len(c.Why)+len(c.Parts) > 0 {
		n.Content = c
	}
	n.linkProblems = nil
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
