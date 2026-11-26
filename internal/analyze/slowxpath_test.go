package analyze

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/scan"
)

// A player's check stopped at "patches: 7000/31477 operations" for good (2026-11-25):
// some expressions cost the engine candidates × document (here `[last()]`, which walks
// every sibling again for each candidate), so one operation on a large list ran for hours
// on one below-normal thread. Such an evaluation now stops at the step budget, the
// operation is reported as not simulated (patch.too-slow) and the check goes on (05 §5.2).
func TestSlowXPathNotSimulated(t *testing.T) {
	root := t.TempDir()
	var body strings.Builder
	const n = 11000 // [last()] over n top-level defs: about n² = 1.2×10⁸ moves, past EvalStepsMin
	for i := 0; i < n; i++ {
		fmt.Fprintf(&body, "<ThingDef><defName>T%d</defName></ThingDef>\n", i)
	}
	core := &Mod{Instance: "core", Lineage: "Lcore", Dir: filepath.Join(root, "core"), Pkg: "ludeon.rimworld", Name: "Core", Pos: 1, SnapHash: []byte("core")}
	write(t, filepath.Join(core.Dir, "Defs", "Things.xml"), defs(body.String()))
	slow := &Mod{Instance: "slow", Lineage: "Lslow", Dir: filepath.Join(root, "slow"), Pkg: "test.slow", Name: "Slow Mod", Pos: 2, SnapHash: []byte("slow"), Decl: scan.Decl{}}
	write(t, filepath.Join(slow.Dir, "Patches", "P.xml"), patch(
		// 1: too slow at the top level
		`<Operation Class="PatchOperationAdd"><xpath>Defs/ThingDef[last()]</xpath><value><a/></value></Operation>`,
		// 2: an ordinary operation after it still runs
		`<Operation Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="T5"]</xpath><value><b/></value></Operation>`,
		// 3: too slow inside a Sequence's Conditional
		`<Operation Class="PatchOperationSequence"><operations>
			<li Class="PatchOperationConditional"><xpath>Defs/ThingDef[position()=last()]/c</xpath><nomatch Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="T6"]</xpath><value><c/></value></nomatch></li>
		</operations></Operation>`,
		// 4: fails as before
		`<Operation Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="Nowhere"]</xpath><value><d/></value></Operation>`,
	))
	was := ProgressEvery
	ProgressEvery = time.Nanosecond
	defer func() { ProgressEvery = was }()
	var lines []string
	t0 := time.Now()
	out, err := Run(Input{MM: "1.6", Mods: []*Mod{core, slow}, Progress: func(l string) { lines = append(lines, l) }})
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(t0); d > 30*time.Second {
		t.Errorf("the check took %v: the budget did not stop the slow evaluations", d)
	}
	for seq, want := range map[int]string{1: ResultUnknown, 2: ResultOK, 3: ResultUnknown, 4: ResultFailed} {
		p := patchBySeq(out, "slow", seq)
		if p == nil || p.Result != want {
			t.Fatalf("operation %d: %+v, want %s", seq, p, want)
		}
		if want == ResultUnknown && !strings.Contains(p.Detail, "step budget") {
			t.Errorf("operation %d detail %q", seq, p.Detail)
		}
	}
	if out.Stats.SlowOps != 2 || out.Stats.Failed != 1 {
		t.Errorf("stats: %d slow, %d failed", out.Stats.SlowOps, out.Stats.Failed)
	}
	if out.Stats.SlowestOp == "" || out.Stats.SlowestMs <= 0 {
		t.Errorf("slowest operation not recorded: %q %d ms", out.Stats.SlowestOp, out.Stats.SlowestMs)
	}
	fs := findingsOf(out, "patch.too-slow")
	if len(fs) != 1 || len(fs[0].Evidence) != 2 || !strings.Contains(fs[0].Evidence[0].Text, "P.xml:3 Defs/ThingDef[last()]") ||
		!strings.Contains(fs[0].Evidence[1].Text, "position()=last()") {
		t.Fatalf("patch.too-slow: %+v", fs)
	}
	if nm := findingsOf(out, "patch.no-match"); len(nm) != 1 {
		t.Errorf("patch.no-match: %d, want only operation 4", len(nm))
	}
	if len(findingsOf(out, "patch.unknown-op")) != 0 {
		t.Error("slow operations reported as mod-defined classes")
	}
	// a line after the slow first operation (ProgressEvery this short), and the last one:
	// the window's clock moves while operations are slow
	for _, want := range []string{"patches: 1/4 operations", "patches: 4/4 operations"} {
		if !slices.Contains(lines, want) {
			t.Errorf("no %q in %q", want, lines)
		}
	}
}
