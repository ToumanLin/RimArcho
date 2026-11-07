package app

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

// nearby lines skip a transpiler's IL listing and show the messages around it (P13).
func TestNearMessagesSkipILDump(t *testing.T) {
	text := "Loading mod A\nAdjustParms - ldfld System.Int32 a [0],\nAdjustParms - ldfld System.Int32 b [1],\n" +
		"AdjustParms - callvirt Verse.Pawn get_RaceProps()\nAdjustParms - stloc.s 5\n" +
		"Exception: boom\n  at Foo.Bar()\nIL_0001: ret\nLoaded mod B\n"
	lf, err := logs.Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	var e *logs.Entry
	for _, x := range lf.Entries {
		if strings.HasPrefix(x.Message, "Exception: boom") {
			e = x
		}
	}
	got := strings.Join(nearMessages(lf, e), "\n")
	if got != "before:1 Loading mod A\nafter:9 Loaded mod B" {
		t.Fatalf("nearby:\n%s", got)
	}
}
