package view

import (
	"math"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/savefile"
)

// The Saves page's right card (06 §2.11 "Right"; 10 §4.3 Play facts, §4.5): the
// playthrough at the selected save from its full read (10 §1.7 L1), the change since the
// colony's previous read save, and the colony over time. Only what the facts hold is
// shown: the biome is not read, so the card has none.

// SavePlay is the playthrough at one save.
type SavePlay struct {
	Day         string `json:"day"`            // "day 214"
	Date        string `json:"date,omitempty"` // "5th of Aprimay, 5502", as the game writes it
	Storyteller string `json:"storyteller,omitempty"`
	Difficulty  string `json:"difficulty,omitempty"`
	Played      string `json:"played,omitempty"` // real play time
	// Since names the save the changes are against: the colony's previous read save
	// ("" when there is none, and the stats carry no change).
	Since     string         `json:"since,omitempty"`
	Stats     []SaveStat     `json:"stats"`
	Colonists []SaveColonist `json:"colonists"`
	More      int            `json:"more,omitempty"` // colonists not listed
	Dead      int            `json:"dead,omitempty"` // dead colonists, listed or not
	Events    []SaveEvent    `json:"events"`
	Timeline  *SaveTimeline  `json:"timeline,omitempty"`
	// Note says the facts are partial (a malformed file read up to where it broke off).
	Note string `json:"note,omitempty"`
}

// SaveStat is one number at this save and its change since the previous read save.
type SaveStat struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Change string `json:"change,omitempty"` // "+3", "−1.2k", "±0"; "" without a previous save
	Down   bool   `json:"down,omitempty"`   // the change is a loss
}

// SaveColonist is one colonist; the dead are struck through.
type SaveColonist struct {
	Name  string `json:"name"`
	Dead  bool   `json:"dead,omitempty"`
	Slave bool   `json:"slave,omitempty"`
}

// SaveEvent is one letter of the game's archive, newest first.
type SaveEvent struct {
	Day   int    `json:"day"`
	Text  string `json:"text"`
	Death bool   `json:"death,omitempty"`
}

// SaveTimeline is the colony over in-game days (10 §4.5). The lines are the selected
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

// Limits that keep the right card within the page's size budget (06 §10, §11 A18).
const (
	TimelineValues = 120 // values per line
	PlayColonists  = 40  // colonists listed
	PlayEvents     = 8   // letters listed
	TimelineDeaths = 60  // death marks
)

// PreviousReadSave is the save the right card's changes are against: the next older save
// of the colony (saves are newest first) that is read and well formed, or nil.
func PreviousReadSave(s *app.Save) *app.Save {
	if s.Colony == nil {
		return nil
	}
	past := false
	for _, o := range s.Colony.Saves {
		if o == s || o.Ref == s.Ref {
			past = true
			continue
		}
		if past && o.Summary != nil && !o.Summary.Malformed {
			return o
		}
	}
	return nil
}

// SavePlayOf is the right card of save s from its full read f, with the changes since the
// colony's previous read save (prevSave and its read prev, both nil when there is none).
func SavePlayOf(s *app.Save, f *savefile.Facts, prevSave *app.Save, prev *savefile.Facts) *SavePlay {
	p := &SavePlay{Day: Tf("day %d", f.Day()), Storyteller: storytellerText(f.Storyteller),
		Difficulty: difficultyText(f.Difficulty, f.Permadeath), Played: playedText(f.RealPlayTime),
		Stats: []SaveStat{}, Colonists: []SaveColonist{}, Events: []SaveEvent{}}
	if f.TicksGame > 0 || f.GameStartAbsTick > 0 {
		y, q, d := f.Date()
		p.Date = Tf("%[1]d%[2]s of %[3]s, %[4]d", d, ordinal(d), quadrumName(q), y)
	}
	if prev != nil && prevSave != nil {
		p.Since = prevSave.Name
	} else {
		prev = nil
	}
	p.Stats = playStats(f, prev)
	// the living first, then the dead; a quarter of the places stay for the dead
	var alive, dead []SaveColonist
	for _, c := range f.Colonists {
		v := SaveColonist{Name: c.Display(), Dead: c.Dead, Slave: c.Slave}
		if c.Dead {
			dead = append(dead, v)
		} else {
			alive = append(alive, v)
		}
	}
	nAlive := min(len(alive), PlayColonists-min(len(dead), PlayColonists/4))
	nDead := min(len(dead), PlayColonists-nAlive)
	p.Colonists = append(append(p.Colonists, alive[:nAlive]...), dead[:nDead]...)
	p.More, p.Dead = len(alive)+len(dead)-nAlive-nDead, len(dead)
	for _, e := range f.Events {
		if len(p.Events) == PlayEvents {
			break
		}
		if t := letterText(e.Label); t != "" {
			p.Events = append(p.Events, SaveEvent{Day: int(e.Tick / savefile.TicksPerDay), Text: t, Death: e.Def == "Death"})
		}
	}
	p.Timeline = saveTimeline(s, f)
	if m := f.Malformed; m != nil {
		p.Note = Tf("the file breaks off at line %d: these facts come from before it", m.Line)
	}
	return p
}

