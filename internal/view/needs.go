package view

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// 需要处理 (06 §7): what the player can act on, grouped by where the problem is, each item one
// sentence in plain words with the actions that fix it. No problem codes and no #refs appear
// here: they stay in the technical information, the CLI and MCP. The top bar's number is
// the number of counted items (06 §4.2).

// Groups, in screen order (06 §7.1).
const (
	GroupList = iota
	GroupDeps
	GroupConflict
	GroupLastRun
	GroupConfirm
	GroupHidden
)

// GroupTitles are the groups' headings (English; T translates them).
var GroupTitles = []string{"this list", "missing dependencies", "conflicts", "last run", "needs your answer", "hidden mods", "this run"}

// Action kinds. An action is data; each interface binds it to a key or a button.
const (
	ActSort            = "sort"             // open the sort preview (S)
	ActSave            = "save"             // open the save preview (w)
	ActEnable          = "enable"           // Refs
	ActDisable         = "disable"          // Refs
	ActChooseDisable   = "choose-disable"   // pick one of Options, then disable its Refs
	ActOpenURL         = "open-url"         // URL in the browser
	ActView            = "view"             // read-only command text: Argv
	ActList            = "list"             // show Title and Lines
	ActFold            = "fold"             // show / hide the item's Parts
	ActIgnoreRule      = "ignore-rule"      // Rule (empty: nothing to ignore)
	ActLoadExternal    = "load-external"    // check out Version (confirm first)
	ActDismissConflict = "dismiss-conflict" // keep the workspace, ignore the outside change
	ActAnswer          = "answer"           // Question, Answer (confirm first)
	ActUnhide          = "unhide"           // Refs[0]: rename About.xml back (confirm first)
	ActUnpin           = "unpin"            // Refs[0]: restore the workshop copy (confirm first)
	ActTrash           = "trash"            // Refs[0]: folder to the recycle bin (confirm first)
	ActIgnore          = "ignore"           // Problem: keep as it is (ignore the problem)
)

// Option is one choice of a choose-disable action.
type Option struct {
	Label string   `json:"label"`
	Refs  []string `json:"refs"`
	Done  string   `json:"done"`
}

// Action is one way to handle an item.
type Action struct {
	Kind  string `json:"kind"`
	Key   string `json:"key"` // the TUI's key for it (06 §7.2)
	Label string `json:"label"`
	// Done is what the status line says afterwards; Confirm, when set, is asked first
	// (06 §9: actions u cannot undo).
	Done    string   `json:"done,omitempty"`
	Confirm string   `json:"confirm,omitempty"` // the TUI's "press again" line
	Warn    string   `json:"warn,omitempty"`    // what a confirmation dialog says
	Refs    []string `json:"refs,omitempty"`
	Options []Option `json:"options,omitempty"`
	Title   string   `json:"title,omitempty"`
	URL     string   `json:"url,omitempty"`
	Argv    []string `json:"argv,omitempty"`
	Lines   []string `json:"lines,omitempty"`
	Rule    string   `json:"rule,omitempty"`
	Version int      `json:"version,omitempty"`
	QID     int64    `json:"question,omitempty"`
	Answer  string   `json:"answer,omitempty"`
	Problem string   `json:"problem,omitempty"` // p:<ref> for ignore
}

// Need is one item of 需要处理.
type Need struct {
	Group   int      `json:"group"`
	Text    string   `json:"text"`
	Actions []Action `json:"actions"`
	Counted bool     `json:"counted"` // in the top bar's number; false for the folded stale log
	Mods    []string `json:"mods,omitempty"`
	Status  int      `json:"status"` // StatusErr, StatusWarn or StatusLog for the row icon
	Dim     bool     `json:"dim,omitempty"`
	// A folded item stands for several findings one action resolves (06 §7.2): its parts
	// are shown on demand; every mod in Mods gets the row icon.
	Parts []string `json:"parts,omitempty"`
	Fold  bool     `json:"fold,omitempty"`
	rank  int      // order within the group (06 §7.2's order)
}

