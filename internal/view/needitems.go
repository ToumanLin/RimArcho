package view

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// itemBuilder turns a problem set into Can it run's items (buildItems): the per-problem
// items as it walks the list, then the folded and list-wide ones.
type itemBuilder struct {
	a  *app.App
	st *app.LoadoutState
	ps *app.ProblemSet

	// what disabling one of two conflicting mods leaves without a requirement (disableOne),
	// read once for all of them
	deps *app.Dependents

	out         []Need
	cur         string // the code of the problem being turned into an item
	linkProblem *app.Problem

	parentSeen   map[string]bool
	parentGroups map[string][]*app.Problem
	versions     []*app.Problem
	// patch.no-match with no inactive target, by subject mod: one row mark each (16 §5)
	patchOrder []string
	patchOther map[string][]*app.Problem
	order      []Need                    // order.violated: folded into one item when there are several
	weak       []Need                    // order.violated of weak or harmless constraints: one dimmed item (08 §1.3)
	logs       map[string][]*app.Problem // current log problems by subject mod ("" unattributed)
	// A mod's patches that miss the same inactive target, and its definitions whose parent
	// is missing, are one item each: one action covers them all (16 §5's merge rule).
	mergeKeys []string
	merges    map[string]*mergedNeed
	// Unresolved entries are grouped by whether an installed copy can be used;
	// a mod needing one of them is covered by the same item.
	missingInfo                     map[string]app.MissingMod
	missing, installed, downloading []missingEntry
	missingOwners                   map[string][]ModOwner
	inList                          map[string]bool
}

// mergedNeed is one item standing for several problems of one mod (16 §5's merge rule).
type mergedNeed struct {
	n    Need
	x, y string
	srcs []string
}

// missingEntry is one list entry that is not installed.
type missingEntry struct {
	pkg, ws, name string
	ref           string // the list row's ref (pkg:<entry>), which keys its icon's reason
	n             Need
}

// problemCtx is what every per-problem item reads of its problem.
type problemCtx struct {
	p    *app.Problem
	subj []string
	x, y string
	ids  []string
	s    int
	ev   *Evidence
}

// newItemBuilder prepares the indexes the per-problem items read.
func newItemBuilder(a *app.App, st *app.LoadoutState, ps *app.ProblemSet, missingMods []app.MissingMod) *itemBuilder {
	b := &itemBuilder{a: a, st: st, ps: ps,
		parentSeen: map[string]bool{}, parentGroups: map[string][]*app.Problem{},
		patchOther: map[string][]*app.Problem{}, logs: map[string][]*app.Problem{},
		merges: map[string]*mergedNeed{}, missingInfo: map[string]app.MissingMod{},
		missingOwners: map[string][]ModOwner{}, inList: map[string]bool{}}
	for _, p := range ps.List {
		if p.Code == "def.parent-missing" {
			b.parentGroups[p.SubjectInstance()] = append(b.parentGroups[p.SubjectInstance()], p)
		}
	}
	for _, m := range missingMods {
		b.missingInfo[strings.ToLower(m.PackageID)] = m
	}
	for _, e := range st.List {
		if e.Missing != "" {
			b.inList[dependencyTarget(st, "pkg:"+e.Missing)] = true
		}
	}
	return b
}

// nm is a token's display name, the Workshop's when the mod is known only by package id.
func (b *itemBuilder) nm(tok string) string {
	name := TokName(b.st, tok)
	if pkg, ok := strings.CutPrefix(tok, "pkg:"); ok && name == strings.ToLower(pkg) {
		if w, ok := b.a.WorkshopFor(pkg); ok && w.Name != "" {
			return w.Name
		}
	}
	return name
}

// dependents lists the mods that need id, reading the dependents once.
func (b *itemBuilder) dependents(id string) []*store.Mod {
	if b.deps == nil {
		d, err := b.a.NewDependents(b.st)
		if err != nil {
			return nil
		}
		b.deps = d
	}
	return b.deps.Of([]string{id})
}

// add appends an item, giving it the current problem's code and link.
func (b *itemBuilder) add(n Need) {
	if n.code == "" {
		n.code = b.cur
	}
	if b.linkProblem != nil {
		n.linkProblems = append(n.linkProblems, b.linkProblem)
	}
	b.out = append(b.out, n)
}

// merge adds n to the merged item under key, starting it when it is the first.
func (b *itemBuilder) merge(key, x, y string, n Need) {
	m := b.merges[key]
	if m == nil {
		n.code = b.cur
		m = &mergedNeed{n: n, x: x, y: y}
		b.merges[key] = m
		b.mergeKeys = append(b.mergeKeys, key)
	}
	if n.Evidence != nil {
		m.srcs = append(m.srcs, n.Evidence.Source)
	}
	m.n.linkProblems = append(m.n.linkProblems, b.linkProblem)
}

