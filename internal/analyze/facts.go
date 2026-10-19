package analyze

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/rimarcho/rimarcho/internal/content"
	"github.com/rimarcho/rimarcho/internal/finding"
	"github.com/rimarcho/rimarcho/internal/xdoc"
)

// Finding is the shared problem type.
type Finding = finding.Finding

// Patch results.
const (
	ResultOK      = "ok"
	ResultFailed  = "failed"
	ResultUnknown = "unknown" // mod-defined operation class: not simulated
)

// No-match reasons (03 §4.4), plus node-absent: the def exists but the path inside it does not.
const (
	ReasonTargetInactive   = "target-inactive"
	ReasonTargetMissing    = "target-missing"
	ReasonTargetLater      = "target-later"
	ReasonWrongVersion     = "wrong-version-route"
	ReasonStaleName        = "stale-name"
	ReasonUnguarded        = "unguarded-optional"
	ReasonNodeAbsent       = "node-absent"
	ReasonUnclassified     = "unclassified"
	ReasonRuntimeException = "exception"
)

// PatchFact is one top-level PatchOperation and its simulated outcome (03 §2).
type PatchFact struct {
	Instance string   `json:"-"`
	Seq      int      `json:"seq"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Class    string   `json:"class"`
	XPath    string   `json:"xpath,omitempty"`
	Desc     string   `json:"desc"` // the game's ToString, as in Player.log
	Result   string   `json:"result"`
	Hits     int      `json:"hits"`
	Reasons  []string `json:"reasons,omitempty"`
	Detail   string   `json:"detail,omitempty"`
	Targets  []string `json:"targets,omitempty"` // "ThingDef/Gun_A" of touched defs
	// Leaf is the innermost failed operation's XPath.
	Leaf string `json:"leaf,omitempty"`
}

type defLoc struct {
	mod  *Mod
	fact DefFact
}

// knowledge indexes the Def facts of every installed mod, for no-match reasons.
type knowledge struct {
	mods      []*Mod
	byDefName map[string][]defLoc // defName → definers
	byName    map[string][]defLoc // Name attribute → definers
	byPkg     map[string][]*Mod
	byName2   map[string][]*Mod // display name → mods
}

func newKnowledge(mods []*Mod, defs map[string][]DefFact) *knowledge {
	k := &knowledge{mods: mods, byDefName: map[string][]defLoc{}, byName: map[string][]defLoc{}, byPkg: map[string][]*Mod{}, byName2: map[string][]*Mod{}}
	for _, m := range mods {
		k.byPkg[m.Pkg] = append(k.byPkg[m.Pkg], m)
		k.byName2[m.Name] = append(k.byName2[m.Name], m)
		for _, f := range defs[m.Instance] {
			if f.DefName != "" {
				k.byDefName[f.DefName] = append(k.byDefName[f.DefName], defLoc{m, f})
			}
			if f.Name != "" {
				k.byName[f.Name] = append(k.byName[f.Name], defLoc{m, f})
			}
		}
	}
	return k
}

type target struct {
	typ  string // element name or "*"
	key  string
	attr bool // @Name
}

func (t target) String() string {
	if t.attr {
		return t.typ + "@" + t.key
	}
	return t.typ + "/" + t.key
}

var (
	targetRe  = regexp.MustCompile(`Defs/([A-Za-z_][\w.\-]*|\*)\s*\[([^\]]*)\]`)
	keyRe     = regexp.MustCompile(`(defName|@Name)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	defNameRe = regexp.MustCompile(`defName\s*=\s*(?:"([^"]*)"|'([^']*)')`)
)

// targets extracts the defs an XPath addresses, e.g. Defs/ThingDef[defName="A" or defName="B"].
func targets(xp string) []target {
	var out []target
	for _, m := range targetRe.FindAllStringSubmatch(xp, -1) {
		for _, k := range keyRe.FindAllStringSubmatch(m[2], -1) {
			out = append(out, target{typ: m[1], key: k[2] + k[3], attr: k[1] == "@Name"})
		}
	}
	if len(out) == 0 {
		// //ThingDef[defName="X"] and similar.
		for _, k := range defNameRe.FindAllStringSubmatch(xp, -1) {
			out = append(out, target{typ: "*", key: k[1] + k[2]})
		}
	}
	return out
}

func (k *knowledge) definers(t target) []defLoc {
	src := k.byDefName[t.key]
	if t.attr {
		src = k.byName[t.key]
	}
	var out []defLoc
	for _, l := range src {
		if t.typ == "*" || l.fact.Type == t.typ {
			out = append(out, l)
		}
	}
	return out
}