// Needs is the screen's content and what it means for the rest of the interface.
type Needs struct {
	Items  []Need         `json:"items"`
	Status map[string]int `json:"status"` // each mod's row icon
	Count  int            `json:"count"`  // the top bar's number
	Never  bool           `json:"never"`  // the mods' files were never checked
	Stale  bool           `json:"stale"`  // the files changed since the last check
}

// BuildNeeds turns the problem set and the environment's open questions into items, and
// counts them (06 §3, §4.2, §7).
func BuildNeeds(a *app.App, st *app.LoadoutState, ps *app.ProblemSet) *Needs {
	n := &Needs{Status: map[string]int{}}
	n.Never = ps.Analysis == nil || ps.Analysis.Run == nil
	n.Stale = !n.Never && !ps.Analysis.Fresh
	if st == nil {
		return n
	}
	n.Items = buildItems(a, st, ps)
	// The number in the top bar is the number of items the player can act on, and a row
	// carries an icon exactly when its mod is in one of them (06 §3, §4.2).
	for _, it := range n.Items {
		if !it.Counted {
			continue
		}
		n.Count++
		icons := it.Mods
		if !it.Fold && len(icons) > 1 {
			icons = icons[:1] // the subject; the others are what it needs or conflicts with
		}
		for _, id := range icons {
			if it.Status > n.Status[id] {
				n.Status[id] = it.Status
			}
		}
	}
	return n
}