// buildItems is Can it run's items, sorted by group and rank.
func buildItems(a *app.App, st *app.LoadoutState, ps *app.ProblemSet, missingMods []app.MissingMod) []Need {
	b := newItemBuilder(a, st, ps, missingMods)
	for _, p := range ps.List {
		b.problem(p)
	}
	b.patchItems()
	// what is added from here on says its own code: cur is the last problem's
	b.cur, b.linkProblem = "", nil
	b.missingOwnersOf()
	b.installedItem()
	b.downloadingItem()
	b.missingItem()
	b.mergedItems()
	b.weakItem()
	b.orderItem()
	b.versionItem()
	if len(ps.Muted) > 0 {
		b.add(mutedVersionsNeed(st, ps.Muted))
	}
	if v := env.MajorMinor(st.FileVersionMismatch()); v != "" {
		b.add(Need{rank: 6, Group: GroupList, Text: Tf("the list was saved for %s; the game resets it when it starts — launching from RimArcho or saving once saves it for the current version", v),
			Actions: []Action{{Kind: ActSave, Key: "w", Label: T("save")}}, Counted: true, Status: StatusWarn,
			Title: Tf("the list was saved for %s", v), Why: T("the game resets it when it starts; saving once fixes that"), Evidence: evidence(EvInferred, Tf("ModsConfig.xml says %s", v))})
	}
	b.lastRunItems()
	b.confirmItems()
	out := b.out
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].rank < out[j].rank
	})
	return out
}

// problem files one problem: a log problem by its mod, a version problem for the list-wide
// item, anything the player can act on as its item.
func (b *itemBuilder) problem(p *app.Problem) {
	if p.Log != nil {
		if !p.Log.Stale() && (p.Severity == finding.Error || p.Severity == finding.Warning) {
			b.logs[p.SubjectInstance()] = append(b.logs[p.SubjectInstance()], p)
		}
		return
	}
	if p.Code == "version.unsupported" {
		b.versions = append(b.versions, p)
		return
	}
	// A cycle is info (the corpus saw no effect, 05 §4.3), but only the player can resolve
	// it: listed with its actions, dimmed and not counted. So are weak order preferences
	// (folded) and a mod's own known error (a note): the player sees them, they are not
	// what stops the game.
	if p.Severity != finding.Error && p.Severity != finding.Warning &&
		p.Code != "order.cycle" && p.Code != "order.violated" && p.Code != "mod.known-error" &&
		p.Code != "dep.observed-missing" && p.Code != "combo.known-bad" {
		return // info: technical information only (16 §5)
	}
	b.cur = p.Code
	b.linkProblem = p
	c := problemCtx{p: p, subj: subjects(p), s: StatusWarn, ev: evidenceFor(p)}
	if len(c.subj) > 0 {
		c.x = c.subj[0]
	}
	if len(c.subj) > 1 {
		c.y = c.subj[1]
	}
	c.ids = instIDs(c.subj)
	if p.Severity == finding.Error {
		c.s = StatusErr
	}
	b.item(c)
}

