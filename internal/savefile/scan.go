package savefile

import (
	"io"
	"sort"
)

// ParserVersion is the version of Scan's output. Bump it whenever Facts changes meaning
// (a field added, removed or computed differently): caches of Facts key on it (12 §1.7).
const ParserVersion = 2

// MinParserVersion is the oldest version whose Facts mean what this version's do, except
// for fields added since (2 added DefRefs). Lists, grouping, play facts and Can it load use
// a row this old; a reader of a newer field reads the save again (12 §1.7). Raise it to
// ParserVersion when a change alters what an existing field means.
const MinParserVersion = 1

// Limits on the lists Facts keeps; counts beyond them are still exact.
const (
	MaxEvents   = 200 // Facts.Events
	MaxDangling = 50  // Facts.Refs.Sample
)

// KeptSeries names the history recorders whose whole series Facts keeps (the timeline of
// 12 §4.5); other recorders keep only their count and last value.
var KeptSeries = []string{"Wealth_Total", "FreeColonists"}

// Scan reads a whole save in one streaming pass (12 §1.7 L1) and returns what it found.
// Memory stays bounded by the save's distinct content (defs, IDs, pawns), not its size:
// large text such as compressed map grids is skipped without being buffered.
//
// The error is only for read errors from r; the facts gathered up to it are returned with
// it. Malformed XML is not an error: Scan records it in Facts.Malformed, stops reading
// there (as the game's XML loader does) and returns what it found before that point.
func Scan(r io.Reader) (*Facts, error) {
	s := newScanner(r)
	err := s.run()
	s.finish()
	return s.f, err
}

// Facts is everything one full read of a save yields. It depends only on the file, so it
// is cached as JSON (12 §1.7). Element paths and their sources are listed in
// docs/research/2026-11-04_save-format-fields.md. Unless noted, a field the save does
// not contain is left at its zero value; Scan applies the game's own defaults where
// Scribe omits values equal to them.
type Facts struct {
	Parser int   `json:"parser"` // ParserVersion that produced these facts
	Bytes  int64 `json:"bytes"`  // bytes read

	// Malformed is set when the XML is not well formed (including a file cut off before
	// its root element closes); reading stopped there. Nil means well formed.
	Malformed *Malformed `json:"malformed,omitempty"`

	// Colony identity (12 §4.1).
	World    World     `json:"world"`
	Player   Faction   `json:"player"`             // the player's faction; Def "" when not found
	Factions []Faction `json:"factions,omitempty"` // every faction, in save order
	Scenario string    `json:"scenario,omitempty"` // scenario name

	// Time.
	TicksGame        int64   `json:"ticksGame"`        // ticks since the game started
	GameStartAbsTick int64   `json:"gameStartAbsTick"` // absolute tick at the start (start season)
	RealPlayTime     float64 `json:"realPlayTime"`     // seconds of real time interacting with the game
	Storyteller      string  `json:"storyteller,omitempty"`
	Difficulty       string  `json:"difficulty,omitempty"`
	Permadeath       bool    `json:"permadeath,omitempty"`

	// People.
	Maps      []Map  `json:"maps,omitempty"`
	Colonists []Pawn `json:"colonists,omitempty"` // humanlike pawns of the player faction, dead included
	Animals   int    `json:"animals"`             // living animals of the player faction
	Mechs     int    `json:"mechs"`               // living player pawns neither humanlike nor animal (mechanoids, entities)
	Prisoners int    `json:"prisoners"`           // living prisoners held by the player faction

	// Stats.
	Stats    Stats      `json:"stats"`
	Research Research   `json:"research"`
	History  []Recorder `json:"history,omitempty"` // the game's history recorders (wealth, colonists, mood…)

	// References for the checks of 12 §4.3 and §4.6.
	Defs    map[string]int            `json:"defs,omitempty"`    // text of every <def> element → occurrences (12 §1.4)
	DefRefs map[string]map[string]int `json:"defRefs,omitempty"` // Def type (empty when unknown) → name → references, excluding configuration/help links
	Classes map[string]int            `json:"classes,omitempty"` // every Class="…" attribute value → occurrences
	Refs    Refs                      `json:"refs"`

	// Events are the letters in the game's archive, newest first, at most MaxEvents.
	Events []Event `json:"events,omitempty"`
}

