package view

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
)

// ProblemPanel is the small decision view; the full Mod card remains archo mod.
type ProblemPanel struct {
	Title    string   `json:"title"`
	Meaning  string   `json:"meaning"`
	Notes    []string `json:"notes,omitempty"`
	Affected []string `json:"affected,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
	Actions  []Action `json:"actions"`
}

func problemAction(ref string) Action {
	return Action{Kind: ActProblem, Key: "enter", Label: T("see the problem and fixes"), Problem: ref, Argv: []string{"problems", "--ref", ref}}
}

func parentType(typ string) string {
	typ = strings.TrimPrefix(typ, "Verse.")
	switch typ {
	case "HediffDef":
		return T("health condition")
	case "PawnKindDef":
		return T("pawn kind")
	case "ThingDef":
		return T("item, building or creature")
	case "RecipeDef":
		return T("recipe")
	case "GeneDef":
		return T("gene")
	case "ResearchProjectDef":
		return T("research project")
	default:
		return T("content")
	}
}
func parentPart(part app.ParentPart) string {
	name := part.Name
	if name == "" {
		name = T("unnamed content")
	}
	return Tf("%s %s cannot load: missing base content %s", parentType(part.Type), name, part.Parent)
}
func parentSummary(parts []app.ParentPart) string {
	var order []string
	counts := map[string]int{}
	for _, p := range parts {
		typ := parentType(p.Type)
		if counts[typ] == 0 {
			order = append(order, typ)
		}
		counts[typ]++
	}
	var out []string
	for _, typ := range order {
		out = append(out, Tf("%d × %s", counts[typ], typ))
	}
	return strings.Join(out, T("; "))
}

const parentRisk = "in shared logs, mods with missing base content were error sources in 16% of logs (4.1% by chance)"
const parentNext = "enable a provider if one is found, or check the mod's Workshop requirements; declared candidates may only be optional companions"

func parentNeed(st *app.LoadoutState, qs []*app.Problem, p *app.Problem) Need {
	var parts []app.ParentPart
	var lines []string
	for _, q := range qs {
		if part, ok := app.MissingParent(q); ok {
			parts = append(parts, part)
			lines = append(lines, parentPart(part))
		}
	}
	title := Tf("%s: %d added things cannot load", TokName(st, finding.Inst(p.SubjectInstance())), len(parts))
	n := Need{Group: GroupDeps, Text: title + ": " + parentSummary(parts), Title: title, Why: parentSummary(parts) + ". " + T(parentRisk) + ". " + T(parentNext),
		Actions: []Action{problemAction(p.Ref)}, Counted: true, Mods: []string{p.SubjectInstance()}, Status: StatusErr, Evidence: evidenceFor(p)}
	if len(lines) > 1 {
		n.Parts = lines
		n.Fold = true
		n.Actions = append(n.Actions, Action{Kind: ActFold, Key: "f", Label: T("show which")})
		n.Evidence = evidence(EvInferred, Tf("%s and %d more", lines[0], len(lines)-1))
	}
	return n
}

func BuildProblemPanel(a *app.App, ps *app.ProblemSet, ref string) (*ProblemPanel, error) {
	var p *app.Problem
	for _, q := range ps.All() {
		if q.Ref == ref {
			p = q
			break
		}
	}
	if p == nil {
		return nil, &app.RefError{Ref: ref, Hint: "archo problems lists current problem references"}
	}
	st := ps.State
	panel := &ProblemPanel{Title: Humanize(st, finding.Expand(p.Claim, st.Ix.Ref))}
	if p.Code == "def.parent-missing" {
		h, err := a.ParentHelp(ps, p)
		if err != nil {
			return nil, err
		}
		n := parentNeed(st, h.Problems, p)
		panel.Title = n.Title
		panel.Meaning = parentSummary(h.Parts)
		panel.Notes = []string{T(parentRisk), T(parentNext)}
		for _, part := range h.Parts {
			panel.Affected = append(panel.Affected, parentPart(part))
		}
		for _, q := range h.Problems {
			for _, ev := range q.Evidence {
				panel.Evidence = appendUnique(panel.Evidence, Humanize(st, finding.Expand(ev.Text, st.Ix.Ref)))
			}
		}
		if h.CacheIncomplete {
			panel.Notes = append(panel.Notes, T("the file check is out of date; check files again before looking for installed providers"))
		}
		for _, fix := range h.Fixes {
			switch fix.Kind {
			case "enable":
				act := Action{Kind: ActEnable, Label: Tf("enable %s", fix.Name), Refs: []string{fix.Ref}, Done: Tf("enabled %s", fix.Name), Undo: true}
				act.Key = fmt.Sprint(len(panel.Actions) + 1)
				panel.Actions = append(panel.Actions, act)
				panel.Evidence = append(panel.Evidence, Tf("%s supplies the missing base content %s (stored file check)", fix.Name, fix.Parent))
			case "subscribe", "search":
				label := Tf("check candidate %s", fix.Name)
				var act Action
				if fix.Kind == "subscribe" {
					act = subscribeAction([]string{fix.Ref}, Tf("subscribe to candidate %s", fix.Name))
					act.URL = fix.URL
				} else {
					act = Action{Kind: ActOpenURL, Label: label, URL: WorkshopSearch(fix.Name), Done: T("opened the Workshop page")}
				}
				act.Key = fmt.Sprint(len(panel.Actions) + 1)
				panel.Actions = append(panel.Actions, act)
				panel.Evidence = append(panel.Evidence, Tf("%s is mentioned in %s; it may supply the missing content", fix.Name, parentBasis(fix.Basis)))
			case "requirements":
				u := fix.URL
				if u == "" {
					u = WorkshopSearch(fix.Name)
				}
				panel.Actions = append(panel.Actions, Action{Kind: ActOpenURL, Key: "o", Label: T("check this mod's Workshop requirements"), URL: u, Done: T("opened the Workshop page")})
			}
		}
		if h.Quiet {
			panel.Notes = append(panel.Notes, T("the last run of this list had no errors attributed to this mod; this is probably harmless content meant for an optional mod"))
			var refs []string
			for _, q := range h.Problems {
				if !q.Ignored {
					refs = append(refs, q.Ref)
				}
			}
			if len(refs) > 0 {
				panel.Actions = append(panel.Actions, Action{Kind: ActIgnore, Key: "I", Label: T("do not warn about this again"), Refs: refs, Done: T("ignored until the mod changes")})
			}
		}
		return panel, nil
	}
	// Reuse each item's existing explanation and fixes, leaving its wording owned by
	// the need table. No mod card or all-patches dump is used for problem inspection.
	for _, n := range BuildNeeds(a, st, ps).Items {
		for _, act := range n.Actions {
			if act.Kind == ActProblem && act.Problem == ref {
				if n.Title != "" {
					panel.Title = n.Title
				} else {
					panel.Title = n.Text
				}
				panel.Meaning = n.Why
				for _, fix := range n.Actions {
					if fix.Kind != ActProblem && fix.Kind != ActFold {
						panel.Actions = append(panel.Actions, fix)
					}
				}
			}
		}
	}
	if p.Code == "mod.known-error" {
		panel.Evidence = append(panel.Evidence, corpusEvidence(p).Source)
	} else if p.Code == "patch.no-match" {
		for _, q := range ps.List {
			if q.Code == p.Code && strings.Join(q.Subjects, "|") == strings.Join(p.Subjects, "|") {
				location := strings.SplitN(q.Claim, " ", 2)[0]
				panel.Affected = appendUnique(panel.Affected, Tf("changes in %s do not take effect", location))
			}
		}
		panel.Evidence = append(panel.Evidence, evidenceFor(p).Source)
	} else {
		for _, ev := range p.Evidence {
			panel.Evidence = appendUnique(panel.Evidence, Humanize(st, finding.Expand(ev.Text, st.Ix.Ref)))
		}
	}
	if m := st.Ix.ByID[p.SubjectInstance()]; m != nil {
		ws := m.WorkshopID()
		if ws == "" {
			if w, ok := a.WorkshopFor(m.PackageID); ok {
				ws = w.WorkshopID
			}
		}
		u := WorkshopSearch(m.DisplayName())
		if ws != "" {
			u = "https://steamcommunity.com/sharedfiles/filedetails/?id=" + ws
		}
		panel.Actions = append(panel.Actions, Action{Kind: ActOpenURL, Key: "o", Label: T("open this mod's Workshop page"), URL: u, Done: T("opened the Workshop page")})
	}
	return panel, nil
}

func appendUnique(lines []string, line string) []string {
	for _, old := range lines {
		if old == line {
			return lines
		}
	}
	return append(lines, line)
}

func parentBasis(basis string) string {
	switch basis {
	case "dependency":
		return T("the author's requirements")
	case "loadAfter":
		return T("the author's load order")
	case "MayRequire":
		return T("optional content references")
	}
	return basis
}
