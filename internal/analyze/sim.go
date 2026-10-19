package analyze

import (
	"errors"
	"regexp"
	"sort"
	"strings"

	"github.com/antchfx/xpath"

	"github.com/ToumanLin/RimArcho/internal/xdoc"
)

type topOp struct {
	mod  *Mod
	file string
	seq  int
	op   *Op
	// folderReq: packages required by the conditional load folder the file is in.
	folderReq []string
	ok        bool
	err       bool

	allocFrom, allocTo int32 // node indices allocated by this operation
}

// guard is the protection an operation runs under.
type guard struct {
	findMods []string // inside FindMod(match)
	mayReq   []string // packageIds from MayRequire on a Sequence item
	cond     bool     // inside a Conditional
	folder   []string // packageIds of the conditional load folder (LoadFolders IfModActive)
}

func (g guard) any() bool {
	return len(g.findMods) > 0 || len(g.mayReq) > 0 || g.cond || len(g.folder) > 0
}

type sim struct {
	doc       *xdoc.Doc
	root      int32
	activePkg map[string]bool
	activeNm  map[string]bool
	cache     map[string]*xpath.Expr
	plans     map[string]*fastPlan
	ix        defIndex
	evals     int
	guards    map[*Op]guard

	mm       string
	folderOf map[string][]string

	cur      *topOp
	contests map[string]map[*Mod][]*topOp // node key → mods that Replace/Remove it
	targets  map[*topOp]map[string]bool   // top-level defs touched
}

var errRuntime = errors.New("runtime exception")

func newSim(doc *xdoc.Doc, root int32, active []*Mod, mm string, folders map[string][]string) *sim {
	s := &sim{doc: doc, root: root, mm: mm, folderOf: folders, activePkg: map[string]bool{}, activeNm: map[string]bool{},
		cache: map[string]*xpath.Expr{}, plans: map[string]*fastPlan{}, guards: map[*Op]guard{},
		contests: map[string]map[*Mod][]*topOp{}, targets: map[*topOp]map[string]bool{}}
	for _, m := range active {
		s.activePkg[m.Pkg] = true
		s.activeNm[m.Name] = true
	}
	s.ix.doc, s.ix.root = doc, root
	s.ix.nameID, s.ix.defNameID = doc.Names().ID("Name"), doc.Names().ID("defName")
	s.ix.dirty = true
	doc.OnTouch = s.ix.touch
	return s
}

func (s *sim) run(t *topOp) {
	s.cur = t
	t.allocFrom = int32(s.doc.Len())
	ok, err := s.apply(t.op, guard{folder: t.folderReq})
	t.ok, t.err = ok && err == nil, err != nil
	t.allocTo = int32(s.doc.Len())
}

func (s *sim) apply(o *Op, g guard) (bool, error) {
	o.ran = true
	s.guards[o] = g
	r, err := s.worker(o, g)
	if err != nil {
		o.err = true
		return false, err
	}
	switch o.success {
	case "Always":
		r = true
	case "Never":
		r = false
	case "Invert":
		r = !r
	}
	o.result = r
	return r, nil
}

func (s *sim) compile(expr string) (*xpath.Expr, error) {
	if e, ok := s.cache[expr]; ok {
		if e == nil {
			return nil, errRuntime
		}
		return e, nil
	}
	e, err := xpath.Compile(expr)
	if err != nil {
		s.cache[expr] = nil
		return nil, errRuntime
	}
	s.cache[expr] = e
	return e, nil
}

func (s *sim) selectNodes(o *Op, one bool) ([]xdoc.Ref, error) {
	e, err := s.compile(o.XPath)
	if err != nil {
		return nil, err
	}
	s.evals++
	var out []xdoc.Ref
	if p := s.plan(o.XPath); p != nil {
		out = s.fastSelect(p, one)
	} else {
		var ok bool
		out, ok = s.doc.Select(e, s.doc.Root(), one)
		if !ok {
			return nil, errRuntime
		}
	}
	o.hits += len(out)
	s.noteTargets(out)
	return out, nil
}

// noteTargets records the top-level defs an operation touched (for `def` and `patches`).
func (s *sim) noteTargets(rs []xdoc.Ref) {
	if len(rs) == 0 {
		return
	}
	m := s.targets[s.cur]
	if m == nil {
		m = map[string]bool{}
		s.targets[s.cur] = m
	}
	for _, r := range rs {
		if len(m) >= 50 {
			return
		}
		if k := s.defKey(r.N); k != "" {
			m[k] = true
		}
	}
}

