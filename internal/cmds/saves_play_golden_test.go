package cmds_test

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/savefile"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// played describes a played colony for testkit.SaveSpec.Body, written at the element paths
// internal/savefile reads (docs/research/2026-11-04_save-format-fields.md). The player
// faction is the SaveSpec's Faction (Faction_1); everything stands on map 0.
type played struct {
	seconds     float64 // real play time
	permadeath  bool
	storyteller string
	difficulty  string
	scenario    string
	stats       [5]int // raids, big threats, colonists killed, colonists launched, greatest population
	research    string
	wealth      []float32 // the Wealth_Total series, oldest first
	colonists   []colonist
	animals     int
	mechs       int
	prisoners   int
	letters     []letter // any order: the scanner sorts them newest first
}

type colonist struct {
	id, nick, first, last, race string
	dead, slave                 bool
}

type letter struct {
	day        int
	class, def string
	label      string
}

func (p played) body() string {
	var b strings.Builder
	fmt.Fprintf(&b, "<info><realPlayTimeInteracting>%g</realPlayTimeInteracting><permadeathMode>%v</permadeathMode></info>",
		p.seconds, map[bool]string{true: "True", false: "False"}[p.permadeath])
	fmt.Fprintf(&b, "<scenario><name>%s</name><playerFaction><def>PlayerFaction</def><factionDef>PlayerColony</factionDef></playerFaction></scenario>", p.scenario)
	fmt.Fprintf(&b, "<storyteller><def>%s</def><difficulty>%s</difficulty></storyteller>", p.storyteller, p.difficulty)
	s := p.stats
	fmt.Fprintf(&b, "<storyWatcher><statsRecord><numRaidsEnemy>%d</numRaidsEnemy><numThreatsQueued>%d</numThreatsQueued><colonistsKilled>%d</colonistsKilled>"+
		"<colonistsLaunched>%d</colonistsLaunched><greatestPopulation>%d</greatestPopulation></statsRecord></storyWatcher>", s[0], s[1], s[2], s[3], s[4])
	if p.research != "" {
		fmt.Fprintf(&b, "<researchManager><currentProj>%s</currentProj><progress><keys><li>%[1]s</li><li>Batteries</li></keys>"+
			"<values><li>400</li><li>1200</li></values></progress></researchManager>", p.research)
	}
	b.WriteString("<history><archive><archivables>")
	for i, l := range p.letters {
		fmt.Fprintf(&b, `<li Class="%s"><ID>%d</ID><def>%s</def><label>%s</label><arrivalTick>%d</arrivalTick></li>`,
			l.class, i+1, l.def, l.label, l.day*savefile.TicksPerDay+1000)
	}
	b.WriteString("</archivables></archive><autoRecorderGroups><li><def>Wealth</def><recorders><li><def>Wealth_Total</def><records>")
	raw := make([]byte, 4*len(p.wealth))
	for i, v := range p.wealth {
		binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(v))
	}
	b.WriteString(base64.StdEncoding.EncodeToString(raw))
	b.WriteString("</records></li></recorders></li></autoRecorderGroups></history>")

	b.WriteString("<maps><li><uniqueID>0</uniqueID><mapInfo><parent>WorldObject_4</parent></mapInfo><things>")
	for _, c := range p.colonists {
		race := c.race
		if race == "" {
			race = "Human"
		}
		pawn := fmt.Sprintf(`<def>%s</def><id>%s</id><faction>Faction_1</faction><kindDef>Colonist</kindDef>`+
			`<name Class="NameTriple"><first>%s</first><nick>%s</nick><last>%s</last></name><story><childhood>Urbworld</childhood></story>`,
			race, c.id, c.first, c.nick, c.last)
		switch {
		case c.dead:
			fmt.Fprintf(&b, `<thing Class="Corpse"><def>Corpse_%s</def><id>Corpse_%s</id><innerContainer><innerList><li>%s`+
				`<healthTracker><healthState>Dead</healthState></healthTracker></li></innerList></innerContainer></thing>`, race, c.id, pawn)
		case c.slave:
			fmt.Fprintf(&b, `<thing Class="Pawn">%s<guest><guestStatus>Slave</guestStatus></guest></thing>`, pawn)
		default:
			fmt.Fprintf(&b, `<thing Class="Pawn">%s</thing>`, pawn)
		}
	}
	for i := range p.animals {
		fmt.Fprintf(&b, `<thing Class="Pawn"><def>Husky</def><id>Husky%d</id><faction>Faction_1</faction><kindDef>Husky</kindDef>`+
			`<story IsNull="True" /><training><learned /></training></thing>`, 200+i)
	}
	for i := range p.mechs {
		fmt.Fprintf(&b, `<thing Class="Pawn"><def>Mech_Militor</def><id>Mech_Militor%d</id><faction>Faction_1</faction>`+
			`<kindDef>Mech_Militor</kindDef><story IsNull="True" /></thing>`, 300+i)
	}
	for i := range p.prisoners {
		fmt.Fprintf(&b, `<thing Class="Pawn"><def>Human</def><id>Human%d</id><faction>Faction_3</faction><kindDef>Villager</kindDef>`+
			`<guest><hostFaction>Faction_1</hostFaction><guestStatus>Prisoner</guestStatus></guest><story /></thing>`, 400+i)
	}
	b.WriteString("</things></li></maps>")
	return b.String()
}