// Malformed says where and why reading stopped.
type Malformed struct {
	Offset int64  `json:"offset"` // byte offset of the error in the file
	Line   int    `json:"line"`   // 1-based line of Offset
	Msg    string `json:"msg"`
	EOF    bool   `json:"eof,omitempty"` // the file ended early (cut off) rather than holding bad syntax
}

// World is game/world/info.
type World struct {
	Name                  string  `json:"name,omitempty"`
	Seed                  string  `json:"seed,omitempty"`
	PersistentRandomValue int     `json:"persistentRandomValue"`
	Coverage              float64 `json:"coverage,omitempty"` // planetCoverage
}

// Faction is one entry of game/world/factionManager/allFactions.
type Faction struct {
	LoadID int    `json:"loadID"`
	Def    string `json:"def"`
	Name   string `json:"name,omitempty"`
}

// Ref is the faction's reference string as other objects in the save write it.
func (f Faction) Ref() string { return "Faction_" + itoa(int64(f.LoadID)) }

// Map is one entry of game/maps.
type Map struct {
	ID      int    `json:"id"`                // uniqueID
	Parent  string `json:"parent,omitempty"`  // the world object the map belongs to (WorldObject_N)
	Name    string `json:"name,omitempty"`    // the parent settlement's name
	Faction string `json:"faction,omitempty"` // the parent's faction (Faction_N)
	Home    bool   `json:"home,omitempty"`    // the parent belongs to the player faction
	Pocket  bool   `json:"pocket,omitempty"`  // a pocket map (Anomaly, Odyssey)
}

// Where a pawn was found.
const (
	WhereMap     = "map"     // on a map, Pawn.Map says which (including inside corpses and containers there)
	WhereWorld   = "world"   // a world pawn (game/world/worldPawns)
	WhereCaravan = "caravan" // in a caravan on the world map
	WhereOther   = "other"   // elsewhere: travelling pods, quests, factions' lists
)

// Pawn is a humanlike pawn of the player faction.
type Pawn struct {
	ID    string `json:"id"`              // ThingID, e.g. Human1234 (referenced as Thing_Human1234)
	First string `json:"first,omitempty"` // NameTriple
	Nick  string `json:"nick,omitempty"`  // NameTriple
	Last  string `json:"last,omitempty"`  // NameTriple
	Name  string `json:"name,omitempty"`  // NameSingle
	Race  string `json:"race"`            // the pawn's ThingDef
	Kind  string `json:"kind,omitempty"`  // PawnKindDef
	Dead  bool   `json:"dead,omitempty"`
	Slave bool   `json:"slave,omitempty"`
	Where string `json:"where"`
	Map   int    `json:"map"` // Map.ID when Where is WhereMap, else -1
}

// Display is the name the game shows in lists: the nickname, else the single name, else
// the first and last name, else the ThingID.
func (p Pawn) Display() string {
	switch {
	case p.Nick != "":
		return p.Nick
	case p.Name != "":
		return p.Name
	case p.First != "" && p.Last != "":
		return p.First + " " + p.Last
	case p.First != "":
		return p.First
	case p.Last != "":
		return p.Last
	}
	return p.ID
}

// Stats is game/storyWatcher/statsRecord.
type Stats struct {
	RaidsEnemy         int `json:"raidsEnemy"`         // numRaidsEnemy
	ThreatsQueued      int `json:"threatsQueued"`      // numThreatsQueued (big threats)
	ColonistsKilled    int `json:"colonistsKilled"`    // colonistsKilled
	ColonistsLaunched  int `json:"colonistsLaunched"`  // colonistsLaunched
	GreatestPopulation int `json:"greatestPopulation"` // greatestPopulation (game default 3)
}

// Research is game/researchManager. Whether a project is finished needs its cost from the
// Def (progress ≥ cost), so that is left to the reader of these facts (12 §1.7 L2).
type Research struct {
	Current  string             `json:"current,omitempty"`  // currentProj
	Progress map[string]float64 `json:"progress,omitempty"` // project defName → research points
}