func buildItems(a *app.App, st *app.LoadoutState, ps *app.ProblemSet) []Need {
	ix := st.Ix
	nm := func(tok string) string { return TokName(st, tok) }
	var out []Need
	add := func(n Need) { out = append(out, n) }
	var versions []*app.Problem
	var order []Need                    // order.violated: folded into one item when there are several
	logs := map[string][]*app.Problem{} // current log problems by subject mod ("" unattributed)
	for _, p := range ps.List {
		if p.Log != nil {
			if !p.Log.Stale() && (p.Severity == finding.Error || p.Severity == finding.Warning) {
				logs[p.SubjectInstance()] = append(logs[p.SubjectInstance()], p)
			}
			continue
		}
		if p.Code == "version.unsupported" {
			versions = append(versions, p)
			continue
		}
		if p.Severity != finding.Error && p.Severity != finding.Warning {
			continue // info: technical information only (06 §7.3)
		}
		subj := subjects(p)
		x, y := "", ""
		if len(subj) > 0 {
			x = subj[0]
		}
		if len(subj) > 1 {
			y = subj[1]
		}
		ids := instIDs(subj)
		s := StatusWarn
		if p.Severity == finding.Error {
			s = StatusErr
		}
		switch p.Code {
		case "official.order":
			add(Need{rank: 0, Group: GroupList, Text: T("Core and the DLCs must come first in the list"), Actions: []Action{sortAction()}, Counted: true, Status: s})
		case "order.violated":
			order = append(order, Need{rank: 1, Group: GroupList, Text: Tf("%s should load after %s", nm(x), nm(y)), Actions: []Action{sortAction()}, Counted: true, Mods: ids, Status: s})
		case "order.cycle":
			add(Need{rank: 2, Group: GroupList, Text: Tf("the order requirements of %d mods contradict each other", len(subj)),
				Actions: []Action{cycleView(st, p), cycleIgnore(st, p)}, Counted: true, Mods: ids, Status: s})
		case "pkg.duplicate-active":
			add(Need{rank: 3, Group: GroupList, Text: Tf("%s is enabled twice", nm(x)), Actions: []Action{keepOne(st, ids)}, Counted: true, Mods: ids, Status: s})
		case "entry.missing":
			pkg := strings.TrimPrefix(x, "pkg:")
			look := Action{Kind: ActOpenURL, Key: "o", Label: T("look for it in the Workshop"), URL: WorkshopSearch(pkg), Done: T("opened the Workshop page")}
			for _, sg := range p.Suggestions {
				if u, ok := strings.CutPrefix(sg, "subscribe: "); ok { // steamDB knows the item (10 §5)
					look.Label, look.URL = T("open it in the Workshop"), u
				}
			}
			add(Need{rank: 4, Group: GroupList, Text: Tf("the list has %s, but it is not installed", pkg),
				Actions: []Action{look, disable(st, "pkg:"+pkg, T("remove it from the list"))}, Counted: true, Status: s})
		case "mod.replaced":
			name := y
			if mt := replacedRe.FindStringSubmatch(p.Claim); mt != nil {
				name = mt[1]
			} else if y != "" {
				name = nm(y)
			}
			acts := []Action{workshop(p, name)}
			if y != "" {
				acts = []Action{enable(st, y), disable(st, x, Tf("disable %s", nm(x)))}
			}
			add(Need{rank: 5, Group: GroupList, Text: Tf("%s has a maintained replacement: %s", nm(x), name), Actions: acts, Counted: true, Mods: ids, Status: s})
		case "mod.unloadable":
			add(Need{rank: 4, Group: GroupList, Text: Tf("%s is enabled, but the game cannot load it", nm(x)), Actions: []Action{disable(st, x, T("disable it"))}, Counted: true, Mods: ids, Status: s})
		case "dep.missing":
			dep := depName(p.Claim, strings.TrimPrefix(y, "pkg:"))
			add(Need{Group: GroupDeps, Text: Tf("%s needs %s, but %s is not installed", nm(x), dep, dep), Actions: []Action{workshop(p, dep)}, Counted: true, Mods: ids, Status: s})
		case "dep.inactive":
			add(Need{Group: GroupDeps, Text: Tf("%s needs %s, but %s is not enabled", nm(x), nm(y), nm(y)), Actions: []Action{enable(st, y)}, Counted: true, Mods: ids, Status: s})
		case "patch.no-match":
			if y == "" {
				continue // no inactive target: nothing the player can do (technical information)
			}
			add(Need{Group: GroupDeps, Text: Tf("part of %s does not take effect: %s, which it targets, is not enabled", nm(x), nm(y)),
				Actions: []Action{enable(st, y), viewAction(T("see what"), "patches", "--mod", TokRef(st, x))}, Counted: true, Mods: ids, Status: s})
		case "incompat.active":
			add(Need{Group: GroupConflict, Text: Tf("%s is incompatible with %s", nm(x), nm(y)), Actions: []Action{disableOne(st, x, y)}, Counted: true, Mods: ids, Status: s})
		case "mod.pinned":
			add(Need{Group: GroupHidden, Text: Tf("%s is pinned to a copy, and the workshop version has been updated since", nm(x)),
				Actions: []Action{viewAction(T("see what changed"), "mod", TokRef(st, y), "--diff", TokRef(st, x)), unpin(st, x)}, Counted: true, Mods: ids, Status: s})
		case "mod.hidden-residue":
			add(Need{Group: GroupHidden, Text: Tf("%s is hidden and no longer subscribed; its folder is left over", nm(x)),
				Actions: []Action{trash(st, x)}, Counted: true, Mods: ids, Status: s})
		case "mod.hidden":
			add(Need{Group: GroupHidden, Text: Tf("%s is hidden from the game (probably by another mod manager)", nm(x)),
				Actions: []Action{keepHidden(p), unhide(st, x), trash(st, x)}, Counted: true, Mods: ids, Status: s})
		case "def.parent-missing":
			add(Need{Group: GroupDeps, Text: Tf("part of %s builds on a definition no enabled mod provides; a dependency may be missing", nm(x)),
				Actions: []Action{viewAction(T("see what"), "mod", TokRef(st, x))}, Counted: true, Mods: ids, Status: s})
		default:
			// Harmony warnings without log evidence, overlapping Defs and patches whose target
			// is simply absent stay in the technical information (06 §7.3).
		}
	}
	switch len(order) {
	case 0:
	case 1:
		add(order[0])
	default:
		f := Need{rank: 1, Group: GroupList, Counted: true, Status: StatusWarn, Fold: true}
		seen := map[string]bool{}
		for _, o := range order {
			f.Parts = append(f.Parts, o.Text)
			if len(o.Mods) > 0 && !seen[o.Mods[0]] {
				seen[o.Mods[0]] = true
				f.Mods = append(f.Mods, o.Mods[0])
			}
		}
		f.Text = Tf("%d mods are out of order", len(seen))
		f.Actions = []Action{sortAction(), {Kind: ActFold, Key: "enter", Label: T("show which")}}
		add(f)
	}
	if n := len(versions); n > 0 {
		warn := 0
		for _, v := range versions {
			if v.Severity == finding.Warning {
				warn++
			}
		}
		if warn > 0 {
			mm := st.GameMM()
			text := Tf("%d mods do not list %s as supported", n, mm)
			if prev := app.PrevMinor(mm); prev != "" {
				text = Tf("%d mods do not list %s as supported (%d of them list %s)", n, mm, n-warn, prev)
			}
			names := make([]string, 0, n)
			for _, v := range versions {
				names = append(names, nm(v.Subjects[0]))
			}
			sort.Strings(names)
			add(Need{rank: 5, Group: GroupList, Text: text, Counted: true, Status: StatusWarn,
				Actions: []Action{{Kind: ActList, Key: "enter", Label: T("see the list"), Title: text, Lines: names}}})
		}
	}
	if v := env.MajorMinor(st.FileVersionMismatch()); v != "" {
		add(Need{rank: 6, Group: GroupList, Text: Tf("the list was saved for %s; the game resets it when it starts — saving once fixes it", v),
			Actions: []Action{{Kind: ActSave, Key: "w", Label: T("save")}}, Counted: true, Status: StatusWarn})
	}
	// Last run: the current log's errors, one item per mod (06 §7.1).
	var subjectsOrder []string
	for id := range logs {
		subjectsOrder = append(subjectsOrder, id)
	}
	sort.Slice(subjectsOrder, func(i, j int) bool { return nm(finding.Inst(subjectsOrder[i])) < nm(finding.Inst(subjectsOrder[j])) })
	for _, id := range subjectsOrder {
		errs, warns := 0, 0
		for _, p := range logs[id] {
			if p.Severity == finding.Error {
				errs += logCount(p)
			} else {
				warns += logCount(p)
			}
		}
		var text string
		switch {
		case id == "" && errs > 0:
			text = Tf("last run, %d errors could not be tied to a mod", errs)
		case id == "":
			text = Tf("last run, %d warnings could not be tied to a mod", warns)
		case errs > 0:
			text = Tf("last run, %s reported %d errors", nm(finding.Inst(id)), errs)
		default:
			text = Tf("last run, %s reported %d warnings", nm(finding.Inst(id)), warns)
		}
		argv := []string{"problems", "--code", "log"}
		var ids []string
		if id != "" {
			argv = append(argv, "--subject", ix.Ref(id))
			ids = []string{id}
		}
		add(Need{Group: GroupLastRun, Text: text, Actions: []Action{viewAction(T("see the log and evidence"), argv...)}, Counted: true, Mods: ids, Status: StatusLog})
	}
	for _, r := range ps.Logs {
		if r.Stale() {
			add(Need{Group: GroupLastRun, Text: Tf("earlier log · %s · %s", ShortDate(r.ModTime), r.MM), Dim: true,
				Actions: []Action{viewAction(T("see it"), "problems", "--code", "log", "--all")}})
		}
	}
	// Needs your answer: identity questions and an external change of the list.
	if qs, err := a.Store.Questions(ix.Env.ID, false); err == nil {
		for _, q := range qs {
			out = append(out, questionNeed(st, q))
		}
	}
	if v := st.WS.Conflict; v != 0 {
		add(Need{Group: GroupConfirm, Text: T("ModsConfig.xml was changed outside RimArcho"), Counted: true, Status: StatusWarn, Actions: []Action{
			{Kind: ActLoadExternal, Key: "L", Label: T("load it into the list"), Version: v,
				Confirm: T("press L again to load it; your unsaved edits are replaced"), Warn: T("your unsaved edits are replaced"),
				Done: T("the outside change is loaded into the list")},
			{Kind: ActDismissConflict, Key: "I", Label: T("ignore it"), Done: T("the outside change is ignored (it stays in the history)")},
		}})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].rank < out[j].rank
	})
	return out
}

