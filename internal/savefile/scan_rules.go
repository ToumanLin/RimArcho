package savefile

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/binary"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
)

// What the scanner extracts, by element path. Paths come from the game's ExposeData
// methods (RimWorld 1.6); docs/research/2026-11-04_save-format-fields.md lists each one
// with its source. Scribe omits a value equal to its default, so every default below is
// the game's.

// Where a pawn under an element is (frame.ctx).
const (
	ctxOther uint8 = iota
	ctxMap
	ctxWorld
	ctxCaravan
)

type act uint8

const (
	aNone act = iota

	// Leaf values.
	aRealPlayTime
	aPermadeath
	aScenarioName
	aPlayerFactionDef
	aTicksGame
	aGameStartAbsTick
	aStatRaids
	aStatThreats
	aStatKilled
	aStatLaunched
	aStatGreatestPop
	aResearchCurrent
	aResearchKey
	aResearchValue
	aStoryteller
	aDifficulty
	aGroupDef
	aRecorderDef
	aRecords
	aRecordsDeflate
	aLetterDef
	aLetterLabel
	aLetterTick
	aWorldName
	aWorldSeed
	aWorldPRV
	aWorldCoverage
	aFactionDef
	aFactionName
	aFactionLoadID
	aWOID
	aWOFaction
	aWOName
	aMapID
	aMapParent
	aMapPocket
	aPawnFirst
	aPawnNick
	aPawnLast
	aPawnSingle
	aPawnHealth
	aPawnGuest
	aPawnHost

	// Elements, acted on when they open and close.
	aStatsRecord
	aGroup
	aRecorder
	aLetter
	aFactionLi
	aWorldObject
	aMapLi
	aIdeoLi
	aQuestLi
	aWorldPawns
	aPawnStory
	aPawnTraining
)

const (
	g      = "savegame/game/"
	world  = g + "world/"
	recPth = g + "history/autoRecorderGroups/li/recorders/li"
)

// pathRules are absolute element paths.
var pathRules = []struct {
	path string
	act  act
}{
	{g + "info/realPlayTimeInteracting", aRealPlayTime},
	{g + "info/permadeathMode", aPermadeath},
	{g + "scenario/name", aScenarioName},
	{g + "scenario/playerFaction/factionDef", aPlayerFactionDef},
	{g + "tickManager/ticksGame", aTicksGame},
	{g + "tickManager/gameStartAbsTick", aGameStartAbsTick},
	{g + "storyWatcher/statsRecord", aStatsRecord},
	{g + "storyWatcher/statsRecord/numRaidsEnemy", aStatRaids},
	{g + "storyWatcher/statsRecord/numThreatsQueued", aStatThreats},
	{g + "storyWatcher/statsRecord/colonistsKilled", aStatKilled},
	{g + "storyWatcher/statsRecord/colonistsLaunched", aStatLaunched},
	{g + "storyWatcher/statsRecord/greatestPopulation", aStatGreatestPop},
	{g + "researchManager/currentProj", aResearchCurrent},
	{g + "researchManager/progress/keys/li", aResearchKey},
	{g + "researchManager/progress/values/li", aResearchValue},
	{g + "storyteller/def", aStoryteller},
	{g + "storyteller/difficulty", aDifficulty},
	{g + "history/autoRecorderGroups/li", aGroup},
	{g + "history/autoRecorderGroups/li/def", aGroupDef},
	{recPth, aRecorder},
	{recPth + "/def", aRecorderDef},
	{recPth + "/records", aRecords},
	{recPth + "/recordsDeflate", aRecordsDeflate},
	{g + "history/archive/archivables/li", aLetter},
	{g + "history/archive/archivables/li/def", aLetterDef},
	{g + "history/archive/archivables/li/label", aLetterLabel},
	{g + "history/archive/archivables/li/arrivalTick", aLetterTick},
	{world + "info/name", aWorldName},
	{world + "info/seedString", aWorldSeed},
	{world + "info/persistentRandomValue", aWorldPRV},
	{world + "info/planetCoverage", aWorldCoverage},
	{world + "factionManager/allFactions/li", aFactionLi},
	{world + "factionManager/allFactions/li/def", aFactionDef},
	{world + "factionManager/allFactions/li/name", aFactionName},
	{world + "factionManager/allFactions/li/loadID", aFactionLoadID},
	{world + "ideoManager/ideos/li", aIdeoLi},
	{world + "worldPawns", aWorldPawns},
	{g + "questManager/quests/li", aQuestLi},
	{g + "maps/li", aMapLi},
	{g + "maps/li/uniqueID", aMapID},
	{g + "maps/li/mapInfo/parent", aMapParent},
	{g + "maps/li/mapInfo/isPocketMap", aMapPocket},
}