// item turns one actionable problem into its item, by code.
func (b *itemBuilder) item(c problemCtx) {
	st, p, x, y, ids, s, ev, nm := b.st, c.p, c.x, c.y, c.ids, c.s, c.ev, b.nm
	switch p.Code {
	case "official.order":
		b.add(Need{rank: 0, Group: GroupList, Text: T("Core and the DLCs must come first in the list"), Actions: []Action{sortAction()}, Counted: true, Status: s,
			Title: T("Core and the DLCs are out of place"), Why: T("Core and the DLCs must come first in the list"), Evidence: ev})
	case "order.violated":
		b.orderViolated(c)
	case "mod.known-error":
		// The mod's own problem (05 §4.3): whatever else is enabled, shared logs show it.
		b.add(Need{rank: 9, Group: GroupConflict, Text: Tf("%s has a known error: this is the mod's own problem", nm(x)),
			Actions: []Action{problemAction(p.Ref)}, Mods: ids, Status: StatusWarn, Dim: true,
			Title: Tf("%s has a known error: this is the mod's own problem", nm(x)), Why: T("not your list's: shared logs show it whatever else is enabled; an update may fix it"), Evidence: ev})
	case "order.cycle":
		b.add(Need{rank: 2, Group: GroupList, Text: Tf("sorting keeps the strongest constraints in a cycle of %d mods", len(c.subj)),
			Actions: []Action{cycleView(st, p)}, Counted: false, Dim: true, Mods: ids,
			Why: cycleResolution(st, p), Evidence: ev})
	case "pkg.duplicate-active":
		b.add(Need{rank: 3, Group: GroupList, Text: Tf("%s is enabled twice", nm(x)), Actions: []Action{keepOne(st, ids)}, Counted: true, Mods: ids, Status: s,
			Why: T("the game loads only one of them; you pick which"), Evidence: ev})
	case "entry.missing":
		b.entryMissing(c)
	case "mod.replaced":
		b.modReplaced(c)
	case "mod.unloadable":
		b.add(Need{rank: 4, Group: GroupList, Text: Tf("%s is enabled, but the game cannot load it", nm(x)), Actions: []Action{disable(st, x, T("disable it"))}, Counted: true, Mods: ids, Status: s,
			Title: Tf("the game cannot load %s", nm(x)), Why: T("it is enabled, but the game skips it; disabling it changes nothing in play"), Evidence: ev})
	case "dep.missing":
		b.depMissing(c)
	case "dep.inactive":
		b.add(Need{Group: GroupDeps, Target: dependencyTarget(st, y), RequiredBy: owners(st, x), Text: Tf("%s needs %s, but %s is not enabled", nm(x), nm(y), nm(y)), Actions: depActions(st, p, enable(st, y), dependencyTarget(st, y), nm(y)), Counted: true, Mods: ids, Status: s,
			Title: Tf("missing dependency: %s", nm(y)), Why: Tf("%s needs it, but it is not enabled.", nm(x)), Evidence: ev})
	case "dep.observed-missing": // undeclared, seen in shared logs (internal/known)
		b.depObservedMissing(c)
	case "combo.known-bad":
		b.comboKnownBad(c)
	case "patch.no-match":
		b.patchNoMatch(c)
	case "incompat.active":
		b.add(Need{Group: GroupConflict, Text: Tf("%s is incompatible with %s", nm(x), nm(y)), Actions: []Action{disableOne(st, b.dependents, x, y)}, Counted: true, Mods: ids, Status: s,
			Why: T("one of them says so; with both enabled the game misbehaves. Disable one."), Evidence: ev})
	case "mod.pinned":
		b.add(Need{Group: GroupHidden, Text: Tf("%s is locked to another copy, and the workshop version has been updated since", nm(x)),
			Actions: []Action{viewAction(T("see what changed"), "mod", TokRef(st, y), "--diff", TokRef(st, x)), unpin(st, x)}, Counted: true, Mods: ids, Status: s,
			Title: Tf("%s: the Workshop has a newer version", nm(x)), Why: T("the list uses a locked copy; see what changed, or use the Workshop version"), Evidence: ev})
	case "texture.stale":
		b.textureStale(c)
	case "mod.hidden-residue":
		b.add(Need{Group: GroupHidden, Text: Tf("%s is hidden and no longer subscribed; its folder is left over", nm(x)),
			Actions: []Action{trash(st, x)}, Counted: true, Mods: ids, Status: s,
			Title: Tf("%s is left over", nm(x)), Why: T("it is hidden and no longer subscribed; its folder still takes space"), Evidence: ev})
	case "mod.hidden":
		b.add(Need{Group: GroupHidden, Text: Tf("%s is hidden from the game (probably by another mod manager)", nm(x)),
			Actions: []Action{keepHidden(p), unhide(st, x), trash(st, x)}, Counted: true, Mods: ids, Status: s,
			Title: Tf("%s is hidden from the game", nm(x)), Why: T("probably by another mod manager; keep it hidden, unhide it, or recycle it"), Evidence: ev})
	case app.CodePackLinks:
		b.add(packLinksNeed(p))
	case app.CodeModsDuplicate:
		b.modsDuplicate(c)
	case "def.parent-missing":
		if !b.parentSeen[x] {
			b.parentSeen[x] = true
			b.add(parentNeed(st, b.parentGroups[p.SubjectInstance()], p))
		}
	default:
		// Harmony warnings without log evidence, overlapping Defs and patches whose target
		// is simply absent stay in the technical information (16 §5).
	}
}

// orderViolated keeps an order constraint for the folded order item, or the weak one.
func (b *itemBuilder) orderViolated(c problemCtx) {
	st, p, x, y, nm := b.st, c.p, c.x, c.y, b.nm
	if p.Severity == finding.Info {
		b.weak = append(b.weak, Need{Text: Tf("%s should load after %s", nm(x), nm(y)), Mods: c.ids})
		b.weak[len(b.weak)-1].linkProblems = []*app.Problem{p}
		return
	}
	b.order = append(b.order, Need{rank: 1, Group: GroupList, code: p.Code, Text: Tf("%s should load after %s", nm(x), nm(y)), Actions: []Action{{Kind: ActMove, Key: "m", Label: Tf("move %s after %s", nm(x), nm(y)), Refs: []string{TokRef(st, x)}, After: TokRef(st, y), Undo: true}, sortAction()}, Counted: true, Mods: c.ids, Status: c.s,
		Why: T("In the wrong order this Mod may not work; sort fixes it."), Evidence: c.ev})
	b.order[len(b.order)-1].linkProblems = []*app.Problem{p}
}