func questionNeed(st *app.LoadoutState, q *store.Question) Need {
	nm := func(id string) string { return TokName(st, finding.Inst(id)) }
	x, y := nm(q.InstanceID), nm(q.CandidateID)
	answer := func(key, label, ans, done string) Action {
		// Merging or splitting lineages is not undone with u: confirm first (06 §9).
		return Action{Kind: ActAnswer, Key: key, Label: label, QID: q.ID, Answer: ans, Done: done, Confirm: Tf("press %s again: %s", key, done), Warn: done}
	}
	return Need{Group: GroupConfirm, Text: Tf("is %s the same mod as %s?", x, y), Counted: true, Status: StatusWarn,
		Mods: []string{q.InstanceID, q.CandidateID},
		Actions: []Action{
			answer("s", T("same"), "same", Tf("%s and %s are one mod", x, y)),
			answer("c", T("a copy"), "copy", Tf("%s is a copy of %s", x, y)),
			answer("u", T("unrelated"), "unrelated", Tf("%s and %s are unrelated", x, y)),
		}}
}

// ---- actions ----

func sortAction() Action { return Action{Kind: ActSort, Key: "S", Label: T("sort")} }

func unhide(st *app.LoadoutState, tok string) Action {
	name := TokName(st, tok)
	return Action{Kind: ActUnhide, Key: "R", Label: T("show it to the game again"), Refs: []string{TokRef(st, tok)},
		Confirm: T("press R again to rename About.xml.disabled back to About.xml"), Warn: Tf("RimArcho renames About.xml.disabled back to About.xml in %s", name),
		Done: Tf("%s is visible to the game again", name)}
}

