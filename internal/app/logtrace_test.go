package app_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/dotnet"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func traceBin(name string) string { return filepath.Join(testkit.DotnetBin(), "trace", name) }

// traceEnv: the stand-in game's Assembly-CSharp.dll in the game folder, and two Mods:
// Colonist Mutterings (tester.mutter, TraceA.dll: passes null) and Genes (tester.genes,
// TraceB.dll: a postfix that dereferences it), scanned and analysed (MCP eval h4).
func traceEnv(t *testing.T) *testkit.Env {
	t.Helper()
	e := testkit.New(t)
	read := func(n string) string {
		b, err := os.ReadFile(traceBin(n))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	e.WriteFile(filepath.Join(e.Game, "RimWorldWin64_Data", "Managed", "Assembly-CSharp.dll"), read("Assembly-CSharp.dll"))
	mods := filepath.Join(e.Game, "Mods")
	for _, m := range []struct{ dir, pkg, name, dll string }{
		{"Mutter", "tester.mutter", "Colonist Mutterings", "TraceA.dll"},
		{"Genes", "tester.genes", "Genes", "TraceB.dll"},
	} {
		e.WriteMod(filepath.Join(mods, m.dir), testkit.Mod{PackageID: m.pkg, Name: m.name, Author: "Tester"})
		e.WriteFile(filepath.Join(mods, m.dir, "Assemblies", m.dll), read(m.dll))
	}
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.genes", "tester.mutter")
	e.AddEnv(false)
	e.MustRun("scan")
	if _, err := e.App.EnsureAnalysis(nil); err != nil {
		t.Fatal(err)
	}
	return e
}

// postfixOffset is the IL offset of "recipient.genes" (ldarg.3) in TraceB's postfix: where
// Mono's frame offset points.
func postfixOffset(t *testing.T) int {
	t.Helper()
	a, err := dotnet.Open(traceBin("TraceB.dll"))
	if err != nil {
		t.Fatal(err)
	}
	row := a.FindMethods("TraceB.PlayLogEntry_Interaction_Patch", "Postfix", 4)[0]
	for _, in := range dotnet.DecodeIL(a.Body(row)) {
		if in.Op == 0x05 {
			return in.Off
		}
	}
	t.Fatal("no ldarg.3")
	return 0
}

func h4Log(off int) string {
	return "RimWorld 1.6.4633 rev1261\n" +
		"[Colonist Mutterings] Error creating mutter: System.NullReferenceException: Object reference not set to an instance of an object\n" +
		"[Ref 2B4FACA6]\n" +
		fmt.Sprintf("  at TraceB.PlayLogEntry_Interaction_Patch.Postfix (Verse.InteractionDef& ___intDef, Verse.InteractionDef intDef, Verse.Pawn initiator, Verse.Pawn recipient) [0x%05x] in <897e673c9c37484b9a9f08faad2075a9>:0 \n", off) +
		"  at Verse.PlayLogEntry_Interaction..ctor (Verse.InteractionDef intDef, Verse.Pawn initiator, Verse.Pawn recipient, System.Collections.Generic.List`1[T] extraSentencePacks) [0x00056] in <61e4173561894da49d210260257b5097>:0 \n" +
		"    - POSTFIX tester.genes: Void TraceB.PlayLogEntry_Interaction_Patch:Postfix(InteractionDef& ___intDef, InteractionDef intDef, Pawn initiator, Pawn recipient)\n" +
		"TraceA.PlayLogEntry_Mutter..ctor(Pawn pawn, String text)\n" +
		"  at TraceA.Mutterings.Mutter (Verse.Pawn pawn) [0x0000c] in <416797debbaa4299ab6a923f96c2f3bf>:0 \n" +
		"\n"
}

func traceCluster(t *testing.T, rep *app.LogReport) *app.LogCluster {
	t.Helper()
	for _, lc := range rep.Clusters {
		if strings.Contains(lc.First.Message, "NullReferenceException") {
			return lc
		}
	}
	t.Fatal("no NullReferenceException cluster")
	return nil
}

// h4 P1: the postfix dereferences recipient, which the Mod's base constructor call passes as
// null. The caller is the source, the patch's Mod a patcher, both listed, caller first.
func TestNullTraceBaseConstructor(t *testing.T) {
	t.Parallel()
	e := traceEnv(t)
	off := postfixOffset(t)
	rep, err := e.App.AnalyzeLogText(h4Log(off))
	if err != nil {
		t.Fatal(err)
	}
	lc := traceCluster(t, rep)
	tr := strings.Join(lc.Trace, "\n")
	for _, want := range []string{
		fmt.Sprintf("frame 1 TraceB.PlayLogEntry_Interaction_Patch.Postfix +0x%x dereferences parameter recipient (ldarg.3, ldfld Verse.Pawn::genes)", off),
		"frame 1 is a Harmony postfix of frame 2 Verse.PlayLogEntry_Interaction..ctor: recipient is the original's parameter recipient",
		"frame 3 TraceA.PlayLogEntry_Mutter..ctor passes null for recipient (ldnull at IL_",
		"base constructor call)",
		"game contract: recipient: game sites 3, null at 0 (field 1, param 2); Mod sites 2, null at 1 (param 1; TraceA.PlayLogEntry_Mutter..ctor)",
	} {
		if !strings.Contains(tr, want) {
			t.Errorf("trace lacks %q:\n%s", want, tr)
		}
	}
	st, _ := e.App.Loadout()
	var got []string
	for _, at := range lc.Attr {
		got = append(got, st.Ix.ByID[at.Instance].PackageID+" "+at.Role)
	}
	if strings.Join(got, ", ") != "tester.mutter source, tester.genes patcher" {
		t.Errorf("attribution %v", got)
	}
	ev := strings.Join(lc.Attr[0].Evidence, "\n")
	if !strings.Contains(ev, "log frame 3 passes null for the argument the throwing frame dereferences") {
		t.Errorf("caller evidence:\n%s", ev)
	}
	if ev := strings.Join(lc.Attr[1].Evidence, "\n"); !strings.Contains(ev, "log frame 1, its Harmony postfix, dereferences the null that log frame 3 passes") {
		t.Errorf("patcher evidence:\n%s", ev)
	}
	if !strings.Contains(strings.Join(lc.Next, "\n"), "archo source callers Verse.PlayLogEntry_Interaction..ctor") {
		t.Errorf("next %v", lc.Next)
	}

	// The CLI shows the offsets and the trace (MCP analyze_log renders the same).
	pl := filepath.Join(e.Root, "userdata", "Player.log")
	e.WriteFile(pl, h4Log(off))
	out := e.MustRun("log", "--file", pl, "--cluster", lc.ID)
	for _, want := range []string{"trace", "game contract:", fmt.Sprintf("TraceB.PlayLogEntry_Interaction_Patch.Postfix +0x%x", off)} {
		if !strings.Contains(out, want) {
			t.Errorf("archo log lacks %q:\n%s", want, out)
		}
	}
	t.Logf("archo log --cluster:\n%s", out)
}

// A parameter passed on: the trace follows it to the frame that passes the null literal.
func TestNullTraceThroughParameter(t *testing.T) {
	t.Parallel()
	e := traceEnv(t)
	off := postfixOffset(t)
	log := "RimWorld 1.6.4633 rev1261\n" +
		"Exception: System.NullReferenceException: Object reference not set to an instance of an object\n" +
		fmt.Sprintf("  at TraceB.PlayLogEntry_Interaction_Patch.Postfix (Verse.InteractionDef& ___intDef, Verse.InteractionDef intDef, Verse.Pawn initiator, Verse.Pawn recipient) [0x%05x] in <8>:0 \n", off) +
		"  at Verse.PlayLogEntry_Interaction..ctor (Verse.InteractionDef intDef, Verse.Pawn initiator, Verse.Pawn recipient, System.Collections.Generic.List`1[T] extraSentencePacks) [0x00056] in <6>:0 \n" +
		"    - POSTFIX tester.genes: Void TraceB.PlayLogEntry_Interaction_Patch:Postfix(InteractionDef& ___intDef, InteractionDef intDef, Pawn initiator, Pawn recipient)\n" +
		"  at TraceA.Mutterings.Relay (Verse.Pawn a, Verse.Pawn r) [0x00005] in <4>:0 \n" +
		"  at TraceA.Mutterings.CallRelay (Verse.Pawn a) [0x00000] in <4>:0 \n\n"
	rep, err := e.App.AnalyzeLogText(log)
	if err != nil {
		t.Fatal(err)
	}
	lc := traceCluster(t, rep)
	tr := strings.Join(lc.Trace, "\n")
	for _, want := range []string{
		"frame 3 TraceA.Mutterings.Relay passes its parameter r for recipient (newobj)",
		"frame 4 TraceA.Mutterings.CallRelay passes null for r (ldnull at IL_",
	} {
		if !strings.Contains(tr, want) {
			t.Errorf("trace lacks %q:\n%s", want, tr)
		}
	}
	if len(lc.Attr) == 0 || lc.Attr[0].Role != app.RoleSource || !strings.Contains(strings.Join(lc.Attr[0].Evidence, "\n"), "log frame 4 passes null") {
		t.Errorf("attribution %+v", lc.Attr)
	}
}

// A trace that cannot be decoded prints nothing and changes nothing: an offset that is not an
// instruction (another build of the Mod), or no offset at all.
func TestNullTraceFails(t *testing.T) {
	t.Parallel()
	e := traceEnv(t)
	for _, log := range []string{h4Log(postfixOffset(t) + 2), strings.Replace(h4Log(postfixOffset(t)), fmt.Sprintf(" [0x%05x]", postfixOffset(t)), "", 1)} {
		rep, err := e.App.AnalyzeLogText(log)
		if err != nil {
			t.Fatal(err)
		}
		lc := traceCluster(t, rep)
		if len(lc.Trace) != 0 {
			t.Errorf("trace %v", lc.Trace)
		}
		for _, at := range lc.Attr {
			if at.Role == app.RolePatcher {
				t.Errorf("attribution changed without a trace: %+v", lc.Attr)
			}
		}
	}
}

// h8 G7: the lord rule's source Mod gets its Lord.AddPawn call sites, unchecked first in next:.
func TestLordAddPawnSites(t *testing.T) {
	t.Parallel()
	e := traceEnv(t)
	const msg = "Tried to add pawn Ann to lord with lord jobTraceA.LordToil_AttendClass but this pawn is already a member of lord with lord jobVerse.AI.Group.Lord. Pawns can't be members of more than one lord at the same time."
	rep, err := e.App.AnalyzeLogText("RimWorld 1.6.4633 rev1261\n" + msg + "\n")
	if err != nil {
		t.Fatal(err)
	}
	var lc *app.LogCluster
	for _, c := range rep.Clusters {
		if strings.HasPrefix(c.First.Message, "Tried to add pawn") {
			lc = c
		}
	}
	if lc == nil {
		t.Fatal("no lord cluster")
	}
	d := strings.Join(lc.Details, "\n")
	for _, want := range []string{
		"Lord.AddPawn call sites in #",
		"TraceA.LordToil_AttendClass.UpdateAllDuties IL_",
		"(no GetLord() check before it in the method)",
		"TraceA.LordToil_AttendClass.Join IL_",
		"(GetLord() called before it in the method)",
	} {
		if !strings.Contains(d, want) {
			t.Errorf("details lack %q:\n%s", want, d)
		}
	}
	if len(lc.Next) == 0 || !strings.Contains(lc.Next[0], "--type TraceA.LordToil_AttendClass") {
		t.Errorf("next %v", lc.Next)
	}
	t.Logf("details:\n%s\nnext: %v", d, lc.Next)
}

// h4 P2: the call-site table of the game constructor, base-constructor calls included.
func TestSourceCallers(t *testing.T) {
	t.Parallel()
	e := traceEnv(t)
	out := e.MustRun("source", "callers", "Verse.PlayLogEntry_Interaction..ctor")
	for _, want := range []string{
		"callers of Verse.PlayLogEntry_Interaction..ctor(InteractionDef intDef, Pawn initiator, Pawn recipient, List<> extraSentencePacks): 5 sites in 2 of 3 assemblies",
		"recipient: game sites 3, null at 0 (field 1, param 2); Mod sites 2, null at 1 (param 1; TraceA.PlayLogEntry_Mutter..ctor)",
		"extraSentencePacks: game sites 3, null at 1 (other 1, param 1; Verse.InteractionWorker.Social); Mod sites 2, null at 2 (TraceA.PlayLogEntry_Mutter..ctor, TraceA.Mutterings.Relay)",
		"game  Verse.InteractionWorker.Social IL_",
		"TraceA.PlayLogEntry_Mutter..ctor IL_0009 base constructor call  intDef=field Verse.InteractionWorker::Chitchat, initiator=param pawn, recipient=null, extraSentencePacks=null",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("source callers lacks %q:\n%s", want, out)
		}
	}
	// A method nobody calls; a bad target.
	if out := e.MustRun("source", "callers", "Verse.AI.Group.Lord.AddPawn(1)"); !strings.Contains(out, "2 sites") {
		t.Errorf("AddPawn:\n%s", out)
	}
	if _, code := e.Run("source", "callers", "NoDots"); code == 0 {
		t.Error("bad target accepted")
	}
	t.Logf("%s", out)
}

