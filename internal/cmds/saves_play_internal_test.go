package cmds

import (
	"errors"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/savefile"
)

// The playthrough's words (12 §4.3 Play facts): the date as the game writes it, real play
// time, where a colonist is, the change line and the read line.
func TestSavePlayText(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		ticks, start int64
		want         string
	}{
		{0, 0, "1st of Aprimay, 5500"},
		{60000, 0, "2nd of Aprimay, 5500"},
		{2 * 60000, 0, "3rd of Aprimay, 5500"},
		{10 * 60000, 0, "11th of Aprimay, 5500"},
		{20 * 60000, 300000, "11th of Jugust, 5500"},
		{59 * 60000, 0, "15th of Decembary, 5500"},
		{60 * 60000, 0, "1st of Aprimay, 5501"},
	} {
		if got := dateText(&savefile.Facts{TicksGame: c.ticks, GameStartAbsTick: c.start}); got != c.want {
			t.Errorf("ticks %d start %d: %q, want %q", c.ticks, c.start, got, c.want)
		}
	}
	for sec, want := range map[float64]string{0: "under a minute", 59: "under a minute", 60: "1m", 3599: "59m", 3600: "1h 00m", 12345.5: "3h 25m", 100 * 3600: "100h 00m"} {
		if got := playedText(sec); got != want {
			t.Errorf("playedText(%g) = %q, want %q", sec, got, want)
		}
	}

	f := &savefile.Facts{Maps: []savefile.Map{{ID: 0, Name: "Tundra Hollow"}, {ID: 3}}}
	for _, c := range []struct {
		p    savefile.Pawn
		want string
	}{
		{savefile.Pawn{Where: savefile.WhereMap, Map: 0}, "map Tundra Hollow"},
		{savefile.Pawn{Where: savefile.WhereMap, Map: 3}, "map"},
		{savefile.Pawn{Where: savefile.WhereCaravan, Map: -1}, "caravan"},
		{savefile.Pawn{Where: savefile.WhereWorld, Map: -1}, "world"},
		{savefile.Pawn{Where: savefile.WhereOther, Map: -1}, "away"},
	} {
		if got := colonistWhere(f, c.p); got != c.want {
			t.Errorf("%+v: %q, want %q", c.p, got, c.want)
		}
	}

	same := playCounts{alive: 3, hasWealth: true, wealth: 100}
	if got := changeText(same, same); got != "no change" {
		t.Errorf("no change: %q", got)
	}
	// Wealth compares only when both saves record it; a loss is negative.
	if got := changeText(playCounts{alive: 2, dead: 1, wealth: 90, hasWealth: true}, playCounts{alive: 3, wealth: 2000}); got != "colonists -1, dead +1" {
		t.Errorf("change: %q", got)
	}
	if got := changeText(playCounts{wealth: 900, hasWealth: true}, playCounts{wealth: 2000, hasWealth: true}); got != "wealth -1,100" {
		t.Errorf("wealth change: %q", got)
	}

	s := &app.Save{}
	if got := readText(nil, s, errors.New("access denied")); got != "failed: access denied" {
		t.Errorf("read error: %q", got)
	}
	s.Writing = true
	if got := readText(nil, s, nil); got != "not yet: the file is being written" {
		t.Errorf("writing: %q", got)
	}
	m := &savefile.Facts{Malformed: &savefile.Malformed{Offset: 12345, Line: 40, Msg: "the file ends inside <maps> (4 elements open)", EOF: true}}
	if got := readText(m, s, nil); got != "stopped at line 40, byte 12,345: the file ends inside <maps> (4 elements open) (the facts below are from before that point)" {
		t.Errorf("malformed: %q", got)
	}
}