func (s *sim) facts(tops []*topOp, k *knowledge) []*PatchFact {
	out := make([]*PatchFact, 0, len(tops))
	for _, t := range tops {
		f := &PatchFact{Instance: t.mod.Instance, Seq: t.seq, File: t.file, Line: t.op.Line, Class: t.op.Class,
			XPath: t.op.XPath, Desc: t.op.String()}
		hits := 0
		t.op.walk(func(o *Op) { hits += o.hits })
		f.Hits = hits
		f.Targets = sortedKeys(s.targets[t])
		switch {
		case !t.op.Known:
			f.Result = ResultUnknown
		case t.ok:
			f.Result = ResultOK
		default:
			f.Result = ResultFailed
			leaf := t.op.failedLeaf()
			f.Leaf = leaf.XPath
			f.Reasons, f.Detail = s.reasons(t, leaf, k, tops)
		}
		out = append(out, f)
	}
	return out
}

// reasons classifies a failed operation (03 §4.4).
func (s *sim) reasons(t *topOp, leaf *Op, k *knowledge, tops []*topOp) ([]string, string) {
	var rs, details []string
	add := func(r, d string) {
		for _, x := range rs {
			if x == r {
				return
			}
		}
		rs = append(rs, r)
		if d != "" {
			details = append(details, d)
		}
	}
	if leaf.err {
		add(ReasonRuntimeException, "the operation threw (tree edit on an attribute node, missing <value>, or an invalid XPath)")
		return rs, strings.Join(details, "; ")
	}
	if leaf.XPath == "" {
		if len(rs) == 0 {
			add(ReasonUnclassified, "")
		}
		return rs, strings.Join(details, "; ")
	}
	g := s.guards[leaf]
	if mr, _ := topMayRequire(t.op); mr != "" {
		details = append(details, "MayRequire on a top-level Operation is ignored by the game ("+mr+")")
	}
	// Created later by another mod's patch?
	if nodes := s.evalAtEnd(leaf.XPath); len(nodes) > 0 {
		by := map[string]bool{}
		for _, n := range nodes {
			for _, o := range tops {
				if n.N >= o.allocFrom && n.N < o.allocTo && o.mod.Pos > t.mod.Pos {
					by[finding.Inst(o.mod.Instance)] = true
				}
			}
		}
		d := "matches once later patches have run"
		if len(by) > 0 {
			d += ": created by " + strings.Join(sortedKeys(by), ", ")
		}
		add(ReasonTargetLater, d)
		return rs, strings.Join(details, "; ")
	}
	tgs := targets(leaf.XPath)
	if len(tgs) == 0 {
		if len(rs) == 0 {
			add(ReasonUnclassified, "no def name in the XPath")
		}
		return rs, strings.Join(details, "; ")
	}
	deps := map[string]bool{}
	for _, d := range t.mod.Decl.Dependencies {
		deps[strings.ToLower(d.PackageID)] = true
	}
	for _, d := range t.mod.Decl.DependenciesByVersion["v"+s.mmOf()] {
		deps[strings.ToLower(d.PackageID)] = true
	}
	for _, tg := range tgs {
		defs := k.definers(tg)
		var activeDef, inactiveDef []defLoc
		for _, l := range defs {
			if l.mod.Pos > 0 {
				activeDef = append(activeDef, l)
			} else {
				inactiveDef = append(inactiveDef, l)
			}
		}
		switch {
		case len(activeDef) > 0:
			l := activeDef[0]
			add(ReasonNodeAbsent, fmt.Sprintf("%s is defined by %s (%s:%d) but the rest of the path is not there", tg, finding.Inst(l.mod.Instance), l.fact.File, l.fact.Line))
		case len(inactiveDef) > 0:
			var who []string
			hard := false
			for _, l := range inactiveDef {
				who = append(who, finding.Inst(l.mod.Instance))
				hard = hard || deps[l.mod.Pkg]
			}
			add(ReasonTargetInactive, fmt.Sprintf("%s is defined by %s, which is not active", tg, strings.Join(uniq(who), ", ")))
			if !g.any() && !hard {
				add(ReasonUnguarded, "the patch is not wrapped in PatchOperationFindMod, a Conditional or a MayRequire list item")
			}
		default:
			if where := s.otherVersion(t.mod, tg, k, g, deps); where != "" {
				add(ReasonWrongVersion, fmt.Sprintf("%s exists on disk but not in the effective content: %s", tg, where))
				continue
			}
			owners := s.intendedOwners(g, deps, k)
			var activeOwners []*Mod
			for _, m := range owners {
				if m.Pos > 0 {
					activeOwners = append(activeOwners, m)
				}
			}
			if len(activeOwners) > 0 {
				c := similar(tg, activeOwners, k)
				d := fmt.Sprintf("%s does not exist in %s", tg, modTokens(activeOwners))
				if len(c) > 0 {
					d += "; similar: " + strings.Join(c, ", ")
				}
				add(ReasonStaleName, d)
				continue
			}
			add(ReasonTargetMissing, fmt.Sprintf("no installed mod defines %s", tg))
		}
	}
	return rs, strings.Join(details, "; ")
}