// rimward is the colony of the play golden: Rimward on day 20, then Rimward 2 on day 25,
// after the player renamed the faction, with two colonists dead (one since Rimward), nine
// newcomers, more raids and wealth.
func rimward() (first, second played) {
	first = played{seconds: 2*3600 + 5*60 + 9, permadeath: true, storyteller: "Cassandra", difficulty: "Rough", scenario: "Crashlanded",
		stats: [5]int{20, 27, 1, 0, 4}, research: "Electricity", wealth: []float32{9000, 10500, 11145.5},
		colonists: []colonist{
			{id: "Human10", nick: "Mara", first: "Mara", last: "Lee"},
			{id: "Kurin_Race11", nick: "Kari", first: "Ka&apos;ri", last: "Fox", race: "Kurin_Race", slave: true},
			{id: "Human12", nick: "Ozzie", first: "Oswald", last: "Brown"},
			{id: "Human13", nick: "Bea", first: "Beatrice", last: "Dee", dead: true},
		},
		animals: 1, letters: []letter{
			{day: 3, class: "StandardLetter", def: "PositiveEvent", label: "Wanderer joins"},
			{day: 9, class: "StandardLetter", def: "ThreatBig", label: "Raid"},
			{day: 14, class: "DeathLetter", def: "Death", label: "Death: Bea"},
			{day: 19, class: "StandardLetter", def: "NegativeEvent", label: "Cold snap"},
		}}
	second = first
	second.seconds, second.stats, second.research = 3*3600+25*60, [5]int{22, 30, 2, 0, 12}, "MicroelectronicsBasics"
	second.wealth = append(append([]float32{}, first.wealth...), 11900, 12345.4)
	second.colonists = append([]colonist{}, first.colonists...)
	second.colonists[2].dead = true
	for i, n := range []string{"Pike", "Rosa", "Tomas", "Ivy", "Hugo", "Nell", "Otto", "Wren", "Yara"} {
		second.colonists = append(second.colonists, colonist{id: fmt.Sprintf("Human%d", 20+i), nick: n, first: n, last: "Smith"})
	}
	second.animals, second.mechs, second.prisoners = 2, 1, 1
	second.letters = append(append([]letter{}, first.letters...),
		letter{day: 21, class: "StandardLetter", def: "ThreatBig", label: "Raid"},
		letter{day: 22, class: "StandardLetter", def: "PositiveEvent", label: "Refugee chased"},
		letter{day: 23, class: "DeathLetter", def: "Death", label: "Death: Ozzie"},
		letter{day: 24, class: "StandardLetter", def: "ThreatBig", label: "Siege"})
	return first, second
}

