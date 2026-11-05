package cmds

import (
	"fmt"
	"math"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/patterns"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/savefile"
)

// The playthrough of saves show (10 §4.3 "Play facts", 06 §2.11 "Right"), from the full
// read (10 §1.7 L1): a short summary by default, the colonists, events and research with
// --play. Changes are against the colony's previous save when its full read is cached.

// Rows --play shows before "… N more" (05 §3.1 rule 5: the whole call stays within 40 lines).
const (
	playColonists = 10
	playEvents    = 6
	summaryEvents = 3 // newest events on the default view's one line
)

// readText is the read: line of saves show: what the full read found.
func readText(f *savefile.Facts, s *app.Save, err error) string {
	switch {
	case err != nil:
		return "failed: " + err.Error()
	case f == nil && s.Writing:
		return "not yet: the file is being written"
	case f == nil:
		return metaOnly
	case f.Malformed != nil:
		m := f.Malformed
		return fmt.Sprintf("stopped at line %d, byte %s: %s (the facts below are from before that point)",
			m.Line, patterns.Thousands(int(m.Offset)), m.Msg)
	}
	return "in full"
}

// quadrums are the game's quadrum names (GenDate), Aprimay first.
var quadrums = [4]string{"Aprimay", "Jugust", "Septober", "Decembary"}

// dateText is the in-game date as the game writes it: "11th of Jugust, 5500". It may be a
// day off the game's own display (savefile.Facts.Date).
func dateText(f *savefile.Facts) string {
	y, q, d := f.Date()
	return fmt.Sprintf("%d%s of %s, %d", d, ordinal(d), quadrums[q&3], y)
}

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