// World objects live in two lists with the same element shape.
var worldObjectLists = []string{world + "worldObjects/worldObjects", world + "pocketMaps"}

var worldObjectRules = []struct {
	path string
	act  act
}{
	{"", aWorldObject},
	{"/ID", aWOID},
	{"/faction", aWOFaction},
	{"/nameInt", aWOName},
}

// pawnRules are relative to a pawn element (one with direct <def>, <id> and <kindDef>).
var pawnRules = []struct {
	path string
	act  act
}{
	{"name/first", aPawnFirst},
	{"name/nick", aPawnNick},
	{"name/last", aPawnLast},
	{"name/name", aPawnSingle},
	{"healthTracker/healthState", aPawnHealth},
	{"guest/guestStatus", aPawnGuest},
	{"guest/hostFaction", aPawnHost},
	{"story", aPawnStory},
	{"training", aPawnTraining},
}

// Element names with fixed IDs in every scanner.
const (
	nDef int32 = iota
	nID
	nLoadID
	nIDUpper
	nUniqueID
	nUniqueIdLower
	nFaction
	nKindDef
)

// knownNames gives every name in a rule a fixed ID; init appends the rule names.
var knownNames = []string{
	nDef: "def", nID: "id", nLoadID: "loadID", nIDUpper: "ID", nUniqueID: "uniqueID",
	nUniqueIdLower: "uniqueId", nFaction: "faction", nKindDef: "kindDef",
}

type ruleKid struct {
	name, node int32
}

type ruleNode struct {
	kids    []ruleKid
	act     act
	capture bool // keep the element's whole text
}

var (
	ruleNodes []ruleNode
	rootNode  int32
	pawnRoot  int32
)

func init() {
	ids := map[string]int32{}
	for i, n := range knownNames {
		ids[n] = int32(i)
	}
	nameID := func(n string) int32 {
		if id, ok := ids[n]; ok {
			return id
		}
		id := int32(len(knownNames))
		knownNames = append(knownNames, n)
		ids[n] = id
		return id
	}
	newNode := func() int32 {
		ruleNodes = append(ruleNodes, ruleNode{})
		return int32(len(ruleNodes) - 1)
	}
	add := func(root int32, path string, a act) {
		n := root
		for _, seg := range strings.Split(path, "/") {
			if seg == "" {
				continue
			}
			id := nameID(seg)
			next := childNode(n, id)
			if next < 0 {
				next = newNode()
				ruleNodes[n].kids = append(ruleNodes[n].kids, ruleKid{id, next})
			}
			n = next
		}
		if ruleNodes[n].act != aNone {
			panic("savefile: two rules for " + path)
		}
		ruleNodes[n].act = a
		ruleNodes[n].capture = a == aRecords || a == aRecordsDeflate
	}
	rootNode = newNode()
	pawnRoot = newNode()
	for _, r := range pathRules {
		add(rootNode, r.path, r.act)
	}
	for _, l := range worldObjectLists {
		for _, r := range worldObjectRules {
			add(rootNode, l+"/li"+r.path, r.act)
		}
	}
	for _, r := range pawnRules {
		add(pawnRoot, r.path, r.act)
	}
}

func childNode(n, name int32) int32 {
	if n < 0 {
		return -1
	}
	for _, k := range ruleNodes[n].kids {
		if k.name == name {
			return k.node
		}
	}
	return -1
}

// Reference kinds (prefixes of GetUniqueLoadID results). The checked kinds are matched
// against the objects the save defines; the others are counted only.
const (
	refChecked uint8 = iota + 1
	refUnchecked
)

var refKinds = func() map[string]uint8 {
	m := map[string]uint8{}
	for _, p := range CheckedRefPrefixes {
		m[strings.TrimSuffix(p, "_")] = refChecked
	}
	for _, p := range []string{"Ability", "ArchivedDialog", "Area", "Battle", "Bill", "Gene",
		"Hediff", "IdeoBuildingPresenceDemand", "Job", "LayoutRoom", "Letter", "LogEntry",
		"Lord", "Message", "MysteriousCorpseTracker", "PassingShip", "PitGateIncident",
		"Plan", "PlanetLayer", "Precept", "PsychicRitual", "QuestPart", "RelationshipRecord",
		"RitualObligation", "RitualRole", "ShipJob", "StorageGroup", "Tale", "TransportShip",
		"Verb", "WorldFeature", "Zone"} {
		m[p] = refUnchecked
	}
	return m
}()

