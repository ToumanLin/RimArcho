package store_test

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func factCount(t *testing.T, s *store.Store, q string, args ...any) int {
	t.Helper()
	var n int
	if err := s.DB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestFactSetsShareVersionsAndUpdates(t *testing.T) {
	e := testkit.New(t)
	dir := e.AddWorkshop("9001", testkit.Mod{PackageID: "test.share", Name: "Shared", Files: map[string]string{"Defs/A.xml": `<Defs><ThingDef Name="Base"><defName>SharedDef</defName></ThingDef></Defs>`}})
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze", "--game-version", "1.6")
	e.MustRun("analyze", "--game-version", "1.7")
	s := e.App.Store
	if n := factCount(t, s, `SELECT count(*) FROM def_fact WHERE def_name='SharedDef'`); n != 1 {
		t.Fatalf("stored %d copies", n)
	}
	if n := factCount(t, s, `SELECT count(DISTINCT c.def_set_id) FROM content_state c JOIN def_fact f ON f.set_id=c.def_set_id WHERE f.def_name='SharedDef'`); n != 1 {
		t.Fatal(n)
	}
	for _, mm := range []string{"1.6", "1.7"} {
		s.MM = mm
		fs, err := s.DefsNamed("Base", true)
		if err != nil || len(fs) != 1 || fs[0].DefName != "SharedDef" {
			t.Fatalf("%s: %v %v", mm, fs, err)
		}
	}
	// A snapshot update keeps the other version's old view until it is rebuilt.
	e.WriteFile(filepath.Join(dir, "Defs", "A.xml"), `<Defs><ThingDef Name="NewBase"><defName>UpdatedDef</defName></ThingDef></Defs>`)
	e.MustRun("scan")
	e.MustRun("analyze", "--game-version", "1.6")
	s.MM = "1.7"
	if fs, err := s.DefsNamed("SharedDef", false); err != nil || len(fs) != 1 {
		t.Fatal("old version view lost", err)
	}
	e.MustRun("analyze", "--game-version", "1.7")
	if n := factCount(t, s, `SELECT count(*) FROM def_fact WHERE def_name='SharedDef'`); n != 0 {
		t.Fatal("old set retained", n)
	}
	if n := factCount(t, s, `SELECT count(*) FROM def_fact WHERE def_name='UpdatedDef'`); n != 1 {
		t.Fatal("updated duplicate", n)
	}
	if err := s.PruneFacts(e.App.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestFactSetsDifferentFoldersAndPriority(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("9002", testkit.Mod{PackageID: "test.routes", Name: "Routes", Files: map[string]string{
		"LoadFolders.xml": `<loadFolders><v1.6><li>A</li><li>B</li></v1.6><v1.7><li>B</li><li>A</li></v1.7></loadFolders>`,
		"A/Defs/A.xml":    `<Defs><ThingDef><defName>FromA</defName></ThingDef></Defs>`,
		"B/Defs/A.xml":    `<Defs><ThingDef><defName>FromB</defName></ThingDef></Defs>`,
	}})
	e.AddEnv(false)
	e.MustRun("scan")
	for _, mm := range []string{"1.6", "1.7"} {
		e.MustRun("analyze", "--game-version", mm)
	}
	s := e.App.Store
	for mm, name := range map[string]string{"1.6": "FromB", "1.7": "FromA"} {
		s.MM = mm
		fs, err := s.DefsNamed(name, false)
		if err != nil || len(fs) != 1 {
			t.Fatalf("%s expected %s: %v %v", mm, name, fs, err)
		}
	}
	if n := factCount(t, s, `SELECT count(*) FROM def_fact WHERE def_name IN ('FromA','FromB')`); n != 2 {
		t.Fatal(n)
	}
}

func TestFactSetsHarmonyViews(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.MustRun("analyze", "--game-version", "1.6")
	s := e.App.Store
	s.MM = "1.6"
	patches, err := s.HarmonyPatches(store.HarmonyFilter{})
	if err != nil {
		t.Fatal(err)
	}
	owners, err := s.TypeOwners("ModA.CompBroken")
	if err != nil {
		t.Fatal(err)
	}
	ids, err := s.HarmonyIDOwners("tester.moda.harmony")
	if err != nil {
		t.Fatal(err)
	}
	n := factCount(t, s, `SELECT count(*) FROM type_fact`)
	e.MustRun("analyze", "--game-version", "1.7")
	s.MM = "1.7"
	if got := factCount(t, s, `SELECT count(*) FROM type_fact`); got != n {
		t.Fatalf("types duplicated %d -> %d", n, got)
	}
	p, err := s.HarmonyPatches(store.HarmonyFilter{})
	if err != nil || !reflect.DeepEqual(patches, p) {
		t.Fatal("Harmony changed", err)
	}
	o, err := s.TypeOwners("ModA.CompBroken")
	if err != nil || !reflect.DeepEqual(owners, o) {
		t.Fatal("type owners changed", err)
	}
	i, err := s.HarmonyIDOwners("tester.moda.harmony")
	if err != nil || !reflect.DeepEqual(ids, i) {
		t.Fatal("ID owners changed", err)
	}
}

func TestFactPruneVisibilitySlotsAndEnvRemoval(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	s := e.App.Store
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	// Shared, private to this game, and private to a different game.
	if _, err := s.DB.Exec(`INSERT INTO environment(id,name,game_root,game_version,created_at) VALUES('other','other','other','1.7.5000',1);
 INSERT INTO instance(id,environment_id,lineage_id,source_id,rel_path,status,first_seen,last_seen) VALUES
 ('shared','','l','x','shared','live',1,1),('own',?,'l','x','own','live',1,1),('private','other','l','x','private','live',1,1)`, st.Ix.Env.ID); err != nil {
		t.Fatal(err)
	}
	out := func(mm string) *analyze.Output {
		o := &analyze.Output{Content: map[string]*analyze.ContentState{}, NewDefs: map[string][]analyze.DefFact{}}
		for _, inst := range []string{"shared", "own", "private"} {
			o.Content[inst] = &analyze.ContentState{Key: inst + "-" + mm}
			o.NewDefs[inst] = []analyze.DefFact{{Type: "ThingDef", DefName: inst + mm, File: "A.xml", Line: 1}}
		}
		return o
	}
	save := func(mm, set string, keep []string, at time.Time) {
		t.Helper()
		if _, err := s.SaveAnalysis(st.Ix.Env.ID, set, set, mm, keep, out(mm), at, at); err != nil {
			t.Fatal(err)
		}
	}
	now := e.App.Now()
	save("1.6", "old", []string{"old"}, now)
	save("1.7", "old", []string{"old"}, now)
	// The 1.6 slot cannot keep a private 1.7 game's 1.6 view alive.
	if n := factCount(t, s, `SELECT count(*) FROM content_state WHERE instance_id='private' AND game_mm='1.6'`); n != 0 {
		t.Fatal("foreign private view kept", n)
	}
	// Editing the plan removes the old enabled-set slot in every version.
	save("1.6", "new", []string{"new"}, now)
	if n := factCount(t, s, `SELECT count(*) FROM analysis WHERE set_key='old'`); n != 0 {
		t.Fatal("old slot kept", n)
	}
	if n := factCount(t, s, `SELECT count(*) FROM content_state WHERE instance_id='own' AND game_mm='1.7'`); n != 0 {
		t.Fatal("unused private view kept", n)
	}
	if n := factCount(t, s, `SELECT count(*) FROM content_state WHERE instance_id='shared' AND game_mm='1.7'`); n != 1 {
		t.Fatal("other game's shared version removed", n)
	}
	save("1.5", "new", []string{"new"}, now)
	if err := s.PruneFacts(now.Add(store.ForeignAnalysisRetention + time.Second)); err != nil {
		t.Fatal(err)
	}
	if n := factCount(t, s, `SELECT count(*) FROM analysis WHERE game_mm='1.6'`); n != 1 {
		t.Fatal("current slot expired", n)
	}
	if n := factCount(t, s, `SELECT count(*) FROM analysis WHERE game_mm='1.5'`); n != 0 {
		t.Fatal("foreign slot did not expire", n)
	}
	if n := factCount(t, s, `SELECT count(*) FROM content_state WHERE game_mm='1.5'`); n != 0 {
		t.Fatal("expired slot views kept", n)
	}
	if err := s.DeleteEnv(st.Ix.Env.ID); err != nil {
		t.Fatal(err)
	}
	if n := factCount(t, s, `SELECT count(*) FROM content_state WHERE instance_id='own' OR game_mm='1.6'`); n != 0 {
		t.Fatal("removed game's views kept", n)
	}
	if n := factCount(t, s, `SELECT count(*) FROM content_state WHERE instance_id='shared' AND game_mm='1.7'`); n != 1 {
		t.Fatal("shared view deleted", n)
	}
	if err := s.DeleteEnv("other"); err != nil {
		t.Fatal(err)
	}
	if n := factCount(t, s, `SELECT count(*) FROM fact_set`); n != 0 {
		t.Fatal("unreferenced sets retained", n)
	}
}

func TestFactSetsDifferentAssemblyInputs(t *testing.T) {
	e := testkit.New(t)
	dll, err := filepath.Abs(filepath.Join(testkit.DotnetBin(), "ModA.dll"))
	if err != nil {
		t.Fatal(err)
	}
	e.AddWorkshop("9003", testkit.Mod{PackageID: "test.dll", Name: "DLL"})
	dir := filepath.Join(e.Workshop, "9003")
	e.CopyTree(filepath.Dir(dll), filepath.Join(dir, "Code", "Assemblies"))
	e.WriteFile(filepath.Join(dir, "LoadFolders.xml"), `<loadFolders><v1.6><li>Code</li><li>A</li></v1.6><v1.7><li>Code</li><li>B</li></v1.7></loadFolders>`)
	e.WriteFile(filepath.Join(dir, "A", "Defs", "A.xml"), `<Defs><ThingDef><defName>A</defName></ThingDef></Defs>`)
	e.WriteFile(filepath.Join(dir, "B", "Defs", "A.xml"), `<Defs><ThingDef><defName>B</defName></ThingDef></Defs>`)
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("enable", "pkg:test.dll")
	for _, mm := range []string{"1.6", "1.7"} {
		e.MustRun("analyze", "--game-version", mm)
	}
	s := e.App.Store
	var id string
	if err := s.DB.QueryRow(`SELECT id FROM instance WHERE rel_path='9003'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	// The code mod has different XML folders and one DLL set.
	if n := factCount(t, s, `SELECT count(*) FROM fact_set WHERE instance_id=? AND kind='dll'`, id); n != 1 {
		t.Fatal(n)
	}
	if n := factCount(t, s, `SELECT count(*) FROM fact_set WHERE instance_id=(SELECT id FROM instance WHERE rel_path='9003') AND kind='def'`); n != 2 {
		t.Fatal(n)
	}
	// A change in skip policy produces a different DLL input key.
	m := &analyze.Mod{Dir: dir, Pkg: "test.dll", SnapHash: []byte("snapshot")}
	k := analyze.AssemblyKey(m, []string{filepath.Join(dir, "Code")}, nil)
	k2 := analyze.AssemblyKey(m, []string{filepath.Join(dir, "Code")}, []string{"*.dll"})
	if k == k2 {
		t.Fatal(fmt.Sprintf("skip policy omitted from DLL key: %s", k))
	}
}

func TestFactSetsEmptyAssemblyViewReplacesOld(t *testing.T) {
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.MustRun("analyze")
	s := e.App.Store
	var inst, envID string
	if err := s.DB.QueryRow(`SELECT id,environment_id FROM instance WHERE rel_path='ModA'`).Scan(&inst, &envID); err != nil {
		t.Fatal(err)
	}
	old, err := s.Assemblies(inst)
	if err != nil || len(old) == 0 {
		t.Fatal("missing initial assemblies", err)
	}
	s.MM = "1.6"
	out := &analyze.Output{Content: map[string]*analyze.ContentState{inst: {Key: s.ContentKey(inst), AssemblyKey: "empty-dll-input"}}}
	now := e.App.Now()
	if _, err := s.SaveAnalysis(envID, "empty-dll-test", "empty-dll-test", "1.6", []string{"empty-dll-test"}, out, now, now); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Assemblies(inst); err != nil || len(got) != 0 {
		t.Fatal("old DLL facts survived empty active result", got, err)
	}
	if n := factCount(t, s, `SELECT count(*) FROM assembly_fact f JOIN fact_set x ON x.id=f.set_id WHERE x.instance_id=?`, inst); n != 0 {
		t.Fatal("orphan assembly set kept", n)
	}
}

// 02 §7.2 step 2: maintenance (automatic or the Compact button) runs the same fact
// cleanup as a saved analysis, so an expired slot for another game version goes even
// when nothing is analysed again.
func TestMaintainPrunesFacts(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	s := e.App.Store
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO instance(id,environment_id,lineage_id,source_id,rel_path,status,first_seen,last_seen) VALUES('own',?,'l','x','own','live',1,1)`, st.Ix.Env.ID); err != nil {
		t.Fatal(err)
	}
	now := e.App.Now()
	for _, mm := range []string{"1.6", "1.5"} {
		o := &analyze.Output{
			Content: map[string]*analyze.ContentState{"own": {Key: "own-" + mm}},
			NewDefs: map[string][]analyze.DefFact{"own": {{Type: "ThingDef", DefName: "D" + mm, File: "A.xml", Line: 1}}},
		}
		if _, err := s.SaveAnalysis(st.Ix.Env.ID, "set", "run", mm, []string{"set"}, o, now, now); err != nil {
			t.Fatal(err)
		}
	}
	if n := factCount(t, s, `SELECT count(*) FROM content_state WHERE game_mm='1.5'`); n != 1 {
		t.Fatal("1.5 view not saved", n)
	}
	r, err := s.Maintain(context.Background(), store.MaintenanceOptions{
		Now: now.Add(store.ForeignAnalysisRetention + time.Hour), FreeSpace: func(string) int64 { return 1 << 40 },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Vacuumed {
		t.Fatal("compact did not vacuum")
	}
	if n := factCount(t, s, `SELECT count(*) FROM analysis WHERE game_mm='1.5'`); n != 0 {
		t.Fatal("expired 1.5 slot kept", n)
	}
	if n := factCount(t, s, `SELECT count(*) FROM content_state WHERE game_mm='1.5'`); n != 0 {
		t.Fatal("1.5 view kept", n)
	}
	if n := factCount(t, s, `SELECT count(*) FROM def_fact`); n != 1 {
		t.Fatal("want only the 1.6 Def left, have", n)
	}
	if n := factCount(t, s, `SELECT count(*) FROM analysis WHERE game_mm='1.6'`); n != 1 {
		t.Fatal("current-version slot removed", n)
	}
}
