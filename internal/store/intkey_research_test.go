package store_test

// Opt-in research harness for docs/research/2026-10-30_archo-db-integer-keys.md (ToDo: integer keys
// and path tables in archo.db). RIMARCHO_INTKEY_BENCH=1 builds a synthetic archo.db shaped
// like a 10-day real library (about 760k Def facts, 980k file rows, 200k Type / Harmony facts)
// on the real v26 schema, copies it into prototype layouts, and logs sizes, query times and
// conversion times. Nothing here is product code: the prototype layouts live only in the copies.
//
// All files go under the test temp directory: point TMP/TEMP at a scratch folder. The test
// fails when that folder grows past RIMARCHO_INTKEY_DISK_MB (default 1400).
// RIMARCHO_INTKEY_SCALE (default 1) shrinks the data for a quick run.

import (
	"database/sql"
	"fmt"
	"math"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/zeebo/xxh3"

	"github.com/ToumanLin/RimArcho/internal/ids"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// ---------- synthetic library ----------

var (
	ikSyll      = []string{"ka", "to", "ri", "mel", "van", "dor", "ex", "pan", "ul", "the", "gra", "lin", "sol", "mar", "qui", "zen", "bor", "fen", "wyl", "tas", "ro", "ne", "sha", "kor", "ith", "al", "ven", "dru", "mo", "esk"}
	ikDefTypes  = []string{"ThingDef", "ThingDef", "ThingDef", "ThingDef", "ThingDef", "RecipeDef", "RecipeDef", "HediffDef", "HediffDef", "ResearchProjectDef", "PawnKindDef", "TerrainDef", "ThoughtDef", "JobDef", "WorkGiverDef", "AbilityDef", "GeneDef", "SoundDef", "StatDef", "TraitDef", "IncidentDef", "QuestScriptDef", "RulePackDef", "ThingCategoryDef", "FactionDef", "BodyPartDef", "DamageDef", "EffecterDef", "FleckDef", "MentalStateDef", "PreceptDef", "BackstoryDef", "ThingSetMakerDef", "LifeStageDef", "XenotypeDef"}
	ikDefDirs   = []string{"ThingDefs_Items", "ThingDefs_Buildings", "ThingDefs_Races", "ThingDefs_Misc", "Buildings", "Apparel", "Weapons", "RecipeDefs", "ResearchProjectDefs", "HediffDefs", "PawnKindDefs", "ThoughtDefs", "JobDefs", "WorkGiverDefs", "AbilityDefs", "GeneDefs", "Misc", "Factions", "Scenarios", "Stats"}
	ikTexDirs   = []string{"Things/Item/Resource", "Things/Item/Equipment/WeaponRanged", "Things/Item/Equipment/WeaponMelee", "Things/Building/Furniture", "Things/Building/Production", "Things/Building/Security", "Things/Pawn/Humanlike/Apparel", "Things/Pawn/Humanlike/Heads", "Things/Pawn/Animal", "UI/Icons", "UI/Commands", "UI/Designators", "Terrain/Surfaces", "Things/Mote", "Things/Plant", "Things/Filth"}
	ikTexSuf    = []string{"_north", "_south", "_east", "", "", "_m", "_a", "_b", "_Mask"}
	ikLangNames = []string{"ChineseSimplified (简体中文)", "English", "Russian", "Japanese (日本語)", "Korean", "German (Deutsch)", "ChineseSimplified"}
	ikParents   = []string{"BaseMeleeWeapon_Sharp_Quality", "BaseHumanMakeableGun", "ApparelMakeableBase", "BuildingBase", "ResourceBase", "BenchBase", "FurnitureWithQualityBase", "AnimalThingBase", "BasePawn", "MakeableDrugBase", "PlantBase", "OrganicProductBase"}
)

type ikGen struct{ r *rand.Rand }

func (g ikGen) word() string {
	var b strings.Builder
	for n := 2 + g.r.IntN(3); n > 0; n-- {
		s := ikSyll[g.r.IntN(len(ikSyll))]
		if b.Len() == 0 {
			s = strings.ToUpper(s[:1]) + s[1:]
		}
		b.WriteString(s)
	}
	return b.String()
}

func (g ikGen) title() string { w := g.word(); return strings.ToUpper(w[:1]) + w[1:] }

func (g ikGen) prefix() string {
	n := 2 + g.r.IntN(3)
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('A' + g.r.IntN(26))
	}
	return string(b)
}

func (g ikGen) lognorm(mean, sigma float64) float64 {
	mu := math.Log(mean) - sigma*sigma/2
	return math.Exp(mu + sigma*g.r.NormFloat64())
}

type ikFile struct {
	rel         string
	size, mtime int64
	hash        []byte
}

// ikMod is one planned instance.
type ikMod struct {
	id, rel, env, src   string
	official, active    bool
	ver, has17          bool
	n, nDef, nDLL, hist int
	seed                uint64
}

type ikPlan struct {
	mods                       []*ikMod
	defsPerFile, typesPerDLL   float64
	targetFiles, targetDefs    int
	targetTypes, targetHarmony int
}

const (
	ikOfficial = 6
	ikLocal    = 74
	ikWorkshop = 1274
	ikActive   = 482
)