// playStats are the numbers at this save: wealth, colonists, animals, research points,
// raids, and prisoners and mechanoids when either save has any.
func playStats(f, prev *savefile.Facts) []SaveStat {
	out := []SaveStat{}
	add := func(label string, v, was float64, compact bool, has bool) {
		st := SaveStat{Label: label, Value: numText(v, compact)}
		if has {
			d := v - was
			switch {
			case math.Abs(d) < 0.5:
				st.Change = "±0"
			case d > 0:
				st.Change = "+" + numText(d, compact)
			default:
				st.Change, st.Down = "−"+numText(-d, compact), true
			}
		}
		out = append(out, st)
	}
	if r := f.Recorder("Wealth_Total"); r != nil && r.Count > 0 {
		was, has := 0.0, false
		if prev != nil {
			if pr := prev.Recorder("Wealth_Total"); pr != nil && pr.Count > 0 {
				was, has = float64(pr.Last), true
			}
		}
		add(T("wealth"), float64(r.Last), was, true, has)
	}
	type num struct {
		label   string
		get     func(*savefile.Facts) float64
		compact bool
		always  bool
	}
	for _, n := range []num{
		{T("colonists"), func(f *savefile.Facts) float64 { return float64(f.Alive()) }, false, true},
		{T("animals"), func(f *savefile.Facts) float64 { return float64(f.Animals) }, false, true},
		{T("research points"), func(f *savefile.Facts) float64 { return researchPoints(f) }, true, true},
		{T("raids"), func(f *savefile.Facts) float64 { return float64(f.Stats.RaidsEnemy) }, false, true},
		{T("prisoners"), func(f *savefile.Facts) float64 { return float64(f.Prisoners) }, false, false},
		{T("mechanoids"), func(f *savefile.Facts) float64 { return float64(f.Mechs) }, false, false},
	} {
		v, was := n.get(f), 0.0
		if prev != nil {
			was = n.get(prev)
		}
		if !n.always && v == 0 && was == 0 {
			continue
		}
		add(n.label, v, was, n.compact, prev != nil)
	}
	return out
}

// researchPoints is the research done: the points of every project, finished or not
// (whether a project is finished needs its cost from the Defs, 10 §1.7 L2).
func researchPoints(f *savefile.Facts) float64 {
	t := 0.0
	for _, v := range f.Research.Progress {
		t += v
	}
	return t
}

// numText is a count, or with compact a large number as 850, 12.4k, 1.2M.
func numText(v float64, compact bool) string {
	switch {
	case !compact || v < 1000:
		return strconv.FormatFloat(math.Round(v), 'f', -1, 64)
	case v < 10_000:
		return strconv.FormatFloat(math.Round(v/100)/10, 'f', -1, 64) + "k"
	case v < 1_000_000:
		return strconv.FormatFloat(math.Round(v/1000), 'f', -1, 64) + "k"
	}
	return strconv.FormatFloat(math.Round(v/100_000)/10, 'f', -1, 64) + "M"
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

// letterText is a letter's label without the game's rich-text tags.
func letterText(s string) string {
	if !strings.Contains(s, "<") {
		return strings.TrimSpace(s)
	}
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>' && in:
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
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
