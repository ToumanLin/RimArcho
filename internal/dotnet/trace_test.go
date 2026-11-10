package dotnet

import (
	"path/filepath"
	"strings"
	"testing"
)

func traceDLL(t *testing.T, name string) *Assembly {
	t.Helper()
	a, err := Open(filepath.Join("..", "..", "testdata", "dotnet", "bin", "trace", name))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func describeArgs(cs CallSite) string {
	var out []string
	for _, s := range cs.Args {
		out = append(out, s.Describe(cs.Names))
	}
	return strings.Join(out, ", ")
}

func TestCallSitesShapes(t *testing.T) {
	game := traceDLL(t, "Assembly-CSharp.dll")
	tg := Target{Owner: "Verse.PlayLogEntry_Interaction", Name: ".ctor", Params: 4}
	sites := game.CallSites(tg)
	got := map[string]string{}
	for _, s := range sites {
		got[s.Caller] = describeArgs(s)
	}
	want := map[string]string{
		"Verse.InteractionWorker.Interacted": "field Verse.InteractionWorker::Chitchat, param initiator, param recipient, param extra",
		"Verse.InteractionWorker.Social":     "field Verse.InteractionWorker::Chitchat, param a, field Verse.InteractionWorker::other, null",
		"Verse.InteractionWorker.Third":      "field Verse.InteractionWorker::Chitchat, param b, param a, other (new List`1)",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	if names := game.ParamNames(game.FindMethods("Verse.PlayLogEntry_Interaction", ".ctor", 4)[0]); strings.Join(names, ",") != "intDef,initiator,recipient,extraSentencePacks" {
		t.Errorf("param names %v", names)
	}

	mod := traceDLL(t, "TraceA.dll")
	sites = mod.CallSites(tg)
	got = map[string]string{}
	for _, s := range sites {
		got[s.Caller] = describeArgs(s)
		if s.Caller == "TraceA.PlayLogEntry_Mutter..ctor" && !s.Base {
			t.Errorf("base ctor call not marked: %+v", s)
		}
	}
	if got["TraceA.PlayLogEntry_Mutter..ctor"] != "field Verse.InteractionWorker::Chitchat, param pawn, null, null" ||
		got["TraceA.Mutterings.Relay"] != "field Verse.InteractionWorker::Chitchat, param a, param r, null" {
		t.Errorf("mod sites: %v", got)
	}
}

func TestDerefsAt(t *testing.T) {
	b := traceDLL(t, "TraceB.dll")
	rows := b.FindMethods("TraceB.PlayLogEntry_Interaction_Patch", "Postfix", 4)
	if len(rows) != 1 {
		t.Fatalf("Postfix rows %v", rows)
	}
	row := rows[0]
	// The statement "if (recipient.genes == null)": ldarg.3 starts it.
	off := -1
	for _, in := range DecodeIL(b.Body(row)) {
		if in.Op == 0x05 {
			off = in.Off
		}
	}
	ds, ok := b.DerefsAt(row, off)
	if !ok || len(ds) != 1 {
		t.Fatalf("derefs at %#x: %v %+v", off, ok, ds)
	}
	names := b.ParamNames(row)
	if v := ValueText(ds[0].Value, names); v != "parameter recipient" {
		t.Errorf("value %q", v)
	}
	if s := b.InstrText(ds[0].At); s != "ldfld Verse.Pawn::genes" {
		t.Errorf("at %q", s)
	}
	if s := b.PushText(row, ds[0].Value); s != "ldarg.3" {
		t.Errorf("push %q", s)
	}
	// Not an instruction boundary: nothing.
	if _, ok := b.DerefsAt(row, off+100); ok {
		t.Error("deref at a bad offset")
	}
	// Len: "n.Length" dereferences local 0, which holds a field of parameter p.
	lr := b.FindMethods("TraceB.Helper", "Len", 1)[0]
	var all []string
	for _, in := range DecodeIL(b.Body(lr)) {
		ds, _ := b.DerefsAt(lr, in.Off)
		for _, d := range ds {
			all = append(all, ValueText(d.Value, b.ParamNames(lr))+" @ "+b.InstrText(d.At))
		}
	}
	if j := strings.Join(all, "; "); !strings.Contains(j, "parameter p @ ldfld Verse.Pawn::Name") {
		t.Errorf("Len derefs: %s", j)
	}
}

func TestParseTarget(t *testing.T) {
	for in, want := range map[string]Target{
		"Verse.PlayLogEntry_Interaction..ctor": {"Verse.PlayLogEntry_Interaction", ".ctor", -1},
		"Verse.AI.Group.Lord.AddPawn":          {"Verse.AI.Group.Lord", "AddPawn", -1},
		"Verse.AI.Group.Lord:AddPawn(1)":       {"Verse.AI.Group.Lord", "AddPawn", 1},
		"Verse.X..ctor(Pawn a, List`1[T] b)":   {"Verse.X", ".ctor", 2},
		"Verse.X.M()":                          {"Verse.X", "M", 0},
	} {
		if got, ok := ParseTarget(in); !ok || got != want {
			t.Errorf("%s: %+v", in, got)
		}
	}
}

func TestDiff(t *testing.T) {
	old := traceDLL(t, filepath.Join("diff", "old", "TraceDiff.dll"))
	cur := traceDLL(t, filepath.Join("diff", "new", "TraceDiff.dll"))
	game := traceDLL(t, "Assembly-CSharp.dll")
	r := Diff(old, cur, func(name string) *Assembly {
		if name == "Assembly-CSharp" {
			return game
		}
		return nil
	})
	if r.Old.Version != "1.0.0.0" || r.New.Version != "1.1.0.0" {
		t.Errorf("versions %+v %+v", r.Old, r.New)
	}
	if strings.Join(r.Changed, "|") != "TraceDiff.Changed::Value(Verse.Pawn)" {
		t.Errorf("changed %v", r.Changed)
	}
	if strings.Join(r.Added, "|") != "TraceDiff.Changed::Added()" || len(r.Removed) != 0 || r.Unchanged == 0 {
		t.Errorf("added %v removed %v unchanged %d", r.Added, r.Removed, r.Unchanged)
	}
	var hidden *RefDiff
	for i, d := range r.RefsAdded {
		if d.Name == "Hidden" {
			hidden = &r.RefsAdded[i]
		}
	}
	if hidden == nil || hidden.Scope != "Assembly-CSharp" || hidden.Access != AccessInternal || !hidden.NonPublic() {
		t.Errorf("refs added %+v", r.RefsAdded)
	}
	attrs := strings.Join(r.AttrsAdded, "|")
	if !strings.Contains(attrs, "System.ObsoleteAttribute on TraceDiff.Changed::Added (use Value)") {
		t.Errorf("attrs added %v removed %v", r.AttrsAdded, r.AttrsRemoved)
	}
	t.Logf("%+v", r)
}
