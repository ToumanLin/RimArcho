package app

// The null trace (06 §2, MCP eval h4 P1): for a NullReferenceException or
// ArgumentNullException thrown in Mod code, decode the IL at the frame's offset to find the
// dereferenced value, and when it is a parameter, follow it down the stack to the frame that
// supplied it, through Harmony patches to the caller of the patched original. A Mod frame
// that passes a null literal for it becomes the source; the throwing Mod is a patcher (its
// frame is a patch) or mentioned. A trace that cannot be decoded prints nothing.

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/dotnet"
	"github.com/ToumanLin/RimArcho/internal/logs"
)

type traceCtx struct {
	asms map[string]*dotnet.Assembly // path → assembly; nil when unreadable
}

func (cx *attrCtx) openDLL(path string) *dotnet.Assembly {
	if cx.trace == nil {
		cx.trace = &traceCtx{asms: map[string]*dotnet.Assembly{}}
	}
	if a, ok := cx.trace.asms[path]; ok {
		return a
	}
	a, err := dotnet.Open(path)
	if err != nil {
		a = nil
	}
	cx.trace.asms[path] = a
	return a
}

// frameParam is one parameter of a frame's printed signature.
type frameParam struct{ typ, name string }

// frameSig reads the member and the parameters of a frame's text: "at Ns.T.M (A a, B b)
// [0x…] …" or the bare "Ns.T.M(A a, B b)".
func frameSig(text string) (member string, params []frameParam, ok bool) {
	t := strings.TrimPrefix(strings.TrimSpace(text), "at ")
	open := strings.IndexByte(t, '(')
	if open <= 0 {
		return "", nil, false
	}
	member = strings.TrimSpace(t[:open])
	depth, end := 0, -1
	for i := open; i < len(t); i++ {
		switch t[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return "", nil, false
	}
	for _, p := range dotnet.SplitParams(t[open+1 : end]) {
		i := strings.LastIndexByte(p, ' ')
		if i < 0 {
			params = append(params, frameParam{typ: p})
			continue
		}
		params = append(params, frameParam{typ: strings.TrimSpace(p[:i]), name: strings.TrimSpace(p[i+1:])})
	}
	return member, params, true
}

func shortType(t string) string {
	t = strings.TrimSuffix(t, "&")
	if i := strings.IndexAny(t, "[<`"); i >= 0 {
		t = t[:i]
	}
	if i := strings.LastIndexAny(t, ".+"); i >= 0 {
		t = t[i+1:]
	}
	return strings.ToLower(t)
}

// frameCode is a frame's method in the assembly that defines it.
type frameCode struct {
	inst  string // "" for game code
	asm   *dotnet.Assembly
	row   uint32
	names []string // parameter names
}

// code finds the method of frame fr: Mod code by the analysed type owners, game code in the
// environment's Assembly-CSharp.dll. Compiler-generated methods (lambdas, iterators) are
// not followed: their frames print the outer method's name.
func (cx *attrCtx) code(fr logs.Frame) (frameCode, bool) {
	member, params, ok := frameSig(fr.Text)
	if !ok || strings.ContainsAny(member, "<>[") || fr.Type == "" || fr.Method == "" || fr.Patched {
		return frameCode{}, false
	}
	var cands []frameCode
	if gameFrame(fr.Type) {
		if p := GameAssemblyPath(cx.st); p != "" {
			if a := cx.openDLL(p); a != nil {
				cands = append(cands, frameCode{asm: a})
			}
		}
	} else if !logs.GameNamespace(fr.Type) {
		owners, _ := cx.a.Store.TypeOwners(fr.Type)
		for _, o := range owners {
			m := cx.st.Ix.ByID[o[0]]
			if m == nil || cx.st.Position(o[0]) == 0 {
				continue
			}
			if a := cx.openDLL(filepath.Join(cx.st.Ix.ModDir(m), filepath.FromSlash(o[1]))); a != nil {
				cands = append(cands, frameCode{inst: o[0], asm: a})
			}
		}
	}
	for _, c := range cands {
		rows := c.asm.FindMethods(fr.Type, fr.Method, len(params))
		if len(rows) > 1 {
			// Overloads: compare the printed parameter types' simple names.
			var keep []uint32
			for _, r := range rows {
				m, _ := c.asm.Member(0x06<<24 | r)
				same := len(m.Types) == len(params)
				for i := 0; same && i < len(params); i++ {
					same = shortType(m.Types[i]) == shortType(params[i].typ)
				}
				if same {
					keep = append(keep, r)
				}
			}
			rows = keep
		}
		if len(rows) != 1 {
			continue
		}
		c.row = rows[0]
		c.names = c.asm.ParamNames(c.row)
		for i, p := range params {
			if i < len(c.names) && p.name != "" {
				c.names[i] = p.name
			}
		}
		return c, true
	}
	return frameCode{}, false
}

var (
	// The innermost exception named in a message: "… ---> System.NullReferenceException: …".
	excTypeRe   = regexp.MustCompile(`([A-Za-z_][\w.]*Exception)(?::|$|\s)`)
	paramNameRe = regexp.MustCompile(`Parameter name: (\w+)|\(Parameter '(\w+)'\)`)
)

func frameName(fr logs.Frame) string {
	return strings.ReplaceAll(fr.Type, "+", ".") + "." + fr.Method
}

// traceStep is where the value is: parameter p (index, name) of exception frame k.
type traceStep struct {
	k    int
	p    int
	name string
}

// nullTrace adds the trace lines and, when it ends at a null literal in a Mod frame, the
// attribution (06 §2).
func (cx *attrCtx) nullTrace(lc *LogCluster) {
	e := lc.First
	ms := excTypeRe.FindAllStringSubmatch(e.Message, -1)
	if len(ms) == 0 {
		return
	}
	exc := strings.TrimPrefix(ms[len(ms)-1][1], "System.")
	if exc != "NullReferenceException" && exc != "ArgumentNullException" {
		return
	}
	// Exception frames with their 1-based numbers in the stack.
	t := &nullTraceRun{cx: cx, lc: lc, contractParam: -1}
	for i, fr := range e.Frames {
		if !fr.Caller {
			t.frames, t.num = append(t.frames, fr), append(t.num, i+1)
		}
	}
	if len(t.frames) == 0 {
		return
	}
	if exc == "NullReferenceException" {
		if !t.startDeref() {
			return
		}
	} else if !t.startArgNull(e.Message + "\n" + strings.Join(e.Extra, "\n")) {
		return
	}
	for hop := 0; hop < 8; hop++ {
		if !t.step() {
			break
		}
	}
	t.attribute()
}

// nullTraceRun is one trace of a null value back through the exception frames.
type nullTraceRun struct {
	cx            *attrCtx
	lc            *LogCluster
	frames        []logs.Frame // the exception frames
	num           []int        // their 1-based numbers in the stack
	at            traceStep
	lines         []string
	throwMod      string
	supplier      string
	supplierFrame int
	contract      *CallersResult
	contractParam int
	patchKind     string
}

// startDeref starts at the value the throwing frame of a NullReferenceException
// dereferences. False when there is nothing to trace further (the trace lines may be set).
func (t *nullTraceRun) startDeref() bool {
	thrower := t.frames[0]
	if logs.GameNamespace(thrower.Type) || !thrower.HasIL {
		return false
	}
	c, ok := t.cx.code(thrower)
	if !ok {
		return false
	}
	ds, ok := c.asm.DerefsAt(c.row, thrower.IL)
	if !ok || len(ds) == 0 {
		return false
	}
	d := ds[0]
	how := c.asm.InstrText(d.At)
	if push := c.asm.PushText(c.row, d.Value); push != "" {
		how = push + ", " + how
	}
	l := fmt.Sprintf("frame %d %s +0x%x dereferences %s (%s)", t.num[0], frameName(thrower), thrower.IL, dotnet.ValueText(d.Value, c.names), how)
	if len(ds) > 1 {
		l += fmt.Sprintf("; %d more dereferences follow in the statement", len(ds)-1)
	}
	t.lines = append(t.lines, l)
	t.throwMod = c.inst
	if d.Value.Kind != dotnet.ShParam {
		t.lc.Trace = t.lines
		return false
	}
	t.at = traceStep{k: 0, p: d.Value.N, name: c.names[d.Value.N]}
	return true
}

// startArgNull starts an ArgumentNullException at the parameter the message names, in the
// innermost frame whose printed signature has it (the thrower, or the library method a Mod
// called).
func (t *nullTraceRun) startArgNull(text string) bool {
	pm := paramNameRe.FindStringSubmatch(text)
	if pm == nil {
		return false
	}
	name := pm[1] + pm[2]
	found := false
	for k := 0; k < len(t.frames) && k < 4 && !found; k++ {
		_, params, ok := frameSig(t.frames[k].Text)
		if !ok {
			continue
		}
		for i, p := range params {
			if p.name == name {
				t.at, found = traceStep{k: k, p: i, name: name}, true
				break
			}
		}
	}
	if !found {
		return false
	}
	fr := t.frames[t.at.k]
	if !logs.GameNamespace(fr.Type) {
		if c, ok := t.cx.code(fr); ok {
			t.throwMod = c.inst
		}
	}
	t.lines = append(t.lines, fmt.Sprintf("frame %d %s throws ArgumentNullException for its parameter %s", t.num[t.at.k], frameName(fr), name))
	return true
}

// step follows the value one frame out: through a Harmony patch to the original, or to
// what the calling frame passes. True when the value is the caller's parameter, to follow
// further.
func (t *nullTraceRun) step() bool {
	cx, at := t.cx, t.at
	cur := t.frames[at.k]
	j := at.k + 1
	if j >= len(t.frames) {
		return false
	}
	next := t.frames[j]
	// A Harmony patch: its parameter is the patched original's parameter of the same name.
	if p := patchOf(cur, next); p != nil {
		_, oparams, ok := frameSig(next.Text)
		if !ok {
			return false
		}
		oname, oi, why := harmonyParam(at.name, oparams)
		if why != "" {
			t.lines = append(t.lines, fmt.Sprintf("frame %d is a Harmony %s of frame %d %s; %s %s", t.num[at.k], p.Kind, t.num[j], frameName(next), at.name, why))
			return false
		}
		t.lines = append(t.lines, fmt.Sprintf("frame %d is a Harmony %s of frame %d %s: %s is the original's parameter %s", t.num[at.k], p.Kind, t.num[j], frameName(next), at.name, oname))
		if at.k == 0 {
			t.patchKind = p.Kind
		}
		t.at = traceStep{k: j, p: oi, name: oname}
		return true
	}
	c, ok := cx.code(next)
	if !ok {
		return false
	}
	_, cparams, _ := frameSig(cur.Text)
	calleeType, calleeName, n := cur.Type, cur.Method, len(cparams)
	match := func(m dotnet.Member) bool {
		return m.Params == n && strings.ReplaceAll(m.Owner, "+", ".") == strings.ReplaceAll(calleeType, "+", ".")
	}
	sites := c.asm.CallSitesOf(c.row, calleeName, match)
	if len(sites) == 0 {
		// A virtual call names the base method: the same name and arity.
		sites = c.asm.CallSitesOf(c.row, calleeName, func(m dotnet.Member) bool { return m.Params == n })
	}
	site, ok := pickSite(sites, next, at.p)
	if !ok {
		return false
	}
	v := site.Args[at.p]
	kind := "call"
	switch {
	case site.Base:
		kind = "base constructor call"
	case site.Op == 0x73:
		kind = "newobj"
	}
	if gameFrame(cur.Type) && t.contract == nil {
		if r, err := cx.a.Callers(cx.st, dotnet.Target{Owner: cur.Type, Name: cur.Method, Params: n}, cx.a.enabledCodeMods(cx.st), true); err == nil && len(r.Sites) > 0 {
			t.contract, t.contractParam = r, at.p
		}
	}
	switch v.Kind {
	case dotnet.ShNull:
		how := c.asm.PushText(c.row, v)
		if v.Text == "local" {
			how = fmt.Sprintf("local %d, only ever set to null", v.N)
		} else if how != "" {
			how += fmt.Sprintf(" at IL_%04x", v.Off)
		}
		t.lines = append(t.lines, fmt.Sprintf("frame %d %s passes null for %s (%s, %s)", t.num[j], frameName(next), at.name, how, kind))
		t.supplier, t.supplierFrame = c.inst, t.num[j]
	case dotnet.ShParam:
		pn := v.Describe(c.names)
		pn = strings.TrimPrefix(pn, "param ")
		t.lines = append(t.lines, fmt.Sprintf("frame %d %s passes its parameter %s for %s (%s)", t.num[j], frameName(next), pn, at.name, kind))
		t.at = traceStep{k: j, p: v.N, name: pn}
		return true
	default:
		t.lines = append(t.lines, fmt.Sprintf("frame %d %s passes %s for %s (%s)", t.num[j], frameName(next), dotnet.ValueText(v, c.names), at.name, kind))
	}
	return false
}

// attribute sets the trace lines and next steps and yields the Mod that supplies the null,
// then the one that dereferences it.
func (t *nullTraceRun) attribute() {
	cx, lc := t.cx, t.lc
	var next []string
	if t.contract != nil && t.contractParam >= 0 {
		t.lines = append(t.lines, "game contract: "+t.contract.Summary(t.contractParam))
		next = append(next, fmt.Sprintf("archo source callers %s", callersArg(t.contract.Target)))
	}
	lc.Trace = t.lines
	defer func() { lc.Next = uniqStr(append(next, lc.Next...)) }()
	if t.supplier == "" || cx.official(t.supplier) {
		return
	}
	ix := cx.st.Ix
	cx.yieldFirst(lc, t.supplier, RoleSource, fmt.Sprintf("log frame %d passes null for the argument the throwing frame dereferences (trace)", t.supplierFrame))
	next = append([]string{fmt.Sprintf("archo source %s --type %s", ix.Ref(t.supplier), outerType(t.frames[indexOfNum(t.num, t.supplierFrame)].Type))}, next...)
	if t.throwMod == "" || t.throwMod == t.supplier {
		return
	}
	role, ev := RoleMentioned, fmt.Sprintf("log frame %d dereferences the null that log frame %d passes (trace)", t.num[0], t.supplierFrame)
	if t.patchKind != "" {
		role = RolePatcher
		ev = fmt.Sprintf("log frame %d, its Harmony %s, dereferences the null that log frame %d passes (trace)", t.num[0], t.patchKind, t.supplierFrame)
	}
	cx.yieldFirst(lc, t.throwMod, role, ev)
}

func indexOfNum(num []int, n int) int {
	for i, x := range num {
		if x == n {
			return i
		}
	}
	return 0
}

// callersArg renders a target for "archo source callers".
func callersArg(t dotnet.Target) string {
	o := strings.ReplaceAll(t.Owner, "+", ".")
	if t.Name == ".ctor" {
		return o + "..ctor"
	}
	return o + "." + t.Name
}

// yieldFirst sets a candidate's role, also to one that ranks below the one it has (the trace
// shows the thrower only dereferenced what another Mod passed), and puts the evidence first:
// it is the reason for the role.
func (cx *attrCtx) yieldFirst(lc *LogCluster, id, role, ev string) {
	for _, at := range lc.Attr {
		if at.Instance == id {
			at.Role = role
			at.Evidence = append([]string{ev}, at.Evidence...)
			return
		}
	}
	lc.add(id, role, ev)
}

// patchOf returns the Harmony line under frame next whose patch method is frame cur.
func patchOf(cur, next logs.Frame) *logs.Patch {
	for i, p := range next.Harmony {
		pt := strings.ReplaceAll(p.PatchType, "+", ".")
		if pt == strings.ReplaceAll(cur.Type, "+", ".") && p.PatchMethod == cur.Method {
			return &next.Harmony[i]
		}
	}
	return nil
}

// harmonyParam maps a patch parameter to the original's (Harmony's injection names); why
// says why it is not one of the original's parameters.
func harmonyParam(name string, orig []frameParam) (string, int, string) {
	switch {
	case name == "__instance":
		return "", 0, "is the patched method's instance"
	case strings.HasPrefix(name, "___"):
		return "", 0, "is the instance's field " + strings.TrimPrefix(name, "___")
	case len(name) > 2 && strings.HasPrefix(name, "__") && isDigits(name[2:]):
		i := atoi(name[2:])
		if i < len(orig) {
			return orig[i].name, i, ""
		}
		return "", 0, "is past the original's parameters"
	case strings.HasPrefix(name, "__"):
		return "", 0, "is injected by Harmony (" + name + ")"
	}
	for i, p := range orig {
		if p.name == name {
			return name, i, ""
		}
	}
	return "", 0, "matches no parameter of the original"
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// pickSite chooses the call that supplied the value: the one at the frame's IL offset (Mono
// prints the start of the statement, so the first call at or after it), else the only call,
// else all calls when they pass the same shape.
func pickSite(sites []dotnet.CallSite, fr logs.Frame, p int) (dotnet.CallSite, bool) {
	var ok []dotnet.CallSite
	for _, s := range sites {
		if p < len(s.Args) {
			ok = append(ok, s)
		}
	}
	if len(ok) == 0 {
		return dotnet.CallSite{}, false
	}
	if fr.HasIL {
		for _, s := range ok {
			if s.Off >= fr.IL {
				return s, true
			}
		}
	}
	if len(ok) == 1 {
		return ok[0], true
	}
	for _, s := range ok[1:] {
		if s.Args[p].Describe(nil) != ok[0].Args[p].Describe(nil) {
			return dotnet.CallSite{}, false
		}
	}
	return ok[0], true
}