type pawnRec struct {
	Pawn
	faction   string
	host      string
	guest     string
	humanlike bool
	animal    bool
	drop      bool
}

type worldObj struct {
	id      int
	faction string
	name    string
}

type extract struct {
	defIdx   map[string]int32
	defNames []string
	defCount []int
	classes  map[string]int
	strs     map[string]string

	defined   map[uint64]struct{}
	refs      map[uint64]uint32 // hash → offset in arena
	arena     []byte
	unchecked map[uint64]struct{}

	pawns     []pawnRec
	pawnStack []int32
	curMap    int
	curGroup  string
	playerDef string
	wos       []worldObj
	resKeys   []string
	resVals   []float64
}

func (x *extract) init() {
	x.defIdx = map[string]int32{}
	x.classes = map[string]int{}
	x.strs = map[string]string{}
	x.defined = map[uint64]struct{}{}
	x.refs = map[uint64]uint32{}
	x.unchecked = map[uint64]struct{}{}
	x.curMap = -1
}

func (x *extract) def(v []byte) int32 {
	if i, ok := x.defIdx[string(v)]; ok {
		x.defCount[i]++
		return i
	}
	i := int32(len(x.defNames))
	n := string(v)
	x.defIdx[n] = i
	x.defNames = append(x.defNames, n)
	x.defCount = append(x.defCount, 1)
	return i
}

func (x *extract) class(v []byte) {
	if n, ok := x.classes[string(v)]; ok {
		x.classes[string(v)] = n + 1
		return
	}
	x.classes[string(v)] = 1
}

// str interns a short string that repeats across the save (faction refs, kind defs).
func (x *extract) str(v []byte) string {
	if s, ok := x.strs[string(v)]; ok {
		return s
	}
	s := string(v)
	x.strs[s] = s
	return s
}

const (
	fnvOffset = 14695981039346656037
	fnvPrime  = 1099511628211
)

func fnv(h uint64, b []byte) uint64 {
	for _, c := range b {
		h ^= uint64(c)
		h *= fnvPrime
	}
	return h
}

func fnvString(h uint64, s string) uint64 {
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= fnvPrime
	}
	return h
}

func (x *extract) define(prefix string, id []byte) {
	x.defined[fnv(fnvString(fnvOffset, prefix), id)] = struct{}{}
}

// ref records v when it looks like a reference: a known kind, '_', then the rest.
func (x *extract) ref(v []byte) {
	if len(v) < 3 || v[0] < 'A' || v[0] > 'Z' {
		return
	}
	w := v
	if len(w) > 32 {
		w = w[:32]
	}
	k := bytes.IndexByte(w, '_')
	if k <= 0 || k == len(v)-1 {
		return
	}
	kind := refKinds[string(v[:k])]
	if kind == 0 {
		return
	}
	h := fnv(fnvOffset, v)
	if kind == refUnchecked {
		x.unchecked[h] = struct{}{}
		return
	}
	if _, ok := x.refs[h]; !ok {
		x.refs[h] = uint32(len(x.arena))
		x.arena = append(x.arena, v...)
		x.arena = append(x.arena, 0)
	}
}

func (x *extract) curPawn() *pawnRec {
	if len(x.pawnStack) == 0 {
		return nil
	}
	return &x.pawns[x.pawnStack[len(x.pawnStack)-1]]
}

func (x *extract) open(s *scanner, f, p *frame, class []byte) {
	fs := s.f
	switch ruleNodes[f.node].act {
	case aStatsRecord:
		fs.Stats.GreatestPopulation = 3
	case aRecorder:
		fs.History = append(fs.History, Recorder{Group: x.curGroup})
		f.obj = int32(len(fs.History) - 1)
	case aLetter:
		fs.Events = append(fs.Events, Event{Class: x.str(class)})
		f.obj = int32(len(fs.Events) - 1)
	case aFactionLi:
		fs.Factions = append(fs.Factions, Faction{})
		f.obj = int32(len(fs.Factions) - 1)
	case aWorldObject:
		x.wos = append(x.wos, worldObj{id: -1})
		f.obj = int32(len(x.wos) - 1)
		if string(class) == "Caravan" {
			f.ctx = ctxCaravan
		} else {
			f.ctx = ctxOther
		}
	case aMapLi:
		fs.Maps = append(fs.Maps, Map{ID: -1})
		f.obj = int32(len(fs.Maps) - 1)
		x.curMap = int(f.obj)
		f.ctx = ctxMap
	case aWorldPawns:
		f.ctx = ctxWorld
	case aPawnStory:
		if pw := x.curPawn(); pw != nil && f.flags&fNull == 0 {
			pw.humanlike = true
		}
	case aPawnTraining:
		if pw := x.curPawn(); pw != nil && f.flags&fNull == 0 {
			pw.animal = true
		}
	}
}

