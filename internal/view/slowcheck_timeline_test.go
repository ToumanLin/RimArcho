package view

import (
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/oplog"
)

// The phases tile the wall time: the child's start and exit around its row, what the
// window did after, and other only for what nothing names (16 §2.14 "Slow checks").
func TestCheckTimeline(t *testing.T) {
	t0 := time.UnixMilli(1_800_000_000_000)
	ms := func(n int) time.Time { return t0.Add(time.Duration(n) * time.Millisecond) }
	clock := &app.CheckClock{Began: t0, Started: ms(100), Exited: ms(1400),
		After: []app.CheckPhase{{Name: "lock", Ms: 5}, {Name: "ui", Ms: 10}}}
	var p app.CheckPhases
	p.Add("read", 200)
	p.Add("defs", 700)
	row := oplog.Row{At: ms(300), Ms: 1000}
	got := checkTimeline(clock, row, p, app.CheckReport{OpenMs: 50}, 1500*time.Millisecond)
	want := []struct {
		n  string
		ms int64
	}{{"spawn", 100}, {"boot", 150}, {"open", 50}, {"read", 200}, {"defs", 700}, {"exit", 100}, {"lock", 5}, {"ui", 10}, {"other", 185}}
	if len(got) != len(want) {
		t.Fatalf("phases %v", got)
	}
	for i, w := range want {
		if got[i].Name != w.n || got[i].Ms != w.ms {
			t.Fatalf("phase %d: %v, want %s %d (all %v)", i, got[i], w.n, w.ms, got)
		}
	}

	// in the same process (tests, AnalyzeRunner): the row's phases and the rest
	got = checkTimeline(&app.CheckClock{Began: t0}, row, p, app.CheckReport{}, 1500*time.Millisecond)
	if len(got) != 3 || got[2].Name != "other" || got[2].Ms != 600 {
		t.Fatalf("in process: %v", got)
	}
}
