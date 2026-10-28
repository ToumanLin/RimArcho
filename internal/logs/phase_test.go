package logs

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func phaseTotals(cs []PhaseCount) map[string]int {
	out := map[string]int{}
	for _, c := range cs {
		out[PhaseName(c.Phase)] += c.Count
	}
	return out
}

func TestPlayerPhases(t *testing.T) {
	const texture = "Could not load Texture2D at 'Things/Example/Empty' for def 'Example'"
	// A synthetic save-loading failure: the phase is the marker, not its error class.
	const saveError = "Could not resolve reference to object with loadID MissingPawn of type Verse.Pawn"
	lines := []string{texture, "[Archo Monitor] phase menu", "[Archo Monitor] phase new", texture, "[Archo Monitor] phase play", texture,
		"[Archo Monitor] phase save", saveError, texture, "[Archo Monitor] phase play", texture, "[Archo Monitor] phase menu"}
	for _, waylith := range []bool{false, true} {
		t.Run(fmt.Sprint(waylith), func(t *testing.T) {
			var b strings.Builder
			if waylith {
				b.WriteString("=== Waylith log ===\nVersion: 1.7.4984.1269\n")
			} else {
				b.WriteString("RimWorld 1.6.4871 rev590\n")
			}
			for i, l := range lines {
				if waylith {
					level := "ERROR"
					if strings.HasPrefix(l, "[Archo Monitor]") {
						level = "INFO"
					}
					fmt.Fprintf(&b, "2026-10-28 09:47:%02d.000 %s [Verse] %s\n", i, level, l)
				} else {
					b.WriteString(l + "\n(Filename: Synthetic.cpp Line: 0)\n\n")
				}
			}
			f, err := Parse(strings.NewReader(b.String()))
			if err != nil {
				t.Fatal(err)
			}
			cs := Clusters(f)
			if !f.HasPhases || len(f.Phases) != 6 || len(cs) != 2 {
				t.Fatalf("phases=%+v clusters=%+v", f.Phases, cs)
			}
			c := cs[0]
			if c.Count != 5 || c.Phase.Name != "loading" || PhaseSummary(c.Phases) != " [loading x1, save x1, new x1, play x2]" {
				t.Fatalf("texture cluster: %+v %s", c, PhaseSummary(c.Phases))
			}
			if cs[1].Phase.Name != "save" || cs[1].Class != ClassSave || cs[1].Count != 1 {
				t.Fatalf("save cluster: %+v", cs[1])
			}
			if waylith && cs[1].Phase.At.Format("15:04") != "09:47" {
				t.Fatal("Waylith marker timestamp lost")
			}
		})
	}
	f, _ := Parse(strings.NewReader(texture + "\n\n" + texture + "\n"))
	c := Clusters(f)[0]
	if f.HasPhases || c.Phase != nil || len(c.Phases) != 0 || PhaseSummary(c.Phases) != "" || c.Count != 2 {
		t.Fatalf("unmarked log changed: %+v", c)
	}
	f, _ = Parse(strings.NewReader("[Archo Monitor] phase guessed\n" + texture + "\n"))
	if f.HasPhases {
		t.Fatal("unknown phase accepted")
	}
}

func TestCompanionPhaseCounts(t *testing.T) {
	c := NewCompanion("synthetic")
	feed := func(kind, phase, save string, n, total int) {
		j := map[string]any{"timestamp": "2026-10-28 09:48:00.000", "source": "archo-monitor", "schema": 1, "kind": kind, "id": "same", "level": "ERROR", "message": "Could not load Texture2D at 'Example'", "count": n, "total": total, "context": map[string]any{"phase": phase, "save": save}}
		if kind == "phase" {
			j["message"] = "[Archo Monitor] phase " + phase
			j["level"] = "INFO"
		}
		b, _ := json.Marshal(j)
		// Reads can stop anywhere, including within the boundary line.
		c.Feed(b[:len(b)/2], false)
		c.Feed(append(b[len(b)/2:], '\n'), false)
	}
	feed("", "", "", 3, 3)
	feed("phase", "menu", "", 1, 0)
	feed("phase", "save", "Colony1.rws", 1, 0)
	feed("", "", "", 2, 5)
	feed("phase", "play", "", 1, 0)
	feed("", "", "", 7, 12)
	feed("phase", "menu", "", 1, 0)
	feed("phase", "save", "Colony1.rws", 1, 0)
	feed("", "", "", 3, 15)
	feed("phase", "play", "", 1, 0)
	feed("", "", "", 1, 16)
	if !c.HasPhases || len(c.Entries) != 1 || len(c.Segments) != 5 || len(c.Phases) != 6 {
		t.Fatalf("entries %d segments %d phases %d", len(c.Entries), len(c.Segments), len(c.Phases))
	}
	e := c.Entries[0]
	if e.Count != 16 || PhaseSummary(e.Phases) != " [loading x3, save x5, play x8]" {
		t.Fatalf("counts: %+v %s", e, PhaseSummary(e.Phases))
	}
	if c.Segments[1].Phase.Save != "Colony1.rws" || c.Segments[1].Count != 2 || c.Segments[3].Count != 3 {
		t.Fatal("save boundaries merged")
	}
	// Two sources overlap; neither their totals nor their per-phase counts are added.
	a := []PhaseCount{{LoadingPhase, 4}, {&Phase{Name: "save"}, 1}, {&Phase{Name: "new"}, 2}}
	merged := MergePhaseCounts(a, e.Phases)
	if got := PhaseSummary(merged); got != " [loading x4, save x5, new x2, play x8]" {
		t.Fatal(got)
	}
}

func TestCompanionTotalDoesNotInventPhaseCounts(t *testing.T) {
	c := NewCompanion("synthetic")
	c.Feed([]byte(`{"kind":"phase","source":"archo-monitor","message":"[Archo Monitor] phase save","context":{"phase":"save"}}`+"\n"+
		`{"source":"archo-monitor","id":"x","level":"ERROR","message":"Config error in Example: no label","count":1,"total":5}`+"\n"), true)
	e := c.Entries[0]
	if e.Count != 5 || PhaseSummary(e.Phases) != " [save x1]" {
		t.Fatalf("total assigned to current phase: %+v", e)
	}
	if got := PhaseSummary(CompletePhaseCounts(e.Phases, e.Count)); got != " [save x1, unknown x4]" {
		t.Fatal(got)
	}
}