// h8 G4: what a rebuild changed beyond the intended method.
func TestSourceDiff(t *testing.T) {
	t.Parallel()
	e := traceEnv(t)
	out := e.MustRun("source", "diff", traceBin(filepath.Join("diff", "old", "TraceDiff.dll")), traceBin(filepath.Join("diff", "new", "TraceDiff.dll")))
	for _, want := range []string{
		"assembly diff TraceDiff 1.0.0.0 → TraceDiff 1.1.0.0: 1 changed, 1 added, 0 removed methods (1 unchanged)",
		"methods changed:",
		"TraceDiff.Changed::Value(Verse.Pawn)",
		"Verse.Secret::Hidden()  (Assembly-CSharp; internal, type internal)  !! non-public and no IgnoresAccessChecksTo(Assembly-CSharp)",
		"~ Assembly-CSharp 1.0.0.0 → 1.1.0.0",
		"System.ObsoleteAttribute on TraceDiff.Changed::Added (use Value)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("source diff lacks %q:\n%s", want, out)
		}
	}
	t.Logf("%s", out)
}

// ArgumentNullException: the parameter the message names, followed like a dereferenced one.
func TestNullTraceArgumentNull(t *testing.T) {
	t.Parallel()
	e := traceEnv(t)
	log := strings.Replace(h4Log(postfixOffset(t)), "System.NullReferenceException: Object reference not set to an instance of an object\n",
		"System.ArgumentNullException: Value cannot be null.\nParameter name: recipient\n", 1)
	rep, err := e.App.AnalyzeLogText(log)
	if err != nil {
		t.Fatal(err)
	}
	var lc *app.LogCluster
	for _, c := range rep.Clusters {
		if strings.Contains(c.First.Message, "ArgumentNullException") {
			lc = c
		}
	}
	if lc == nil {
		t.Fatal("no cluster")
	}
	tr := strings.Join(lc.Trace, "\n")
	if !strings.Contains(tr, "frame 1 TraceB.PlayLogEntry_Interaction_Patch.Postfix throws ArgumentNullException for its parameter recipient") ||
		!strings.Contains(tr, "frame 3 TraceA.PlayLogEntry_Mutter..ctor passes null for recipient") {
		t.Errorf("trace:\n%s", tr)
	}
}