// playedText is real play time: "3h 25m", "25m", "under a minute".
func playedText(sec float64) string {
	m := int(sec / 60)
	switch {
	case m < 1:
		return "under a minute"
	case m < 60:
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh %02dm", m/60, m%60)
}

// playCounts are the numbers the change line compares.
type playCounts struct {
	alive, slaves, dead, animals, mechs, prisoners int
	wealth                                         int
	hasWealth, hasStats                            bool
	stats                                          savefile.Stats
}

func countsOf(f *savefile.Facts) playCounts {
	c := playCounts{animals: f.Animals, mechs: f.Mechs, prisoners: f.Prisoners, stats: f.Stats,
		hasStats: f.Stats != savefile.Stats{}}
	for _, p := range f.Colonists {
		switch {
		case p.Dead:
			c.dead++
		case p.Slave:
			c.alive++
			c.slaves++
		default:
			c.alive++
		}
	}
	if f.Recorder("Wealth_Total") != nil {
		c.hasWealth, c.wealth = true, int(math.Round(float64(f.Wealth())))
	}
	return c
}

// peopleText is the colonists: line: "3 (1 slave), 2 dead; animals 2, mechanoids 1, prisoners 1".
// Without the player faction nothing in the save can be counted as the player's.
func peopleText(f *savefile.Facts, c playCounts) string {
	if f.Player.Def == "" {
		return "unknown: no player faction found in the save"
	}
	s := fmt.Sprint(c.alive)
	if c.slaves > 0 {
		s += fmt.Sprintf(" (%s)", render.Plural(c.slaves, "slave"))
	}
	if c.dead > 0 {
		s += fmt.Sprintf(", %d dead", c.dead)
	}
	return s + fmt.Sprintf("; animals %d, mechanoids %d, prisoners %d", c.animals, c.mechs, c.prisoners)
}

// statsText is the stats: line: the wealth, then every statsRecord number.
func statsText(c playCounts) string {
	var parts []string
	if c.hasWealth {
		parts = append(parts, "wealth "+patterns.Thousands(c.wealth))
	}
	if c.hasStats {
		st := c.stats
		parts = append(parts, fmt.Sprintf("raids %d, big threats %d, colonists killed %d, colonists launched %d, greatest population %d",
			st.RaidsEnemy, st.ThreatsQueued, st.ColonistsKilled, st.ColonistsLaunched, st.GreatestPopulation))
	}
	return strings.Join(parts, ", ")
}

// changeText lists what differs from the previous save's numbers, only the changed ones.
func changeText(now, was playCounts) string {
	var parts []string
	add := func(name string, a, b int) {
		if d := a - b; d != 0 {
			parts = append(parts, fmt.Sprintf("%s %s", name, signed(d)))
		}
	}
	add("colonists", now.alive, was.alive)
	add("slaves", now.slaves, was.slaves)
	add("dead", now.dead, was.dead)
	add("animals", now.animals, was.animals)
	add("mechanoids", now.mechs, was.mechs)
	add("prisoners", now.prisoners, was.prisoners)
	if now.hasWealth && was.hasWealth {
		add("wealth", now.wealth, was.wealth)
	}
	if now.hasStats && was.hasStats {
		a, b := now.stats, was.stats
		add("raids", a.RaidsEnemy, b.RaidsEnemy)
		add("big threats", a.ThreatsQueued, b.ThreatsQueued)
		add("colonists killed", a.ColonistsKilled, b.ColonistsKilled)
		add("colonists launched", a.ColonistsLaunched, b.ColonistsLaunched)
		add("greatest population", a.GreatestPopulation, b.GreatestPopulation)
	}
	if len(parts) == 0 {
		return "no change"
	}
	return strings.Join(parts, ", ")
}

func signed(n int) string {
	if n > 0 {
		return "+" + patterns.Thousands(n)
	}
	return "-" + patterns.Thousands(-n)
}

// previousSave is the colony's save just older than s (nil for its oldest).
func previousSave(s *app.Save) *app.Save {
	if s.Colony == nil {
		return nil
	}
	for i, o := range s.Colony.Saves {
		if o == s && i+1 < len(s.Colony.Saves) {
			return s.Colony.Saves[i+1]
		}
	}
	return nil
}

// playPrev is the previous save and its cached facts, for the change line.
type playPrev struct {
	save  *app.Save
	facts *savefile.Facts // nil: not read in full yet
}

// sinceLine is the change line's key and value, "" when the colony has no previous save.
func sinceLine(p *playPrev, now playCounts) (string, string) {
	if p == nil || p.save == nil {
		return "", ""
	}
	key := "since " + p.save.Name + " " + p.save.Ref
	switch f := p.facts; {
	case f == nil:
		return key, "not compared: that save is not read in full yet (archo saves index)"
	case f.Malformed != nil:
		return key, "not compared: reading that save stopped early (malformed)"
	default:
		return key + fmt.Sprintf(" (day %d)", f.Day()), changeText(now, countsOf(f))
	}
}

// eventText is one letter: "day 402 Death: Ozzie".
func eventText(e savefile.Event) string {
	return fmt.Sprintf("day %d %s", e.Tick/savefile.TicksPerDay, oneLine(e.Label))
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// colonistWhere is where a living colonist is: its map's settlement, a caravan, the world
// pawns or elsewhere (travelling, a quest).
func colonistWhere(f *savefile.Facts, p savefile.Pawn) string {
	switch p.Where {
	case savefile.WhereMap:
		for _, m := range f.Maps {
			if m.ID == p.Map && m.Name != "" {
				return "map " + m.Name
			}
		}
		return "map"
	case savefile.WhereCaravan:
		return "caravan"
	case savefile.WhereWorld:
		return "world"
	}
	return "away"
}

// addPlay writes the playthrough under root: a summary, or with full every colonist and
// event (limit rows each; 0: all).
func addPlay(root *render.Node, s *app.Save, f *savefile.Facts, prev *playPrev, full bool, limit int) {
	head := ""
	if f.Player.Name != "" {
		head = f.Player.Name + ", "
	}
	head += fmt.Sprintf("day %d (%s)", f.Day(), dateText(f))
	if f.RealPlayTime > 0 {
		head += ", " + playedText(f.RealPlayTime) + " played"
	}
	n := root.KV("play", head)
	if full {
		w := f.World.Name
		if f.Scenario != "" {
			w = strings.TrimPrefix(w+", scenario "+f.Scenario, ", ")
		}
		n.KV("world", w)
	}
	var story []string
	if f.Storyteller != "" {
		story = append(story, f.Storyteller)
	}
	if f.Difficulty != "" {
		story = append(story, "difficulty "+f.Difficulty)
	}
	if f.Permadeath {
		story = append(story, "permadeath")
	}
	n.KV("storyteller", strings.Join(story, ", "))
	now := countsOf(f)
	if !full {
		n.KV("colonists", peopleText(f, now))
	}
	n.KV("stats", statsText(now))
	if k, v := sinceLine(prev, now); k != "" {
		n.KV(k, v)
	}
	if !full {
		if len(f.Events) > 0 {
			var es []string
			for _, e := range f.Events[:min(summaryEvents, len(f.Events))] {
				es = append(es, eventText(e))
			}
			line := strings.Join(es, "; ")
			if more := len(f.Events) - summaryEvents; more > 0 {
				line += fmt.Sprintf("; … %d more", more)
			}
			n.KV("events", line)
		}
		if len(f.Colonists) > 0 || len(f.Events) > 0 {
			n.Add("(every colonist and event: archo saves show " + s.Ref + " --play)")
		}
		return
	}

	if r := f.Research; r.Current != "" || len(r.Progress) > 0 {
		line := r.Current
		if line == "" {
			line = "nothing"
		}
		n.KV("research", fmt.Sprintf("%s now; %s with progress", line, render.Plural(len(r.Progress), "project")))
	}
	expand := "archo saves show " + s.Ref + " --play --all"
	cn := n.KV("colonists", peopleText(f, now))
	var was map[string]savefile.Pawn
	if prev != nil && prev.facts != nil && prev.facts.Malformed == nil {
		was = map[string]savefile.Pawn{}
		for _, p := range prev.facts.Colonists {
			was[p.ID] = p
		}
	}
	var rows [][]string
	for _, p := range f.Colonists {
		state := "dead"
		if !p.Dead {
			state = colonistWhere(f, p)
			if p.Slave {
				state += ", slave"
			}
		}
		mark := ""
		if was != nil {
			switch o, ok := was[p.ID]; {
			case !ok:
				mark = "new"
			case p.Dead && !o.Dead:
				mark = "died"
			}
		}
		rows = append(rows, []string{p.Display(), p.Race, state, mark})
	}
	cn.AddColumns(rows)
	cn.Truncate(min(limit, playColonists), expand)
	if len(f.Events) > 0 {
		what := render.Plural(len(f.Events), "letter")
		if len(f.Events) == savefile.MaxEvents {
			what = fmt.Sprintf("the newest %d letters", savefile.MaxEvents)
		}
		en := n.KV("events", what+", newest first")
		rows = rows[:0]
		for _, e := range f.Events {
			rows = append(rows, []string{fmt.Sprintf("day %d", e.Tick/savefile.TicksPerDay), oneLine(e.Label), e.Def})
		}
		en.AddColumns(rows)
		en.Truncate(min(limit, playEvents), expand)
	}
}