func (x *extract) close(s *scanner, f, p *frame) {
	fs := s.f
	if f.pawn >= 0 {
		pw := &x.pawns[f.pawn]
		if (pw.faction == "" || pw.faction == "null") && pw.guest != "Prisoner" {
			pw.drop = true
			if int(f.pawn) == len(x.pawns)-1 {
				x.pawns = x.pawns[:f.pawn]
			}
		}
		x.pawnStack = x.pawnStack[:len(x.pawnStack)-1]
		return
	}
	if f.node < 0 {
		return
	}
	switch ruleNodes[f.node].act {
	case aGroup:
		x.curGroup = ""
	case aLetter:
		if fs.Events[f.obj].Label == "" && int(f.obj) == len(fs.Events)-1 {
			fs.Events = fs.Events[:f.obj] // a message or dialog, not a letter
		}
	case aFactionLi:
		x.define("Faction_", strconv.AppendInt(nil, int64(fs.Factions[f.obj].LoadID), 10))
	case aWorldObject:
		x.define("WorldObject_", strconv.AppendInt(nil, int64(x.wos[f.obj].id), 10))
	case aMapLi:
		x.define("Map_", strconv.AppendInt(nil, int64(fs.Maps[f.obj].ID), 10))
		x.curMap = -1
	case aIdeoLi:
		x.define("Ideo_", idOrZero(f.id))
	case aQuestLi:
		x.define("Quest_", idOrZero(f.id))
	}
}

func idOrZero(b []byte) []byte {
	if len(b) == 0 {
		return []byte{'0'}
	}
	return b
}

func (x *extract) leaf(s *scanner, f, p *frame, v []byte) {
	switch f.name {
	case nDef:
		if p != nil && len(v) > 0 && string(v) != "null" {
			p.def = x.def(v)
		}
	case nID:
		if p != nil {
			p.id = append(p.id[:0], v...)
			if p.def >= 0 && !allDigits(v) {
				x.define("Thing_", v)
			}
		}
	case nFaction:
		if p != nil {
			p.fac = append(p.fac[:0], v...)
		}
		x.ref(v)
	case nKindDef:
		if p != nil && p.def >= 0 && len(p.id) > 0 && p.pawn < 0 {
			x.newPawn(s, p, v)
		}
	case nLoadID, nIDUpper, nUniqueID, nUniqueIdLower:
		// An object's own ID, not a reference (Verb loadIDs even start with "Thing_").
	default:
		x.ref(v)
	}
	if f.node >= 0 {
		x.rule(s, f, p, v)
	}
}

func (x *extract) newPawn(s *scanner, p *frame, kind []byte) {
	r := pawnRec{
		Pawn:    Pawn{ID: string(p.id), Race: x.defNames[p.def], Kind: x.str(kind), Map: -1},
		faction: x.str(p.fac),
	}
	switch p.ctx {
	case ctxMap:
		r.Where = WhereMap
		if x.curMap >= 0 {
			r.Map = s.f.Maps[x.curMap].ID
		}
	case ctxWorld:
		r.Where = WhereWorld
	case ctxCaravan:
		r.Where = WhereCaravan
	default:
		r.Where = WhereOther
	}
	x.pawns = append(x.pawns, r)
	p.pawn = int32(len(x.pawns) - 1)
	p.node = pawnRoot
	x.pawnStack = append(x.pawnStack, p.pawn)
}

