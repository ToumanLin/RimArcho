package logs

import (
	"strings"
	"testing"
)

// The IL offset of a Mono frame is kept (06 §1); frames without one have none.
func TestFrameILOffset(t *testing.T) {
	text := "[Colonist Mutterings] Error creating mutter: System.NullReferenceException: Object reference not set to an instance of an object\n" +
		"[Ref 2B4FACA6]\n" +
		"  at TraceB.PlayLogEntry_Interaction_Patch.Postfix (Verse.InteractionDef& ___intDef, Verse.InteractionDef intDef, Verse.Pawn initiator, Verse.Pawn recipient) [0x00076] in <897e673c9c37484b9a9f08faad2075a9>:0 \n" +
		"  at Verse.PlayLogEntry_Interaction..ctor (Verse.InteractionDef intDef, Verse.Pawn initiator, Verse.Pawn recipient, System.Collections.Generic.List`1[T] extraSentencePacks) [0x00056] in <61e4173561894da49d210260257b5097>:0 \n" +
		"    - POSTFIX tester.traceb: Void TraceB.PlayLogEntry_Interaction_Patch:Postfix(InteractionDef& ___intDef, InteractionDef intDef, Pawn initiator, Pawn recipient)\n" +
		"TraceA.PlayLogEntry_Mutter..ctor(Pawn pawn, String text)\n" +
		"  at TraceA.Mutterings.Mutter (Verse.Pawn pawn) [0x0000c] in <416797debbaa4299ab6a923f96c2f3bf>:0 \n"
	f, err := Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	fr := f.Entries[0].Frames
	if len(fr) != 4 {
		t.Fatalf("frames %+v", fr)
	}
	for i, want := range []struct {
		il int
		ok bool
	}{{0x76, true}, {0x56, true}, {0, false}, {0xc, true}} {
		if fr[i].IL != want.il || fr[i].HasIL != want.ok {
			t.Errorf("frame %d %s: IL %#x %v", i+1, fr[i].Text, fr[i].IL, fr[i].HasIL)
		}
	}
}
