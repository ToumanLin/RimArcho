package savefile

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
)

func readFixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func scanBytes(t testing.TB, b []byte) *Facts {
	t.Helper()
	f, err := Scan(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return f
}

func TestScanColony(t *testing.T) {
	f := scanBytes(t, readFixture(t, "colony.rws"))
	if f.Malformed != nil {
		t.Fatalf("malformed: %+v", f.Malformed)
	}
	if f.Parser != ParserVersion || f.Bytes != int64(len(readFixture(t, "colony.rws"))) {
		t.Errorf("parser %d, bytes %d", f.Parser, f.Bytes)
	}

	// Colony identity.
	if want := (World{Name: "Tundra", Seed: "hollow", PersistentRandomValue: -123456, Coverage: 0.3}); f.World != want {
		t.Errorf("world = %+v, want %+v", f.World, want)
	}
	if want := (Faction{LoadID: 1, Def: "PlayerColony", Name: "New Arrivals"}); f.Player != want {
		t.Errorf("player = %+v, want %+v", f.Player, want)
	}
	wantFactions := []Faction{{1, "PlayerColony", "New Arrivals"}, {3, "OutlanderCivil", "Kingdom of Ash"}, {0, "Ancients", "Ancients"}}
	if !reflect.DeepEqual(f.Factions, wantFactions) {
		t.Errorf("factions = %+v", f.Factions)
	}
	if f.Scenario != "Crashlanded" {
		t.Errorf("scenario = %q", f.Scenario)
	}

	// Time.
	if f.TicksGame != 25860000 || f.GameStartAbsTick != 300000 || f.Day() != 431 {
		t.Errorf("ticks %d start %d day %d", f.TicksGame, f.GameStartAbsTick, f.Day())
	}
	if y, q, d := f.Date(); y != 5507 || q != 1 || d != 2 {
		t.Errorf("date = %d %d %d, want 5507 1 (Jugust) 2", y, q, d)
	}
	if f.RealPlayTime != 12345.5 || f.Storyteller != "Cassandra" || f.Difficulty != "Rough" || !f.Permadeath {
		t.Errorf("play %v storyteller %q difficulty %q permadeath %v", f.RealPlayTime, f.Storyteller, f.Difficulty, f.Permadeath)
	}

	// People.
	wantMaps := []Map{{ID: 0, Parent: "WorldObject_4", Name: "Tundra Hollow", Faction: "Faction_1", Home: true}}
	if !reflect.DeepEqual(f.Maps, wantMaps) {
		t.Errorf("maps = %+v", f.Maps)
	}
	wantCol := []Pawn{
		{ID: "Human12", First: "Oswald", Nick: "Ozzie", Last: "Brown", Race: "Human", Kind: "Colonist", Dead: true, Where: WhereWorld, Map: -1},
		{ID: "Human13", First: "Pike", Nick: "Pike", Last: "Gray", Race: "Human", Kind: "Colonist", Where: WhereCaravan, Map: -1},
		{ID: "Human10", First: "Mara", Nick: "Mara", Last: "Lee", Race: "Human", Kind: "Colonist", Where: WhereMap, Map: 0},
		{ID: "Human14", First: "Tiny", Nick: "Bub", Last: "Lee", Race: "Human", Kind: "Colonist", Where: WhereMap, Map: 0},
		{ID: "Kurin_Race11", First: "Ka'ri", Nick: "Kari", Last: "Fox", Race: "Kurin_Race", Kind: "Kurin_Colonist", Slave: true, Where: WhereMap, Map: 0},
		{ID: "Human42", First: "Beatrice", Nick: "Bea", Last: "Dee", Race: "Human", Kind: "Colonist", Dead: true, Where: WhereMap, Map: 0},
	}
	if !reflect.DeepEqual(f.Colonists, wantCol) {
		got, _ := json.MarshalIndent(f.Colonists, "", " ")
		t.Errorf("colonists = %s", got)
	}
	if f.Alive() != 4 || f.Animals != 1 || f.Mechs != 1 || f.Prisoners != 1 {
		t.Errorf("alive %d animals %d mechs %d prisoners %d", f.Alive(), f.Animals, f.Mechs, f.Prisoners)
	}
	if f.Colonists[3].Display() != "Bub" {
		t.Errorf("display = %q", f.Colonists[3].Display())
	}

	// Stats.
	if want := (Stats{RaidsEnemy: 22, ThreatsQueued: 30, ColonistsKilled: 1, GreatestPopulation: 3}); f.Stats != want {
		t.Errorf("stats = %+v, want %+v", f.Stats, want)
	}
	wantRes := Research{Current: "MicroelectronicsBasics", Progress: map[string]float64{"Electricity": 1600, "Batteries": 400, "MicroelectronicsBasics": 600.5}}
	if !reflect.DeepEqual(f.Research, wantRes) {
		t.Errorf("research = %+v", f.Research)
	}
	wantHist := []Recorder{
		{Group: "Wealth", Def: "Wealth_Total", Count: 4, Last: 184000, Values: []float32{12000, 15000.5, 31000, 184000}},
		{Group: "Wealth", Def: "Wealth_Items", Count: 60, Last: 1590},
		{Group: "Population", Def: "FreeColonists", Count: 5, Last: 11, Values: []float32{3, 3, 4, 5, 11}},
	}
	if !reflect.DeepEqual(f.History, wantHist) {
		t.Errorf("history = %+v", f.History)
	}
	if f.Wealth() != 184000 {
		t.Errorf("wealth = %v", f.Wealth())
	}

	// References.
	if f.Defs["Human"] != 7 || f.Defs["Kurin_Shrine"] != 1 || f.Defs["Settlement"] != 2 || len(f.Defs) != 27 {
		t.Errorf("defs (%d) = %v", len(f.Defs), f.Defs)
	}
	if _, ok := f.Defs["null"]; ok {
		t.Error(`"null" counted as a def`)
	}
	if f.Classes["NameTriple"] != 7 || f.Classes["Pawn"] != 6 || f.Classes["Kurin.Building_Shrine"] != 1 || len(f.Classes) != 19 {
		t.Errorf("classes (%d) = %v", len(f.Classes), f.Classes)
	}
	wantRefs := Refs{Defined: 24, Checked: 13, Dangling: 2, Sample: []string{"Faction_7", "Thing_Human99"}, Unchecked: 1}
	if !reflect.DeepEqual(f.Refs, wantRefs) {
		t.Errorf("refs = %+v, want %+v", f.Refs, wantRefs)
	}

	// Events: letters only, newest first.
	wantEv := []Event{
		{Tick: 24120000, Def: "Death", Label: "Death: Ozzie", Class: "DeathLetter"},
		{Tick: 1200000, Def: "ThreatBig", Label: "Raid", Class: "StandardLetter"},
	}
	if !reflect.DeepEqual(f.Events, wantEv) {
		t.Errorf("events = %+v", f.Events)
	}
}

func TestScanJSONRoundTrip(t *testing.T) {
	f := scanBytes(t, readFixture(t, "colony.rws"))
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	var g Facts
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f, &g) {
		t.Errorf("round trip differs:\n%s", b)
	}
	b2, _ := json.Marshal(&g)
	if !bytes.Equal(b, b2) {
		t.Error("JSON is not deterministic")
	}
}