func (x *extract) rule(s *scanner, f, p *frame, v []byte) {
	fs := s.f
	switch ruleNodes[f.node].act {
	case aRealPlayTime:
		fs.RealPlayTime = parseFloat(v)
	case aPermadeath:
		fs.Permadeath = string(v) == "True"
	case aScenarioName:
		fs.Scenario = string(v)
	case aPlayerFactionDef:
		x.playerDef = string(v)
	case aTicksGame:
		fs.TicksGame = parseInt(v)
	case aGameStartAbsTick:
		fs.GameStartAbsTick = parseInt(v)
	case aStatRaids:
		fs.Stats.RaidsEnemy = int(parseInt(v))
	case aStatThreats:
		fs.Stats.ThreatsQueued = int(parseInt(v))
	case aStatKilled:
		fs.Stats.ColonistsKilled = int(parseInt(v))
	case aStatLaunched:
		fs.Stats.ColonistsLaunched = int(parseInt(v))
	case aStatGreatestPop:
		fs.Stats.GreatestPopulation = int(parseInt(v))
	case aResearchCurrent:
		if string(v) != "null" {
			fs.Research.Current = string(v)
		}
	case aResearchKey:
		x.resKeys = append(x.resKeys, string(v))
	case aResearchValue:
		x.resVals = append(x.resVals, parseFloat(v))
	case aStoryteller:
		fs.Storyteller = string(v)
	case aDifficulty:
		fs.Difficulty = string(v)
	case aGroupDef:
		x.curGroup = x.str(v)
	case aRecorderDef:
		if p != nil && p.obj >= 0 {
			fs.History[p.obj].Def = string(v)
		}
	case aRecords, aRecordsDeflate:
		if p != nil && p.obj >= 0 {
			setRecords(&fs.History[p.obj], v, ruleNodes[f.node].act == aRecordsDeflate)
		}
	case aLetterDef:
		if p != nil && p.obj >= 0 {
			fs.Events[p.obj].Def = x.str(v)
		}
	case aLetterLabel:
		if p != nil && p.obj >= 0 {
			fs.Events[p.obj].Label = string(v)
		}
	case aLetterTick:
		if p != nil && p.obj >= 0 {
			fs.Events[p.obj].Tick = parseInt(v)
		}
	case aWorldName:
		fs.World.Name = string(v)
	case aWorldSeed:
		fs.World.Seed = string(v)
	case aWorldPRV:
		fs.World.PersistentRandomValue = int(parseInt(v))
	case aWorldCoverage:
		fs.World.Coverage = parseFloat(v)
	case aFactionDef:
		if p != nil && p.obj >= 0 {
			fs.Factions[p.obj].Def = string(v)
		}
	case aFactionName:
		if p != nil && p.obj >= 0 {
			fs.Factions[p.obj].Name = string(v)
		}
	case aFactionLoadID:
		if p != nil && p.obj >= 0 {
			fs.Factions[p.obj].LoadID = int(parseInt(v))
		}
	case aWOID:
		if p != nil && p.obj >= 0 {
			x.wos[p.obj].id = int(parseInt(v))
		}
	case aWOFaction:
		if p != nil && p.obj >= 0 {
			x.wos[p.obj].faction = x.str(v)
		}
	case aWOName:
		if p != nil && p.obj >= 0 {
			x.wos[p.obj].name = string(v)
		}
	case aMapID:
		if p != nil && p.obj >= 0 {
			fs.Maps[p.obj].ID = int(parseInt(v))
		}
	case aMapParent:
		if x.curMap >= 0 && string(v) != "null" {
			fs.Maps[x.curMap].Parent = string(v)
		}
	case aMapPocket:
		if x.curMap >= 0 {
			fs.Maps[x.curMap].Pocket = string(v) == "True"
		}
	case aPawnFirst, aPawnNick, aPawnLast, aPawnSingle, aPawnHealth, aPawnGuest, aPawnHost:
		pw := x.curPawn()
		if pw == nil {
			return
		}
		switch ruleNodes[f.node].act {
		case aPawnFirst:
			pw.First = string(v)
		case aPawnNick:
			pw.Nick = string(v)
		case aPawnLast:
			pw.Last = string(v)
		case aPawnSingle:
			pw.Name = string(v)
		case aPawnHealth:
			pw.Dead = string(v) == "Dead"
		case aPawnGuest:
			pw.guest = x.str(v)
		case aPawnHost:
			pw.host = x.str(v)
		}
	}
}