func unpin(st *app.LoadoutState, tok string) Action {
	name := TokName(st, tok)
	return Action{Kind: ActUnpin, Key: "P", Label: T("use the workshop version"), Refs: []string{TokRef(st, tok)},
		Confirm: T("press P again to restore the workshop copy and use it"), Warn: Tf("RimArcho restores the workshop copy of %s and uses it instead of the pinned copy; the copy stays on disk", name),
		Done: Tf("the list now uses the workshop version of %s (not saved yet)", name)}
}

func trash(st *app.LoadoutState, tok string) Action {
	name := TokName(st, tok)
	return Action{Kind: ActTrash, Key: "D", Label: T("move to the recycle bin"), Refs: []string{TokRef(st, tok)},
		Confirm: T("press D again to move the folder to the recycle bin"), Warn: Tf("the folder of %s goes to the recycle bin; you can restore it from there", name),
		Done: Tf("%s is in the recycle bin", name)}
}

func keepHidden(p *app.Problem) Action {
	return Action{Kind: ActIgnore, Key: "I", Label: T("keep it hidden"), Problem: p.Ref, Done: T("kept hidden; asked again if the mod changes")}
}

func viewAction(label string, argv ...string) Action {
	return Action{Kind: ActView, Key: "enter", Label: label, Argv: argv}
}

func enable(st *app.LoadoutState, tok string) Action {
	name := TokName(st, tok)
	return Action{Kind: ActEnable, Key: "space", Label: Tf("enable %s", name), Refs: []string{TokRef(st, tok)}, Done: Tf("enabled %s", name)}
}

func disable(st *app.LoadoutState, tok, label string) Action {
	return Action{Kind: ActDisable, Key: "space", Label: label, Refs: []string{TokRef(st, tok)}, Done: Tf("disabled %s", TokName(st, tok))}
}

// disableOne: choose which of two to disable (incompatible mods).
func disableOne(st *app.LoadoutState, x, y string) Action {
	nx, ny := TokName(st, x), TokName(st, y)
	return Action{Kind: ActChooseDisable, Key: "space", Label: T("choose which to disable"), Title: T("disable which one?"), Options: []Option{
		{Label: Tf("disable %s", nx), Refs: []string{TokRef(st, x)}, Done: Tf("disabled %s", nx)},
		{Label: Tf("disable %s", ny), Refs: []string{TokRef(st, y)}, Done: Tf("disabled %s", ny)},
	}}
}