// entryMissing files a list entry that is not installed: usable copy, downloading or missing.
func (b *itemBuilder) entryMissing(c problemCtx) {
	st, p := b.st, c.p
	pkg := strings.TrimPrefix(c.x, "pkg:")
	look := Action{Kind: ActOpenURL, Key: "o", Label: T("look for it in the Workshop"), URL: WorkshopSearch(pkg), Done: T("opened the Workshop page")}
	for _, sg := range p.Suggestions {
		if u, ok := strings.CutPrefix(sg, "subscribe: "); ok { // steamDB knows the item (08 §2.3)
			look.Label, look.URL = T("open it in the Workshop"), u
		}
	}
	me := missingEntry{pkg: pkg, name: pkg, ref: c.x, n: Need{rank: 4, Group: GroupList, code: p.Code, Text: Tf("the list has %s, but it is not installed", pkg),
		Actions: []Action{look, disable(st, "pkg:"+pkg, T("remove it from the list"))}, Counted: true, Status: c.s,
		Title: Tf("not installed: %s", pkg), Why: T("the list has it, but no mod folder does"), Evidence: c.ev}}
	me.n.linkProblems = []*app.Problem{p}
	if w, ok := b.missingInfo[strings.TrimSuffix(strings.ToLower(pkg), "_steam")]; ok {
		me.ws = w.WorkshopID
		if w.Name != "" {
			me.name = w.Name
			me.n.Text = Tf("the list has %s, but it is not installed", w.Name)
			me.n.Title = Tf("not installed: %s", w.Name)
		}
		if w.CanEnable {
			me.pkg = w.PackageID
			b.installed = append(b.installed, me)
			return
		}
		if w.Downloading { // Steam is not done with it yet (04 §1.6)
			me.n.Status = StatusWarn
			me.n.Text = Tf("%s is still downloading in Steam", me.name)
			me.n.Title = Tf("still downloading: %s", me.name)
			me.n.Why = T("Steam has it subscribed, but its folder is not complete yet; RimArcho reads it as soon as Steam is done")
			me.n.Actions = []Action{look, disable(st, "pkg:"+pkg, T("remove it from the list"))}
			b.downloading = append(b.downloading, me)
			return
		}
	}
	b.missing = append(b.missing, me)
}

// modReplaced is the item of a mod with a maintained replacement.
func (b *itemBuilder) modReplaced(c problemCtx) {
	st, p, x, y, nm := b.st, c.p, c.x, c.y, b.nm
	name := y
	if mt := replacedRe.FindStringSubmatch(p.Claim); mt != nil {
		name = mt[1]
	} else if y != "" {
		name = nm(y)
	}
	acts := []Action{workshop(p, name)}
	if y != "" {
		// One step, in A's place (16 §4): two actions on space only ever enabled B, and with
		// B named like A the pair read "enable MendAndRecycle / disable MendAndRecycle".
		acts = []Action{{Kind: ActReplace, Key: "space", Label: Tf("use %s instead", name), Refs: []string{TokRef(st, x)}, Undo: true,
			Done: T("replaced with recommended mods")}}
	}
	// The player may keep another version on purpose (an unofficial port for a newer game).
	acts = append(acts, Action{Kind: ActIgnore, Key: "I", Label: T("ignore it"), Refs: []string{p.Ref}, Problem: p.Ref,
		Done: Tf("ignored; shown again when %s changes", nm(x))})
	b.add(Need{rank: 5, Group: GroupList, Text: Tf("%s has a maintained replacement: %s", nm(x), name), Actions: acts, Counted: true, Mods: c.ids, Status: c.s,
		Title: Tf("%s has a maintained replacement", nm(x)), Why: Tf("%s is no longer kept up; %s does the same and is kept up", nm(x), name), Evidence: c.ev})
}

// depMissing is a missing dependency's item, unless a missing list entry covers it.
func (b *itemBuilder) depMissing(c problemCtx) {
	st, p, x, y, nm := b.st, c.p, c.x, c.y, b.nm
	if b.inList[dependencyTarget(st, y)] {
		b.missingOwners[dependencyTarget(st, y)] = append(b.missingOwners[dependencyTarget(st, y)], owners(st, x)...)
		return // the missing-entry card covers the dependency as well
	}
	dep := depName(p.Claim, strings.TrimPrefix(y, "pkg:"))
	if dep == strings.TrimPrefix(y, "pkg:") {
		dep = nm(y)
	}
	b.add(Need{Group: GroupDeps, Target: dependencyTarget(st, y), RequiredBy: owners(st, x), Text: Tf("%s needs %s, but %s is not installed", nm(x), dep, dep), Actions: depActions(st, p, workshop(p, dep), dependencyTarget(st, y), dep), Counted: true, Mods: c.ids, Status: c.s,
		Title: Tf("missing dependency: %s", dep), Why: Tf("%s needs it, but it is not installed.", nm(x)), Evidence: c.ev})
}

