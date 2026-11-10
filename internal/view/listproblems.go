package view

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
)

type ModOwner struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type ModProblems struct {
	Count      int      `json:"count"`
	Severity   string   `json:"severity"`
	Summary    string   `json:"summary"`
	ProblemIDs []string `json:"problemIds"`
}

// ListProblems projects the snapshot that supplies Needs and editor row badges.
type ListProblems struct {
	Environment string                  `json:"environment"`
	Plan        int64                   `json:"plan"`
	ListVersion string                  `json:"listVersion"`
	Sequence    uint64                  `json:"sequence"`
	Revision    string                  `json:"revision"`
	Updating    bool                    `json:"updating"`
	Error       string                  `json:"error,omitempty"`
	Count       int                     `json:"count"`
	Severity    string                  `json:"severity"`
	Mods        map[string]*ModProblems `json:"mods"`
	Items       []Need                  `json:"items"`
	Status      map[string]int          `json:"status"`
	Reasons     map[string][]Reason     `json:"reasons"`
}

func ProblemsOf(n *Needs) *ListProblems {
	p := &ListProblems{Environment: n.Environment, Plan: n.Plan, ListVersion: n.ListVersion, Sequence: n.Sequence, Revision: n.Revision, Updating: n.Updating, Error: n.Error, Count: n.Count, Severity: "info", Mods: map[string]*ModProblems{}, Items: n.Items}
	p.Status, p.Reasons = n.Status, n.Reasons
	for _, it := range n.Items {
		if it.Counted && it.Status == StatusErr {
			p.Severity = "error"
		} else if it.Counted && it.Status == StatusWarn && p.Severity != "error" {
			p.Severity = "warning"
		}
	}
	for _, it := range n.Items {
		seen := map[string]bool{}
		for _, id := range it.Mods {
			if seen[id] {
				continue
			}
			seen[id] = true
			m := p.Mods[id]
			if m == nil {
				m = &ModProblems{Severity: "info"}
				p.Mods[id] = m
			}
			title := it.Title
			if title == "" {
				title = it.Text
			}
			if r, ok := it.rowReasons[id]; ok {
				title = r.Text
			}
			sev := problemSeverity(it.Status)
			if it.Dim && !it.Counted {
				sev = "info"
			}
			if m.Summary == "" || sev == "error" && m.Severity != "error" || sev == "warning" && m.Severity == "info" {
				m.Summary = title
				m.Severity = sev
			}
			if it.Counted {
				m.Count++
			}
			m.ProblemIDs = append(m.ProblemIDs, it.ID)
		}
	}
	return p
}
func problemSeverity(status int) string {
	if status == StatusErr {
		return "error"
	}
	if status == StatusWarn {
		return "warning"
	}
	return "info"
}
func owners(st *app.LoadoutState, tok string) []ModOwner {
	if id := finding.InstID(tok); id != "" {
		return []ModOwner{{ID: id, Name: TokName(st, tok)}}
	}
	return nil
}
func dependencyTarget(st *app.LoadoutState, tok string) string {
	if id := finding.InstID(tok); id != "" {
		if m := st.Ix.ByID[id]; m != nil {
			return strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")
		}
	}
	pkg := strings.TrimSpace(strings.TrimPrefix(tok, "pkg:"))
	if r := st.Reg.Resolve(pkg); r.Instance != "" {
		if m := st.Ix.ByID[r.Instance]; m != nil {
			pkg = m.PackageID
		}
	}
	return strings.TrimSuffix(strings.ToLower(pkg), "_steam")
}

// ignoreRefs: the problems the ignore actions of merged items cover, each once.
func ignoreRefs(lists ...[]Action) []string {
	var out []string
	for _, acts := range lists {
		for _, a := range acts {
			if a.Kind == ActIgnore {
				for _, r := range a.Refs {
					if !slices.Contains(out, r) {
						out = append(out, r)
					}
				}
			}
		}
	}
	return out
}

// withIgnoreRefs gives the ignore action every merged item's problems: ignoring a dependency
// ignores it for every mod that needs it.
func withIgnoreRefs(acts []Action, refs []string) []Action {
	out := slices.Clone(acts)
	for i := range out {
		if out[i].Kind == ActIgnore {
			out[i].Refs = refs
		}
	}
	return out
}

func mergeDependencies(st *app.LoadoutState, items []Need) []Need {
	var out []Need
	by := map[string]int{}
	for _, it := range items {
		if len(it.RequiredBy) > 0 {
			it.Mods = nil
			for _, o := range it.RequiredBy {
				it.Mods = append(it.Mods, o.ID)
			}
		}
		if it.Target == "" {
			out = append(out, it)
			continue
		}
		key := it.Target
		if it.Evidence != nil && it.Evidence.Kind == EvCorpus {
			key = "corpus|" + key
		}
		if i, ok := by[key]; ok {
			n := &out[i]
			n.linkProblems = append(n.linkProblems, it.linkProblems...)
			for _, o := range it.RequiredBy {
				if !slices.ContainsFunc(n.RequiredBy, func(x ModOwner) bool { return x.ID == o.ID }) {
					n.RequiredBy = append(n.RequiredBy, o)
				}
			}
			for _, id := range it.Mods {
				if !slices.Contains(n.Mods, id) {
					n.Mods = append(n.Mods, id)
				}
			}
			if it.Status > n.Status {
				n.Status = it.Status
				n.Title = it.Title
			}
			ignored := ignoreRefs(n.Actions, it.Actions)
			if len(it.Actions) > 0 && it.Actions[0].Kind == ActSubscribe {
				n.Actions = it.Actions
			}
			n.Actions = withIgnoreRefs(n.Actions, ignored)
			if len(it.Parts) > 0 {
				n.Parts = append(n.Parts, it.Parts...)
			} else {
				n.Parts = append(n.Parts, it.Text)
			}
			n.Fold = true
			names := make([]string, 0, len(n.RequiredBy))
			for _, o := range n.RequiredBy {
				names = append(names, o.Name)
			}
			n.Why = Tf("required by: %s", strings.Join(names, ", "))
			if n.Evidence != nil && n.Evidence.Kind == EvCorpus {
				n.Why = Tf("may be needed by: %s; try adding it if errors mention a missing dependency.", strings.Join(names, ", "))
			}
			n.Text = n.Title + "  " + n.Why
			continue
		}
		by[key] = len(out)
		if len(it.Parts) == 0 {
			it.Parts = []string{it.Text}
		}
		out = append(out, it)
	}
	return out
}
func identifyNeed(n *Need) {
	mods := slices.Clone(n.Mods)
	sort.Strings(mods)
	key := n.code + "|" + n.Phase + "|" + n.Target + "|" + strings.Join(mods, ",")
	if n.Target != "" {
		key = "dependency|" + n.Target
		if n.Evidence != nil && n.Evidence.Kind == EvCorpus {
			key = "corpus|" + key
		}
	} else {
		key += "|" + n.Text
	}
	sum := sha256.Sum256([]byte(key))
	n.ID = fmt.Sprintf("need:%x", sum[:8])
	for i := range n.Actions {
		a := &n.Actions[i]
		a.ID = ""
		b, _ := json.Marshal(a)
		h := sha256.Sum256(b)
		a.ID = fmt.Sprintf("action:%x", h[:8])
	}
}