// setRecords decodes a HistoryAutoRecorder series: base64 of little-endian float32s,
// raw-deflated when the element is recordsDeflate (DataExposeUtility.LookByteArray).
func setRecords(r *Recorder, v []byte, deflated bool) {
	clean := make([]byte, 0, len(v))
	for _, c := range v {
		if !isSpace(c) {
			clean = append(clean, c)
		}
	}
	raw := make([]byte, base64.StdEncoding.DecodedLen(len(clean)))
	n, err := base64.StdEncoding.Decode(raw, clean)
	if err != nil {
		return
	}
	raw = raw[:n]
	if deflated {
		b, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(raw)), maxCapture))
		if err != nil {
			return
		}
		raw = b
	}
	cnt := len(raw) / 4
	r.Count = cnt
	if cnt == 0 {
		return
	}
	r.Last = math.Float32frombits(binary.LittleEndian.Uint32(raw[(cnt-1)*4:]))
	if slices.Contains(KeptSeries, r.Def) {
		r.Values = make([]float32, cnt)
		for i := range r.Values {
			r.Values[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
		}
	}
}

func parseInt(v []byte) int64 {
	n, err := strconv.ParseInt(string(bytes.TrimSpace(v)), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func parseFloat(v []byte) float64 {
	n, err := strconv.ParseFloat(string(bytes.TrimSpace(v)), 64)
	if err != nil {
		return 0
	}
	return n
}

func allDigits(v []byte) bool {
	for _, c := range v {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(v) > 0
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// finish turns what the pass collected into Facts.
func (s *scanner) finish() {
	f, x := s.f, &s.x

	// The player faction: the one with the scenario's player faction def, else the first
	// whose def is named like a player faction (PlayerColony, PlayerTribe…).
	for i := range f.Factions {
		if x.playerDef != "" && f.Factions[i].Def == x.playerDef {
			f.Player = f.Factions[i]
			break
		}
	}
	if f.Player.Def == "" {
		for i := range f.Factions {
			if strings.Contains(f.Factions[i].Def, "Player") {
				f.Player = f.Factions[i]
				break
			}
		}
	}
	playerRef := ""
	if f.Player.Def != "" {
		playerRef = f.Player.Ref()
	}

	byID := map[int]*worldObj{}
	for i := range x.wos {
		byID[x.wos[i].id] = &x.wos[i]
	}
	for i := range f.Maps {
		m := &f.Maps[i]
		id, ok := strings.CutPrefix(m.Parent, "WorldObject_")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(id)
		if err != nil {
			continue
		}
		if w := byID[n]; w != nil {
			m.Name, m.Faction = w.name, w.faction
			m.Home = playerRef != "" && w.faction == playerRef
		}
	}

	for i := range x.pawns {
		pw := &x.pawns[i]
		switch {
		case pw.drop:
		case playerRef != "" && pw.faction == playerRef:
			switch {
			case pw.humanlike:
				p := pw.Pawn
				p.Slave = pw.guest == "Slave"
				f.Colonists = append(f.Colonists, p)
			case pw.Dead:
			case pw.animal:
				f.Animals++
			default:
				f.Mechs++
			}
		case pw.guest == "Prisoner" && playerRef != "" && pw.host == playerRef && !pw.Dead:
			f.Prisoners++
		}
	}

	if len(x.resKeys) > 0 {
		f.Research.Progress = make(map[string]float64, len(x.resKeys))
		for i, k := range x.resKeys {
			if i < len(x.resVals) {
				f.Research.Progress[k] = x.resVals[i]
			}
		}
	}

	if len(x.defNames) > 0 {
		f.Defs = make(map[string]int, len(x.defNames))
		for i, n := range x.defNames {
			f.Defs[n] = x.defCount[i]
		}
	}
	if len(x.classes) > 0 {
		f.Classes = x.classes
	}

	if f.Malformed == nil {
		f.Refs.Defined = len(x.defined)
		f.Refs.Checked = len(x.refs)
		f.Refs.Unchecked = len(x.unchecked)
		var sample []string
		for h, off := range x.refs {
			if _, ok := x.defined[h]; ok {
				continue
			}
			f.Refs.Dangling++
			end := bytes.IndexByte(x.arena[off:], 0)
			sample = append(sample, string(x.arena[off:int(off)+end]))
		}
		slices.Sort(sample)
		if len(sample) > MaxDangling {
			sample = sample[:MaxDangling]
		}
		f.Refs.Sample = sample
	}

	slices.SortStableFunc(f.Events, func(a, b Event) int {
		switch {
		case a.Tick > b.Tick:
			return -1
		case a.Tick < b.Tick:
			return 1
		}
		return 0
	})
	if len(f.Events) > MaxEvents {
		f.Events = f.Events[:MaxEvents]
	}
}
