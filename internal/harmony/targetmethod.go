package harmony

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/dotnet"
)

// resolveTargetMethods turns dynamic [HarmonyPatch] classes into static targets when their
// TargetMethod() body is simple: it returns one method built from constants, such as
// AccessTools.Method(typeof(T), "Name"), AccessTools.Constructor(typeof(T), new[] { … }),
// AccessTools.PropertyGetter(typeof(T), "P") or typeof(T).GetMethod("Name") (05 §5.3). The
// body is evaluated with the manual-patch evaluator (manual.go), without helpers of helpers:
// a method that calls another of the mod's methods to pick the target stays dynamic, and so
// does TargetMethods() (several targets).
func resolveTargetMethods(a *dotnet.Assembly, out *Assembly) {
	resolved := map[string]*target{} // patch class → its one target; nil = not resolvable
	var s *scanner
	for i := range out.Patches {
		p := &out.Patches[i]
		if !p.Dynamic || p.Manual || p.TargetType != "" {
			continue
		}
		t, done := resolved[p.PatchType]
		if !done {
			if s == nil {
				s = evaluated(a)
			}
			t = s.targetMethod(p.PatchType)
			resolved[p.PatchType] = t
		}
		if t == nil {
			continue
		}
		p.Dynamic = false
		p.TargetType, p.TargetMethod, p.Args = t.typ, t.method, t.args
		p.FromTargetMethod = true
	}
}

// evaluated runs the evaluator over every method of the assembly for two rounds, so that
// static fields (static readonly MethodInfo set in a static constructor) and the return
// values of the mod's own helper methods are known when TargetMethod() is evaluated.
// Harmony.Patch calls found on the way are dropped: scanManual reports those.
func evaluated(a *dotnet.Assembly) *scanner {
	s := &scanner{a: a, sums: map[uint32]*summary{}, fields: map[string]*val{}, out: &Assembly{}, seen: map[string]bool{}}
	n := uint32(a.MethodCount())
	for r := 0; r < 2; r++ {
		s.next = map[uint32]*summary{}
		s.fnext = map[string][]*val{}
		s.called = map[uint32]bool{}
		for m := uint32(1); m <= n; m++ {
			s.run(m)
		}
		s.sums = s.next
		s.fields = map[string]*val{}
		for k, vs := range s.fnext {
			v := vs[0]
			for _, x := range vs[1:] {
				if x.key() != v.key() {
					v = unk("field")
					break
				}
			}
			s.fields[k] = v
		}
	}
	return s
}

// targetMethod evaluates TargetMethod() of a patch class; nil when it does not return one
// statically known method.
func (s *scanner) targetMethod(patchType string) *target {
	row := s.a.TypeRow(patchType)
	if row == 0 {
		return nil
	}
	var m uint32
	for _, x := range s.a.Methods(row) {
		if x.Name == "TargetMethod" && x.Params == 0 {
			m = x.Row
		}
	}
	if m == 0 {
		return nil
	}
	s.next = map[uint32]*summary{}
	s.fnext = map[string][]*val{}
	s.called = map[uint32]bool{}
	s.run(m)
	sm := s.next[m]
	if sm == nil || sm.ret == nil {
		return nil
	}
	ts, why := methods(sm.ret)
	if why != "" || len(ts) != 1 || ts[0].typ == "" || ts[0].method == "" || strings.ContainsAny(ts[0].typ, "!") {
		return nil
	}
	return &ts[0]
}
