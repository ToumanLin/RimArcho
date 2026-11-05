package savefile

import (
	"bufio"
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkScan27MB scans a generated save shaped like a long 1.6 colony (10 §1.7 L1
// target: at most 0.3 s per 27 MB, flat memory). The file is written to
// $SAVEFILE_BENCH_DIR (default: a temporary directory) and removed afterwards.
func BenchmarkScan27MB(b *testing.B) {
	dir := os.Getenv("SAVEFILE_BENCH_DIR")
	if dir == "" {
		dir = b.TempDir()
	}
	path := filepath.Join(dir, "synthetic-27mb.rws")
	if err := writeSyntheticFile(path); err != nil {
		b.Fatal(err)
	}
	defer os.Remove(path)
	st, _ := os.Stat(path)

	scanFile := func() *Facts {
		fh, err := os.Open(path)
		if err != nil {
			b.Fatal(err)
		}
		defer fh.Close()
		f, err := Scan(fh)
		if err != nil {
			b.Fatal(err)
		}
		return f
	}
	var f *Facts
	peak := peakHeap(func() { f = scanFile() })
	if f.Malformed != nil || len(f.Colonists) != 25 || f.Refs.Dangling != 1 || len(f.Defs) < 1300 {
		b.Fatalf("unexpected facts: malformed %+v colonists %d dangling %d defs %d", f.Malformed, len(f.Colonists), f.Refs.Dangling, len(f.Defs))
	}
	b.SetBytes(st.Size())
	b.ReportAllocs()
	for b.Loop() {
		scanFile()
	}
	b.ReportMetric(float64(peak)/(1<<20), "peak-heap-MB")
	b.ReportMetric(float64(st.Size())/(1<<20), "file-MB")
}

// peakHeap runs fn and returns the largest heap growth seen while it ran (live objects
// and not yet collected garbage).
func peakHeap(fn func()) uint64 {
	runtime.GC()
	s := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	metrics.Read(s)
	base := s[0].Value.Uint64()
	var peak atomic.Uint64
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		s := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
		t := time.NewTicker(500 * time.Microsecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				metrics.Read(s)
				if v := s[0].Value.Uint64(); v > peak.Load() {
					peak.Store(v)
				}
			}
		}
	}()
	fn()
	close(done)
	<-stopped
	if p := peak.Load(); p > base {
		return p - base
	}
	return 0
}

func writeSyntheticFile(path string) error {
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(fh, 1<<20)
	writeSyntheticSave(w)
	if err := w.Flush(); err != nil {
		fh.Close()
		return err
	}
	return fh.Close()
}