func (s *sim) mmOf() string { return s.mm }

func topMayRequire(o *Op) (string, bool) {
	if o.mayReq != "" {
		return o.mayReq, true
	}
	if o.mayReqAny != "" {
		return o.mayReqAny, true
	}
	return "", false
}

// intendedOwners are the mods a patch evidently targets: FindMod names, MayRequire
// packages, and hard dependencies.
func (s *sim) intendedOwners(g guard, deps map[string]bool, k *knowledge) []*Mod {
	seen := map[*Mod]bool{}
	var out []*Mod
	addAll := func(ms []*Mod) {
		for _, m := range ms {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	for _, n := range g.findMods {
		addAll(k.byName2[n])
	}
	for _, p := range g.mayReq {
		addAll(k.byPkg[p])
	}
	for _, p := range g.folder {
		addAll(k.byPkg[p])
	}
	for p := range deps {
		if !strings.HasPrefix(p, "brrainz.harmony") && !strings.HasPrefix(p, "ludeon.") {
			addAll(k.byPkg[p])
		}
	}
	return out
}

// otherVersion searches the non-effective Defs of the patching mod and the mods it
// targets for the def (wrong-version-route).
func (s *sim) otherVersion(m *Mod, tg target, k *knowledge, g guard, deps map[string]bool) string {
	cands := append([]*Mod{m}, s.intendedOwners(g, deps, k)...)
	needle := []byte(">" + tg.key + "<")
	if tg.attr {
		needle = []byte(`Name="` + tg.key + `"`)
	}
	for _, c := range cands {
		for _, a := range content.OtherXML(c.Dir, s.folders(c), "Defs") {
			b, err := os.ReadFile(a.Path)
			if err == nil && strings.Contains(string(b), string(needle)) {
				return finding.Inst(c.Instance) + " " + a.Rel
			}
		}
	}
	return ""
}

func (s *sim) folders(m *Mod) []string { return s.folderOf[m.Instance] }

func similar(tg target, owners []*Mod, k *knowledge) []string {
	inOwner := map[string]bool{}
	for _, m := range owners {
		inOwner[m.Instance] = true
	}
	type cand struct {
		name string
		d    int
	}
	var cs []cand
	seen := map[string]bool{}
	src := k.byDefName
	if tg.attr {
		src = k.byName
	}
	limit := max(3, len(tg.key)/3)
	for name, locs := range src {
		if seen[name] || abs(len(name)-len(tg.key)) > limit {
			continue
		}
		for _, l := range locs {
			if !inOwner[l.mod.Instance] || (tg.typ != "*" && l.fact.Type != tg.typ) {
				continue
			}
			if d := editDistance(strings.ToLower(name), strings.ToLower(tg.key)); d <= limit {
				cs = append(cs, cand{name, d})
				seen[name] = true
			}
			break
		}
	}
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].d != cs[j].d {
			return cs[i].d < cs[j].d
		}
		return cs[i].name < cs[j].name
	})
	var out []string
	for i := 0; i < len(cs) && i < 3; i++ {
		out = append(out, cs[i].name)
	}
	return out
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			c := 1
			if ra[i-1] == rb[j-1] {
				c = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+c)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

