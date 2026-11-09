package view

import (
	"math"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/savefile"
)

// The Saves page's right card header and the middle card's Over time (16 §2.11; 12 §4.3
// Play facts, §4.5), from the selected save's full read (12 §1.7 L1). Only what the facts
// hold is shown: the biome is not read, so there is none.

// SavePlay is the colony at one save: its header facts, the dead and the timeline.
type SavePlay struct {
	Day         string `json:"day"`            // "day 214"
	Date        string `json:"date,omitempty"` // "5th of Aprimay, 5502", as the game writes it
	Storyteller string `json:"storyteller,omitempty"`
	Difficulty  string `json:"difficulty,omitempty"`
	Played      string `json:"played,omitempty"` // real play time
	// Colonists are the dead the chart's legend names (16 §2.11 Over time).
	Colonists []SaveColonist `json:"colonists"`
	Dead      int            `json:"dead,omitempty"` // dead colonists, listed or not
	Timeline  *SaveTimeline  `json:"timeline,omitempty"`
	// Note says the facts are partial (a malformed file read up to where it broke off).
	Note string `json:"note,omitempty"`
}

// SaveColonist is one colonist the legend names.
type SaveColonist struct {
	Name string `json:"name"`
	Dead bool   `json:"dead,omitempty"`
}

// SaveTimeline is the colony over in-game days (12 §4.5). The lines are the selected
// save's own history (the game's Wealth_Total and FreeColonists records), spread evenly
// from day 0 to its day (Upto) and thinned to at most TimelineValues each; the points are
// the colony's read saves at their day, on the wealth line when it reaches their day.
type SaveTimeline struct {
	Days   int             `json:"days"`             // the axis: day 0 to the colony's last read day
	Upto   float64         `json:"upto"`             // the selected save's day, where the lines end
	Wealth []int           `json:"wealth,omitempty"` // total wealth
	People []int           `json:"people,omitempty"` // free colonists
	Deaths []int           `json:"deaths,omitempty"` // days of death letters
	Points []SaveTimepoint `json:"points"`
}

// SaveTimepoint is one read save of the colony; a click opens it.
type SaveTimepoint struct {
	Ref  string `json:"ref"`
	Name string `json:"name"`
	Day  int    `json:"day"`
	// Wealth is the wealth line's value at Day; -1 when the line does not reach it (a save
	// later than the selected one, or no wealth record).
	Wealth int  `json:"wealth"`
	Auto   bool `json:"auto,omitempty"`
}

// Limits that keep the page within its size budget (16 §10, §11 A18).
const (
	TimelineValues = 120 // values per line
	PlayDead       = 40  // dead colonists the legend names
	TimelineDeaths = 60  // death marks
)

// SavePlayOf is the right card's header and the middle card's Over time of save s, from
// its full read f.
func SavePlayOf(s *app.Save, f *savefile.Facts) *SavePlay {
	p := &SavePlay{Day: Tf("day %d", f.Day()), Storyteller: storytellerText(f.Storyteller),
		Difficulty: difficultyText(f.Difficulty, f.Permadeath), Played: playedText(f.RealPlayTime),
		Colonists: []SaveColonist{}}
	if f.TicksGame > 0 || f.GameStartAbsTick > 0 {
		y, q, d := f.Date()
		p.Date = Tf("%[1]d%[2]s of %[3]s, %[4]d", d, ordinal(d), quadrumName(q), y)
	}
	for _, c := range f.Colonists {
		if !c.Dead {
			continue
		}
		if p.Dead++; len(p.Colonists) < PlayDead {
			p.Colonists = append(p.Colonists, SaveColonist{Name: c.Display(), Dead: true})
		}
	}
	p.Timeline = saveTimeline(s, f)
	if m := f.Malformed; m != nil {
		p.Note = Tf("the file breaks off at line %d: these facts come from before it", m.Line)
	}
	return p
}

// playedText is real play time as saves show writes it: "3h 25m", "25m", "under a
// minute".
func playedText(sec float64) string {
	m := int(sec / 60)
	switch {
	case sec <= 0:
		return ""
	case m < 1:
		return T("under a minute")
	case m < 60:
		return Tf("%dm", m)
	}
	return Tf("%dh %02dm", m/60, m%60)
}