// Recorder is one of the game's history recorders (game/history/autoRecorderGroups):
// one value every recordTicksFrequency ticks of its HistoryAutoRecorderDef (30000, half
// a day, for the vanilla wealth and colonist recorders).
type Recorder struct {
	Group  string    `json:"group"`            // HistoryAutoRecorderGroupDef
	Def    string    `json:"def"`              // HistoryAutoRecorderDef
	Count  int       `json:"count"`            // number of values recorded
	Last   float32   `json:"last"`             // the newest value
	Values []float32 `json:"values,omitempty"` // the whole series, oldest first (KeptSeries only)
}

// Refs compares the object IDs the save references with the objects it defines. Only the
// reference kinds the scanner can match exactly are checked (Checked); references of
// other kinds are only counted (Unchecked). Left empty when the save is malformed.
type Refs struct {
	Defined   int      `json:"defined"`          // distinct objects of the checked kinds the save defines
	Checked   int      `json:"checked"`          // distinct references of the checked kinds
	Dangling  int      `json:"dangling"`         // of those, how many point to no object in the save
	Sample    []string `json:"sample,omitempty"` // dangling references, sorted, at most MaxDangling
	Unchecked int      `json:"unchecked"`        // distinct references of other kinds
}

// CheckedRefPrefixes are the reference kinds Refs checks.
var CheckedRefPrefixes = []string{"Thing_", "Faction_", "Map_", "WorldObject_", "Ideo_", "Quest_"}

// Event is one letter from the game's archive.
type Event struct {
	Tick  int64  `json:"tick"`            // arrivalTick, in TicksGame
	Def   string `json:"def,omitempty"`   // LetterDef, e.g. ThreatBig, Death, PositiveEvent
	Label string `json:"label,omitempty"` // as shown when it arrived, in the player's language then
	Class string `json:"class,omitempty"` // the letter's class, e.g. StandardLetter
}

// Ticks per in-game unit (GenDate).
const (
	TicksPerDay  = 60000
	TicksPerYear = 3600000
)

// Day is the number of whole in-game days since the game started.
func (f *Facts) Day() int { return int(f.TicksGame / TicksPerDay) }

// TicksAbs is the absolute tick of the save, from which the game derives the date.
func (f *Facts) TicksAbs() int64 { return f.GameStartAbsTick + f.TicksGame }

// Date is the in-game date: the year (5500 is the first), the quadrum (0 Aprimay,
// 1 Jugust, 2 Septober, 3 Decembary) and the 1-based day of the quadrum. The game shifts
// the date by the time zone of the viewed tile's longitude, which the save does not hold
// in readable form, so this may differ from the game's display by one day.
func (f *Facts) Date() (year, quadrum, day int) {
	abs := f.TicksAbs()
	year = 5500 + int(floorDiv(abs, TicksPerYear))
	doy := int(posMod(abs, TicksPerYear) / TicksPerDay) // 0..59
	return year, doy / 15, doy%15 + 1
}

// Recorder returns the history recorder with def, or nil.
func (f *Facts) Recorder(def string) *Recorder {
	for i := range f.History {
		if f.History[i].Def == def {
			return &f.History[i]
		}
	}
	return nil
}

// Wealth is the newest total wealth of the player's home maps (the Wealth_Total
// recorder), or 0 when the save has none.
func (f *Facts) Wealth() float32 {
	if r := f.Recorder("Wealth_Total"); r != nil {
		return r.Last
	}
	return 0
}

// Alive counts living colonists.
func (f *Facts) Alive() int {
	n := 0
	for _, p := range f.Colonists {
		if !p.Dead {
			n++
		}
	}
	return n
}

// SortedDefs returns the distinct <def> values, sorted.
func (f *Facts) SortedDefs() []string { return sortedKeys(f.Defs) }

// SortedClasses returns the distinct Class="…" values, sorted.
func (f *Facts) SortedClasses() []string { return sortedKeys(f.Classes) }

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func posMod(a, b int64) int64 {
	m := a % b
	if m < 0 {
		m += b
	}
	return m
}
