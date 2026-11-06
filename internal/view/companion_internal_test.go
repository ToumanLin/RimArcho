package view

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
)

// 06 §7: a file the companion log names becomes the Game log chip of the last run's item.
func TestCompanionChip(t *testing.T) {
	t.Parallel()
	p := &app.Problem{Finding: &finding.Finding{Claim: "log:E1 x9: Config error in a_Thing: has no label",
		Evidence: []finding.Evidence{{Source: finding.Log, Text: "source: log file (ArchoMonitor.log): Defs/ThingDefs/a.xml (while loading a_Thing)"}}}}
	ev := logEvidence([]*app.Problem{p}, "10 月 21 日 14:05")
	if ev.Kind != EvLog || ev.Source != "file Defs/ThingDefs/a.xml  ArchoMonitor.log  10 月 21 日 14:05" {
		t.Fatalf("chip %+v", ev)
	}
	q := &app.Problem{Finding: &finding.Finding{Claim: "log:E2 x1: other", Evidence: []finding.Evidence{{Source: finding.Log, Text: "source: log file (RimCrow_Realtime.log): Defs/b.xml"}}}}
	if ev := logEvidence([]*app.Problem{p, q}, ""); ev.Source != "file Defs/ThingDefs/a.xml and 1 more  ArchoMonitor.log" {
		t.Fatalf("two files: %+v", ev)
	}
	if ev := logEvidence([]*app.Problem{{Finding: &finding.Finding{Claim: "log:E3 x1: x"}}}, ""); ev.Source != "Player.log, 1 error groups" {
		t.Fatalf("no file: %+v", ev)
	}
}
