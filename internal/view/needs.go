package view

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// 需要处理 (06 §5): what the player can act on, grouped by where the problem is, each item one
// sentence in plain words with the actions that fix it. No problem codes and no #refs appear
// here: they stay in the technical information, the CLI and MCP. The top bar's number is
// the number of counted items (06 §2.2).

// Groups, in screen order (06 §2.4).
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
	ActLocateDirectory = "locate-directory" // Refs[0]: managed directory ID; Lines[0]: old path
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
	ActUnlinkPacks     = "unlink-packs"     // remove the packs' links from Mods/ (08 §5.3)
	ActRepairLinks     = "repair-links"
	ActSwap            = "swap"           // Refs[0] → Refs[1] in the plan (one undoable step)
	ActSubscribe       = "subscribe"      // Refs: Workshop IDs to subscribe to (a background task)
	ActMoveLocked      = "move-to-locked" // Refs[0]: a Mods/ folder into the lock library (confirm first)
	ActTextures        = "textures"       // Refs: convert their textures to DDS again (confirm first)
	ActTexClean        = "textures-clean" // Refs: delete the DDS RimArcho wrote (confirm first)
	ActAddVersion      = "add-version"    // Refs[0]: the list's name, Refs[1]: a game version to allow (04 §1.6)
	ActManageLists     = "manage-lists"   // the list management page (06 §2.17); Argv: the CLI's view of it
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
	Key   string `json:"key"` // the TUI's key for it (06 §5)
	Label string `json:"label"`
	// Done is what the status line says afterwards; Confirm, when set, is asked first
	// (06 §6: actions u cannot undo).
	Done    string   `json:"done,omitempty"`
	Confirm string   `json:"confirm,omitempty"` // the TUI's "press again" line
	Warn    string   `json:"warn,omitempty"`    // what a confirmation dialog says
	Undo    bool     `json:"undo,omitempty"`    // one workspace step: the status line offers u
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

	LinkGroups []LinkGroup `json:"linkGroups,omitempty"`
}

// Need is one item of 需要处理.
type Need struct {
	Group   int    `json:"group"`
	Section int    `json:"section"` // the section it is shown in (SectionOf, 06 §2.4)
	Text    string `json:"text"`    // one sentence (the TUI's line)
	// Title and Why are the GUI card's bold line and its plain explanation (Title empty:
	// the card shows Text); Evidence is where the claim comes from.
	Title    string    `json:"title,omitempty"`
	Why      string    `json:"why,omitempty"`
	Evidence *Evidence `json:"evidence,omitempty"`
	Actions  []Action  `json:"actions"`
	Counted  bool      `json:"counted"` // in the top bar's number; false for the folded stale log
	Mods     []string  `json:"mods,omitempty"`
	Status   int       `json:"status"` // StatusErr, StatusWarn or StatusLog for the row icon
	Dim      bool      `json:"dim,omitempty"`
	// A folded item stands for several findings one action resolves (06 §5): its parts
	// are shown on demand; every mod in Mods gets the row icon.
	Parts []string `json:"parts,omitempty"`
	Fold  bool     `json:"fold,omitempty"`

	LinkGroups []LinkGroup `json:"linkGroups,omitempty"`

	rank int    // order within the group (06 §5's order)
	code string // the problem code it stands for (LaunchHint)
}

// Needs is the screen's content and what it means for the rest of the interface.
type Needs struct {
	Items  []Need         `json:"items"`
	Status map[string]int `json:"status"` // each mod's row icon
	Count  int            `json:"count"`  // the top bar's number
	Never  bool           `json:"never"`  // the mods' files were never checked
	Stale  bool           `json:"stale"`  // the files changed since the last check
	// Launch is the most likely trouble of a launch (app.LaunchChecks among the counted
	// items), for the launch page's tab and the TUI's launch line (06 §2.3); nil when none.
	Launch *LaunchHint `json:"launch,omitempty"`
	// LastRun says which run wrote the last-run log: "the list · 9 月 29 日 19:41" or
	// "pack X · …" (08 §5.3: a pack's errors are not the list's).
	LastRun    string `json:"lastRun,omitempty"`
	LastRunDay string `json:"lastRunDay,omitempty"` // its day: "10 月 21 日"
}