func ikPlanLibrary(scale float64, env string) *ikPlan {
	r := rand.New(rand.NewPCG(7, 30))
	g := ikGen{r}
	p := &ikPlan{targetFiles: int(980_000 * scale), targetDefs: int(760_000 * scale), targetTypes: int(170_000 * scale), targetHarmony: int(30_000 * scale)}
	nMods := int(float64(ikOfficial+ikLocal+ikWorkshop) * math.Min(1, scale*1.0))
	if scale >= 1 {
		nMods = ikOfficial + ikLocal + ikWorkshop
	}
	cr := rand.NewChaCha8([32]byte{1, 2, 3})
	for i := 0; i < nMods; i++ {
		m := &ikMod{id: ids.New(cr), seed: uint64(i)*7919 + 11}
		switch {
		case i < ikOfficial:
			m.official, m.active, m.env, m.src = true, true, env, "bench-data"
			m.rel = []string{"Core", "Royalty", "Ideology", "Biotech", "Anomaly", "Odyssey"}[i]
			m.n = 900 + r.IntN(400)
			if i == 0 {
				m.n = 3000
			}
		case i < ikOfficial+ikLocal:
			m.env, m.src = env, "bench-local"
			m.rel = g.title() + "_" + strconv.Itoa(i)
		default:
			m.env, m.src = "", "bench-ws"
			m.rel = strconv.Itoa(2_000_000_000 + r.IntN(1_500_000_000))
		}
		if !m.official {
			m.n = max(4, min(9000, int(g.lognorm(193, 1.3))))
			m.ver = r.Float64() < 0.45
			m.has17 = m.ver && r.Float64() < 0.4
		}
		sem := max(4, int(float64(m.n)*0.28))
		if m.official {
			sem = int(float64(m.n) * 0.25)
		}
		m.nDLL = 0
		if !m.official && r.Float64() < 0.5 {
			m.nDLL = 1 + r.IntN(3)
			if m.ver {
				m.nDLL *= 2
			}
		}
		m.nDef = max(1, sem-4-m.nDLL-sem/7)
		p.mods = append(p.mods, m)
	}
	// active: the official ones plus a random 476 of the rest
	perm := r.Perm(len(p.mods) - ikOfficial)
	for _, k := range perm[:min(len(perm), int(float64(ikActive-ikOfficial)*math.Min(1, scale)))] {
		p.mods[ikOfficial+k].active = true
	}
	// history snapshots until the file rows reach the target
	cur := 0
	for _, m := range p.mods {
		cur += m.n
	}
	for total := cur; total < p.targetFiles; {
		m := p.mods[ikOfficial+r.IntN(len(p.mods)-ikOfficial)]
		if m.hist >= 3 {
			continue
		}
		m.hist++
		total += m.n
	}
	setFiles, dlls := 0, 0
	for _, m := range p.mods {
		switch {
		case !m.ver:
			setFiles += m.nDef
		case m.has17:
			setFiles += 2 * (m.nDef * 35 / 100)
		default:
			setFiles += m.nDef * 55 / 100
		}
		if m.active && m.nDLL > 0 {
			k := m.nDLL
			if m.ver && !m.has17 {
				k = m.nDLL / 2
			}
			dlls += k
		}
	}
	p.defsPerFile = float64(p.targetDefs) / float64(max(1, setFiles))
	p.typesPerDLL = float64(p.targetTypes) / float64(max(1, dlls))
	return p
}

