// Package harmony extracts the Harmony layer of a mod assembly (03 §2, §5.3): declared
// HarmonyPatch targets with their patch kind, bool prefixes that may skip the original,
// manual Harmony.Patch calls, Harmony ID candidates from the #US heap, and the TypeDef
// index used to attribute stack frames.
package harmony

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/dotnet"
)

// Patch kinds.
const (
	Prefix        = "prefix"
	Postfix       = "postfix"
	Transpiler    = "transpiler"
	Finalizer     = "finalizer"
	ILManipulator = "ilmanipulator"
)

// Patch is one declared patch method.
type Patch struct {
	PatchType    string // class holding the patch method
	PatchMethod  string
	Kind         string
	TargetType   string // CLR full name, '+' for nested types; "" if dynamic
	TargetMethod string // method name; get_X / set_X / .ctor / .cctor for MethodType
	Args         string // "(int, string)" when given, else ""
	Skips        bool   // a prefix returning bool can skip the original
	Dynamic      bool   // TargetMethod()/TargetMethods(), or a manual target not statically known
	Manual       bool   // found at a Harmony.Patch call site (manual.go), not from attributes
	Unresolved   string // manual patch whose original was not resolved: why
}

// Target is "Type.Method" (or "Type.Method(args)").
func (p *Patch) Target() string {
	if p.Dynamic && p.Unresolved != "" {
		return "<manual: " + p.Unresolved + ">"
	}
	if p.Dynamic && p.TargetType == "" {
		return "<dynamic " + p.PatchType + ".TargetMethod()>"
	}
	t := strings.ReplaceAll(p.TargetType, "+", ".") + "." + p.TargetMethod
	return t + p.Args
}

// Assembly is the Harmony layer of one DLL.
type Assembly struct {
	Types     []string
	Patches   []Patch
	Manual    bool // references HarmonyLib.Harmony::Patch
	UsesHarm  bool // references Harmony at all
	IDs       []string
	AttrError int // HarmonyPatch attributes that could not be decoded
	// Manual patching (manual.go): Harmony.Patch calls evaluated (helper calls counted at each
	// call site), those with a resolved original and patch method, and why the others were not.
	ManualCalls    int
	ManualResolved int
	Unresolved     map[string]int
}

// DefaultSkip are library assemblies whose Harmony facts are not the mod's own (03 §5.3).
// Their TypeDefs are still indexed (frames inside them are attributed as "library").
var DefaultSkip = []string{"0harmony.dll", "harmony.dll", "harmonylib.dll", "hugslib.dll", "0multiplayerapi.dll", "0prepatcherapi.dll",
	"mono.cecil*.dll", "system.*.dll", "microsoft.*.dll", "newtonsoft.json.dll", "unityengine*.dll", "assembly-csharp*.dll"}

// OwnLibrary maps a skipped library DLL to the mod that is that library. In its own mod the DLL's
// patches are the mod's own (HugsLib patches the game itself), so it is read there and skipped only
// when another mod ships a copy.
var OwnLibrary = map[string]string{"hugslib.dll": "unlimitedhugs.hugslib"}

// SkippedFor is Skipped for a DLL of the mod with packageId pkg (any case, "_steam" ignored).
func SkippedFor(file, pkg string, list []string) bool {
	own := OwnLibrary[strings.ToLower(file)]
	if own != "" && own == strings.TrimSuffix(strings.ToLower(pkg), "_steam") {
		return false
	}
	return Skipped(file, list)
}

// Skipped reports whether a DLL file name matches the skip list (simple * globs, case-insensitive).
func Skipped(file string, list []string) bool {
	f := strings.ToLower(file)
	for _, p := range list {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if i := strings.IndexByte(p, '*'); i >= 0 {
			if strings.HasPrefix(f, p[:i]) && strings.HasSuffix(f, p[i+1:]) {
				return true
			}
		} else if f == p {
			return true
		}
	}
	return false
}

func isHarmonyAttr(t, name string) bool {
	return t == "HarmonyLib."+name || t == "Harmony."+name
}

var methodTypes = []string{"Normal", "Getter", "Setter", "Constructor", "StaticConstructor", "Enumerator", "Async"}

type info struct {
	typ, method, args, mtype string
	set                      bool
}

func (in *info) merge(at dotnet.Attr) {
	in.set = true
	var strs []string
	for _, v := range at.Fixed {
		switch {
		case v.Kind == "type":
			in.typ = dotnet.CLRName(v.S)
		case v.Kind == "string":
			strs = append(strs, v.S)
		case strings.HasSuffix(v.Kind, "MethodType"):
			if i, err := strconv.Atoi(v.S); err == nil && i >= 0 && i < len(methodTypes) {
				in.mtype = methodTypes[i]
			}
		case v.Kind == "array":
			var as []string
			for _, e := range v.Arr {
				if e.Kind == "type" {
					as = append(as, dotnet.CSName(e.S))
				}
			}
			if len(as) > 0 || len(v.Arr) == 0 {
				in.args = "(" + strings.Join(as, ", ") + ")"
			}
		}
	}
	switch len(strs) {
	case 1:
		in.method = strs[0]
	case 2: // HarmonyPatch(string typeName, string methodName)
		in.typ, in.method = strs[0], strs[1]
	}
	for _, nv := range at.Named {
		switch nv.Name {
		case "methodName":
			in.method = nv.V.S
		case "declaringType":
			in.typ = dotnet.CLRName(nv.V.S)
		}
	}
}

func (in info) with(o info) info {
	r := in
	if o.typ != "" {
		r.typ = o.typ
	}
	if o.method != "" {
		r.method = o.method
	}
	if o.args != "" {
		r.args = o.args
	}
	if o.mtype != "" {
		r.mtype = o.mtype
	}
	r.set = in.set || o.set
	return r
}