// Buffer refills between any two bytes must not change the result.
func TestScanSmallReads(t *testing.T) {
	b := readFixture(t, "colony.rws")
	want := scanBytes(t, b)
	for name, r := range map[string]io.Reader{
		"one byte": iotest.OneByteReader(bytes.NewReader(b)),
		"half":     iotest.HalfReader(bytes.NewReader(b)),
		"crlf":     bytes.NewReader(bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n"))),
		"bom":      io.MultiReader(bytes.NewReader([]byte{0xEF, 0xBB, 0xBF}), bytes.NewReader(b)),
	} {
		got, err := Scan(r)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got.Bytes = want.Bytes
		if got.Malformed != nil && name == "crlf" {
			// History series keep their values; only Malformed would differ.
			t.Fatalf("%s: malformed %+v", name, got.Malformed)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: facts differ", name)
		}
	}
}

// A save cut off anywhere before its root closes is malformed with EOF set, keeps the facts
// before the cut, and reports no references.
func TestScanTruncated(t *testing.T) {
	b := readFixture(t, "colony.rws")
	rootEnd := bytes.LastIndex(b, []byte("</savegame>")) + len("</savegame>")
	ticks := bytes.Index(b, []byte("</ticksGame>"))
	for cut := 0; cut < len(b); cut++ {
		f := scanBytes(t, b[:cut])
		if cut >= rootEnd {
			if f.Malformed != nil {
				t.Fatalf("cut %d after the root: %+v", cut, f.Malformed)
			}
			continue
		}
		if f.Malformed == nil || !f.Malformed.EOF {
			t.Fatalf("cut %d: malformed = %+v", cut, f.Malformed)
		}
		if f.Refs.Checked != 0 || f.Refs.Dangling != 0 {
			t.Fatalf("cut %d: refs reported for a cut-off file", cut)
		}
		if cut > ticks+len("</ticksGame>") && f.TicksGame != 25860000 {
			t.Fatalf("cut %d: lost ticksGame", cut)
		}
		if cut%97 == 0 && xmlWellFormed(b[:cut]) {
			t.Fatalf("cut %d: encoding/xml accepts it", cut)
		}
	}
	// A crash often leaves NUL bytes in place of the rest.
	crash := append(append([]byte{}, b[:len(b)/2]...), make([]byte, 4096)...)
	f := scanBytes(t, crash)
	if f.Malformed == nil || f.Malformed.EOF || f.Malformed.Msg != "NUL byte" || f.Malformed.Offset < int64(len(b)/2)-200 {
		t.Errorf("NUL tail: %+v", f.Malformed)
	}
}

func TestScanMalformed(t *testing.T) {
	const head = "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<savegame>\n<game>\n"
	bad := []struct {
		name, body, msg string
		line            int
	}{
		{"mismatched end tag", "<tickManager><ticksGame>5</ticksGam></tickManager>", "does not match <ticksGame>", 4},
		{"unquoted attribute", "<thing Class=Pawn><def>Human</def></thing>", "not quoted", 4},
		{"attribute without value", "<thing Class><def>Human</def></thing>", "has no value", 4},
		{"undeclared entity", "<name>Ka&rsquo;ri</name>", "undeclared entity", 4},
		{"bare ampersand", "<name>Salt & pepper</name>", "not followed by an entity", 4},
		{"lt in attribute", "<thing Class=\"a<b\"/>", "'<' in an attribute value", 4},
		{"invalid name", "<1thing/>", "invalid element name", 4},
		{"quote in a name", "<thing\"x\">a</thing\"x\">", "invalid element name", 4},
		{"NUL in text", "<name>a\x00b</name>", "NUL byte", 4},
		{"stray end tag", "</game></savegame></game>", "without a start tag", 4},
	}
	for _, c := range bad {
		doc := head + c.body + "\n</game>\n</savegame>\n"
		f := scanBytes(t, []byte(doc))
		if f.Malformed == nil {
			t.Errorf("%s: not reported", c.name)
			continue
		}
		if !strings.Contains(f.Malformed.Msg, c.msg) || f.Malformed.Line != c.line || f.Malformed.EOF {
			t.Errorf("%s: %+v, want %q on line %d", c.name, f.Malformed, c.msg, c.line)
		}
		if off := f.Malformed.Offset; off < int64(len(head)) || off > int64(len(head)+len(c.body)) {
			t.Errorf("%s: offset %d outside the bad part", c.name, off)
		}
		if xmlWellFormed([]byte(doc)) {
			t.Errorf("%s: encoding/xml accepts it, so the case is not malformed", c.name)
		}
	}
	whole := []struct{ name, doc, msg string }{
		{"text after the root", head + "</game></savegame>\njunk", "text outside the root element"},
		{"second root", head + "</game></savegame><savegame/>", "a second root element"},
		{"empty file", "", "no root element"},
		{"only a declaration", "<?xml version=\"1.0\"?>\n", "no root element"},
	}
	for _, c := range whole {
		f := scanBytes(t, []byte(c.doc))
		if f.Malformed == nil || !strings.Contains(f.Malformed.Msg, c.msg) {
			t.Errorf("%s: %+v, want %q", c.name, f.Malformed, c.msg)
		}
	}

	f := scanBytes(t, readFixture(t, "malformed.rws"))
	if f.Malformed == nil || f.Malformed.Line != 27 || !strings.Contains(f.Malformed.Msg, "</things> does not match <thing>") {
		t.Errorf("malformed.rws: %+v", f.Malformed)
	}
	// Facts before the error are kept.
	if f.TicksGame != 600000 || f.Player.Name != "Glacier" || len(f.Colonists) != 1 || !reflect.DeepEqual(f.Refs, Refs{}) {
		t.Errorf("malformed.rws facts: ticks %d player %+v colonists %d refs %+v", f.TicksGame, f.Player, len(f.Colonists), f.Refs)
	}
}

func TestScanWellFormedEdges(t *testing.T) {
	cases := []struct{ name, body string }{
		{"comment", "<!-- a > b - c --><tickManager><ticksGame>5<!-- x --></ticksGame></tickManager>"},
		{"CDATA", "<scenario><name><![CDATA[a & <b>]]></name></scenario>"},
		{"processing instruction", "<?pi data?><tickManager/>"},
		{"single quotes and > in a value", "<thing Class='Verse.Box`1[a>b]'><def>Human</def></thing>"},
		{"entities in Class", "<thing Class=\"A&amp;B\"/>"},
		{"character references", "<scenario><name>&#75;&#x61;</name></scenario>"},
		{"spaces in end tag", "<tickManager><ticksGame>5</ticksGame   ></tickManager >"},
		{"long text", "<grid><tileBiomeDeflate>" + strings.Repeat("QUJD", 1<<18) + "</tileBiomeDeflate></grid>"},
	}
	for _, c := range cases {
		doc := "<!DOCTYPE savegame>\n<savegame><game>" + c.body + "</game></savegame>\n<!-- after -->\n"
		f := scanBytes(t, []byte(doc))
		if f.Malformed != nil {
			t.Errorf("%s: %+v", c.name, f.Malformed)
		}
		if !xmlWellFormed([]byte(doc)) {
			t.Errorf("%s: encoding/xml rejects it", c.name)
		}
		if len(doc) > 4096 {
			continue
		}
		rootEnd := strings.Index(doc, "</savegame>") + len("</savegame>")
		for cut := 0; cut < rootEnd; cut++ {
			if f := scanBytes(t, []byte(doc[:cut])); f.Malformed == nil || !f.Malformed.EOF {
				t.Fatalf("%s cut at %d: %+v", c.name, cut, f.Malformed)
			}
		}
	}
	f := scanBytes(t, []byte("<savegame><game><scenario><name><![CDATA[a & <b>]]></name></scenario><tickManager><ticksGame>7<!-- c -->0</ticksGame></tickManager></game></savegame>"))
	if f.Scenario != "a & <b>" || f.TicksGame != 70 {
		t.Errorf("scenario %q ticks %d", f.Scenario, f.TicksGame)
	}
	f = scanBytes(t, []byte(`<savegame><game><x Class="A&amp;B"/></game></savegame>`))
	if f.Classes["A&B"] != 1 {
		t.Errorf("classes = %v", f.Classes)
	}
}

// The file name plays no part in L1: a RimSaves name reads like any other.
func TestScanRimSavesName(t *testing.T) {
	f := scanBytes(t, readFixture(t, "Ashfall#§#Landing.rws"))
	if f.Malformed != nil || f.World.Name != "Ashfall" || f.Player.Def != "PlayerTribe" || f.Day() != 2 {
		t.Errorf("facts: %+v", f)
	}
}

type errAfter struct {
	r io.Reader
	n int
}

func (e *errAfter) Read(p []byte) (int, error) {
	if e.n <= 0 {
		return 0, errors.New("disk on fire")
	}
	if len(p) > e.n {
		p = p[:e.n]
	}
	n, err := e.r.Read(p)
	e.n -= n
	return n, err
}

func TestScanReadError(t *testing.T) {
	b := readFixture(t, "colony.rws")
	cut := bytes.Index(b, []byte("<storyteller>"))
	f, err := Scan(&errAfter{r: bytes.NewReader(b), n: cut})
	if err == nil || err.Error() != "disk on fire" {
		t.Fatalf("err = %v", err)
	}
	if f == nil || f.TicksGame != 25860000 || f.Malformed != nil {
		t.Errorf("partial facts: %+v", f)
	}
}

// xmlWellFormed reports whether encoding/xml in strict mode reads doc without error.
func xmlWellFormed(doc []byte) bool {
	d := xml.NewDecoder(bytes.NewReader(doc))
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	depth, roots := 0, 0
	for {
		t, err := d.Token()
		if err == io.EOF {
			return depth == 0 && roots == 1
		}
		if err != nil {
			return false
		}
		switch t := t.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(t)) > 0 {
				return false
			}
		}
	}
}
