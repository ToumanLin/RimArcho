package harmony

import (
	"slices"
	"testing"
)

// TargetMethod() bodies that return one method built from constants become static targets
// (05 §5.3); a computed name stays dynamic.
func TestTargetMethodResolved(t *testing.T) {
	got := patches(t, "ModCompThrower.dll")
	want := []string{
		"postfix Verse.Pawn..ctor(int) <- CompThrower.Dyn_Ctor.Postfix",
		"postfix Verse.Thing.get_HitPoints <- CompThrower.Dyn_Getter.Postfix",
		"prefix <dynamic CompThrower.Dyn_Computed.TargetMethod()> <- CompThrower.Dyn_Computed.Prefix",
		"prefix Verse.Pawn.Kill <- CompThrower.Dyn_Method.Prefix",
		"prefix Verse.ThingWithComps.Tick <- CompThrower.Tick_Patch.Prefix skips",
	}
	if !slices.Equal(got, want) {
		t.Errorf("ModCompThrower =\n%q\nwant\n%q", got, want)
	}
	a, err := Read(fixture("ModCompThrower.dll"), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range a.Patches {
		if want := p.PatchType != "CompThrower.Tick_Patch" && p.PatchType != "CompThrower.Dyn_Computed"; p.FromTargetMethod != want {
			t.Errorf("%s FromTargetMethod = %v", p.PatchType, p.FromTargetMethod)
		}
	}
}