func (in info) targetMethod() string {
	switch in.mtype {
	case "Getter":
		return "get_" + in.method
	case "Setter":
		return "set_" + in.method
	case "Constructor":
		return ".ctor"
	case "StaticConstructor":
		return ".cctor"
	}
	return in.method
}

var kindByName = map[string]string{"Prefix": Prefix, "Postfix": Postfix, "Transpiler": Transpiler, "Finalizer": Finalizer}
var kindAttr = map[string]string{"HarmonyPrefix": Prefix, "HarmonyPostfix": Postfix, "HarmonyTranspiler": Transpiler, "HarmonyFinalizer": Finalizer}

// Read analyses one DLL.
func Read(path string, skipHarmony bool) (*Assembly, error) {
	a, err := dotnet.Open(path)
	if err != nil {
		return nil, err
	}
	out := &Assembly{Types: a.TypeNames()}
	out.UsesHarm = a.References("HarmonyLib.") || a.References("Harmony.")
	out.Manual = a.HasMemberRef("HarmonyLib.Harmony", "Patch") || a.HasMemberRef("Harmony.HarmonyInstance", "Patch")
	if skipHarmony || !out.UsesHarm {
		return out, nil
	}
	attrs := a.AttributesOf(func(t string) bool {
		if !strings.HasPrefix(t, "HarmonyLib.") && !strings.HasPrefix(t, "Harmony.") {
			return false
		}
		n := t[strings.IndexByte(t, '.')+1:]
		return n == "HarmonyPatch" || kindAttr[n] != ""
	})
	classInfo := map[uint32]info{}
	methodInfo := map[uint32]info{}
	methodKind := map[uint32]string{}
	for _, x := range attrs {
		n := x.Type[strings.IndexByte(x.Type, '.')+1:]
		if x.Errors != "" && n == "HarmonyPatch" {
			out.AttrError++
		}
		switch {
		case n == "HarmonyPatch" && x.ParentKind == dotnet.ParentType:
			in := classInfo[x.ParentRow]
			in.merge(x.Attr)
			classInfo[x.ParentRow] = in
		case n == "HarmonyPatch" && x.ParentKind == dotnet.ParentMethod:
			in := methodInfo[x.ParentRow]
			in.merge(x.Attr)
			methodInfo[x.ParentRow] = in
		case kindAttr[n] != "" && x.ParentKind == dotnet.ParentMethod:
			methodKind[x.ParentRow] = kindAttr[n]
		}
	}
	// Types holding patch methods: every type with a class-level HarmonyPatch, and the owners
	// of methods with method-level HarmonyPatch or patch-kind attributes.
	rows := map[uint32]bool{}
	for r := range classInfo {
		rows[r] = true
	}
	for m := range methodInfo {
		rows[a.MethodOwner(m)] = true
	}
	for m := range methodKind {
		rows[a.MethodOwner(m)] = true
	}
	var sorted []uint32
	for r := range rows {
		sorted = append(sorted, r)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	for _, r := range sorted {
		ci, hasClass := classInfo[r]
		ms := a.Methods(r)
		dynamic := false
		for _, m := range ms {
			if m.Name == "TargetMethod" || m.Name == "TargetMethods" {
				dynamic = true
			}
		}
		for _, m := range ms {
			kind := methodKind[m.Row]
			if kind == "" {
				kind = kindByName[m.Name]
			}
			mi, hasMethod := methodInfo[m.Row]
			if kind == "" || (!hasClass && !hasMethod && methodKind[m.Row] == "") {
				continue
			}
			in := ci.with(mi)
			p := Patch{PatchType: a.TypeName(r), PatchMethod: m.Name, Kind: kind, TargetType: in.typ,
				TargetMethod: in.targetMethod(), Args: in.args, Skips: kind == Prefix && m.ReturnBool}
			if dynamic && (in.typ == "" || in.method == "" && in.mtype == "") {
				p.Dynamic = true
				p.TargetType, p.TargetMethod, p.Args = "", "", ""
			}
			if !p.Dynamic && p.TargetType == "" && p.TargetMethod == "" {
				continue // a helper named Prefix outside any patch class
			}
			out.Patches = append(out.Patches, p)
		}
	}
	// Harmony.Patch calls, direct or through a wrapper taking HarmonyMethods (manual.go).
	if out.Manual || a.References("HarmonyLib.HarmonyMethod") || a.References("Harmony.HarmonyMethod") {
		scanManual(a, out)
		out.Manual = out.Manual || out.ManualCalls > 0
	}
	out.IDs = idCandidates(a.UserStrings())
	return out, nil
}

var idRe = regexp.MustCompile(`^[A-Za-z][\w\-]*(\.[\w\-]+)+$`)

var notID = []string{"system.", "verse.", "rimworld.", "unityengine.", "harmonylib.", "mono.", "microsoft."}
var fileExt = regexp.MustCompile(`(?i)\.(xml|png|dds|dll|txt|json|cs|jpg|wav|ogg|psd|exe|zip)$`)

// idCandidates: #US strings shaped like Harmony IDs (reverse-domain or Author.Mod); heuristic.
func idCandidates(us []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range us {
		if len(s) < 3 || len(s) > 80 || !idRe.MatchString(s) || fileExt.MatchString(s) || seen[s] {
			continue
		}
		l := strings.ToLower(s)
		bad := false
		for _, p := range notID {
			if strings.HasPrefix(l, p) {
				bad = true
			}
		}
		allNum := true
		for _, seg := range strings.Split(s, ".") {
			for _, c := range seg {
				if c < '0' || c > '9' {
					allNum = false
				}
			}
		}
		if bad || allNum {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
