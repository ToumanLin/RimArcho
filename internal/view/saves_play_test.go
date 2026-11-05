package view_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/savefile"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// colonyOf makes a colony of saves, newest first; days < 0 are saves not read yet.
func colonyOf(days ...int) *app.Colony {
	c := &app.Colony{Key: "w", Name: "Iron Pact", By: "world"}
	for i, d := range days {
		s := &app.Save{Ref: fmt.Sprintf("sv:%06d", i), Name: fmt.Sprintf("Save %d", i), Colony: c}
		if d >= 0 {
			s.Summary = &app.SaveSummary{Ticks: int64(d) * savefile.TicksPerDay, Day: d}
		}
		c.Saves = append(c.Saves, s)
	}
	return c
}

func series(n int, f func(i int) float32) []float32 {
	v := make([]float32, n)
	for i := range v {
		v[i] = f(i)
	}
	return v
}

// The right card from the full read (06 §2.11 "Right", 10 §4.5): only what the facts
// hold, the change since the colony's previous read save, and lists kept short.
func TestSavePlayOf(t *testing.T) {
	t.Parallel()
	// newest first: a later save (day 60), the selected one (day 40), one not read, the previous (day 20)
	c := colonyOf(60, 40, -1, 20)
	sel, prevSave := c.Saves[1], c.Saves[3]
	if got := view.PreviousReadSave(sel); got != prevSave {
		t.Fatalf("previous read save %v", got)
	}
	f := &savefile.Facts{TicksGame: 40*savefile.TicksPerDay + 1000, GameStartAbsTick: 300000, Storyteller: "Kiiro_Storyteller",
		Difficulty: "Extreme", Permadeath: true, RealPlayTime: 1500, Animals: 2, Prisoners: 1,
		Stats:    savefile.Stats{RaidsEnemy: 7},
		Research: savefile.Research{Progress: map[string]float64{"Electricity": 1600, "Batteries": 400}},
		History: []savefile.Recorder{
			{Def: "Wealth_Total", Count: 401, Last: 400 * 300, Values: series(401, func(i int) float32 { return float32(i * 300) })},
			{Def: "FreeColonists", Count: 401, Last: 5, Values: series(401, func(i int) float32 { return float32(3 + i/200) })},
		},
		Events:    []savefile.Event{{Tick: 39 * savefile.TicksPerDay, Def: "Death", Label: "Death: <color=#ff0000>Ozzie</color>"}, {Tick: 30 * savefile.TicksPerDay, Def: "ThreatBig", Label: "Raid"}},
		Malformed: &savefile.Malformed{Line: 812},
	}
	for i := range 45 {
		f.Colonists = append(f.Colonists, savefile.Pawn{Nick: fmt.Sprintf("P%d", i), Dead: i == 0, Slave: i == 1})
	}
	prev := &savefile.Facts{TicksGame: 20 * savefile.TicksPerDay, Animals: 2, Stats: savefile.Stats{RaidsEnemy: 3},
		History: []savefile.Recorder{{Def: "Wealth_Total", Count: 1, Last: 150000}}}
	for range 40 {
		prev.Colonists = append(prev.Colonists, savefile.Pawn{Nick: "x"})
	}

	p := view.SavePlayOf(sel, f, prevSave, prev)
	if p.Day != "day 40" || p.Date != "1st of Decembary, 5500" || p.Storyteller != "Kiiro_Storyteller" || p.Difficulty != "losing is fun  commitment mode" ||
		p.Played != "25m" || p.Since != "Save 3" || p.Note == "" {
		t.Fatalf("head %+v", p)
	}
	var stats []string
	for _, s := range p.Stats {
		stats = append(stats, fmt.Sprintf("%s=%s(%s)%v", s.Label, s.Value, s.Change, s.Down))
	}
	want := []string{"wealth=120k(−30k)true", "colonists=44(+4)false", "animals=2(±0)false", "research points=2k(+2k)false",
		"raids=7(+4)false", "prisoners=1(+1)false"}
	if !slices.Equal(stats, want) {
		t.Fatalf("stats\n%q\nwant\n%q", stats, want)
	}
	// alive first, the dead last; at most PlayColonists
	last := p.Colonists[len(p.Colonists)-1]
	if len(p.Colonists) != view.PlayColonists || p.More != 5 || p.Dead != 1 || p.Colonists[0].Name != "P1" || !p.Colonists[0].Slave ||
		p.Colonists[0].Dead || last.Name != "P0" || !last.Dead {
		t.Fatalf("colonists %d more %d dead %d first %+v last %+v", len(p.Colonists), p.More, p.Dead, p.Colonists[0], last)
	}
	if len(p.Events) != 2 || p.Events[0].Text != "Death: Ozzie" || p.Events[0].Day != 39 || !p.Events[0].Death {
		t.Fatalf("events %+v", p.Events)
	}

	tl := p.Timeline
	if tl == nil || tl.Days != 60 || tl.Upto < 40 || tl.Upto > 40.1 || len(tl.Wealth) != view.TimelineValues || tl.Wealth[len(tl.Wealth)-1] != 120000 ||
		len(tl.People) != view.TimelineValues || !slices.Equal(tl.Deaths, []int{39}) {
		t.Fatalf("timeline %+v", tl)
	}
	// points oldest first, read saves only; a later save is off this save's history
	var pts []string
	for _, pt := range tl.Points {
		pts = append(pts, fmt.Sprintf("%s@%d:%d", pt.Name, pt.Day, pt.Wealth))
	}
	if !slices.Equal(pts, []string{"Save 3@20:59975", "Save 1@40:120000", "Save 0@60:-1"}) {
		t.Fatalf("points %q", pts)
	}

	// Without a previous read save: no changes; a lone save without history draws nothing.
	lone := colonyOf(5)
	p = view.SavePlayOf(lone.Saves[0], &savefile.Facts{TicksGame: 5 * savefile.TicksPerDay, Storyteller: "Cassandra", Difficulty: "Rough"}, nil, nil)
	if p.Since != "" || p.Timeline != nil || p.Storyteller != "Cassandra Classic" || p.Difficulty != "strive to survive" || p.Played != "" {
		t.Fatalf("lone save %+v", p)
	}
	for _, s := range p.Stats {
		if s.Change != "" {
			t.Fatalf("a change without a previous save: %+v", s)
		}
		if s.Label == "wealth" || s.Label == "prisoners" {
			t.Fatalf("a number the save does not hold: %+v", s)
		}
	}
}

// Left rows show the in-game day once a save is read, keeping the date for the hover.
func TestSaveDay(t *testing.T) {
	t.Parallel()
	c := colonyOf(214, -1)
	c.Saves[1].Summary = &app.SaveSummary{Malformed: true} // broke off before its time
	l := &app.SaveList{Colonies: []*app.Colony{c}}
	p := view.SavesRows(l)
	r := p.Colonies[0].Saves
	if r[0].When != "day 214" || r[1].When == "day 0" {
		t.Fatalf("rows %+v", r)
	}
}

// The date names the quadrum as the game's translation does (not parallel: SetLang is global).
func TestSavePlayDateZh(t *testing.T) {
	defer view.SetLang("en")
	c := colonyOf(25)
	f := &savefile.Facts{TicksGame: 25 * savefile.TicksPerDay}
	if p := view.SavePlayOf(c.Saves[0], f, nil, nil); p.Date != "11th of Jugust, 5500" {
		t.Fatalf("en date %q", p.Date)
	}
	view.SetLang("zh")
	if p := view.SavePlayOf(c.Saves[0], f, nil, nil); p.Date != "5500 年赫象 11 日" {
		t.Fatalf("zh date %q", p.Date)
	}
}
