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
	text := e.Message + "\n" + strings.Join(e.Extra, "\n")
	ms := excTypeRe.FindAllStringSubmatch(e.Message, -1)
	if len(ms) == 0 {
		return
	}
	exc := strings.TrimPrefix(ms[len(ms)-1][1], "System.")
	if exc != "NullReferenceException" && exc != "ArgumentNullException" {
		return
	}
	// Exception frames with their 1-based numbers in the stack.
	var frames []logs.Frame
	var num []int
	for i, fr := range e.Frames {
		if !fr.Caller {
			frames, num = append(frames, fr), append(num, i+1)
		}
	}
	if len(frames) == 0 {
		return
	}
	var lines []string
	var at traceStep
	throwMod := ""
	thrower := frames[0]
	if exc == "NullReferenceException" {
		if logs.GameNamespace(thrower.Type) || !thrower.HasIL {
			return
		}
		c, ok := cx.code(thrower)
		if !ok {
			return
		}
		ds, ok := c.asm.DerefsAt(c.row, thrower.IL)
		if !ok || len(ds) == 0 {
			return
		}
		d := ds[0]
		how := c.asm.InstrText(d.At)
		if push := c.asm.PushText(c.row, d.Value); push != "" {
			how = push + ", " + how
		}
		l := fmt.Sprintf("frame %d %s +0x%x dereferences %s (%s)", num[0], frameName(thrower), thrower.IL, dotnet.ValueText(d.Value, c.names), how)
		if len(ds) > 1 {
			l += fmt.Sprintf("; %d more dereferences follow in the statement", len(ds)-1)
		}
		lines = append(lines, l)
		throwMod = c.inst
		if d.Value.Kind != dotnet.ShParam {
			lc.Trace = lines
			return
		}
		at = traceStep{k: 0, p: d.Value.N, name: c.names[d.Value.N]}
	} else {
		// ArgumentNullException: the parameter the message names, in the innermost frame whose
		// printed signature has it (the thrower, or the library method a Mod called).
		pm := paramNameRe.FindStringSubmatch(text)
		if pm == nil {
			return
		}
		name := pm[1] + pm[2]
		found := false
		for k := 0; k < len(frames) && k < 4 && !found; k++ {
			_, params, ok := frameSig(frames[k].Text)
			if !ok {
				continue
			}
			for i, p := range params {
				if p.name == name {
					at, found = traceStep{k: k, p: i, name: name}, true
					break
				}
			}
		}
		if !found {
			return
		}
		fr := frames[at.k]
		if !logs.GameNamespace(fr.Type) {
			if c, ok := cx.code(fr); ok {
				throwMod = c.inst
			}
		}
		lines = append(lines, fmt.Sprintf("frame %d %s throws ArgumentNullException for its parameter %s", num[at.k], frameName(fr), name))
	}
	supplier := ""
	var supplierFrame int
	var contract *CallersResult
	contractParam := -1
	patchKind := ""
	for hop := 0; hop < 8; hop++ {
		cur := frames[at.k]
		j := at.k + 1
		if j >= len(frames) {
			break
		}
		next := frames[j]
		// A Harmony patch: its parameter is the patched original's parameter of the same name.
		if p := patchOf(cur, next); p != nil {
			_, oparams, ok := frameSig(next.Text)
			if !ok {
				break
			}
			oname, oi, why := harmonyParam(at.name, oparams)
			if why != "" {
				lines = append(lines, fmt.Sprintf("frame %d is a Harmony %s of frame %d %s; %s %s", num[at.k], p.Kind, num[j], frameName(next), at.name, why))
				break
			}
			lines = append(lines, fmt.Sprintf("frame %d is a Harmony %s of frame %d %s: %s is the original's parameter %s", num[at.k], p.Kind, num[j], frameName(next), at.name, oname))
			if at.k == 0 {
				patchKind = p.Kind
			}
			at = traceStep{k: j, p: oi, name: oname}
			continue
		}
		c, ok := cx.code(next)
		if !ok {
			break
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
			break
		}
		v := site.Args[at.p]
		kind := "call"
		switch {
		case site.Base:
			kind = "base constructor call"
		case site.Op == 0x73:
			kind = "newobj"
		}
		if gameFrame(cur.Type) && contract == nil {
			if r, err := cx.a.Callers(cx.st, dotnet.Target{Owner: cur.Type, Name: cur.Method, Params: n}, cx.a.enabledCodeMods(cx.st), true); err == nil && len(r.Sites) > 0 {
				contract, contractParam = r, at.p
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
			lines = append(lines, fmt.Sprintf("frame %d %s passes null for %s (%s, %s)", num[j], frameName(next), at.name, how, kind))
			supplier, supplierFrame = c.inst, num[j]
		case dotnet.ShParam:
			pn := v.Describe(c.names)
			pn = strings.TrimPrefix(pn, "param ")
			lines = append(lines, fmt.Sprintf("frame %d %s passes its parameter %s for %s (%s)", num[j], frameName(next), pn, at.name, kind))
			at = traceStep{k: j, p: v.N, name: pn}
			continue
		default:
			lines = append(lines, fmt.Sprintf("frame %d %s passes %s for %s (%s)", num[j], frameName(next), dotnet.ValueText(v, c.names), at.name, kind))
		}
		break
	}
	var next []string
	if contract != nil && contractParam >= 0 {
		lines = append(lines, "game contract: "+contract.Summary(contractParam))
		next = append(next, fmt.Sprintf("archo source callers %s", callersArg(contract.Target)))
	}
	lc.Trace = lines
	defer func() { lc.Next = uniqStr(append(next, lc.Next...)) }()
	if supplier == "" || cx.official(supplier) {
		return
	}
	ix := cx.st.Ix
	cx.yieldFirst(lc, supplier, RoleSource, fmt.Sprintf("log frame %d passes null for the argument the throwing frame dereferences (trace)", supplierFrame))
	next = append([]string{fmt.Sprintf("archo source %s --type %s", ix.Ref(supplier), outerType(frames[indexOfNum(num, supplierFrame)].Type))}, next...)
	if throwMod == "" || throwMod == supplier {
		return
	}
	role, ev := RoleMentioned, fmt.Sprintf("log frame %d dereferences the null that log frame %d passes (trace)", num[0], supplierFrame)
	if patchKind != "" {
		role = RolePatcher
		ev = fmt.Sprintf("log frame %d, its Harmony %s, dereferences the null that log frame %d passes (trace)", num[0], patchKind, supplierFrame)
	}
	cx.yieldFirst(lc, throwMod, role, ev)
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