// topDef returns the top-level def containing node n (None if n is not under one).
func (s *sim) topDef(n int32) int32 {
	for n != xdoc.None {
		p := s.doc.Parent(n)
		if p == s.root {
			return n
		}
		n = p
	}
	return xdoc.None
}

// defKey is "Type/defName" (or "Type@Name") of the def containing n.
func (s *sim) defKey(n int32) string {
	t := s.topDef(n)
	if t == xdoc.None {
		return ""
	}
	if c := s.doc.Child(t, "defName"); c != xdoc.None {
		return s.doc.Name(t) + "/" + strings.TrimSpace(s.doc.InnerText(c))
	}
	if v, ok := s.doc.Attr(t, "Name"); ok {
		return s.doc.Name(t) + "@" + v
	}
	return ""
}

// nodeKey identifies a node by its def and element path, for patch.contested.
func (s *sim) nodeKey(n int32) string {
	var path []string
	t := n
	for t != xdoc.None && s.doc.Parent(t) != s.root {
		path = append(path, s.doc.Name(t))
		t = s.doc.Parent(t)
	}
	if t == xdoc.None {
		return ""
	}
	k := s.defKey(t)
	if k == "" {
		return ""
	}
	for i := len(path) - 1; i >= 0; i-- {
		k += "/" + path[i]
	}
	return k
}

func (s *sim) contest(nodes []xdoc.Ref) {
	for _, r := range nodes {
		k := s.nodeKey(r.N)
		if k == "" {
			continue
		}
		m := s.contests[k]
		if m == nil {
			m = map[*Mod][]*topOp{}
			s.contests[k] = m
		}
		l := m[s.cur.mod]
		if len(l) == 0 || l[len(l)-1] != s.cur {
			m[s.cur.mod] = append(l, s.cur)
		}
	}
}

func (s *sim) valueChildren(o *Op) []int32 {
	if o.value == xdoc.None {
		return nil
	}
	return o.valDoc.ChildNodes(o.value)
}

func (s *sim) clone(o *Op, k int32) int32 { return s.doc.Clone(o.valDoc, k) }