// keepOne: choose which copy to keep (the same mod enabled twice).
func keepOne(st *app.LoadoutState, ids []string) Action {
	act := Action{Kind: ActChooseDisable, Key: "space", Label: T("choose which to keep"), Title: T("keep which copy?")}
	for i, id := range ids {
		mod := st.Ix.ByID[id]
		if mod == nil {
			continue
		}
		var off []string
		for j, other := range ids {
			if j != i {
				off = append(off, st.Ix.Ref(other))
			}
		}
		act.Options = append(act.Options, Option{Label: Tf("keep the one from %s", SourceText(mod)+" · "+mod.Rel), Refs: off, Done: T("kept one copy")})
	}
	return act
}

// workshop: open the Workshop page of a missing dependency (its id when the declaration
// names it, else a Workshop search for its name).
func workshop(p *app.Problem, name string) Action {
	u := ""
	for _, s := range p.Suggestions {
		if strings.HasPrefix(s, "subscribe: ") {
			u = strings.TrimPrefix(s, "subscribe: ")
		}
	}
	if u == "" {
		u = WorkshopSearch(name)
	}
	return Action{Kind: ActOpenURL, Key: "o", Label: Tf("open %s in the Workshop", name), URL: u, Done: T("opened the Workshop page")}
}

// WorkshopSearch is the Workshop search page for a text.
func WorkshopSearch(q string) string {
	return "https://steamcommunity.com/workshop/browse/?appid=294100&searchtext=" + url.QueryEscape(q)
}

// cycleView: the requirements that contradict each other, in words.
func cycleView(st *app.LoadoutState, p *app.Problem) Action {
	var lines []string
	for _, e := range p.Evidence {
		lines = append(lines, EdgeWords(st, e.Text))
	}
	return Action{Kind: ActList, Key: "enter", Label: T("see which"), Title: T("order requirements that contradict each other"), Lines: lines}
}

// cycleIgnore: ignore the requirement the sort suggests dropping (its first suggestion).
func cycleIgnore(st *app.LoadoutState, p *app.Problem) Action {
	act := Action{Kind: ActIgnoreRule, Key: "x", Label: T("ignore the suggested one"), Done: T("that requirement is ignored from now on")}
	for _, s := range p.Suggestions {
		if rest, ok := strings.CutPrefix(s, "archo rule add ignore "); ok {
			act.Rule = finding.Expand(rest, st.Ix.Ref)
			break
		}
	}
	return act
}

// ErrNothing: the action has nothing to do (an ignore with no suggestion).
var ErrNothing = errors.New("nothing to do")

// Do runs an action that changes data (enable, disable, choose-disable with the chosen
// option, ignore-rule, load-external, dismiss-conflict, answer), the same for every
// interface. The interface asks Confirm first; Done is the text to show afterwards.
// Navigation kinds (sort, save, view, list, fold, open-url) are the interface's own.
func Do(a *app.App, act Action, option int) (done string, err error) {
	switch act.Kind {
	case ActEnable:
		_, err = a.Enable(act.Refs, app.Placement{})
	case ActDisable:
		_, err = a.Disable(act.Refs)
	case ActChooseDisable:
		if option < 0 || option >= len(act.Options) {
			return "", fmt.Errorf("no option %d", option)
		}
		o := act.Options[option]
		_, err = a.Disable(o.Refs)
		return o.Done, err
	case ActIgnoreRule:
		if act.Rule == "" {
			return T("nothing to ignore here"), ErrNothing
		}
		_, err = a.AddRule(app.RuleSpec{Kind: "ignore", Target: act.Rule})
	case ActLoadExternal:
		_, _, err = a.Checkout(fmt.Sprintf("v%d", act.Version))
	case ActDismissConflict:
		err = a.DismissConflict()
	case ActAnswer:
		_, err = a.ResolveQuestion(act.QID, act.Answer)
	case ActUnhide:
		_, err = a.Unhide(act.Refs[0], false)
	case ActUnpin:
		_, err = a.Unpin(act.Refs[0], false)
	case ActTrash:
		_, err = a.Trash(act.Refs[0], false)
	case ActIgnore:
		_, err = a.IgnoreProblem(act.Problem, false)
	default:
		return "", fmt.Errorf("%s is not a data action", act.Kind)
	}
	return act.Done, err
}