// writeSyntheticSave writes a deterministic save of about 27 MB: 1400 distinct defs,
// 25 colonists, 1100 world pawns, 52 000 map things, compressed grids and history series.
// One reference (Thing_Gone1) points to nothing.
func writeSyntheticSave(w io.Writer) {
	r := rand.New(rand.NewPCG(1, 2))
	defs := make([]string, 1400)
	for i := range defs {
		defs[i] = fmt.Sprintf("Mod%02d_Def%04d", i%40, i)
	}
	pick := func() string { return defs[r.IntN(len(defs))] }
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }
	blob := func(tag string, n int) {
		var sb strings.Builder
		raw := make([]byte, n)
		for i := range raw {
			raw[i] = byte(r.IntN(256))
		}
		s := base64.StdEncoding.EncodeToString(raw)
		for i := 0; i < len(s); i += 100 {
			sb.WriteString(s[i:min(i+100, len(s))])
			sb.WriteByte('\n')
		}
		p("<%s>\n%s</%s>\n", tag, sb.String(), tag)
	}
	series := func(n int) string {
		var raw bytes.Buffer
		for i := 0; i < n; i++ {
			binary.Write(&raw, binary.LittleEndian, math.Float32bits(float32(1000+i*37)))
		}
		var c bytes.Buffer
		fw, _ := flate.NewWriter(&c, flate.DefaultCompression)
		fw.Write(raw.Bytes())
		fw.Close()
		return base64.StdEncoding.EncodeToString(c.Bytes())
	}
	thingID := 1000 // above the colonists' fixed IDs Human1…Human25
	nextID := func(def string) string { thingID++; return fmt.Sprintf("%s%d", def, thingID) }
	pawn := func(tag, id, race, faction string, colonist bool) {
		p("<%s Class=\"Pawn\">\n<def>%s</def>\n<id>%s</id>\n<map>0</map>\n<pos>(%d, 0, %d)</pos>\n<faction>%s</faction>\n<kindDef>%s</kindDef>\n<gender>Female</gender>\n",
			tag, race, id, r.IntN(250), r.IntN(250), faction, pick())
		p("<name Class=\"NameTriple\">\n<first>First%d</first>\n<nick>Nick%d</nick>\n<last>Last%d</last>\n</name>\n", thingID, thingID, thingID)
		p("<mindState>\n<duty IsNull=\"True\" />\n<lastAttackTargetTick>%d</lastAttackTargetTick>\n<meleeThreat>null</meleeThreat>\n<enemyTarget>null</enemyTarget>\n</mindState>\n", r.IntN(1e7))
		p("<jobs>\n<curJob>\n<def>%s</def>\n<loadID>%d</loadID>\n<targetA>(%d, 0, %d)</targetA>\n</curJob>\n<jobQueue>\n<jobs />\n</jobQueue>\n</jobs>\n", pick(), r.IntN(1e6), r.IntN(250), r.IntN(250))
		p("<verbTracker>\n<verbs>\n<li Class=\"Verb_MeleeAttackDamage\">\n<loadID>Thing_%s_0_Smash</loadID>\n<lastShotTick>-999999</lastShotTick>\n</li>\n</verbs>\n</verbTracker>\n", id)
		p("<apparel>\n<wornApparel>\n<innerList>\n")
		for k := 0; k < 4; k++ {
			d := pick()
			p("<li>\n<def>%s</def>\n<id>%s</id>\n<health>%d</health>\n<stuff>%s</stuff>\n<quality>Normal</quality>\n<sourcePrecept>null</sourcePrecept>\n<everSeenByPlayer>True</everSeenByPlayer>\n</li>\n", d, nextID(d), r.IntN(300), pick())
		}
		p("</innerList>\n</wornApparel>\n<lockedApparel />\n</apparel>\n")
		if colonist || r.IntN(3) > 0 {
			p("<story>\n<childhood>%s</childhood>\n<adulthood>%s</adulthood>\n<traits>\n<allTraits>\n", pick(), pick())
			for k := 0; k < 3; k++ {
				p("<li>\n<def>%s</def>\n<degree>%d</degree>\n<sourceGene>null</sourceGene>\n</li>\n", pick(), r.IntN(3))
			}
			p("</allTraits>\n</traits>\n<hairDef>%s</hairDef>\n<bodyType>Female</bodyType>\n<headType>%s</headType>\n<skinColorOverride>RGBA(0.5, 0.4, 0.3, 1)</skinColorOverride>\n</story>\n", pick(), pick())
		} else {
			p("<story IsNull=\"True\" />\n")
		}
		p("<healthTracker>\n<hediffSet>\n<hediffs>\n")
		for k := 0; k < 5; k++ {
			p("<li Class=\"Hediff_Injury\">\n<loadID>%d</loadID>\n<def>%s</def>\n<ageTicks>%d</ageTicks>\n<part>\n<body>Human</body>\n<index>%d</index>\n</part>\n<severity>%.3f</severity>\n</li>\n", r.IntN(1e6), pick(), r.IntN(1e7), r.IntN(50), r.Float64()*10)
		}
		p("</hediffs>\n</hediffSet>\n<surgeryBills>\n<bills />\n</surgeryBills>\n<immunity>\n<imList />\n</immunity>\n</healthTracker>\n")
		p("<records>\n<records>\n<vals>\n")
		for k := 0; k < 60; k++ {
			p("<li>%d</li>\n", r.IntN(100000))
		}
		p("</vals>\n</records>\n<battleActive>null</battleActive>\n</records>\n")
		p("<needs>\n<needs>\n")
		for k := 0; k < 10; k++ {
			p("<li Class=\"Need_Food\">\n<def>%s</def>\n<curLevel>%.4f</curLevel>\n</li>\n", pick(), r.Float64())
		}
		p("</needs>\n</needs>\n<skills>\n<skills>\n")
		for k := 0; k < 12; k++ {
			p("<li>\n<def>%s</def>\n<level>%d</level>\n<xpSinceLastLevel>%.2f</xpSinceLastLevel>\n<passion>Minor</passion>\n</li>\n", pick(), r.IntN(20), r.Float64()*1000)
		}
		p("</skills>\n</skills>\n<workSettings>\n<priorities>\n<vals>\n")
		for k := 0; k < 40; k++ {
			p("<li>%d</li>\n", r.IntN(4))
		}
		p("</vals>\n</priorities>\n</workSettings>\n<social>\n<directRelations>\n<li>\n<def>%s</def>\n<otherPawn>Thing_Human%d</otherPawn>\n<startTicks>%d</startTicks>\n</li>\n</directRelations>\n</social>\n", pick(), 1+r.IntN(25), r.IntN(1e7))
		p("<training IsNull=\"True\" />\n<guest>\n<guestStatus>Guest</guestStatus>\n</guest>\n</%s>\n", tag)
	}

	p("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<savegame>\n<meta>\n<gameVersion>1.6.4633 rev1261</gameVersion>\n<modIds>\n")
	for i := 0; i < 200; i++ {
		p("<li>tester.mod%03d</li>\n", i)
	}
	p("</modIds>\n</meta>\n<game>\n<info>\n<realPlayTimeInteracting>360000</realPlayTimeInteracting>\n</info>\n")
	p("<scenario>\n<name>Crashlanded</name>\n<playerFaction>\n<def>PlayerFaction</def>\n<factionDef>PlayerColony</factionDef>\n</playerFaction>\n</scenario>\n")
	p("<tickManager>\n<ticksGame>51600000</ticksGame>\n<gameStartAbsTick>300000</gameStartAbsTick>\n</tickManager>\n")
	p("<storyWatcher>\n<statsRecord>\n<numRaidsEnemy>47</numRaidsEnemy>\n<greatestPopulation>25</greatestPopulation>\n</statsRecord>\n</storyWatcher>\n")
	p("<researchManager>\n<progress>\n<keys>\n")
	for i := 0; i < 300; i++ {
		p("<li>%s</li>\n", defs[i])
	}
	p("</keys>\n<values>\n")
	for i := 0; i < 300; i++ {
		p("<li>%d</li>\n", r.IntN(5000))
	}
	p("</values>\n</progress>\n</researchManager>\n<storyteller>\n<def>Randy</def>\n<difficulty>Rough</difficulty>\n</storyteller>\n")
	p("<history>\n<archive>\n<archivables>\n")
	for i := 0; i < 200; i++ {
		p("<li Class=\"StandardLetter\">\n<ID>%d</ID>\n<def>ThreatBig</def>\n<label>Raid %d</label>\n<arrivalTick>%d</arrivalTick>\n<text>Raiders have arrived from the north.</text>\n</li>\n", i, i, i*250000)
	}
	p("</archivables>\n</archive>\n<autoRecorderGroups>\n<li>\n<def>Wealth</def>\n<recorders>\n")
	for _, d := range []string{"Wealth_Total", "Wealth_Items", "Wealth_Buildings", "Wealth_Pawns", "FreeColonists", "Prisoners", "ColonistMood", "ThreatPoints", "Adaptation", "PopAdaptation", "PopIntent"} {
		p("<li>\n<def>%s</def>\n<recordsDeflate>%s</recordsDeflate>\n</li>\n", d, series(1720))
	}
	p("</recorders>\n</li>\n</autoRecorderGroups>\n</history>\n")

	p("<world>\n<info>\n<name>Synthetic</name>\n<seedString>bench</seedString>\n<persistentRandomValue>42</persistentRandomValue>\n</info>\n<grid>\n")
	for _, t := range []string{"tileBiomeDeflate", "tileElevationDeflate", "tileHillinessDeflate", "tileTemperatureDeflate", "tileRainfallDeflate", "tileSwampinessDeflate", "tileFeatureDeflate", "tileRoadOriginsDeflate"} {
		blob(t, 220000)
	}
	p("</grid>\n<factionManager>\n<allFactions>\n<li>\n<def>PlayerColony</def>\n<name>Bench Colony</name>\n<loadID>1</loadID>\n</li>\n")
	for i := 2; i < 26; i++ {
		p("<li>\n<def>%s</def>\n<name>Faction %d</name>\n<loadID>%d</loadID>\n<relations>\n", pick(), i, i)
		for k := 1; k < 26; k++ {
			p("<li>\n<other>Faction_%d</other>\n<kind>Neutral</kind>\n<baseGoodwill>%d</baseGoodwill>\n</li>\n", k, r.IntN(200)-100)
		}
		p("</relations>\n</li>\n")
	}
	p("</allFactions>\n</factionManager>\n<worldPawns>\n<pawnsAlive>\n")
	for i := 0; i < 1100; i++ {
		pawn("li", nextID("Human"), "Human", fmt.Sprintf("Faction_%d", 2+r.IntN(24)), false)
	}
	p("</pawnsAlive>\n</worldPawns>\n<worldObjects>\n<worldObjects>\n<li Class=\"Settlement\">\n<def>Settlement</def>\n<tile>1</tile>\n<ID>1</ID>\n<faction>Faction_1</faction>\n<nameInt>Bench Base</nameInt>\n</li>\n")
	for i := 2; i < 300; i++ {
		p("<li Class=\"Settlement\">\n<def>Settlement</def>\n<tile>%d</tile>\n<ID>%d</ID>\n<faction>Faction_%d</faction>\n<nameInt>Town %d</nameInt>\n</li>\n", i, i, 2+r.IntN(24), i)
	}
	p("</worldObjects>\n</worldObjects>\n</world>\n<maps>\n<li>\n<uniqueID>0</uniqueID>\n<mapInfo>\n<size>(250, 1, 250)</size>\n<parent>WorldObject_1</parent>\n</mapInfo>\n")
	for _, t := range []string{"compressedThingMapDeflate", "topGridDeflate", "underGridDeflate", "roofGridDeflate", "fogGridDeflate", "snowGridDeflate"} {
		blob(t, 300000)
	}
	p("<things>\n")
	for i := 1; i <= 25; i++ {
		pawn("thing", fmt.Sprintf("Human%d", i), "Human", "Faction_1", true)
	}
	p("<thing Class=\"Building\">\n<def>Wall</def>\n<id>Wall0</id>\n<target>Thing_Gone1</target>\n</thing>\n")
	for i := 0; i < 52000; i++ {
		d := pick()
		p("<thing Class=\"ThingWithComps\">\n<def>%s</def>\n<id>%s</id>\n<map>0</map>\n<pos>(%d, 0, %d)</pos>\n<health>%d</health>\n<stackCount>%d</stackCount>\n<stuff>%s</stuff>\n<faction>Faction_1</faction>\n<questTags IsNull=\"True\" />\n</thing>\n",
			d, nextID(d), r.IntN(250), r.IntN(250), r.IntN(300), 1+r.IntN(75), pick())
	}
	p("</things>\n</li>\n</maps>\n</game>\n</savegame>\n")
}