func (s *sim) worker(o *Op, g guard) (bool, error) {
	d := s.doc
	switch o.Class {
	case "PatchOperationAdd":
		if o.value == xdoc.None {
			return false, errRuntime
		}
		nodes, err := s.selectNodes(o, false)
		if err != nil {
			return false, err
		}
		kids := s.valueChildren(o)
		for _, n := range nodes {
			if n.IsAttr() {
				return false, errRuntime
			}
			if o.order == "Prepend" {
				for i := len(kids) - 1; i >= 0; i-- {
					d.PrependChild(n.N, s.clone(o, kids[i]))
				}
			} else {
				for _, k := range kids {
					d.AppendChild(n.N, s.clone(o, k))
				}
			}
		}
		return len(nodes) > 0, nil
	case "PatchOperationInsert":
		if o.value == xdoc.None {
			return false, errRuntime
		}
		nodes, err := s.selectNodes(o, false)
		if err != nil {
			return false, err
		}
		kids := s.valueChildren(o)
		for _, n := range nodes {
			if n.IsAttr() || d.Parent(n.N) == xdoc.None {
				return false, errRuntime
			}
			if o.order == "Append" {
				// InsertAfter(child, xmlNode) per child in order: later children end up first.
				for _, k := range kids {
					d.InsertAfter(n.N, s.clone(o, k))
				}
			} else {
				for i := len(kids) - 1; i >= 0; i-- {
					d.InsertBefore(n.N, s.clone(o, kids[i]))
				}
			}
			if d.Kind(n.N) == xdoc.KindText {
				d.Normalize(d.Parent(n.N))
			}
		}
		return len(nodes) > 0, nil
	case "PatchOperationReplace":
		if o.value == xdoc.None {
			return false, errRuntime
		}
		nodes, err := s.selectNodes(o, false)
		if err != nil {
			return false, err
		}
		s.contest(nodes)
		kids := s.valueChildren(o)
		for _, n := range nodes {
			if n.IsAttr() || d.Parent(n.N) == xdoc.None {
				return false, errRuntime
			}
			for _, k := range kids {
				d.InsertBefore(n.N, s.clone(o, k))
			}
			d.Detach(n.N)
		}
		return len(nodes) > 0, nil
	case "PatchOperationRemove":
		nodes, err := s.selectNodes(o, false)
		if err != nil {
			return false, err
		}
		s.contest(nodes)
		for _, n := range nodes {
			if n.IsAttr() || d.Parent(n.N) == xdoc.None {
				return false, errRuntime
			}
			d.Detach(n.N)
		}
		return len(nodes) > 0, nil
	case "PatchOperationAttributeAdd", "PatchOperationAttributeSet", "PatchOperationAttributeRemove":
		nodes, err := s.selectNodes(o, false)
		if err != nil {
			return false, err
		}
		res := false
		for _, n := range nodes {
			if n.IsAttr() || d.Kind(n.N) != xdoc.KindElement {
				if o.Class == "PatchOperationAttributeSet" {
					return false, errRuntime // Attributes is null on non-elements
				}
				continue
			}
			_, has := d.Attr(n.N, o.attribute)
			switch o.Class {
			case "PatchOperationAttributeAdd":
				if !has {
					d.SetAttr(n.N, o.attribute, o.attrValue)
					res = true
				}
			case "PatchOperationAttributeSet":
				d.SetAttr(n.N, o.attribute, o.attrValue)
				res = true
			case "PatchOperationAttributeRemove":
				if has {
					d.RemoveAttr(n.N, o.attribute)
					res = true
				}
			}
		}
		return res, nil
	case "PatchOperationSetName":
		nodes, err := s.selectNodes(o, false)
		if err != nil {
			return false, err
		}
		for _, n := range nodes {
			if n.IsAttr() || d.Parent(n.N) == xdoc.None {
				return false, errRuntime
			}
			nn := d.NewElement(o.name)
			for _, c := range d.ChildNodes(n.N) {
				d.AppendChild(nn, d.Clone(d, c))
			}
			d.InsertBefore(n.N, nn)
			d.Detach(n.N)
		}
		return len(nodes) > 0, nil
	case "PatchOperationAddModExtension":
		if o.value == xdoc.None {
			return false, errRuntime
		}
		nodes, err := s.selectNodes(o, false)
		if err != nil {
			return false, err
		}
		kids := s.valueChildren(o)
		for _, n := range nodes {
			if n.IsAttr() {
				return false, errRuntime
			}
			ext := d.Child(n.N, "modExtensions")
			if ext == xdoc.None {
				ext = d.NewElement("modExtensions")
				d.AppendChild(n.N, ext)
			}
			for _, k := range kids {
				d.AppendChild(ext, s.clone(o, k))
			}
		}
		return len(nodes) > 0, nil
	case "PatchOperationTest":
		n, err := s.selectNodes(o, true)
		return len(n) > 0, err
	case "PatchOperationConditional":
		n, err := s.selectNodes(o, true)
		if err != nil {
			return false, err
		}
		cg := g
		cg.cond = true
		if len(n) > 0 {
			if o.match != nil {
				return s.apply(o.match, cg)
			}
		} else if o.nomatch != nil {
			return s.apply(o.nomatch, cg)
		}
		if o.match == nil {
			return o.nomatch != nil, nil
		}
		return true, nil
	case "PatchOperationFindMod":
		found := false
		for _, m := range o.mods {
			if s.activeNm[m] {
				found = true
				break
			}
		}
		if found {
			if o.match != nil {
				mg := g
				mg.findMods = append(append([]string(nil), g.findMods...), o.mods...)
				return s.apply(o.match, mg)
			}
		} else if o.nomatch != nil {
			return s.apply(o.nomatch, g)
		}
		return true, nil
	case "PatchOperationSequence":
		for _, c := range o.ops {
			if !s.listItemActive(c) {
				continue
			}
			cg := g
			if c.mayReq != "" || c.mayReqAny != "" {
				cg.mayReq = append(append([]string(nil), g.mayReq...), splitPkgs(c.mayReq+","+c.mayReqAny)...)
			}
			ok, err := s.apply(c, cg)
			if err != nil {
				return false, err
			}
			if !ok {
				o.lastFailed = c
				return false, nil
			}
		}
		return true, nil
	case "PatchOperation":
		return false, nil
	}
	// A mod-defined operation class cannot be simulated; assume it succeeds.
	return true, nil
}

func splitPkgs(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(p)), "_steam"); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// listItemActive mirrors DirectXmlToObject.ListFromXml's MayRequire handling.
func (s *sim) listItemActive(o *Op) bool {
	if o.mayReq != "" {
		for _, id := range splitPkgs(o.mayReq) {
			if !s.activePkg[id] {
				return false
			}
		}
	}
	if o.mayReqAny != "" {
		for _, id := range splitPkgs(o.mayReqAny) {
			if s.activePkg[id] {
				return true
			}
		}
		return false
	}
	return true
}

// ---- defName/@Name index fast path (spikes/xpathcompat/index.go) ----