// depObservedMissing is an undeclared dependency seen in shared logs.
func (b *itemBuilder) depObservedMissing(c problemCtx) {
	st, p, x, y, nm := b.st, c.p, c.x, c.y, b.nm
	if strings.HasPrefix(y, "pkg:") {
		dep := nm(y)
		b.add(Need{Group: GroupDeps, Target: dependencyTarget(st, y), RequiredBy: owners(st, x), Text: Tf("%s seems to need %s (seen in shared logs), which is not installed", nm(x), dep), Actions: []Action{workshop(p, dep)}, Counted: true, Mods: c.ids, Status: c.s,
			Title: Tf("probably missing: %s", dep), Why: Tf("%s does not declare it, but in shared logs it fails without it. It is not installed.", nm(x)), Evidence: c.ev})
	} else {
		b.add(Need{Group: GroupDeps, Target: dependencyTarget(st, y), RequiredBy: owners(st, x), Text: Tf("%s seems to need %s (seen in shared logs), which is not enabled", nm(x), nm(y)), Actions: []Action{enable(st, y)}, Counted: true, Mods: c.ids, Status: c.s,
			Title: Tf("probably missing: %s", nm(y)), Why: Tf("%s does not declare it, but in shared logs it fails without it. It is not enabled.", nm(x)), Evidence: c.ev})
	}
}

// comboKnownBad is a pair, or a version, that shared logs show failing.
func (b *itemBuilder) comboKnownBad(c problemCtx) {
	st, x, y, nm := b.st, c.x, c.y, b.nm
	if y != "" {
		b.add(Need{Group: GroupConflict, Text: Tf("%s and %s often produce errors together (seen in shared logs)", nm(x), nm(y)), Actions: []Action{disableOne(st, b.dependents, x, y)}, Counted: true, Mods: c.ids, Status: c.s,
			Title: Tf("%s and %s often fail together", nm(x), nm(y)), Why: T("in shared logs this pair produces errors far more often than either one alone. Disable one if you see errors."), Evidence: c.ev})
	} else {
		b.add(Need{Group: GroupConflict, Text: Tf("this version of %s often produces errors (seen in shared logs); other versions did not", nm(x)), Actions: []Action{disable(st, x, Tf("disable %s", nm(x)))}, Counted: true, Mods: c.ids, Status: c.s,
			Title: Tf("this version of %s often fails", nm(x)), Why: T("in shared logs this version produces errors; other versions did not. Updating it usually helps."), Evidence: c.ev})
	}
}

// patchNoMatch merges a patch whose target is not enabled, or keeps it for the quiet items.
func (b *itemBuilder) patchNoMatch(c problemCtx) {
	st, p, x, y, nm := b.st, c.p, c.x, c.y, b.nm
	if id := finding.InstID(y); id != "" && st.Position(id) > 0 {
		return
	}
	if y == "" {
		// no inactive target: the Mod does not fit what it changes; one quiet item per
		// Mod, after the loop (patchItems)
		if b.patchOther[x] == nil {
			b.patchOrder = append(b.patchOrder, x)
		}
		b.patchOther[x] = append(b.patchOther[x], p)
		return
	}
	b.merge("patch|"+x+"|"+y, nm(x), nm(y), Need{Group: GroupDeps, Target: dependencyTarget(st, y), RequiredBy: owners(st, x), Text: Tf("part of %s does not take effect: %s, which it targets, is not enabled", nm(x), nm(y)),
		Actions: []Action{enable(st, y), problemAction(p.Ref)}, Counted: true, Mods: c.ids, Status: c.s,
		Title: Tf("part of %s does not take effect", nm(x)), Why: Tf("it changes things %s adds, and %s is not enabled.", nm(y), nm(y)), Evidence: c.ev})
}

// textureStale is a mod changed since its textures were converted.
func (b *itemBuilder) textureStale(c problemCtx) {
	x, nm := c.x, b.nm
	ref := TokRef(b.st, x)
	b.add(Need{Group: GroupHidden, Text: Tf("%s changed since its textures were converted", nm(x)),
		Actions: []Action{
			{Kind: ActTextures, Key: "T", Label: T("convert again"), Refs: []string{ref}, Confirm: T("press T again to convert its textures again"),
				Warn: T("RimArcho writes the DDS again next to the PNGs that changed"), Done: Tf("converted the textures of %s again", nm(x))},
			{Kind: ActTexClean, Key: "C", Label: T("remove the converted textures"), Refs: []string{ref}, Confirm: T("press C again to delete the DDS RimArcho wrote"),
				Warn: T("the DDS RimArcho wrote are deleted; the PNGs stay and the game uses them again"), Done: Tf("removed the converted textures of %s", nm(x))},
		}, Counted: true, Mods: c.ids, Status: c.s,
		Title: Tf("%s changed since its textures were converted", nm(x)), Why: T("the game may show its old textures: convert them again, or remove them"), Evidence: c.ev})
}

// modsDuplicate is the duplicate-copies item, its move-to-locked actions warned with their links.
func (b *itemBuilder) modsDuplicate(c problemCtx) {
	n := modsDuplicateNeed(b.st, c.p, c.ids)
	for i := range n.Actions {
		act := &n.Actions[i]
		if act.Kind == ActMoveLocked && len(act.Refs) > 0 {
			if r, err := b.a.MoveToLocked(act.Refs[0], true); err == nil {
				act.Warn += "\n" + strings.Join(r.Links.Lines(), "\n")
			} else {
				act.Warn += "\n" + err.Error()
			}
		}
	}
	b.add(n)
}