// playFixture: one game with two saves of colony Rimward (one world), a malformed save of
// another world, and an autosave being written.
func playFixture(t *testing.T) *testkit.Env {
	e := testkit.New(t)
	e.AddWorkshop("3000000002", testkit.Sample("tester.alpha", "Alpha"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.alpha")
	e.AddEnv(false)
	e.MustRun("scan")
	now := e.Now()
	mods := []string{"ludeon.rimworld", "tester.alpha"}
	first, second := rimward()
	world := testkit.SaveSpec{Mods: mods, World: "Ennawrin", Seed: "hollow", PRV: -7}
	s := world
	s.Faction, s.Ticks, s.Body, s.ModTime = "New Arrivals", 20*savefile.TicksPerDay+1234, first.body(), now.Add(-5*time.Hour)
	e.WriteSave("Rimward.rws", s)
	s = world
	s.Faction, s.Ticks, s.Body, s.ModTime = "Iron Pact", 25*savefile.TicksPerDay+40000, second.body(), now.Add(-4*time.Hour)
	e.WriteSave("Rimward 2.rws", s)
	// The game was writing this one a moment ago.
	s.ModTime = now.Add(-2 * time.Second)
	e.WriteSave("Autosave-1.rws", s)
	// Bad XML in the middle of the map: reading stops there.
	e.WriteSave("Glacier.rws", testkit.SaveSpec{Mods: mods, World: "Rime", Seed: "glacier", PRV: 3, Faction: "Glacier Folk",
		Ticks: 10 * savefile.TicksPerDay, ModTime: now.Add(-6 * time.Hour),
		Body: `<maps><li><uniqueID>0</uniqueID><things><thing Class="Building_Door"><def>Door</def><id>Door2</id></things></li></maps>`})
	return e
}

// archo saves with the full read (05 §4.2, 10 §1.7): lists before and after archo saves
// index, one save read inline (its colony regrouped by world), the playthrough and its
// change since the colony's previous save, --play and --all, a malformed save, a save being
// written, a new save that is meta only, and saves index: first run, second run, --only,
// --progress and an ambiguous reference. Not parallel: --progress writes to cmds.Progress.
func TestGoldenSavesPlay(t *testing.T) {
	e := playFixture(t)
	runSavesGolden(t, "saves_play", e, []step{
		cmd("saves", "list"),
		cmd("saves", "show", "Rimward 2"),
		cmd("saves", "index", "--progress"),
		cmd("saves", "index"),
		cmd("saves", "list"),
		cmd("saves", "show", "Rimward 2"),
		cmd("saves", "show", "Rimward 2", "--play"),
		cmd("saves", "show", "Rimward 2", "--play", "--all"),
		cmd("saves", "show", "Rimward", "--play"),
		cmd("saves", "show", "Glacier"),
		cmd("saves", "show", "Autosave-1"),
		act(func(e *testkit.Env) {
			e.WriteSave("Outpost.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}, ModTime: e.Now().Add(-time.Hour)})
			e.WriteFile(filepath.Join(e.Home, "packs", "P1", "pack.json"), `{"format":1,"name":"P1","entries":[]}`)
			p := filepath.Join(e.Home, "packs", "P1", "userdata", "Saves", "Rimward.rws")
			e.WriteFile(p, testkit.SaveXML(testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}}))
			at := e.Now().Add(-7 * time.Hour)
			os.Chtimes(p, at, at)
		}),
		cmd("saves", "list"),
		cmd("saves", "index", "Outpost", "--only", "--progress"),
		cmd("saves", "index", "Rimward"),
		cmd("saves", "index", "--only"),
		cmd("saves", "list", "--colony", "Iron Pact"),
	})
}