// ordinal is the English suffix of a day of the quadrum ("st", "nd", "rd", "th").
func ordinal(n int) string {
	if n%100 >= 11 && n%100 <= 13 {
		return "th"
	}
	switch n % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	}
	return "th"
}

// The vanilla storytellers' and difficulties' names as the game shows them; a Mod's are
// shown by their Def name.
func storytellerText(def string) string {
	switch def {
	case "Cassandra":
		return T("Cassandra Classic")
	case "Phoebe":
		return T("Phoebe Chillax")
	case "Randy":
		return T("Randy Random")
	}
	return def
}

func difficultyText(def string, permadeath bool) string {
	t := def
	switch def {
	case "Peaceful":
		t = T("peaceful")
	case "Easy":
		t = T("community builder")
	case "Medium":
		t = T("adventure story")
	case "Rough":
		t = T("strive to survive")
	case "Hard":
		t = T("blood and dust")
	case "Extreme":
		t = T("losing is fun")
	case "Custom":
		t = T("custom")
	}
	if permadeath && t != "" {
		t += "  " + T("commitment mode")
	}
	return t
}

// quadrumName is the game's name of a quadrum (0 Aprimay … 3 Decembary), in the
// interface language as the game's own translation names it (docs/glossary.md, Saves).
func quadrumName(q int) string {
	return T([...]string{"Aprimay", "Jugust", "Septober", "Decembary"}[q&3])
}

// saveTimeline is the colony over time, from f (the selected save s) and the summaries of
// the colony's other read saves; nil when there is nothing to draw (one point, no line).
func saveTimeline(s *app.Save, f *savefile.Facts) *SaveTimeline {
	upto := float64(f.TicksGame) / savefile.TicksPerDay
	tl := &SaveTimeline{Upto: math.Round(upto*100) / 100, Points: []SaveTimepoint{}, Days: f.Day()}
	var wealth []float32
	if r := f.Recorder("Wealth_Total"); r != nil && len(r.Values) > 1 {
		wealth = r.Values
		tl.Wealth = thin(wealth)
	}
	if r := f.Recorder("FreeColonists"); r != nil && len(r.Values) > 1 {
		tl.People = thin(r.Values)
	}
	for _, e := range f.Events {
		if e.Def == "Death" && len(tl.Deaths) < TimelineDeaths {
			tl.Deaths = append(tl.Deaths, int(e.Tick/savefile.TicksPerDay))
		}
	}
	at := func(day float64) int {
		if wealth == nil || day > upto+0.01 || upto <= 0 {
			return -1
		}
		x := day / upto * float64(len(wealth)-1)
		i := int(x)
		if i >= len(wealth)-1 {
			return int(math.Round(float64(wealth[len(wealth)-1])))
		}
		fr := float32(x - float64(i))
		return int(math.Round(float64(wealth[i] + (wealth[i+1]-wealth[i])*fr)))
	}
	if s.Colony != nil {
		for i := len(s.Colony.Saves) - 1; i >= 0; i-- { // oldest first
			o := s.Colony.Saves[i]
			day, ok := SaveDay(o)
			if o.Ref == s.Ref {
				day, ok = f.Day(), true
			} else if o.Summary == nil || o.Summary.Malformed {
				ok = false
			}
			if !ok {
				continue
			}
			d := float64(day)
			if o.Ref == s.Ref {
				d = upto
			}
			tl.Points = append(tl.Points, SaveTimepoint{Ref: o.Ref, Name: o.Name, Day: day, Wealth: at(d), Auto: o.Auto})
			tl.Days = max(tl.Days, day)
		}
	}
	if len(tl.Points) < 2 && tl.Wealth == nil && tl.People == nil {
		return nil
	}
	return tl
}

// thin keeps at most TimelineValues values of a series, evenly spaced, the last one kept.
func thin(v []float32) []int {
	n := len(v)
	m := min(n, TimelineValues)
	out := make([]int, m)
	for i := range m {
		j := i
		if m < n {
			j = int(math.Round(float64(i) * float64(n-1) / float64(m-1)))
		}
		out[i] = int(math.Round(float64(v[j])))
	}
	return out
}