// patchItems adds the quiet patch items, folded when there are many.
func (b *itemBuilder) patchItems() {
	b.cur, b.linkProblem = "patch.no-match", nil
	var quiet []Need
	for _, x := range b.patchOrder {
		if it := patchItem(b.st, x, b.nm(x), b.patchOther[x], b.logs[finding.InstID(x)]); it.Counted {
			b.add(it) // the log showed it: its own item
		} else {
			quiet = append(quiet, it)
		}
	}
	if len(quiet) > 3 {
		// many Mods (an old list on a new game): one folded item, each row keeps its reason
		b.add(foldPatchItems(quiet))
	} else {
		for _, it := range quiet {
			b.add(it)
		}
	}
}

// missingOwnersOf gives each missing entry the mods that need it.
func (b *itemBuilder) missingOwnersOf() {
	for _, entries := range [][]missingEntry{b.missing, b.installed, b.downloading} {
		for i := range entries {
			m := &entries[i]
			m.n.RequiredBy = b.missingOwners[dependencyTarget(b.st, "pkg:"+m.pkg)]
			for _, o := range m.n.RequiredBy {
				m.n.Mods = append(m.n.Mods, o.ID)
			}
			m.n.Fold = len(m.n.Mods) > 1
		}
	}
}

// foldEntries collects the entries' links, owners, mods and names into the folded item f.
func foldEntries(f *Need, entries []missingEntry) {
	for _, m := range entries {
		f.linkProblems = append(f.linkProblems, m.n.linkProblems...)
		f.RequiredBy = append(f.RequiredBy, m.n.RequiredBy...)
		f.Mods = append(f.Mods, m.n.Mods...)
		f.Parts = append(f.Parts, m.name)
	}
}

// installedItem is the one item for the entries an installed copy can stand in for.
func (b *itemBuilder) installedItem() {
	installed := b.installed
	if len(installed) == 0 {
		return
	}
	f := Need{rank: 4, Group: GroupList, Counted: true, Status: installed[0].n.Status, Fold: true, code: "entry.missing",
		Text:     Tf("%d mods in the list can use copies already installed", len(installed)),
		Why:      T("use the installed copies in the same positions; any needed links are created when you save or launch"),
		Evidence: installed[0].n.Evidence}
	var packages []string
	foldEntries(&f, installed)
	for _, m := range installed {
		packages = append(packages, m.pkg)
	}
	f.Actions = []Action{
		{Kind: ActUseInstalled, Key: "space", Label: T("use installed copies"), Refs: packages, Undo: true, Done: T("using installed copies")},
		{Kind: ActFold, Key: "f", Label: T("show which")},
	}
	f.refReasons = map[string]Reason{}
	for _, m := range installed {
		f.refReasons[m.ref] = missingRowReason(m.n, f.Actions[0])
	}
	b.add(f)
}

// downloadingItem is the item, folded for several, of the entries Steam is still downloading.
func (b *itemBuilder) downloadingItem() {
	downloading := b.downloading
	switch len(downloading) {
	case 0:
	case 1:
		n := downloading[0].n
		r := missingRowReason(n)
		r.Status = StatusWarn
		n.refReasons = map[string]Reason{downloading[0].ref: r}
		b.add(n)
	default:
		f := Need{rank: 4, Group: GroupList, Counted: true, Status: StatusWarn, Fold: true, code: "entry.missing",
			Text:     Tf("%d mods in the list are still downloading in Steam", len(downloading)),
			Why:      T("Steam has them subscribed, but their folders are not complete yet; RimArcho reads them as soon as Steam is done"),
			Evidence: downloading[0].n.Evidence, Actions: []Action{{Kind: ActFold, Key: "f", Label: T("show which")}}}
		f.refReasons = map[string]Reason{}
		foldEntries(&f, downloading)
		for _, m := range downloading {
			r := missingRowReason(m.n)
			r.Status = StatusWarn
			f.refReasons[m.ref] = r
		}
		b.add(f)
	}
}