// ---- names and refs ----

// subjects: the finding's subjects that are mods or packages.
func subjects(p *app.Problem) []string {
	var out []string
	for _, s := range p.Subjects {
		if finding.InstID(s) != "" || strings.HasPrefix(s, "pkg:") {
			out = append(out, s)
		}
	}
	return out
}

func instIDs(toks []string) []string {
	var out []string
	for _, t := range toks {
		if id := finding.InstID(t); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// TokName: a mod token's display name; a package id names an installed mod when it can.
func TokName(st *app.LoadoutState, tok string) string {
	ix := st.Ix
	if id := finding.InstID(tok); id != "" {
		if mod := ix.ByID[id]; mod != nil {
			return mod.DisplayName()
		}
		return T("a removed mod")
	}
	pkg := strings.ToLower(strings.TrimPrefix(tok, "pkg:"))
	for _, mod := range ix.Mods {
		if mod.PackageID == pkg && mod.Status == identity.Live {
			return mod.DisplayName()
		}
	}
	return pkg
}

// TokRef: the CLI reference of a token (for the commands actions run; never shown).
func TokRef(st *app.LoadoutState, tok string) string {
	if id := finding.InstID(tok); id != "" {
		return st.Ix.Ref(id)
	}
	return tok
}

var replacedRe = regexp.MustCompile(`has a replacement for [^:]+: (.+) by `)

var depNameRe = regexp.MustCompile(`requires (?:the )?(.+?)(?: DLC)? \(pkg:`)

func depName(claim, pkg string) string {
	if mt := depNameRe.FindStringSubmatch(claim); mt != nil && !strings.Contains(mt[1], "@{") {
		return mt[1]
	}
	return pkg
}

var logCountRe = regexp.MustCompile(` x(\d+): `)

func logCount(p *app.Problem) int {
	n := 1
	if mt := logCountRe.FindStringSubmatch(p.Claim); mt != nil {
		fmt.Sscan(mt[1], &n)
	}
	return n
}

var edgeRe = regexp.MustCompile(`^(?:decl|community|rule r:\d+|builtin) (\S+)(?: About\.xml)? (\S+) (pkg:\S+)`)

// EdgeWords: "decl @{a} About.xml loadAfter pkg:b  [suggested to ignore]" → "A wants to load
// after B · suggested to ignore".
func EdgeWords(st *app.LoadoutState, t string) string {
	sug := strings.Contains(t, "[suggested to ignore]")
	mt := edgeRe.FindStringSubmatch(t)
	if mt == nil {
		return Humanize(st, finding.Expand(t, st.Ix.Ref))
	}
	x, field, y := TokName(st, mt[1]), strings.ToLower(mt[2]), TokName(st, mt[3])
	var s string
	switch {
	case strings.Contains(field, "before"):
		s = Tf("%s wants to load before %s", x, y)
	default:
		s = Tf("%s wants to load after %s", x, y)
	}
	if sug {
		s += "  ·  " + T("suggested to ignore (x)")
	}
	return s
}

var idRe = regexp.MustCompile(`#[0-9a-z]{3,8}\b`)

// Humanize replaces instance references by mod names and drops the "error: " prefix.
func Humanize(st *app.LoadoutState, s string) string {
	s = strings.TrimPrefix(strings.TrimSpace(s), "error: ")
	if st == nil {
		return s
	}
	return idRe.ReplaceAllStringFunc(s, func(r string) string {
		if mod, err := st.Ix.ResolveOne(r); err == nil {
			return mod.DisplayName()
		}
		return r
	})
}

// ShortDate: "Oct 17" / "10 月 17 日".
func ShortDate(t time.Time) string {
	t = t.Local()
	if lang == "zh" {
		return fmt.Sprintf("%d 月 %d 日", int(t.Month()), t.Day())
	}
	return t.Format("Jan 2")
}
