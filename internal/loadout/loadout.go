// Package loadout models the ordered mod list (04 §1–§3): entries, the game's registry of
// loadable instances with its packageId tokens (`_steam`), resolution of ModsConfig.xml
// entries, and diffs between lists.
package loadout

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/rimarcho/rimarcho/internal/source"
)

// Entry is one position of a Loadout: an Instance, or an entry that matched nothing
// (kept, never silently dropped — 04 §2.4).
type Entry struct {
	Instance string `json:"i,omitempty"`
	Missing  string `json:"m,omitempty"` // the token as found in ModsConfig.xml
}

// Key identifies an entry within a list.
func (e Entry) Key() string {
	if e.Instance != "" {
		return e.Instance
	}
	return "missing:" + e.Missing
}

// List is an ordered Loadout.
type List []Entry

// Encode serialises a list for storage.
func (l List) Encode() string {
	if l == nil {
		l = List{}
	}
	b, _ := json.Marshal(l)
	return string(b)
}

// Decode parses a stored list.
func Decode(s string) List {
	var l List
	json.Unmarshal([]byte(s), &l)
	return l
}

// Index returns the position of an instance, or -1.
func (l List) Index(id string) int {
	for i, e := range l {
		if e.Instance == id {
			return i
		}
	}
	return -1
}

// Equal compares two lists.
func (l List) Equal(o List) bool {
	if len(l) != len(o) {
		return false
	}
	for i := range l {
		if l[i] != o[i] {
			return false
		}
	}
	return true
}

// Clone copies a list.
func (l List) Clone() List { return append(List(nil), l...) }

// Candidate is an Instance the game would register: live, not residue, not unloadable,
// and reachable from Data/, Mods/ or the subscribed workshop (directly or via an alias).
type Candidate struct {
	ID        string
	PackageID string      // lower case
	Kind      source.Kind // official, local or workshop: the game-visible location
	Folder    string      // folder name at that location
	Lineage   string
}

// Registry mirrors ModLister.RebuildModList + TryAddMod (07 §6.1 item 5): official, then
// Mods/, then workshop, each in folder order. A workshop copy of a packageId already taken
// by a non-workshop copy is registered as <pkg>_steam; any other duplicate is rejected.
type Registry struct {
	byToken  map[string]*Candidate
	byFolder map[string]*Candidate
	token    map[string]string // instance → token
	byID     map[string]*Candidate
	Rejected []*Candidate // duplicates the game refuses to register (pkg.duplicate-installed)
	byPkg    map[string][]*Candidate
}

func kindRank(k source.Kind) int {
	switch k {
	case source.Official:
		return 0
	case source.Local:
		return 1
	default:
		return 2
	}
}

// NewRegistry registers candidates in game order.
func NewRegistry(cands []Candidate) *Registry {
	cs := append([]Candidate(nil), cands...)
	sort.SliceStable(cs, func(i, j int) bool {
		if a, b := kindRank(cs[i].Kind), kindRank(cs[j].Kind); a != b {
			return a < b
		}
		return cs[i].Folder < cs[j].Folder
	})
	r := &Registry{byToken: map[string]*Candidate{}, byFolder: map[string]*Candidate{}, token: map[string]string{},
		byID: map[string]*Candidate{}, byPkg: map[string][]*Candidate{}}
	for i := range cs {
		c := &cs[i]
		r.byID[c.ID] = c
		r.byPkg[c.PackageID] = append(r.byPkg[c.PackageID], c)
		tok := c.PackageID
		if prev, dup := r.byToken[tok]; dup {
			if c.Kind == source.Workshop && prev.Kind != source.Workshop {
				tok += "_steam"
			}
			if _, dup2 := r.byToken[tok]; dup2 {
				r.Rejected = append(r.Rejected, c)
				continue
			}
		}
		r.byToken[tok] = c
		r.token[c.ID] = tok
		if _, ok := r.byFolder[strings.ToLower(c.Folder)]; !ok {
			r.byFolder[strings.ToLower(c.Folder)] = c
		}
	}
	return r
}

// Loadable reports whether the game registers this instance.
func (r *Registry) Loadable(id string) bool { _, ok := r.token[id]; return ok }

// Known reports whether id is a candidate at all (possibly rejected as a duplicate).
func (r *Registry) Known(id string) bool { _, ok := r.byID[id]; return ok }

// Token is the ModsConfig.xml identifier of a registered instance ("" if not registered).
func (r *Registry) Token(id string) string { return r.token[id] }

// SamePackage returns the other candidates declaring id's packageId.
func (r *Registry) SamePackage(id string) []*Candidate {
	c := r.byID[id]
	if c == nil {
		return nil
	}
	var out []*Candidate
	for _, o := range r.byPkg[c.PackageID] {
		if o.ID != id {
			out = append(out, o)
		}
	}
	return out
}

// Resolution explains how one ModsConfig.xml token was resolved.
type Resolution struct {
	Token    string
	Instance string // "" when missing
	Via      string // "id", "folder", "steam-suffix", "missing", "duplicate"
}

// Resolve maps one token per 04 §2.4: full id, then folder name, then id without _steam.
func (r *Registry) Resolve(token string) Resolution {
	t := strings.ToLower(strings.TrimSpace(token))
	if c := r.byToken[t]; c != nil {
		return Resolution{Token: token, Instance: c.ID, Via: "id"}
	}
	if c := r.byFolder[t]; c != nil && r.Loadable(c.ID) {
		return Resolution{Token: token, Instance: c.ID, Via: "folder"}
	}
	if base, ok := strings.CutSuffix(t, "_steam"); ok {
		if c := r.byToken[base]; c != nil {
			return Resolution{Token: token, Instance: c.ID, Via: "steam-suffix"}
		}
	}
	return Resolution{Token: token, Via: "missing"}
}

// FromTokens resolves activeMods into a list. An instance enabled twice keeps only its
// first position (the game deactivates the later one).
func (r *Registry) FromTokens(tokens []string) (List, []Resolution) {
	var l List
	var res []Resolution
	seen := map[string]bool{}
	for _, t := range tokens {
		x := r.Resolve(t)
		switch {
		case x.Instance == "":
			l = append(l, Entry{Missing: strings.ToLower(strings.TrimSpace(t))})
		case seen[x.Instance]:
			x.Via = "duplicate"
		default:
			seen[x.Instance] = true
			l = append(l, Entry{Instance: x.Instance})
		}
		res = append(res, x)
	}
	return l, res
}

// Tokens renders a list as activeMods identifiers; missing entries are written back as
// they were so the list survives a mod being temporarily absent.
func (r *Registry) Tokens(l List) []string {
	out := make([]string, 0, len(l))
	for _, e := range l {
		if e.Instance != "" {
			if t := r.Token(e.Instance); t != "" {
				out = append(out, t)
			}
			continue
		}
		out = append(out, e.Missing)
	}
	return out
}