// BuildNeeds turns the problem set and the environment's open questions into items, and
// counts them (06 §7, §4.2, §7).
func BuildNeeds(a *app.App, st *app.LoadoutState, ps *app.ProblemSet) *Needs {
	n := &Needs{Status: map[string]int{}}
	n.Never = ps.Analysis == nil || ps.Analysis.Run == nil
	n.Stale = !n.Never && !ps.Analysis.Fresh
	if st == nil {
		return n
	}
	n.Items = append(buildItems(a, st, ps), directoryNeeds(a)...)
	n.Items = append(n.Items, linkNeeds(a, ps.GameLinks)...)
	for i := range n.Items {
		n.Items[i].Section = SectionOf(n.Items[i].Group)
	}
	n.Launch = launchHint(n.Items)
	for _, r := range ps.Logs {
		if r.Stale() || r.ModTime.IsZero() {
			continue
		}
		when := ShortDate(r.ModTime) + " " + r.ModTime.In(a.Loc).Format("15:04")
		n.LastRunDay = ShortDate(r.ModTime)
		if pack := a.RunOfLog(st.Ix.Env.ID, r.ModTime); pack != "" {
			n.LastRun = Tf("pack %s", pack) + "  " + when
		} else {
			n.LastRun = T("the list") + "  " + when
		}
		break
	}
	// The number in the top bar is the number of items the player can act on, and a row
	// carries an icon exactly when its mod is in one of them (06 §7, §4.2).
	for _, it := range n.Items {
		if !it.Counted {
			continue
		}
		// Hidden mods are housekeeping, not something that stops the game: they are listed
		// on the page but not counted in the sidebar's number (06 §2.1).
		if it.Group != GroupHidden {
			n.Count++
		}
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
	cur := "" // the code of the problem being turned into an item
	add := func(n Need) {
		if n.code == "" {
			n.code = cur
		}
		out = append(out, n)
	}
	var versions []*app.Problem
	var order []Need                    // order.violated: folded into one item when there are several
	var weak []Need                     // order.violated of weak or harmless constraints: one dimmed item (04 §5.3)
	logs := map[string][]*app.Problem{} // current log problems by subject mod ("" unattributed)
	// A mod's patches that miss the same inactive target, and its definitions whose parent
	// is missing, are one item each: one action covers them all (06 §5's merge rule).
	type merged struct {
		n    Need
		x, y string
		srcs []string
	}
	var mergeKeys []string
	merges := map[string]*merged{}
	// Entries the list has but nothing installed matches (an imported list, say) are one
	// item with one action for all of them; a mod needing one of them is covered by it.
	type missingEntry struct {
		pkg, ws, name string
		n             Need
	}
	var missing []missingEntry
	inList := map[string]bool{}
	for _, e := range st.List {
		if e.Missing != "" {
			inList[strings.TrimSuffix(e.Missing, "_steam")] = true
		}
	}
	merge := func(key, x, y string, n Need) {
		m := merges[key]
		if m == nil {
			n.code = cur
			m = &merged{n: n, x: x, y: y}
			merges[key] = m
			mergeKeys = append(mergeKeys, key)
		}
		if n.Evidence != nil {
			m.srcs = append(m.srcs, n.Evidence.Source)
		}
	}
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
		// A cycle is info (the corpus saw no effect, 03 §4.3), but only the player can resolve
		// it: listed with its actions, dimmed and not counted. So are weak order preferences
		// (folded) and a mod's own known error (a note): the player sees them, they are not
		// what stops the game.
		if p.Severity != finding.Error && p.Severity != finding.Warning &&
			p.Code != "order.cycle" && p.Code != "order.violated" && p.Code != "mod.known-error" {
			continue // info: technical information only (06 §5)
		}
		cur = p.Code
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
		ev := evidenceFor(p)
		switch p.Code {
		case "official.order":
			add(Need{rank: 0, Group: GroupList, Text: T("Core and the DLCs must come first in the list"), Actions: []Action{sortAction()}, Counted: true, Status: s,
				Title: T("Core and the DLCs are out of place"), Why: T("Core and the DLCs must come first in the list"), Evidence: ev})
		case "order.violated":
			if p.Severity == finding.Info {
				weak = append(weak, Need{Text: Tf("%s should load after %s", nm(x), nm(y)), Mods: ids})
				continue
			}
			order = append(order, Need{rank: 1, Group: GroupList, Text: Tf("%s should load after %s", nm(x), nm(y)), Actions: []Action{sortAction()}, Counted: true, Mods: ids, Status: s,
				Why: T("a strong requirement (a dependency or loadBefore): breaking it goes with more errors in shared logs; sorting puts it right"), Evidence: ev})
		case "mod.known-error":
			// The mod's own problem (03 §4.3): whatever else is enabled, shared logs show it.
			add(Need{rank: 9, Group: GroupConflict, Text: Tf("%s has a known error: this is the mod's own problem", nm(x)),
				Actions: []Action{viewAction(T("see the error"), "problems", "show", p.Ref)}, Mods: ids, Status: StatusWarn, Dim: true,
				Title: Tf("%s has a known error: this is the mod's own problem", nm(x)), Why: T("not your list's: shared logs show it whatever else is enabled; an update may fix it"), Evidence: ev})
		case "order.cycle":
			add(Need{rank: 2, Group: GroupList, Text: Tf("the order requirements of %d mods contradict each other", len(subj)),
				Actions: []Action{cycleView(st, p), cycleIgnore(st, p)}, Counted: p.Severity != finding.Info, Dim: p.Severity == finding.Info, Mods: ids, Status: s,
				Why: T("they stay in the current order; ignoring the suggested requirement lets sorting work"), Evidence: ev})
		case "pkg.duplicate-active":
			add(Need{rank: 3, Group: GroupList, Text: Tf("%s is enabled twice", nm(x)), Actions: []Action{keepOne(st, ids)}, Counted: true, Mods: ids, Status: s,
				Why: T("the game loads only one of them; you pick which"), Evidence: ev})
		case "entry.missing":
			pkg := strings.TrimPrefix(x, "pkg:")
			look := Action{Kind: ActOpenURL, Key: "o", Label: T("look for it in the Workshop"), URL: WorkshopSearch(pkg), Done: T("opened the Workshop page")}
			for _, sg := range p.Suggestions {
				if u, ok := strings.CutPrefix(sg, "subscribe: "); ok { // steamDB knows the item (04 §6.3)
					look.Label, look.URL = T("open it in the Workshop"), u
				}
			}
			me := missingEntry{pkg: pkg, name: pkg, n: Need{rank: 4, Group: GroupList, Text: Tf("the list has %s, but it is not installed", pkg),
				Actions: []Action{look, disable(st, "pkg:"+pkg, T("remove it from the list"))}, Counted: true, Status: s,
				Title: Tf("not installed: %s", pkg), Why: T("the list has it, but no mod folder does"), Evidence: ev}}
			if w, ok := a.WorkshopFor(pkg); ok {
				me.ws = w.WorkshopID
				if w.Name != "" {
					me.name = w.Name
					me.n.Text = Tf("the list has %s, but it is not installed", w.Name)
					me.n.Title = Tf("not installed: %s", w.Name)
				}
			}
			missing = append(missing, me)
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
			add(Need{rank: 5, Group: GroupList, Text: Tf("%s has a maintained replacement: %s", nm(x), name), Actions: acts, Counted: true, Mods: ids, Status: s,
				Title: Tf("%s has a maintained replacement", nm(x)), Why: Tf("%s is no longer kept up; %s does the same and is", nm(x), name), Evidence: ev})
		case "mod.unloadable":
			add(Need{rank: 4, Group: GroupList, Text: Tf("%s is enabled, but the game cannot load it", nm(x)), Actions: []Action{disable(st, x, T("disable it"))}, Counted: true, Mods: ids, Status: s,
				Title: Tf("the game cannot load %s", nm(x)), Why: T("it is enabled, but the game skips it; disabling it changes nothing in play"), Evidence: ev})
		case "dep.missing":
			if inList[strings.TrimSuffix(strings.TrimPrefix(y, "pkg:"), "_steam")] {
				continue // the list has it, not installed: the missing entries' item covers it
			}
			dep := depName(p.Claim, strings.TrimPrefix(y, "pkg:"))
			add(Need{Group: GroupDeps, Text: Tf("%s needs %s, but %s is not installed", nm(x), dep, dep), Actions: []Action{workshop(p, dep)}, Counted: true, Mods: ids, Status: s,
				Title: Tf("missing dependency: %s", dep), Why: Tf("%s needs it, but it is not installed.", nm(x)), Evidence: ev})
		case "dep.inactive":
			add(Need{Group: GroupDeps, Text: Tf("%s needs %s, but %s is not enabled", nm(x), nm(y), nm(y)), Actions: []Action{enable(st, y)}, Counted: true, Mods: ids, Status: s,
				Title: Tf("missing dependency: %s", nm(y)), Why: Tf("%s needs it, but it is not enabled.", nm(x)), Evidence: ev})
		case "dep.observed-missing": // undeclared, seen in shared logs (internal/known)
			if strings.HasPrefix(y, "pkg:") {
				dep := strings.TrimPrefix(y, "pkg:")
				add(Need{Group: GroupDeps, Text: Tf("%s seems to need %s (seen in shared logs), which is not installed", nm(x), dep), Actions: []Action{workshop(p, dep)}, Counted: true, Mods: ids, Status: s,
					Title: Tf("probably missing: %s", dep), Why: Tf("%s does not declare it, but in shared logs it fails without it. It is not installed.", nm(x)), Evidence: ev})
			} else {
				add(Need{Group: GroupDeps, Text: Tf("%s seems to need %s (seen in shared logs), which is not enabled", nm(x), nm(y)), Actions: []Action{enable(st, y)}, Counted: true, Mods: ids, Status: s,
					Title: Tf("probably missing: %s", nm(y)), Why: Tf("%s does not declare it, but in shared logs it fails without it. It is not enabled.", nm(x)), Evidence: ev})
			}
		case "combo.known-bad":
			if y != "" {
				add(Need{Group: GroupConflict, Text: Tf("%s and %s often produce errors together (seen in shared logs)", nm(x), nm(y)), Actions: []Action{disableOne(st, x, y)}, Counted: true, Mods: ids, Status: s,
					Title: Tf("%s and %s often fail together", nm(x), nm(y)), Why: T("in shared logs this pair produces errors far more often than either one alone. Disable one if you see errors."), Evidence: ev})
			} else {
				add(Need{Group: GroupConflict, Text: Tf("this version of %s often produces errors (seen in shared logs); other versions did not", nm(x)), Actions: []Action{disable(st, x, Tf("disable %s", nm(x)))}, Counted: true, Mods: ids, Status: s,
					Title: Tf("this version of %s often fails", nm(x)), Why: T("in shared logs this version produces errors; other versions did not. Updating it usually helps."), Evidence: ev})
			}
		case "patch.no-match":
			if y == "" {
				continue // no inactive target: nothing the player can do (technical information)
			}
			merge("patch|"+x+"|"+y, nm(x), nm(y), Need{Group: GroupDeps, Text: Tf("part of %s does not take effect: %s, which it targets, is not enabled", nm(x), nm(y)),
				Actions: []Action{enable(st, y), viewAction(T("see what"), "patches", "--mod", TokRef(st, x))}, Counted: true, Mods: ids, Status: s,
				Title: Tf("part of %s does not take effect", nm(x)), Why: Tf("it changes things %s adds, and %s is not enabled.", nm(y), nm(y)), Evidence: ev})
		case "incompat.active":
			add(Need{Group: GroupConflict, Text: Tf("%s is incompatible with %s", nm(x), nm(y)), Actions: []Action{disableOne(st, x, y)}, Counted: true, Mods: ids, Status: s,
				Why: T("one of them says so; with both enabled the game misbehaves. Disable one."), Evidence: ev})
		case "mod.pinned":
			add(Need{Group: GroupHidden, Text: Tf("%s is locked to another copy, and the workshop version has been updated since", nm(x)),
				Actions: []Action{viewAction(T("see what changed"), "mod", TokRef(st, y), "--diff", TokRef(st, x)), unpin(st, x)}, Counted: true, Mods: ids, Status: s,
				Title: Tf("%s: the Workshop has a newer version", nm(x)), Why: T("the list uses a locked copy; see what changed, or use the Workshop version"), Evidence: ev})
		case "texture.stale":
			ref := TokRef(st, x)
			add(Need{Group: GroupHidden, Text: Tf("%s changed since its textures were converted", nm(x)),
				Actions: []Action{
					{Kind: ActTextures, Key: "T", Label: T("convert again"), Refs: []string{ref}, Confirm: T("press T again to convert its textures again"),
						Warn: T("RimArcho writes the DDS again next to the PNGs that changed"), Done: Tf("converted the textures of %s again", nm(x))},
					{Kind: ActTexClean, Key: "C", Label: T("remove the converted textures"), Refs: []string{ref}, Confirm: T("press C again to delete the DDS RimArcho wrote"),
						Warn: T("the DDS RimArcho wrote are deleted; the PNGs stay and the game uses them again"), Done: Tf("removed the converted textures of %s", nm(x))},
				}, Counted: true, Mods: ids, Status: s,
				Title: Tf("%s changed since its textures were converted", nm(x)), Why: T("the game may show its old textures: convert them again, or remove them"), Evidence: ev})
		case "mod.hidden-residue":
			add(Need{Group: GroupHidden, Text: Tf("%s is hidden and no longer subscribed; its folder is left over", nm(x)),
				Actions: []Action{trash(st, x)}, Counted: true, Mods: ids, Status: s,
				Title: Tf("%s is left over", nm(x)), Why: T("it is hidden and no longer subscribed; its folder still takes space"), Evidence: ev})
		case "mod.hidden":
			add(Need{Group: GroupHidden, Text: Tf("%s is hidden from the game (probably by another mod manager)", nm(x)),
				Actions: []Action{keepHidden(p), unhide(st, x), trash(st, x)}, Counted: true, Mods: ids, Status: s,
				Title: Tf("%s is hidden from the game", nm(x)), Why: T("probably by another mod manager; keep it hidden, unhide it, or recycle it"), Evidence: ev})
		case app.CodePackLinks:
			add(packLinksNeed(p))
		case app.CodeModsDuplicate:
			n := modsDuplicateNeed(st, p, ids)
			for i := range n.Actions {
				act := &n.Actions[i]
				if act.Kind == ActMoveLocked && len(act.Refs) > 0 {
					if r, err := a.MoveToLocked(act.Refs[0], true); err == nil {
						act.Warn += "\n" + strings.Join(r.Links.Lines(), "\n")
					} else {
						act.Warn += "\n" + err.Error()
					}
				}
			}
			add(n)
		case "def.parent-missing":
			merge("def|"+x, nm(x), "", Need{Group: GroupDeps, Text: Tf("part of %s builds on a definition no enabled mod provides; a dependency may be missing", nm(x)),
				Actions: []Action{viewAction(T("see what"), "mod", TokRef(st, x))}, Counted: true, Mods: ids, Status: s,
				Title: Tf("part of %s may be missing a dependency", nm(x)), Why: T("it builds on a definition no enabled mod provides."), Evidence: ev})
		default:
			// Harmony warnings without log evidence, overlapping Defs and patches whose target
			// is simply absent stay in the technical information (06 §5).
		}
	}
	switch len(missing) {
	case 0:
	case 1:
		n := missing[0].n
		if ws := missing[0].ws; ws != "" {
			n.Actions = append([]Action{subscribeAction([]string{ws}, T("subscribe to it"))}, n.Actions...)
		}
		add(n)
	default:
		f := Need{rank: 4, Group: GroupList, Counted: true, Status: missing[0].n.Status, Fold: true, code: "entry.missing",
			Text:     Tf("%d mods in the list are not installed", len(missing)),
			Why:      T("the list has them, but no mod folder does; once subscribed and downloaded they take their place in the list"),
			Evidence: missing[0].n.Evidence}
		var ws, refs []string
		for _, m := range missing {
			f.Parts = append(f.Parts, m.name)
			refs = append(refs, TokRef(st, "pkg:"+m.pkg))
			if m.ws != "" {
				ws = append(ws, m.ws)
			}
		}
		if len(ws) > 0 {
			label := Tf("subscribe to all %d", len(ws))
			if len(ws) < len(missing) {
				label = Tf("subscribe to the %d on the Workshop", len(ws))
			}
			f.Actions = append(f.Actions, subscribeAction(ws, label))
		}
		f.Actions = append(f.Actions,
			Action{Kind: ActDisable, Key: "x", Label: T("remove them from the list"), Refs: refs, Done: Tf("removed %d mods from the list", len(refs))},
			Action{Kind: ActFold, Key: "f", Label: T("show which")})
		add(f)
	}
	for _, k := range mergeKeys {
		m := merges[k]
		n := m.n
		if c := len(m.srcs); c > 1 {
			if strings.HasPrefix(k, "def|") {
				n.Title = Tf("%d parts of %s may be missing a dependency", c, m.x)
				n.Text = Tf("%d parts of %s build on definitions no enabled mod provides; a dependency may be missing", c, m.x)
				n.Why = T("they build on definitions no enabled mod provides.")
			} else {
				n.Title = Tf("%d patches of %s do not take effect", c, m.x)
				n.Text = Tf("%d patches of %s do not take effect: %s, which they target, is not enabled", c, m.x, m.y)
				n.Why = Tf("they change things %s adds, and %s is not enabled.", m.y, m.y)
			}
			n.Parts, n.Fold = m.srcs, true
			ev := *n.Evidence
			ev.Source = Tf("%s and %d more", m.srcs[0], c-1)
			n.Evidence = &ev
			n.Actions = append(append([]Action{}, n.Actions...), Action{Kind: ActFold, Key: "f", Label: T("show which")})
		}
		add(n)
	}
	if len(weak) > 0 {
		// Weak and harmless constraints (04 §5.3): breaking them went with no more errors in
		// shared logs. One dimmed, uncounted item; sorting still puts them right.
		f := Need{rank: 8, Group: GroupList, Status: StatusWarn, Dim: true, Fold: true, code: "order.violated",
			Text:     Tf("%d weaker order preferences are not met", len(weak)),
			Why:      T("loadAfter and community rules: breaking them showed no more errors in shared logs; sorting puts them right"),
			Evidence: evidence(EvCorpus, T("load-order constraint strength"))}
		for _, w := range weak {
			f.Parts = append(f.Parts, w.Text)
		}
		f.Actions = []Action{sortAction(), {Kind: ActFold, Key: "f", Label: T("show which")}}
		add(f)
	}
	switch len(order) {
	case 0:
	case 1:
		add(order[0])
	default:
		f := Need{rank: 1, Group: GroupList, Counted: true, Status: StatusWarn, Fold: true, code: "order.violated"}
		seen := map[string]bool{}
		for _, o := range order {
			f.Parts = append(f.Parts, o.Text)
			if len(o.Mods) > 0 && !seen[o.Mods[0]] {
				seen[o.Mods[0]] = true
				f.Mods = append(f.Mods, o.Mods[0])
			}
		}
		f.Text = Tf("%d mods are out of order", len(seen))
		f.Why = T("sorting puts them right in one step; show which to see each one")
		f.Evidence = order[0].Evidence
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
				text = Tf("%d mods do not list %s as supported (%d of them list %s)", n, mm, warn, prev)
			}
			names := make([]string, 0, n)
			for _, v := range versions {
				names = append(names, nm(v.Subjects[0]))
			}
			sort.Strings(names)
			add(Need{rank: 5, Group: GroupList, Text: text, Counted: true, Status: StatusWarn,
				Why: T("they may still work, but their authors have not said they do on this version"), Evidence: evidence(EvDeclared, T("About.xml supportedVersions")),
				Actions: []Action{{Kind: ActList, Key: "enter", Label: T("see the list"), Title: text, Lines: names}}})
		}
	}
	if v := env.MajorMinor(st.FileVersionMismatch()); v != "" {
		add(Need{rank: 6, Group: GroupList, Text: Tf("the list was written for %s; the game resets it when it starts — launching from RimArcho or writing to the game once writes it for the current version", v),
			Actions: []Action{{Kind: ActSave, Key: "w", Label: T("write to the game")}}, Counted: true, Status: StatusWarn,
			Title: Tf("the list was written for %s", v), Why: T("the game resets it when it starts; writing it to the game once fixes that"), Evidence: evidence(EvInferred, Tf("ModsConfig.xml says %s", v))})
	}
	// Last run: the current log's errors, one item per mod (06 §2.4).
	logWhen := ""
	for _, r := range ps.Logs {
		if !r.Stale() && !r.ModTime.IsZero() {
			logWhen = ShortDate(r.ModTime) + " " + r.ModTime.In(a.Loc).Format("15:04")
			break
		}
	}
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
		var text, why string
		switch {
		case id == "":
			why = T("the log does not say which mod they come from")
		default:
			why = T("they most likely come from this mod; see the log and evidence before changing anything")
		}
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
		add(Need{Group: GroupLastRun, Text: text, Actions: []Action{viewAction(T("see the log and evidence"), argv...)}, Counted: true, Mods: ids, Status: StatusLog,
			Why: why, Evidence: logEvidence(logs[id], logWhen)})
	}
	for _, r := range ps.Logs {
		if r.Stale() {
			add(Need{Group: GroupLastRun, Text: Tf("earlier log  %s  %s", ShortDate(r.ModTime), r.MM), Dim: true,
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
		add(Need{Group: GroupConfirm, Text: T("ModsConfig.xml was changed outside RimArcho"), Counted: true, Status: StatusWarn,
			Why: T("this list was changed after it too: use the outside change, or keep yours"), Evidence: evidence(EvInferred, T("ModsConfig.xml differs from what RimArcho wrote")), Actions: []Action{
				viewAction(T("see the difference"), "loadout", "diff", fmt.Sprintf("v%d", v), "workspace"),
				// One step: u brings the list back (06 §6), so no confirmation.
				{Kind: ActLoadExternal, Key: "L", Label: T("use the outside change"), Version: v, Undo: true,
					Done: T("the outside change is in use")},
				{Kind: ActDismissConflict, Key: "I", Label: T("ignore it"), Done: T("the outside change is ignored (it stays in the history)")},
			}})
	}
	// The game now is not among the versions the list allows (04 §1.6): something to
	// confirm, not a block; version.unsupported still checks each mod against the game.
	if pv, err := a.PlanVersionsOf(st, st.Plan); err == nil && pv.Game != "" && len(pv.Allowed) > 0 && !slices.Contains(pv.Allowed, pv.Game) {
		name, allowed := st.Plan.Name, app.FormatVersions(pv.Allowed)
		add(Need{Group: GroupConfirm, Counted: true, Status: StatusWarn,
			Text:     Tf("%s allows %s; the game now is %s", name, allowed, pv.Game),
			Title:    Tf("%s allows %s; the game now is %s", name, allowed, pv.Game),
			Why:      T("the list says which game versions it is for; add this one if you play it here. Launching is not stopped."),
			Evidence: evidence(EvInferred, T("game versions")),
			Actions: []Action{
				{Kind: ActAddVersion, Key: "V", Label: Tf("add %s", pv.Game), Refs: []string{name, pv.Game}},
				{Kind: ActManageLists, Key: "M", Label: T("go to list management"), Argv: []string{"plan", "versions", name}},
			}})
	}
	// Other plans of the environment with an unloaded outside change (04 §4.3): resolved
	// after switching to them.
	if cs, err := a.Conflicts(st); err == nil {
		for _, c := range cs {
			if !c.Current {
				add(Need{Group: GroupConfirm, Text: Tf("ModsConfig.xml was changed outside RimArcho; list %s keeps it as v%d, not used yet", c.Plan.Name, c.Version),
					Counted: true, Status: StatusWarn})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].rank < out[j].rank
	})
	return out
}

// packLinksNeed: a pack's links left in Mods/ (08 §5.3). Launching from RimArcho removes
// them anyway, so it is something to decide, not something that breaks the game: in
// 要你确认, with one action that removes only the links (no confirmation: launching the
// pack makes them again).
func packLinksNeed(p *app.Problem) Need {
	var packs []string
	for _, s := range p.Subjects {
		if n, ok := strings.CutPrefix(s, "pack:"); ok {
			packs = append(packs, n)
		}
	}
	name := strings.Join(packs, ", ")
	if name == "" {
		name = "?"
	}
	var src []string
	for _, e := range p.Evidence {
		src = append(src, e.Text)
	}
	act := Action{Kind: ActUnlinkPacks, Key: "X", Label: T("remove the links"), Done: Tf("the links of pack %s are out of Mods/", name)}
	n := Need{Group: GroupConfirm, rank: -1, Counted: true, Status: StatusWarn, Actions: []Action{act},
		Text:     Tf("pack %s left its links in Mods/: a game started from Steam loads the pack's copies", name),
		Title:    Tf("pack %s left its links in Mods/", name),
		Why:      T("a game started from Steam, not from RimArcho, loads the pack's copies instead of this list's mods; launching from RimArcho removes them, and launching the pack makes them again"),
		Evidence: evidence(EvInferred, strings.Join(src, ", "))}
	if p.Key == "duplicate" {
		// Next to other copies of the same mods: the game reports an error at every start
		// from Steam, so it is one that breaks the game.
		n.Group, n.rank, n.Status = GroupList, 3, StatusErr
		n.Title = Tf("pack %s's links put mods twice in Mods/", name)
		n.Text = Tf("pack %s left its links in Mods/ next to other copies of the same mods: the game reports an error for each", name)
		n.Why = T("a game started from Steam finds two folders with one packageId, reports an error and ignores one; launching from RimArcho removes the links, and launching the pack makes them again without the duplicates")
	}
	if p.Key == "running" {
		n.Status = StatusErr
		n.Text = Tf("the game is running with pack %s's copies: it was not started from RimArcho", name)
		n.Title = Tf("the game is running with pack %s's copies", name)
		n.Why = T("it was started from Steam while the pack's links were in Mods/, so it loaded the pack's copies; close the game, then remove the links")
	}
	return n
}

// modsDuplicateNeed: two folders of one packageId in Mods/ (08 §3). The one action never
// moves a folder RimArcho did not make: it uses the copy in Mods/ instead of RimArcho's link,
// or takes the linked mod out of the plan; with only the player's folders it says what to do.
func modsDuplicateNeed(st *app.LoadoutState, p *app.Problem, ids []string) Need {
	pkg := p.Key
	var where []string
	for _, e := range p.Evidence {
		// "Mods/<name> (<who made it>)": the part in brackets in the UI language
		t := e.Text
		if i := strings.LastIndex(t, " ("); i > 0 && strings.HasSuffix(t, ")") {
			t = t[:i] + " (" + T(t[i+2:len(t)-1]) + ")"
		}
		where = append(where, t)
	}
	n := Need{Group: GroupList, rank: 3, Counted: true, Status: StatusErr, Mods: ids,
		Title:    Tf("Mods/ holds %s twice", pkg),
		Text:     Tf("Mods/ has %d folders with packageId %s: the game reports an error and ignores all but one", len(where), pkg),
		Evidence: evidence(EvInferred, strings.Join(where, ", "))}
	var move *Action
	sgs := p.Suggestions
	if len(sgs) > 0 && strings.HasPrefix(sgs[0], "archo library move-to-locked ") {
		tok := strings.TrimPrefix(sgs[0], "archo library move-to-locked ")
		name := TokName(st, tok)
		move = &Action{Kind: ActMoveLocked, Key: "M", Label: T("move into the lock library"), Refs: []string{TokRef(st, tok)},
			Confirm: T("press M again to move the folder into the lock library"),
			Warn:    Tf("the folder of %s leaves Mods/ for the lock library; lists that use it keep it and get it through a link", name),
			Done:    Tf("%s is in the lock library", name)}
		sgs = sgs[1:]
	}
	sg := ""
	if len(sgs) > 0 {
		sg = sgs[0]
	}
	f := strings.Fields(sg)
	switch {
	case len(f) == 4 && f[1] == "swap":
		from, to := f[2], f[3]
		n.Why = Tf("RimArcho links %s for this list, and Mods/ already has the same mod; use the copy in Mods/ (the link goes the next time the list is written to the game)", TokName(st, from))
		n.Actions = []Action{{Kind: ActSwap, Key: "x", Label: T("use the copy in Mods/"), Refs: []string{TokRef(st, from), TokRef(st, to)}, Undo: true,
			Done: Tf("the list uses the copy of %s in Mods/ (not written yet)", TokName(st, to))}}
	case len(f) == 3 && f[1] == "disable":
		n.Why = Tf("RimArcho links %s for this list, and Mods/ has another mod with the same packageId; disable it, or move the folder out of Mods/", TokName(st, f[2]))
		n.Actions = []Action{disable(st, f[2], T("disable it"))}
	default:
		n.Why = T("RimArcho did not make these folders and does not move them: move all but one out of Mods/ yourself")
		n.Actions = []Action{{Kind: ActList, Key: "enter", Label: T("see which"), Title: Tf("Mods/ holds %s twice", pkg), Lines: where}}
	}
	if move != nil {
		// 收进锁定库 first: the plain folder leaves Mods/, plans keep it (08 §3)
		n.Why = T("move the plain folder into the lock library: it leaves Mods/, and lists that use it keep it and get it through a link")
		n.Actions = append([]Action{*move}, n.Actions...)
	}
	return n
}

func questionNeed(st *app.LoadoutState, q *store.Question) Need {
	nm := func(id string) string { return TokName(st, finding.Inst(id)) }
	x, y := nm(q.InstanceID), nm(q.CandidateID)
	answer := func(key, label, ans, done string) Action {
		// Merging or splitting lineages is not undone with u: confirm first (06 §6).
		return Action{Kind: ActAnswer, Key: key, Label: label, QID: q.ID, Answer: ans, Done: done, Confirm: Tf("press %s again: %s", key, done), Warn: done}
	}
	return Need{Group: GroupConfirm, Text: Tf("is %s the same mod as %s?", x, y), Counted: true, Status: StatusWarn,
		Why: T("they look alike; your answer decides whether lists and history treat them as one"), Evidence: evidence(EvInferred, T("RimArcho compared their folders")),
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
	return Action{Kind: ActUnhide, Key: "R", Label: T("unhide"), Refs: []string{TokRef(st, tok)},
		Confirm: T("press R again to rename About.xml.disabled back to About.xml"), Warn: Tf("RimArcho renames About.xml.disabled back to About.xml in %s", name),
		Done: Tf("%s is visible to the game again", name)}
}

func unpin(st *app.LoadoutState, tok string) Action {
	name := TokName(st, tok)
	return Action{Kind: ActUnpin, Key: "P", Label: T("use the workshop version"), Refs: []string{TokRef(st, tok)},
		Confirm: T("press P again to unhide the workshop copy and use it"), Warn: Tf("RimArcho unhides the workshop copy of %s and uses it instead of the locked one; that one stays on disk", name),
		Done: Tf("the list now uses the workshop version of %s (not written to the game yet)", name)}
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

// subscribeAction subscribes to Workshop items (the interface asks Steam or SteamCMD).
func subscribeAction(ws []string, label string) Action {
	return Action{Kind: ActSubscribe, Key: "s", Label: label, Refs: ws, Argv: append([]string{"subscribe", "workshop"}, ws...),
		Done: Tf("subscribing to %d mods in the background", len(ws))}
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
		act.Options = append(act.Options, Option{Label: Tf("keep the one from %s", SourceText(mod)+"  "+mod.Rel), Refs: off, Done: T("kept one copy")})
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
	case ActUnlinkPacks:
		_, err = a.UnlinkPacks(false)
	case ActRepairLinks:
		if len(act.Refs) != 1 {
			return "", fmt.Errorf("missing link repair preview")
		}
		err = a.ConfirmLinkRepair(act.Refs[0])
	case ActSwap:
		_, err = a.Swap(act.Refs[0], act.Refs[1])
	case ActMoveLocked:
		_, err = a.MoveToLocked(act.Refs[0], false)
	case ActTextures:
		r, err := a.OptimizeTextures(app.TextureRun{Refs: act.Refs})
		if err != nil {
			return "", err
		}
		return TextureRunText(r), nil
	case ActTexClean:
		_, err = a.CleanTextures(act.Refs, app.TexRefs, false)
	case ActAddVersion:
		st, err := a.Loadout()
		if err != nil {
			return "", err
		}
		p, err := a.FindPlan(act.Refs[0])
		if err != nil {
			return "", err
		}
		vs := append(append([]string{}, a.PlanAllowedVersions(st, p)...), act.Refs[1])
		r, err := a.SetPlanVersions(p.Name, vs)
		if err != nil {
			return "", err
		}
		return Tf("%s: game versions %s", p.Name, app.FormatVersions(r.After.Allowed)), nil
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
		s += "  " + T("suggested to ignore (x)")
	}
	return s
}

var idRe = regexp.MustCompile(`#[0-9a-z]{3,8}\b`)

// Humanize replaces instance references by mod names and drops the "error: " prefix.
func Humanize(st *app.LoadoutState, s string) string {
	s = Text(strings.TrimPrefix(strings.TrimSpace(s), "error: "))
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
	if Lang() == "zh" {
		return fmt.Sprintf("%d 月 %d 日", int(t.Month()), t.Day())
	}
	return t.Format("Jan 2")
}