type defIndex struct {
	doc               *xdoc.Doc
	root              int32
	nameID, defNameID int32
	dirty             bool
	byDefName         map[string][]int32
	byName            map[string][]int32
	hits, rebuilds    int
}

// touch marks the index dirty when a change can alter the key set: a top-level node, a
// defName child of a top-level def, text inside such a defName, or a top-level Name attribute.
func (ix *defIndex) touch(p, n, attr int32) {
	if p == xdoc.None {
		return
	}
	if attr >= 0 {
		if p == ix.root && attr == ix.nameID {
			ix.dirty = true
		}
		return
	}
	switch {
	case p == ix.root:
		ix.dirty = true
	case ix.doc.Parent(p) == ix.root && ix.doc.Kind(n) == xdoc.KindElement && ix.doc.NameID(n) == ix.defNameID:
		ix.dirty = true
	case ix.doc.Kind(p) == xdoc.KindElement && ix.doc.NameID(p) == ix.defNameID && ix.doc.Parent(ix.doc.Parent(p)) == ix.root:
		ix.dirty = true
	}
}

func (ix *defIndex) ensure() {
	if !ix.dirty && ix.byDefName != nil {
		return
	}
	ix.rebuilds++
	ix.byDefName = make(map[string][]int32, 40000)
	ix.byName = make(map[string][]int32, 8000)
	d := ix.doc
	for t := d.FirstChild(ix.root); t != xdoc.None; t = d.Next(t) {
		if d.Kind(t) != xdoc.KindElement {
			continue
		}
		for _, a := range d.Attrs(t) {
			if a.Name == ix.nameID {
				ix.byName[a.Value] = append(ix.byName[a.Value], t)
			}
		}
		for c := d.FirstChild(t); c != xdoc.None; c = d.Next(c) {
			if d.Kind(c) == xdoc.KindElement && d.NameID(c) == ix.defNameID {
				k := d.InnerText(c)
				ix.byDefName[k] = append(ix.byDefName[k], t)
			}
		}
	}
	ix.dirty = false
}

type fastPlan struct {
	elem  string // element name or "*"
	attr  bool   // @Name instead of defName
	value string
	rest  *xpath.Expr // nil = the candidate itself
}

var fastRe = regexp.MustCompile(`^/?Defs/([A-Za-z_][\w.\-]*|\*)\[\s*(defName|@Name)\s*=\s*(?:"([^"]*)"|'([^']*)')\s*\](.*)$`)

func (s *sim) plan(expr string) *fastPlan {
	if p, ok := s.plans[expr]; ok {
		return p
	}
	var p *fastPlan
	if m := fastRe.FindStringSubmatch(strings.TrimSpace(expr)); m != nil {
		rest := strings.TrimSpace(m[5])
		// Absolute references or unions in the remainder need the whole document.
		if !strings.Contains(rest, "/Defs") && !strings.Contains(rest, "..") && !strings.Contains(rest, "ancestor") &&
			!strings.Contains(rest, "preceding") && !strings.Contains(rest, "following") && !strings.Contains(rest, "|") && !strings.HasPrefix(rest, "//") {
			p = &fastPlan{elem: m[1], attr: m[2] == "@Name", value: m[3] + m[4]}
			if rest != "" {
				var src string
				switch rest[0] {
				case '/':
					src = "." + rest
				case '[':
					src = "self::node()" + rest
				}
				if src != "" {
					if e, err := xpath.Compile(src); err == nil {
						p.rest = e
					} else {
						p = nil
					}
				} else {
					p = nil
				}
			}
		}
	}
	s.plans[expr] = p
	return p
}

func (s *sim) fastSelect(p *fastPlan, one bool) []xdoc.Ref {
	s.ix.ensure()
	s.ix.hits++
	cands := s.ix.byDefName[p.value]
	if p.attr {
		cands = s.ix.byName[p.value]
	}
	var out []xdoc.Ref
	for _, c := range cands {
		if p.elem != "*" && s.doc.Name(c) != p.elem {
			continue
		}
		if p.rest == nil {
			out = append(out, xdoc.Ref{N: c, A: -1})
		} else if r, ok := s.doc.Select(p.rest, c, one); ok {
			out = append(out, r...)
		}
		if one && len(out) > 0 {
			return out[:1]
		}
	}
	return out
}

// evalAtEnd re-evaluates an expression against the final document (target-later check).
func (s *sim) evalAtEnd(expr string) []xdoc.Ref {
	e, err := s.compile(expr)
	if err != nil {
		return nil
	}
	if p := s.plan(expr); p != nil {
		return s.fastSelect(p, false)
	}
	r, _ := s.doc.Select(e, s.doc.Root(), false)
	return r
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