func uniq(l []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range l {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func modTokens(ms []*Mod) string {
	var l []string
	for _, m := range ms {
		l = append(l, finding.Inst(m.Instance))
	}
	return strings.Join(uniq(l), ", ")
}

// ---- findings ----

func defFindings(active []*Mod, defs map[string][]DefFact, isActive func(string) bool) []*Finding {
	var out []*Finding
	type loc struct {
		m *Mod
		f DefFact
	}
	byDef := map[string][]loc{}
	byName := map[string][]loc{}
	names := map[string]bool{}
	var order []string
	for _, m := range active {
		for _, f := range defs[m.Instance] {
			if !f.Loaded(isActive) {
				continue
			}
			if f.DefName != "" && !f.Abstract {
				k := f.Type + "/" + f.DefName
				if byDef[k] == nil {
					order = append(order, k)
				}
				byDef[k] = append(byDef[k], loc{m, f})
			}
			if f.Name != "" {
				byName[f.Name] = append(byName[f.Name], loc{m, f})
				names[f.Name] = true
			}
		}
	}
	distinct := func(ls []loc) int {
		seen := map[*Mod]bool{}
		for _, l := range ls {
			seen[l.m] = true
		}
		return len(seen)
	}
	ev := func(l loc, note string) finding.Evidence {
		return finding.Evidence{Source: finding.Declared, Ref: finding.Inst(l.m.Instance), Text: fmt.Sprintf("%s:%d%s", l.f.File, l.f.Line, note)}
	}
	for _, k := range order {
		ls := byDef[k]
		if distinct(ls) < 2 {
			continue
		}
		win := ls[len(ls)-1]
		f := &Finding{Code: "def.override", Severity: finding.Info, Layer: "def", Key: k, Confidence: finding.Certain,
			Subjects: []string{finding.Inst(win.m.Instance), "def:" + k},
			Claim:    fmt.Sprintf("def:%s is defined by %d mods; %s loads last and wins", k, distinct(ls), finding.Inst(win.m.Instance))}
		for i, l := range ls {
			note := ""
			if i == len(ls)-1 {
				note = "  ← effective"
			}
			f.Evidence = append(f.Evidence, ev(l, note))
			if i < len(ls)-1 && l.m != win.m {
				f.Subjects = append(f.Subjects, finding.Inst(l.m.Instance))
			}
		}
		f.Suggestions = []string{"archo def " + k}
		out = append(out, f)
	}
	var nameKeys []string
	for n := range byName {
		nameKeys = append(nameKeys, n)
	}
	sort.Strings(nameKeys)
	for _, n := range nameKeys {
		ls := byName[n]
		if distinct(ls) < 2 {
			continue
		}
		win := ls[len(ls)-1]
		f := &Finding{Code: "def.abstract-collision", Severity: finding.Warning, Layer: "def", Key: n, Confidence: finding.Certain,
			Subjects: []string{finding.Inst(win.m.Instance)},
			Claim:    fmt.Sprintf("Name=%q is defined by %d mods; children inherit from %s, which loads last", n, distinct(ls), finding.Inst(win.m.Instance))}
		for _, l := range ls {
			f.Evidence = append(f.Evidence, ev(l, ""))
			if l.m != win.m {
				f.Subjects = append(f.Subjects, finding.Inst(l.m.Instance))
			}
		}
		out = append(out, f)
	}
	return out
}

// parentFindings checks ParentName against the patched document: patches run before
// inheritance is resolved, and mods do add Name attributes to existing defs.
func (s *sim) parentFindings(origin map[int32]defLoc, tops []*topOp, isActive func(string) bool) []*Finding {
	d := s.doc
	loaded := func(t int32) bool {
		mr, _ := d.Attr(t, "MayRequire")
		mra, _ := d.Attr(t, "MayRequireAnyOf")
		return mayRequireOK(mr, mra, isActive)
	}
	names := map[string]bool{}
	for t := d.FirstChild(s.root); t != xdoc.None; t = d.Next(t) {
		if d.Kind(t) == xdoc.KindElement && loaded(t) {
			if n, ok := d.Attr(t, "Name"); ok {
				names[n] = true
			}
		}
	}
	var out []*Finding
	for t := d.FirstChild(s.root); t != xdoc.None; t = d.Next(t) {
		if d.Kind(t) != xdoc.KindElement || !loaded(t) {
			continue
		}
		parent, ok := d.Attr(t, "ParentName")
		if !ok || names[parent] {
			continue
		}
		id := ""
		if c := d.Child(t, "defName"); c != xdoc.None {
			id = strings.TrimSpace(d.InnerText(c))
		} else if n, ok := d.Attr(t, "Name"); ok {
			id = "@" + n
		}
		typ := d.Name(t)
		f := &Finding{Code: "def.parent-missing", Severity: finding.Error, Layer: "def", Confidence: finding.Likely,
			Key:   typ + "/" + id + "<" + parent,
			Claim: fmt.Sprintf("%s %s inherits ParentName=%q, which no loaded def has as its Name (after patches)", typ, id, parent)}
		if o, ok := origin[t]; ok {
			f.Subjects = []string{finding.Inst(o.mod.Instance)}
			f.Evidence = []finding.Evidence{{Source: finding.Declared, Ref: finding.Inst(o.mod.Instance), Text: fmt.Sprintf("%s:%d", o.fact.File, o.fact.Line)}}
		} else {
			for _, o := range tops {
				if t >= o.allocFrom && t < o.allocTo {
					f.Subjects = []string{finding.Inst(o.mod.Instance)}
					f.Evidence = []finding.Evidence{{Source: finding.Inferred, Ref: finding.Inst(o.mod.Instance), Text: fmt.Sprintf("added by the patch at %s:%d", o.file, o.op.Line)}}
					break
				}
			}
		}
		if len(f.Subjects) == 0 {
			continue
		}
		f.Suggestions = []string{"archo def " + typ + "@" + parent}
		out = append(out, f)
	}
	return out
}


func (s *sim) findings(tops []*topOp, facts []*PatchFact) []*Finding {
	var out []*Finding
	unknown := map[*Mod]map[string]int{}
	var umods []*Mod
	for i, t := range tops {
		p := facts[i]
		switch p.Result {
		case ResultUnknown:
			if unknown[t.mod] == nil {
				unknown[t.mod] = map[string]int{}
				umods = append(umods, t.mod)
			}
			unknown[t.mod][t.op.Class]++
		case ResultFailed:
			f := &Finding{Code: "patch.no-match", Severity: finding.Warning, Layer: "patch", Confidence: finding.Likely,
				Key:      fmt.Sprintf("%s|%s", p.File, p.Desc),
				Subjects: []string{finding.Inst(t.mod.Instance)},
				Claim:    fmt.Sprintf("%s:%d %s matched nothing (%s)", p.File, p.Line, shortDesc(p), strings.Join(p.Reasons, ", "))}
			for _, r := range p.Reasons {
				f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Inferred, Text: "reason " + r})
			}
			if p.Detail != "" {
				f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Inferred, Text: p.Detail})
			}
			for _, r := range p.Reasons {
				if r == ReasonTargetInactive {
					for _, tok := range regexp.MustCompile(`@\{[a-z0-9]+\}`).FindAllString(p.Detail, -1) {
						f.Subjects = append(f.Subjects, tok)
						f.Suggestions = append(f.Suggestions, "archo enable "+tok)
					}
				}
			}
			f.Suggestions = append(f.Suggestions, fmt.Sprintf("archo patches --mod %s", finding.Inst(t.mod.Instance)))
			out = append(out, f)
		}
	}
	for _, m := range umods {
		var cls []string
		n := 0
		for c, k := range unknown[m] {
			cls = append(cls, fmt.Sprintf("%s ×%d", c, k))
			n += k
		}
		sort.Strings(cls)
		out = append(out, &Finding{Code: "patch.unknown-op", Severity: finding.Info, Layer: "patch", Confidence: finding.Possible,
			Key: "unknown", Subjects: []string{finding.Inst(m.Instance)},
			Claim:    fmt.Sprintf("%d operations use mod-defined classes that cannot be simulated; their effect is not known", n),
			Evidence: []finding.Evidence{{Source: finding.Inferred, Text: strings.Join(cls, ", ")}}})
	}
	var keys []string
	for k, ms := range s.contests {
		if len(ms) > 1 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		ms := s.contests[k]
		var mods []*Mod
		for m := range ms {
			mods = append(mods, m)
		}
		sort.Slice(mods, func(i, j int) bool { return mods[i].Pos < mods[j].Pos })
		f := &Finding{Code: "patch.contested", Severity: finding.Info, Layer: "patch", Confidence: finding.Certain, Key: k,
			Claim: fmt.Sprintf("%d mods replace or remove %s; %s runs last", len(mods), k, finding.Inst(mods[len(mods)-1].Instance))}
		for i := len(mods) - 1; i >= 0; i-- {
			f.Subjects = append(f.Subjects, finding.Inst(mods[i].Instance))
		}
		for _, m := range mods {
			for _, t := range ms[m] {
				f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Declared, Ref: finding.Inst(m.Instance),
					Text: fmt.Sprintf("%s:%d %s", t.file, t.op.Line, t.op.Class)})
			}
		}
		out = append(out, f)
	}
	return out
}

func shortDesc(p *PatchFact) string {
	d := p.Desc
	if len(d) > 160 {
		d = d[:157] + "..."
	}
	return d
}

var _ = xdoc.None