// files builds a mod's current manifest; its def files per version folder.
func (m *ikMod) files() (files []ikFile, defs map[string][]string, dlls map[string][]string) {
	r := rand.New(rand.NewPCG(m.seed, 99))
	g := ikGen{r}
	pre := g.prefix()
	defs, dlls = map[string][]string{}, map[string][]string{}
	seen := map[string]bool{}
	t0 := int64(1_760_000_000_000_000_000) + r.Int64N(50_000_000_000_000_000)
	add := func(rel string, sem bool, size int64) {
		if seen[strings.ToLower(rel)] {
			return
		}
		seen[strings.ToLower(rel)] = true
		f := ikFile{rel: rel, size: size, mtime: t0 + r.Int64N(1_000_000_000_000)}
		if sem {
			h := make([]byte, 16)
			for i := range h {
				h[i] = byte(r.Uint32())
			}
			f.hash = h
		}
		files = append(files, f)
	}
	vers := []string{""}
	if m.ver {
		vers = []string{"1.5/", "1.6/"}
		if m.has17 {
			vers = append(vers, "1.7/")
		}
	}
	verFor := func(i int) string {
		if !m.ver {
			return ""
		}
		x := r.Float64()
		if m.has17 {
			switch {
			case x < 0.30:
				return "1.5/"
			case x < 0.65:
				return "1.6/"
			}
			return "1.7/"
		}
		if x < 0.45 {
			return "1.5/"
		}
		return "1.6/"
	}
	if !m.official {
		add("About/About.xml", true, 2000+r.Int64N(4000))
		add("About/Preview.png", true, 200_000+r.Int64N(800_000))
		add("About/PublishedFileId.txt", true, 10)
		if r.Float64() < 0.5 {
			add("About/ModIcon.png", true, 20_000)
		}
		if m.ver && r.Float64() < 0.6 {
			add("LoadFolders.xml", true, 600)
		}
	} else {
		add("About/About.xml", true, 3000)
		add("About/Preview.png", true, 300_000)
	}
	for i := 0; i < m.nDef; i++ {
		v := verFor(i)
		var rel string
		if r.Float64() < 0.3 {
			rel = v + "Defs/" + pre + "_" + g.title() + g.word() + ".xml"
		} else {
			d := ikDefDirs[r.IntN(len(ikDefDirs))]
			rel = v + "Defs/" + d + "/" + pre + "_" + strings.TrimSuffix(d, "Defs") + "_" + g.title() + g.word() + ".xml"
		}
		if m.official {
			rel = "Defs/" + ikDefDirs[r.IntN(len(ikDefDirs))] + "/" + g.title() + "_" + g.word() + ".xml"
		}
		before := len(files)
		add(rel, true, 1000+r.Int64N(40_000))
		if len(files) > before {
			defs[v] = append(defs[v], rel)
		}
	}
	for i := 0; i < len(files)/7+1 && !m.official; i++ {
		add(verFor(i)+"Patches/"+pre+"_"+g.word()+".xml", true, 500+r.Int64N(8000))
	}
	for i := 0; i < m.nDLL; i++ {
		v := vers[min(len(vers)-1, 1+i%max(1, len(vers)-1))]
		if !m.ver {
			v = ""
		}
		rel := v + "Assemblies/" + pre + g.title() + ".dll"
		if i == 0 && r.Float64() < 0.15 {
			rel = v + "Assemblies/0Harmony.dll"
		}
		before := len(files)
		add(rel, true, 20_000+r.Int64N(800_000))
		if len(files) > before {
			dlls[v] = append(dlls[v], rel)
		}
	}
	// non-semantic: textures (groups of facings), languages, sounds, other
	nonSem := m.n - len(files)
	langs := []string{}
	if !m.official && r.Float64() < 0.4 {
		for k := 1 + r.IntN(3); k > 0; k-- {
			langs = append(langs, ikLangNames[r.IntN(len(ikLangNames))])
		}
	}
	if m.official {
		langs = []string{"English"}
	}
	nLang := 0
	if len(langs) > 0 {
		nLang = nonSem * 12 / 100
	}
	nSound := nonSem * 5 / 100
	nOther := nonSem * 3 / 100
	for i := 0; i < nLang; i++ {
		l := langs[i%len(langs)]
		var rel string
		if r.Float64() < 0.3 {
			rel = "Languages/" + l + "/Keyed/" + pre + "_" + g.word() + ".xml"
		} else {
			rel = "Languages/" + l + "/DefInjected/" + ikDefTypes[r.IntN(len(ikDefTypes))] + "/" + pre + "_" + g.word() + ".xml"
		}
		add(rel, false, 500+r.Int64N(20_000))
	}
	for i := 0; i < nSound; i++ {
		w := g.title()
		add("Sounds/"+pre+"/"+w+"/"+w+"_"+strconv.Itoa(1+r.IntN(4))+".ogg", false, 20_000+r.Int64N(200_000))
	}
	for i := 0; i < nOther; i++ {
		switch r.IntN(3) {
		case 0:
			add("Source/"+pre+"/"+g.title()+"/"+g.title()+".cs", false, 2000+r.Int64N(20_000))
		case 1:
			add([]string{"README.md", "LICENSE", "Changelog.txt", ".gitattributes"}[r.IntN(4)], false, 3000)
		default:
			add("Common/Materials/"+g.title()+".mat", false, 1000)
		}
	}
	for len(files) < m.n {
		base := ""
		if m.ver && r.Float64() < 0.15 {
			base = vers[r.IntN(len(vers))]
		} else if m.ver && r.Float64() < 0.2 {
			base = "Common/"
		}
		d := ikTexDirs[r.IntN(len(ikTexDirs))]
		name := pre + "_" + g.title() + g.word()
		dir := base + "Textures/" + d + "/"
		if r.Float64() < 0.35 {
			dir += name + "/"
		}
		facings := 1
		if strings.HasPrefix(d, "Things/") && r.Float64() < 0.6 {
			facings = 3
		}
		for k := 0; k < facings && len(files) < m.n; k++ {
			suf := ikTexSuf[r.IntN(len(ikTexSuf))]
			if facings == 3 {
				suf = []string{"_north", "_south", "_east"}[k]
			}
			add(dir+name+suf+".png", false, 5000+r.Int64N(400_000))
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	return files, defs, dlls
}

// older derives an earlier manifest: a few files changed, added or removed.
func ikOlder(files []ikFile, r *rand.Rand) []ikFile {
	out := make([]ikFile, 0, len(files))
	for _, f := range files {
		x := r.Float64()
		switch {
		case x < 0.01:
			continue
		case f.hash != nil && x < 0.06:
			h := slices.Clone(f.hash)
			h[0] ^= 0x5a
			f.hash, f.size, f.mtime = h, f.size+int64(r.IntN(100)), f.mtime-1_000_000_000
		}
		out = append(out, f)
		if r.Float64() < 0.005 {
			g := f
			g.rel = strings.TrimSuffix(f.rel, filepath.Ext(f.rel)) + "_old" + filepath.Ext(f.rel)
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return slices.CompactFunc(out, func(a, b ikFile) bool { return a.rel == b.rel })
}

type ikDef struct {
	typ, defName, name, parent, file, mr, mra string
	abstract, line                            int
}

func ikDefsOf(g ikGen, pre string, files []string, perFile float64) []ikDef {
	var out []ikDef
	for _, f := range files {
		n := max(1, int(g.lognorm(perFile, 0.9)+0.5))
		line := 2
		for k := 0; k < n; k++ {
			d := ikDef{typ: ikDefTypes[g.r.IntN(len(ikDefTypes))], file: f, line: line}
			line += 6 + g.r.IntN(60)
			x := g.r.Float64()
			switch {
			case x < 0.05:
				d.name = pre + "_" + g.title() + "Base"
				d.abstract = 1
			case x < 0.07:
				d.name = pre + "_" + g.title() + "Base"
				d.defName = pre + "_" + g.title() + g.word()
			default:
				d.defName = pre + "_" + g.title() + g.word()
			}
			if g.r.Float64() < 0.35 {
				if g.r.Float64() < 0.5 {
					d.parent = ikParents[g.r.IntN(len(ikParents))]
				} else {
					d.parent = pre + "_" + g.title() + "Base"
				}
			}
			if g.r.Float64() < 0.03 {
				d.mr = "Ludeon.RimWorld.Ideology"
			}
			out = append(out, d)
		}
	}
	return out
}

// ---------- baseline loader ----------

type ikSample struct {
	defNames, names, types, dllNames []string
	insts                            []string    // instance IDs with defs
	snaps                            []int64     // current snapshot IDs
	cached                           [][2]string // inactive (instance, 1.6 content key): CachedDefs
	manifests                        []ikManifest
	defSets                          []ikDefSet
	envID                            string
}

func ikLoad(t *testing.T, db *sql.DB, p *ikPlan, envID string) *ikSample {
	t.Helper()
	smp := &ikSample{envID: envID}
	ex := func(tx *sql.Tx, q string, args ...any) sql.Result {
		r, err := tx.Exec(q, args...)
		if err != nil {
			t.Fatalf("%s: %v", q[:min(60, len(q))], err)
		}
		return r
	}
	{
		tx, _ := db.Begin()
		ex(tx, `CREATE TABLE bench_active(id TEXT PRIMARY KEY) WITHOUT ROWID`)
		for _, s := range [][3]string{{"bench-ws", "", "workshop"}, {"bench-local", envID, "mods"}, {"bench-data", envID, "official"}} {
			ex(tx, `INSERT INTO source(id,environment_id,kind,label,root,status) VALUES(?,?,?,?,?,'ok')`, s[0], s[1], s[2], s[0], `D:\bench\`+s[0])
		}
		for k := 0; k < 8; k++ {
			ex(tx, `INSERT INTO scan(environment_id,started_at,finished_at) VALUES(?,?,?)`, envID, testkit.Start.Unix()+int64(k), testkit.Start.Unix()+int64(k))
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	var firstScan int64
	db.QueryRow(`SELECT min(id) FROM scan WHERE environment_id=? AND started_at>=?`, envID, testkit.Start.Unix()).Scan(&firstScan)
	decl := `{"packageId":"%s","name":"%s","authors":["%s"],"supportedVersions":["1.5","1.6"],"modDependencies":[{"packageId":"brrainz.harmony","displayName":"Harmony"}],"loadAfter":["ludeon.rimworld"],"description":"` + strings.Repeat("Synthetic description text. ", 18) + `"}`
	nSets := 0
	for start := 0; start < len(p.mods); start += 60 {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		insInst, _ := tx.Prepare(`INSERT INTO instance(id,environment_id,lineage_id,source_id,rel_path,status,first_seen,last_seen) VALUES(?,?,?,?,?,'live',1,1)`)
		insLin, _ := tx.Prepare(`INSERT INTO lineage(id,environment_id,primary_instance_id,created_at) VALUES(?,'',?,1)`)
		insSnap, _ := tx.Prepare(`INSERT INTO snapshot(instance_id,scan_id,snapshot_hash,package_id,package_id_raw,name,authors,decl_blob,file_count,total_size,semantic_count) VALUES(?,?,?,?,?,?,?,?,?,?,?)`)
		insFile, _ := tx.Prepare(`INSERT INTO snapshot_file(snapshot_id,rel_path,size,mtime,hash) VALUES(?,?,?,?,?)`)
		insEv, _ := tx.Prepare(`INSERT INTO change_event(scan_id,instance_id,kind,before_snapshot,after_snapshot) VALUES(?,?,?,?,?)`)
		insSet, _ := tx.Prepare(`INSERT INTO fact_set(instance_id,kind,content_key) VALUES(?,?,?)`)
		insCS, _ := tx.Prepare(`INSERT INTO content_state(instance_id,game_mm,content_key,folders,def_files,patch_files,def_set_id,dll_set_id) VALUES(?,?,?,?,?,?,?,?)`)
		insDef, _ := tx.Prepare(`INSERT INTO def_fact(set_id,def_type,def_name,name,parent,abstract,file,line,may_require,may_require_any) VALUES(?,?,?,?,?,?,?,?,?,?)`)
		insAsm, _ := tx.Prepare(`INSERT INTO assembly_fact(set_id,file,uses_harmony,types,patches) VALUES(?,?,1,?,?)`)
		insType, _ := tx.Prepare(`INSERT INTO type_fact(set_id,file,type_name) VALUES(?,?,?)`)
		insH, _ := tx.Prepare(`INSERT INTO harmony_fact(set_id,file,patch_type,patch_method,kind,target,target_type,target_method,args) VALUES(?,?,?,?,?,?,?,?,?)`)
		insHID, _ := tx.Prepare(`INSERT INTO harmony_id(set_id,file,id) VALUES(?,?,?)`)
		insAct, _ := tx.Prepare(`INSERT INTO bench_active(id) VALUES(?)`)
		run := func(st *sql.Stmt, args ...any) int64 {
			r, err := st.Exec(args...)
			if err != nil {
				t.Fatal(err)
			}
			id, _ := r.LastInsertId()
			return id
		}
		for _, m := range p.mods[start:min(len(p.mods), start+60)] {
			r := rand.New(rand.NewPCG(m.seed, 5))
			g := ikGen{r}
			run(insInst, m.id, m.env, m.id, m.src, m.rel)
			run(insLin, m.id, m.id)
			if m.active {
				run(insAct, m.id)
			}
			files, defFiles, dllFiles := m.files()
			pre := g.prefix()
			// history first (older), then current
			manifests := [][]ikFile{files}
			for k := 0; k < m.hist; k++ {
				manifests = append([][]ikFile{ikOlder(manifests[0], r)}, manifests...)
			}
			var prev, snap int64
			for k, fs := range manifests {
				var total int64
				sem := 0
				for _, f := range fs {
					total += f.size
					if f.hash != nil {
						sem++
					}
				}
				h := xxh3.HashString128(fmt.Sprint(m.id, k)).Bytes()
				snap = run(insSnap, m.id, firstScan+int64(k), h[:], strings.ToLower(pre)+".synthetic", pre+".Synthetic", m.rel, pre, fmt.Sprintf(decl, pre, m.rel, pre), len(fs), total, sem)
				for _, f := range fs {
					var hash any
					if f.hash != nil {
						hash = f.hash
					}
					run(insFile, snap, f.rel, f.size, f.mtime, hash)
				}
				kind := "discovered"
				var before any
				if k > 0 {
					kind, before = "updated", prev
				}
				run(insEv, firstScan+int64(k), m.id, kind, before, snap)
				prev = snap
			}
			if _, err := tx.Exec(`UPDATE instance SET current_snapshot_id=? WHERE id=?`, snap, m.id); err != nil {
				t.Fatal(err)
			}
			smp.snaps = append(smp.snaps, snap)
			// Def sets: one per distinct effective folder list
			type vset struct {
				folders string
				files   []string
			}
			var v16, v17 vset
			if !m.ver {
				v16 = vset{"", defFiles[""]}
				v17 = v16
			} else {
				v16 = vset{"1.6\n", defFiles["1.6/"]}
				v17 = v16
				if m.has17 {
					v17 = vset{"1.7\n", defFiles["1.7/"]}
				}
			}
			mkKey := func(folders, kind string) string {
				return fmt.Sprintf("%x|f%x|%s|8", xxh3.HashString128(m.id+"snap").Bytes(), xxh3.HashString128(folders).Bytes(), kind)
			}
			defSet := map[string]int64{}
			for _, v := range []vset{v16, v17} {
				if _, ok := defSet[v.folders]; ok {
					continue
				}
				id := run(insSet, m.id, "def", mkKey(v.folders, "def"))
				nSets++
				defSet[v.folders] = id
				for _, d := range ikDefsOf(g, pre, v.files, p.defsPerFile) {
					run(insDef, id, d.typ, d.defName, d.name, d.parent, d.abstract, d.file, d.line, d.mr, d.mra)
					if r.IntN(600) == 0 && d.defName != "" {
						smp.defNames = append(smp.defNames, d.defName)
					}
					if d.name != "" && r.IntN(40) == 0 {
						smp.names = append(smp.names, d.name)
					}
				}
			}
			if len(v16.files) > 0 && r.IntN(10) == 0 {
				smp.insts = append(smp.insts, m.id)
			}
			// DLL sets for active instances only (03 §3)
			dllSet := map[string]int64{}
			if m.active && len(dllFiles) > 0 {
				d16, d17 := dllFiles[""], dllFiles[""]
				if m.ver {
					d16, d17 = dllFiles["1.6/"], dllFiles["1.6/"]
					if m.has17 {
						d17 = dllFiles["1.7/"]
					}
				}
				for vi, ds := range [][]string{d16, d17} {
					k := strings.Join(ds, ",")
					if _, ok := dllSet[k]; ok || (vi == 1 && !m.has17) {
						continue
					}
					id := run(insSet, m.id, "dll", mkKey(k, "dll"))
					dllSet[k] = id
					for _, f := range ds {
						nt := max(5, int(g.lognorm(p.typesPerDLL, 1.0)))
						nh := nt * p.targetHarmony / max(1, p.targetTypes) * 85 / 100
						run(insAsm, id, f, nt, nh)
						ns := pre + "." + g.title()
						for j := 0; j < nt; j++ {
							tn := ns + "." + g.title() + g.word()
							if j%4 == 3 {
								tn += "+<>c__DisplayClass" + strconv.Itoa(j%20) + "_0"
							}
							run(insType, id, f, tn)
							if r.IntN(800) == 0 {
								smp.types = append(smp.types, tn)
							}
						}
						for j := 0; j < nh; j++ {
							tt := []string{"Verse.Pawn", "RimWorld.Pawn_HealthTracker", "Verse.Thing", "RimWorld.JobGiver_Work", "Verse.GenSpawn", "RimWorld.CompPowerTrader"}[r.IntN(6)]
							tm := []string{"Tick", "SpawnSetup", "PostApplyDamage", "TryIssueJobPackage", "Spawn", "CompTick"}[r.IntN(6)]
							run(insH, id, f, ns+".HarmonyPatches."+tt[strings.LastIndex(tt, ".")+1:]+"_"+tm+"_Patch", []string{"Prefix", "Postfix", "Transpiler"}[r.IntN(3)], []string{"prefix", "postfix", "transpiler"}[r.IntN(3)], tt+"."+tm+"()", tt, tm, "")
						}
						for j := 0; j < 1+r.IntN(3); j++ {
							run(insHID, id, f, "com."+strings.ToLower(pre)+"."+strings.ToLower(g.word()))
						}
						smp.dllNames = append(smp.dllNames, filepath.Base(f))
					}
				}
			}
			for _, mm := range []string{"1.6", "1.7"} {
				v := v16
				if mm == "1.7" {
					v = v17
				}
				var dll any
				if m.active && len(dllFiles) > 0 {
					d := dllFiles[""]
					if m.ver {
						d = dllFiles["1.6/"]
						if mm == "1.7" && m.has17 {
							d = dllFiles["1.7/"]
						}
					}
					if id, ok := dllSet[strings.Join(d, ",")]; ok {
						dll = id
					}
				}
				run(insCS, m.id, mm, mkKey(v.folders, "def"), v.folders, len(v.files), 0, defSet[v.folders], dll)
			}
		}
		for _, st := range []*sql.Stmt{insInst, insLin, insSnap, insFile, insEv, insSet, insCS, insDef, insAsm, insType, insH, insHID, insAct} {
			st.Close()
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	// analysis slots with patch facts and findings (three slots: 1.6 two lists, 1.7 one)
	tx, _ := db.Begin()
	var active []string
	rows, _ := tx.Query(`SELECT id FROM bench_active ORDER BY id`)
	for rows.Next() {
		var id string
		rows.Scan(&id)
		active = append(active, id)
	}
	rows.Close()
	r := rand.New(rand.NewPCG(3, 3))
	g := ikGen{r}
	for slot, mm := range []string{"1.6", "1.6", "1.7"} {
		res := ex(tx, `INSERT INTO analysis(environment_id,run_key,game_mm,version,started_at,finished_at,set_key) VALUES(?,?,?,8,1,1,?)`, envID, fmt.Sprint("run", slot), mm, fmt.Sprint("set", slot))
		aid, _ := res.LastInsertId()
		st, _ := tx.Prepare(`INSERT INTO patch_fact(analysis_id,instance_id,seq,file,line,class,xpath,descr,result,hits,targets) VALUES(?,?,?,?,?,?,?,?,?,?,?)`)
		for k := 0; k < int(5433*math.Min(1, float64(p.targetDefs)/760_000)); k++ {
			inst := active[r.IntN(len(active))]
			w := g.title()
			st.Exec(aid, inst, k, "1.6/Patches/"+w+"_Patches.xml", 3+r.IntN(300), []string{"PatchOperationAdd", "PatchOperationReplace", "PatchOperationConditional", "PatchOperationFindMod"}[r.IntN(4)],
				`Defs/ThingDef[defName="`+w+g.word()+`"]/comps`, "add <li Class=\"CompProperties_"+w+"\"> to "+w, []string{"applied", "applied", "applied", "failed", "no-match"}[r.IntN(5)], r.IntN(4), "ThingDef/"+w+g.word())
		}
		st.Close()
		fs, _ := tx.Prepare(`INSERT INTO finding(analysis_id,layer,code,severity,key,subject_instance,data) VALUES(?,?,?,?,?,?,?)`)
		for k := 0; k < 700; k++ {
			inst := active[r.IntN(len(active))]
			fs.Exec(aid, "def", "def.duplicate", "info", fmt.Sprint("k", k), inst, `{"code":"def.duplicate","subject":"`+inst+`","evidence":[{"kind":"declared","ref":"def:ThingDef/`+g.word()+`","file":"Defs/`+g.word()+`.xml"}],"text":"`+strings.Repeat("x", 200)+`"}`)
		}
		fs.Close()
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	t.Logf("PLAN mods=%d defsPerFile=%.1f typesPerDLL=%.0f defSets=%d", len(p.mods), p.defsPerFile, p.typesPerDLL, nSets)
	return smp
}

// ---------- sizes ----------

type ikSize struct {
	file, live int64
	byName     map[string]int64
}

func ikSizes(t *testing.T, db *sql.DB) ikSize {
	t.Helper()
	var pages, psize, free int64
	db.QueryRow(`PRAGMA page_count`).Scan(&pages)
	db.QueryRow(`PRAGMA page_size`).Scan(&psize)
	db.QueryRow(`PRAGMA freelist_count`).Scan(&free)
	out := ikSize{file: pages * psize, live: (pages - free) * psize, byName: map[string]int64{}}
	rows, err := db.Query(`SELECT name, sum(pgsize) FROM dbstat GROUP BY name`)
	if err != nil {
		t.Logf("dbstat unavailable: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		var b int64
		rows.Scan(&n, &b)
		out.byName[n] = b
	}
	return out
}

// ikGroups maps tables and their indexes to report groups.
func ikGroup(db *sql.DB, name string) string {
	var tbl string
	if err := db.QueryRow(`SELECT tbl_name FROM sqlite_schema WHERE name=?`, name).Scan(&tbl); err != nil {
		tbl = name
	}
	switch tbl {
	case "def_fact":
		return "def_fact"
	case "type_fact", "harmony_fact", "harmony_id", "assembly_fact":
		return "dll_facts"
	case "snapshot_file":
		return "snapshot_file"
	case "snapshot_list", "file_blob":
		return "file_blobs"
	case "path", "mod_path":
		return "paths"
	case "fact_set", "content_state", "instance_key":
		return "sets+state+keys"
	case "patch_fact", "finding", "analysis":
		return "analysis"
	case "snapshot":
		return "snapshot"
	}
	return "other"
}

func ikLogSizes(t *testing.T, db *sql.DB, label string, s ikSize) map[string]int64 {
	groups := map[string]int64{}
	var names []string
	for n, b := range s.byName {
		groups[ikGroup(db, n)] += b
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return s.byName[names[i]] > s.byName[names[j]] })
	var parts []string
	for _, k := range []string{"def_fact", "dll_facts", "snapshot_file", "file_blobs", "paths", "sets+state+keys", "analysis", "snapshot", "other"} {
		parts = append(parts, fmt.Sprintf("%s=%.2f", k, float64(groups[k])/1e6))
	}
	t.Logf("SIZE %s file=%.2fMB live=%.2fMB | %s", label, float64(s.file)/1e6, float64(s.live)/1e6, strings.Join(parts, " "))
	var top []string
	for _, n := range names[:min(14, len(names))] {
		top = append(top, fmt.Sprintf("%s=%.2f", n, float64(s.byName[n])/1e6))
	}
	t.Logf("SIZE %s top: %s", label, strings.Join(top, " "))
	return groups
}

// ---------- layouts ----------

type ikLayout struct {
	name  string
	ik    bool // instance_key mapping; fact_set / content_state / patch_fact hold integers
	path  int  // 0 text, 1 global path (UNIQUE text), 2 global path (hash index), 3 per-instance path, 4 file list per fact set (facts only)
	blob  int  // 0 rows, 1 non-semantic rows in one zstd blob per snapshot, 2 blobs shared by content
	slim  int  // 1: inactive instances keep only the Def name layer; 2: inactive drop parent / abstract
	kinds bool // snapshot.kinds / snapshot.langs derived at write time (comes with blob)
}

func (l ikLayout) inst() (col, join string) {
	if l.ik {
		return "k.id", " JOIN instance_key k ON k.n=c.inst"
	}
	return "c.instance_id", ""
}

func (l ikLayout) instIs() string {
	if l.ik {
		return "c.inst=(SELECT n FROM instance_key WHERE id=?)"
	}
	return "c.instance_id=?"
}

// file returns the expression and join for a fact row's file (f alias, instance int c.inst).
func (l ikLayout) file(instExpr string) (col, join string) {
	switch l.path {
	case 1, 2:
		return "coalesce(p.rel,'')", " LEFT JOIN path p ON p.id=f.file_id"
	case 3:
		return "coalesce(p.rel,'')", " LEFT JOIN mod_path p ON p.inst=" + instExpr + " AND p.id=f.file_id"
	case 4:
		return "coalesce(json_extract(fs.files, '$[' || f.file_id || ']'),'')", " JOIN fact_set fs ON fs.id=f.set_id"
	}
	return "f.file", ""
}

// rel returns the expression and join for snapshot_file f's path; instExpr is the instance int.
func (l ikLayout) rel(instExpr string) (col, join string) {
	switch l.path {
	case 4:
		return "f.rel_path", ""
	case 1, 2:
		return "p.rel", " JOIN path p ON p.id=f.path_id"
	case 3:
		return "p.rel", " JOIN mod_path p ON p.inst=" + instExpr + " AND p.id=f.path_id"
	}
	return "f.rel_path", ""
}

// ---------- conversions (the prototype "migrations") ----------

func ikExec(t *testing.T, tx *sql.Tx, qs ...string) {
	t.Helper()
	for _, q := range qs {
		for _, s := range strings.Split(q, ";\n") {
			if strings.TrimSpace(s) == "" {
				continue
			}
			if _, err := tx.Exec(s); err != nil {
				t.Fatalf("%v\n%s", err, s)
			}
		}
	}
}

func ikConvertInstKey(t *testing.T, tx *sql.Tx) {
	ikExec(t, tx, `
CREATE TABLE instance_key(n INTEGER PRIMARY KEY, id TEXT NOT NULL UNIQUE);
INSERT INTO instance_key(id) SELECT id FROM instance UNION SELECT instance_id FROM fact_set UNION SELECT instance_id FROM content_state UNION SELECT instance_id FROM patch_fact ORDER BY 1;
CREATE TABLE fact_set2(id INTEGER PRIMARY KEY, inst INTEGER NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('def','dll')), content_key TEXT NOT NULL, UNIQUE(inst,kind,content_key));
INSERT INTO fact_set2 SELECT f.id, k.n, f.kind, f.content_key FROM fact_set f JOIN instance_key k ON k.id=f.instance_id;
DROP TABLE fact_set;
ALTER TABLE fact_set2 RENAME TO fact_set;
CREATE TABLE content_state2(
 inst INTEGER NOT NULL, game_mm TEXT NOT NULL, content_key TEXT NOT NULL, folders TEXT NOT NULL DEFAULT '',
 def_files INTEGER NOT NULL DEFAULT 0, patch_files INTEGER NOT NULL DEFAULT 0, parse_errors TEXT NOT NULL DEFAULT '',
 def_set_id INTEGER NOT NULL REFERENCES fact_set(id), dll_set_id INTEGER REFERENCES fact_set(id),
 PRIMARY KEY(inst,game_mm));
INSERT INTO content_state2 SELECT k.n, c.game_mm, c.content_key, c.folders, c.def_files, c.patch_files, c.parse_errors, c.def_set_id, c.dll_set_id FROM content_state c JOIN instance_key k ON k.id=c.instance_id;
DROP TABLE content_state;
ALTER TABLE content_state2 RENAME TO content_state;
CREATE INDEX content_state_def ON content_state(def_set_id,game_mm);
CREATE INDEX content_state_dll ON content_state(dll_set_id,game_mm);
CREATE INDEX content_state_mm ON content_state(game_mm,inst);
CREATE TABLE patch_fact2(
  analysis_id INTEGER NOT NULL, inst INTEGER NOT NULL, seq INTEGER NOT NULL, file TEXT NOT NULL, line INTEGER NOT NULL,
  class TEXT NOT NULL, xpath TEXT NOT NULL DEFAULT '', descr TEXT NOT NULL, result TEXT NOT NULL, hits INTEGER NOT NULL,
  reasons TEXT NOT NULL DEFAULT '', detail TEXT NOT NULL DEFAULT '', targets TEXT NOT NULL DEFAULT '', leaf TEXT NOT NULL DEFAULT '');
INSERT INTO patch_fact2 SELECT p.analysis_id, k.n, p.seq, p.file, p.line, p.class, p.xpath, p.descr, p.result, p.hits, p.reasons, p.detail, p.targets, p.leaf FROM patch_fact p JOIN instance_key k ON k.id=p.instance_id ORDER BY p.rowid;
DROP TABLE patch_fact;
ALTER TABLE patch_fact2 RENAME TO patch_fact;
CREATE INDEX patch_fact_analysis ON patch_fact(analysis_id, inst)`)
}

const ikFactTables = `def_fact:set_id,def_type,def_name,name,parent,abstract,FILE,line,may_require,may_require_any|` +
	`assembly_fact:set_id,FILE,skipped,manual,uses_harmony,types,patches,error|` +
	`type_fact:set_id,FILE,type_name|` +
	`harmony_fact:set_id,FILE,patch_type,patch_method,kind,target,target_type,target_method,args,skips,dynamic|` +
	`harmony_id:set_id,FILE,id`

func ikFactDDL(table string) string {
	switch table {
	case "def_fact":
		return `CREATE TABLE def_fact2(set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE,
 def_type TEXT NOT NULL, def_name TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '',
 parent TEXT NOT NULL DEFAULT '', abstract INTEGER NOT NULL DEFAULT 0, file_id INTEGER, line INTEGER NOT NULL,
 may_require TEXT NOT NULL DEFAULT '', may_require_any TEXT NOT NULL DEFAULT '');
CREATE INDEX def_fact_set ON def_fact2(set_id);
CREATE INDEX def_fact_lookup ON def_fact2(def_name,set_id);
CREATE INDEX def_fact_named ON def_fact2(name,set_id) WHERE name<>''`
	case "assembly_fact":
		return `CREATE TABLE assembly_fact2(set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE,
 file_id INTEGER, skipped INTEGER NOT NULL DEFAULT 0, manual INTEGER NOT NULL DEFAULT 0,
 uses_harmony INTEGER NOT NULL DEFAULT 0, types INTEGER NOT NULL DEFAULT 0, patches INTEGER NOT NULL DEFAULT 0, error TEXT NOT NULL DEFAULT '');
CREATE INDEX assembly_fact_set ON assembly_fact2(set_id)`
	case "type_fact":
		return `CREATE TABLE type_fact2(set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE, file_id INTEGER, type_name TEXT NOT NULL);
CREATE INDEX type_fact_set ON type_fact2(set_id);
CREATE INDEX type_fact_lookup ON type_fact2(type_name,set_id)`
	case "harmony_fact":
		return `CREATE TABLE harmony_fact2(set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE,
 file_id INTEGER,patch_type TEXT NOT NULL,patch_method TEXT NOT NULL,kind TEXT NOT NULL,
 target TEXT NOT NULL,target_type TEXT NOT NULL DEFAULT '',target_method TEXT NOT NULL DEFAULT '',args TEXT NOT NULL DEFAULT '',
 skips INTEGER NOT NULL DEFAULT 0,dynamic INTEGER NOT NULL DEFAULT 0);
CREATE INDEX harmony_fact_set ON harmony_fact2(set_id)`
	case "harmony_id":
		return `CREATE TABLE harmony_id2(set_id INTEGER NOT NULL REFERENCES fact_set(id) ON DELETE CASCADE,file_id INTEGER,id TEXT NOT NULL COLLATE NOCASE);
CREATE INDEX harmony_id_set ON harmony_id2(set_id);
CREATE INDEX harmony_id_lookup ON harmony_id2(id,set_id)`
	}
	panic(table)
}