// missingItem is the item, folded for several, of the entries that are not installed.
func (b *itemBuilder) missingItem() {
	missing := b.missing
	switch len(missing) {
	case 0:
	case 1:
		n := missing[0].n
		if ws := missing[0].ws; ws != "" {
			n.Actions = append([]Action{subscribeAction([]string{ws}, T("subscribe to it"))}, n.Actions...)
		}
		n.refReasons = map[string]Reason{missing[0].ref: missingRowReason(n)}
		b.add(n)
	default:
		f := Need{rank: 4, Group: GroupList, Counted: true, Status: missing[0].n.Status, Fold: true, code: "entry.missing",
			Text:     Tf("%d mods in the list are not installed", len(missing)),
			Why:      T("the list has them, but no mod folder does; once subscribed and downloaded they take their place in the list"),
			Evidence: missing[0].n.Evidence}
		var ws, refs []string
		foldEntries(&f, missing)
		for _, m := range missing {
			refs = append(refs, TokRef(b.st, "pkg:"+m.pkg))
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
		removeAll := Action{Kind: ActDisable, Key: "x", Label: T("remove them from the list"), Refs: refs, Done: Tf("removed %d mods from the list", len(refs))}
		f.Actions = append(f.Actions, removeAll, Action{Kind: ActFold, Key: "f", Label: T("show which")})
		// Each row's icon offers its own fixes and removing every entry that is not
		// installed, so clearing a list after unsubscribing is one click from any row.
		removeAll.Label = Tf("remove all %d not installed", len(refs))
		f.refReasons = map[string]Reason{}
		for _, m := range missing {
			f.refReasons[m.ref] = missingRowReason(m.n, removeAll)
		}
		b.add(f)
	}
}

// mergedItems adds the merged items, worded for the number of problems each stands for.
func (b *itemBuilder) mergedItems() {
	for _, k := range b.mergeKeys {
		m := b.merges[k]
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
		b.add(n)
	}
}

// weakItem is the one dimmed item of the weak order preferences not met.
func (b *itemBuilder) weakItem() {
	if len(b.weak) == 0 {
		return
	}
	// Weak and harmless constraints (08 §1.3): breaking them went with no more errors in
	// shared logs. One dimmed, uncounted item; sorting still puts them right.
	f := Need{rank: 8, Group: GroupList, Status: StatusWarn, Dim: true, Fold: true, code: "order.violated",
		Text:     Tf("%d weaker order preferences are not met", len(b.weak)),
		Why:      T("loadAfter and community rules: breaking them showed no more errors in shared logs; sorting puts them right"),
		Evidence: cutoffChip(evidence(EvCorpus, T("load-order constraint strength")), b.st.Ix.Known().Cutoff)}
	for _, w := range b.weak {
		f.linkProblems = append(f.linkProblems, w.linkProblems...)
		f.Mods = append(f.Mods, w.Mods...)
		f.Parts = append(f.Parts, w.Text)
	}
	f.Actions = []Action{sortAction(), {Kind: ActFold, Key: "f", Label: T("show which")}}
	b.add(f)
}

// orderItem is the order item, folded when several constraints are broken.
func (b *itemBuilder) orderItem() {
	order := b.order
	switch len(order) {
	case 0:
	case 1:
		b.add(order[0])
	default:
		f := Need{rank: 1, Group: GroupList, Counted: true, Status: StatusWarn, Fold: true, code: "order.violated"}
		seen := map[string]bool{}
		for _, o := range order {
			f.linkProblems = append(f.linkProblems, o.linkProblems...)
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
		b.add(f)
	}
}

// versionItem is the one item of the mods that do not list the game as supported.
func (b *itemBuilder) versionItem() {
	versions, st := b.versions, b.st
	n := len(versions)
	if n == 0 {
		return
	}
	warn := 0
	for _, v := range versions {
		if v.Severity == finding.Warning {
			warn++
		}
	}
	if warn == 0 {
		return
	}
	mm := st.GameMM()
	text := Tf("%d mods do not list %s as supported", n, mm)
	if prev := app.PrevMinor(mm); prev != "" {
		text = Tf("%d mods do not list %s as supported (%d of them list %s)", n, mm, warn, prev)
	}
	versionMods := make([]string, 0, n)
	reasons := make(map[string]Reason, n)
	reasonOf := versionReasonBuilder(mm, false)
	for _, v := range versions {
		id := v.SubjectInstance()
		versionMods = append(versionMods, id)
		reasons[id] = reasonOf(st.Ix.ByID[id])
	}
	b.add(Need{rank: 5, Group: GroupList, Text: text, Counted: true, Status: StatusWarn, Mods: versionMods, Fold: true,
		code: "version.unsupported", rowReasons: reasons,
		Why: Tf("They may still work; their authors have not marked %s.", mm), Evidence: evidence(EvDeclared, T("About.xml supportedVersions")),
		Actions: []Action{{Kind: ActVersionList, Key: "enter", Label: T("see the list"), Title: Tf("does not list %s as supported", mm), GameVersion: mm}, versionBatch(st, versions, false)}})
}

// lastRunItems adds the current log's errors, one item per mod (16 §2.4), and the earlier logs.
func (b *itemBuilder) lastRunItems() {
	st, ix, nm := b.st, b.st.Ix, b.nm
	logWhen := ""
	for _, r := range b.ps.Logs {
		if !r.Stale() && !r.ModTime.IsZero() {
			logWhen = ShortDate(r.ModTime) + " " + r.ModTime.In(b.a.Loc).Format("15:04")
			break
		}
	}
	var subjectsOrder []string
	for id := range b.logs {
		subjectsOrder = append(subjectsOrder, id)
	}
	sort.Slice(subjectsOrder, func(i, j int) bool { return nm(finding.Inst(subjectsOrder[i])) < nm(finding.Inst(subjectsOrder[j])) })
	for _, id := range subjectsOrder {
		argv := []string{"problems", "--code", "log"}
		var ids []string
		name := T("unknown source")
		if id != "" {
			argv = append(argv, "--subject", ix.Ref(id))
			ids = []string{id}
			name = nm(finding.Inst(id))
		}
		for _, n := range lastRunNeeds(name, b.logs[id], logWhen, id != "") {
			// a save or mod used its content but it is not in the list (missing references)
			if id != "" && st.Position(id) == 0 {
				n.Why += T("; it is not in the list")
				n.Text = n.Title + T(": ") + n.Why
			}
			n.Mods = ids
			n.Actions[0].Argv = argv
			b.add(n)
		}
	}
	for _, r := range b.ps.Logs {
		if r.Stale() {
			b.add(Need{Group: GroupLastRun, Text: Tf("earlier log  %s  %s", ShortDate(r.ModTime), r.MM), Dim: true,
				Actions: []Action{{Kind: ActGameLog, Key: "enter", Label: T("open game log"), LogPath: r.Path, Argv: []string{"log", "--file", r.Path}}}})
		}
	}
}

// confirmItems adds what needs the player's answer: identity questions, outside changes of
// the list, dropped mods and a game version the list does not allow.
func (b *itemBuilder) confirmItems() {
	a, st := b.a, b.st
	if qs, err := a.Store.Questions(st.Ix.Env.GameID, false); err == nil {
		for _, q := range qs {
			b.out = append(b.out, questionNeed(st, q))
		}
	}
	if v := st.WS.Conflict; v != 0 {
		b.add(Need{Group: GroupConfirm, Text: T("ModsConfig.xml was changed outside RimArcho"), Counted: true, Status: StatusWarn,
			Why: T("this list was changed after it too: use the outside change, or keep yours"), Evidence: evidence(EvInferred, T("ModsConfig.xml differs from what RimArcho wrote")), Actions: []Action{
				viewAction(T("see the difference"), "loadout", "diff", fmt.Sprintf("v%d", v), "workspace"),
				// One step: u brings the list back (16 §6), so no confirmation.
				{Kind: ActLoadExternal, Key: "L", Label: T("use the outside change"), Version: v, Undo: true,
					Done: T("the outside change is in use")},
				{Kind: ActDismissConflict, Key: "I", Label: T("ignore it"), Done: T("the outside change is ignored (it stays in the history)")},
			}})
	}
	if n := droppedNeed(a, st); n != nil {
		b.add(*n)
	}
	b.planVersionItem()
	b.otherConflictItems()
}

// planVersionItem: the game now is not among the versions the list allows (07 §1.6),
// something to confirm, not a block; version.unsupported still checks each mod against the game.
func (b *itemBuilder) planVersionItem() {
	st := b.st
	pv, err := b.a.PlanVersionsOf(st, st.Plan)
	if err != nil || pv.Game == "" || len(pv.Allowed) == 0 || slices.Contains(pv.Allowed, pv.Game) {
		return
	}
	name, allowed := st.Plan.Name, app.FormatVersions(pv.Allowed)
	b.add(Need{Group: GroupConfirm, Counted: true, Status: StatusWarn,
		Text:     Tf("%s allows %s; the game now is %s", name, allowed, pv.Game),
		Title:    Tf("%s allows %s; the game now is %s", name, allowed, pv.Game),
		Why:      T("the list says which game versions it is for; add this one if you play it here. Launching is not stopped."),
		Evidence: evidence(EvInferred, T("game versions")),
		Actions: []Action{
			{Kind: ActAddVersion, Key: "V", Label: Tf("add %s", pv.Game), Refs: []string{name, pv.Game}},
			{Kind: ActManageLists, Key: "M", Label: T("go to list management"), Argv: []string{"plan", "versions", name}},
		}})
}

// otherConflictItems: other plans of the environment with an unloaded outside change
// (07 §4.3), resolved after switching to them.
func (b *itemBuilder) otherConflictItems() {
	cs, err := b.a.Conflicts(b.st)
	if err != nil {
		return
	}
	for _, c := range cs {
		if !c.Current {
			// It does not stop this list's launch (07 §4.1): settled here, without switching.
			b.add(Need{Group: GroupConfirm, Text: Tf("ModsConfig.xml was changed outside RimArcho; list %s keeps it as v%d, not used yet", c.Plan.Name, c.Version),
				Why:     Tf("the change was made to %s, the list the game had; launching this list is not stopped. Switch to %s to use it, or ignore it.", c.Plan.Name, c.Plan.Name),
				Counted: true, Status: StatusWarn, Actions: []Action{
					{Kind: ActDismissConflict, Key: "I", Label: T("ignore it"), Refs: []string{c.Plan.Name}, Done: T("the outside change is ignored (it stays in the history)")},
					{Kind: ActManageLists, Key: "M", Label: T("go to list management"), Argv: []string{"plan", "list"}},
				}})
		}
	}
}
