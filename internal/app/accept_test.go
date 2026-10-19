package app_test

// Acceptance tests A1–A11 of 02 §9, plus residue, identity questions, lineage editing and gc.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rimarcho/rimarcho/internal/identity"
	"github.com/rimarcho/rimarcho/internal/scan"
	"github.com/rimarcho/rimarcho/internal/source"
	"github.com/rimarcho/rimarcho/internal/store"
	"github.com/rimarcho/rimarcho/internal/testkit"
)

func setup(t *testing.T, steamcmd bool) *testkit.Env {
	e := testkit.New(t)
	e.AddEnv(steamcmd)
	return e
}

func scanNow(t *testing.T, e *testkit.Env) int64 {
	t.Helper()
	r, err := e.App.Scan(false)
	if err != nil {
		t.Fatal(err)
	}
	return r.ScanID
}

func allMods(t *testing.T, e *testkit.Env) []*store.Mod {
	t.Helper()
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	return ix.Mods
}

func named(t *testing.T, e *testkit.Env, name string) []*store.Mod {
	t.Helper()
	var out []*store.Mod
	for _, m := range allMods(t, e) {
		if m.Name == name {
			out = append(out, m)
		}
	}
	return out
}

func one(t *testing.T, e *testkit.Env, name string) *store.Mod {
	t.Helper()
	ms := named(t, e, name)
	if len(ms) != 1 {
		t.Fatalf("want exactly one instance named %q, got %d", name, len(ms))
	}
	return ms[0]
}

// eventsOf returns "kind" or "kind:instance" strings of one scan.
func eventsOf(t *testing.T, e *testkit.Env, scanID int64) []*store.Event {
	t.Helper()
	ix, _ := e.App.Index()
	evs, err := e.App.Store.Events(store.EventFilter{EnvID: ix.Env.ID, MinScan: scanID, MaxScan: scanID})
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func kinds(evs []*store.Event) map[string]int {
	m := map[string]int{}
	for _, e := range evs {
		m[e.Kind]++
	}
	return m
}

func wantKinds(t *testing.T, evs []*store.Event, want map[string]int) {
	t.Helper()
	got := kinds(evs)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		var lines []string
		for _, e := range evs {
			lines = append(lines, e.Kind+" "+e.InstanceID[:6]+" "+e.Detail)
		}
		t.Fatalf("events = %v, want %v\n%s", got, want, strings.Join(lines, "\n"))
	}
}

// A1: first scan of 300+ mods — one Instance per folder, one discovered event each.
func TestA1FirstScanManyMods(t *testing.T) {
	e := setup(t, false)
	for i := 0; i < 300; i++ {
		id := fmt.Sprint(3000000000 + i)
		e.AddWorkshop(id, testkit.Sample(fmt.Sprintf("tester.ws%d", i), fmt.Sprintf("Workshop Mod %d", i)))
	}
	for i := 0; i < 20; i++ {
		e.WriteMod(filepath.Join(e.Game, "Mods", fmt.Sprintf("Local%02d", i)), testkit.Sample(fmt.Sprintf("tester.local%d", i), fmt.Sprintf("Local Mod %d", i)))
	}
	sid := scanNow(t, e)
	mods := allMods(t, e)
	if len(mods) != 321 { // + Core
		t.Fatalf("instances = %d, want 321", len(mods))
	}
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvDiscovered: 321})
	seen := map[string]bool{}
	for _, m := range mods {
		k := m.SourceLabel + "/" + m.Rel
		if seen[k] {
			t.Fatalf("two instances at %s", k)
		}
		seen[k] = true
		if m.Status != identity.Live || len(m.ID) != 26 {
			t.Fatalf("bad instance %+v", m)
		}
	}
	// An unchanged rescan is quiet and reuses every hash.
	r, err := e.App.Scan(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventsOf(t, e, r.ScanID)) != 0 || r.Stats.Hashed != 0 || r.Stats.Reused == 0 {
		t.Fatalf("rescan: events %v, hashed %d, reused %d", kinds(eventsOf(t, e, r.ScanID)), r.Stats.Hashed, r.Stats.Reused)
	}
	// --full re-hashes but still finds nothing new.
	r, err = e.App.Scan(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventsOf(t, e, r.ScanID)) != 0 || r.Stats.Hashed == 0 {
		t.Fatalf("full rescan: events %v, hashed %d", kinds(eventsOf(t, e, r.ScanID)), r.Stats.Hashed)
	}
}

// A2: the workshop updates a mod — same Instance, `updated`.
func TestA2WorkshopUpdate(t *testing.T) {
	e := setup(t, false)
	dir := e.AddWorkshop("1001", testkit.Sample("tester.alpha", "Alpha"))
	scanNow(t, e)
	before := one(t, e, "Alpha")
	e.WriteFile(filepath.Join(dir, "Defs", "ThingDefs", "alpha.xml"), "<Defs><ThingDef><defName>alpha_Thing</defName><label>v2</label></ThingDef></Defs>\n")
	e.WriteFile(filepath.Join(dir, "Defs", "New.xml"), "<Defs/>\n")
	sid := scanNow(t, e)
	after := one(t, e, "Alpha")
	if after.ID != before.ID || after.LineageID != before.LineageID {
		t.Fatalf("identity changed: %s → %s", before.ID, after.ID)
	}
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvUpdated: 1})
	// A texture-only change is also an update (size layer), a touch without change is not.
	os.Chtimes(filepath.Join(dir, "Defs", "New.xml"), time.Now(), time.Now().Add(time.Hour))
	sid = scanNow(t, e)
	wantKinds(t, eventsOf(t, e, sid), map[string]int{})
}

// Steam re-downloads an item without content change: a new snapshot records the new
// manifest, its file list is copied from the previous snapshot, and nothing is reported.
func TestManifestOnlyChange(t *testing.T) {
	e := setup(t, false)
	e.AddWorkshop("1012", testkit.Sample("tester.same", "Same"))
	scanNow(t, e)
	before := one(t, e, "Same")
	e.SetManifest("1012", 777)
	sid := scanNow(t, e)
	wantKinds(t, eventsOf(t, e, sid), map[string]int{})
	after := one(t, e, "Same")
	if after.SnapID == before.SnapID || after.ACFManifest != "777" || after.FileCount != before.FileCount || after.FileCount == 0 {
		t.Fatalf("snapshot %d→%d manifest %q files %d→%d", before.SnapID, after.SnapID, after.ACFManifest, before.FileCount, after.FileCount)
	}
	sem, err := e.App.Store.SemanticSet(after.SnapID)
	if err != nil || len(sem) != after.SemanticCount || len(sem) == 0 {
		t.Fatalf("copied manifest: %d semantic rows, want %d (%v)", len(sem), after.SemanticCount, err)
	}
}

// A3: the author changes the packageId in an update — same Instance, `package-id-changed`,
// lineage (where Loadout entries and user rules hang) unchanged.
func TestA3PackageIDChange(t *testing.T) {
	e := setup(t, false)
	m := testkit.Sample("oldauthor.thing", "Thing")
	dir := e.AddWorkshop("1002", m)
	scanNow(t, e)
	before := one(t, e, "Thing")
	m.PackageID = "newauthor.thing"
	m.Author = "New Author"
	e.WriteFile(filepath.Join(dir, "About", "About.xml"), m.AboutXML())
	sid := scanNow(t, e)
	after := one(t, e, "Thing")
	if after.ID != before.ID || after.LineageID != before.LineageID || after.PackageID != "newauthor.thing" {
		t.Fatalf("got %s/%s pkg %s, want %s/%s", after.ID, after.LineageID, after.PackageID, before.ID, before.LineageID)
	}
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvUpdated: 1, identity.EvPackageIDChanged: 1, identity.EvAuthorChanged: 1})
}

// A4: the whole game folder moves to another drive and the environment path is updated —
// sources re-bind, every Instance ID stays.
func TestA4GameFolderMoved(t *testing.T) {
	e := setup(t, false)
	for i := 0; i < 6; i++ {
		e.WriteMod(filepath.Join(e.Game, "Mods", fmt.Sprintf("Local%d", i)), testkit.Sample(fmt.Sprintf("tester.l%d", i), fmt.Sprintf("Local %d", i)))
	}
	e.AddWorkshop("1003", testkit.Sample("tester.w", "Workshop W"))
	scanNow(t, e)
	ids := map[string]string{}
	for _, m := range allMods(t, e) {
		ids[m.Name] = m.ID
	}
	newGame := filepath.Join(e.Root, "OtherDrive", "Games", "RimWorld")
	e.Move(e.Game, newGame)
	// Scanning before the path is fixed must not tombstone anything: the roots are missing.
	sid := scanNow(t, e)
	wantKinds(t, eventsOf(t, e, sid), map[string]int{})
	out := e.MustRun("env", "set", "main", "--game", newGame)
	if strings.Count(out, "  rebound  ") != 2 {
		t.Fatalf("expected official and local to re-bind:\n%s", out)
	}
	sid = scanNow(t, e)
	wantKinds(t, eventsOf(t, e, sid), map[string]int{})
	mods := allMods(t, e)
	if len(mods) != len(ids) {
		t.Fatalf("instances = %d, want %d", len(mods), len(ids))
	}
	for _, m := range mods {
		if ids[m.Name] != m.ID || m.Status != identity.Live {
			t.Fatalf("%s: id %s status %s, want %s live", m.Name, m.ID, m.Status, ids[m.Name])
		}
	}
}

// A4 (negative): a new root with different mods is not re-bound automatically.
func TestA4RebindRejected(t *testing.T) {
	e := setup(t, false)
	for i := 0; i < 5; i++ {
		e.WriteMod(filepath.Join(e.Game, "Mods", fmt.Sprintf("Local%d", i)), testkit.Sample(fmt.Sprintf("tester.l%d", i), fmt.Sprintf("Local %d", i)))
	}
	scanNow(t, e)
	other := filepath.Join(e.Root, "Other", "RimWorld")
	e.CopyTemplate("core", filepath.Join(other, "Data", "Core"))
	e.WriteFile(filepath.Join(other, "RimWorldWin64.exe"), "")
	for i := 0; i < 5; i++ {
		e.WriteMod(filepath.Join(other, "Mods", fmt.Sprintf("Other%d", i)), testkit.Sample(fmt.Sprintf("tester.o%d", i), fmt.Sprintf("Other %d", i)))
	}
	out := e.MustRun("env", "set", "main", "--game", other)
	if !strings.Contains(out, "src:local  rebind-pending") || !strings.Contains(out, "src:official  rebound") {
		t.Fatalf("want official rebound, local pending:\n%s", out)
	}
	sid := scanNow(t, e) // pending source is skipped: nothing disappears
	wantKinds(t, eventsOf(t, e, sid), map[string]int{})
	e.MustRun("env", "rebind", "src:local", "no")
	sid = scanNow(t, e)
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvDisappeared: 5, identity.EvDiscovered: 5})
}

// A5: a workshop mod copied by hand into Mods/ — new Instance in the same lineage, inferred.
func TestA5ManualCopy(t *testing.T) {
	e := setup(t, false)
	src := e.AddWorkshop("1005", testkit.Sample("tester.copyme", "Copy Me"))
	scanNow(t, e)
	e.CopyTree(src, filepath.Join(e.Game, "Mods", "Copy Me Local"))
	sid := scanNow(t, e)
	ms := named(t, e, "Copy Me")
	if len(ms) != 2 || ms[0].LineageID != ms[1].LineageID {
		t.Fatalf("want 2 instances in one lineage, got %d", len(ms))
	}
	var local *store.Mod
	for _, m := range ms {
		if m.Kind == source.Local {
			local = m
		}
	}
	if local.LineageBasis != identity.BasisInferred {
		t.Fatalf("basis = %q, want inferred", local.LineageBasis)
	}
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvCopied: 1})
	// The workshop copy stays the lineage's primary.
	ix, _ := e.App.Index()
	if p := ix.Lineages[local.LineageID].PrimaryID; ix.ByID[p].Kind != source.Workshop {
		t.Fatalf("primary is %s, want the workshop instance", ix.ByID[p].SourceLabel)
	}
	// Same on a first scan where both already exist.
	e2 := setup(t, false)
	s2 := e2.AddWorkshop("1005", testkit.Sample("tester.copyme", "Copy Me"))
	e2.CopyTree(s2, filepath.Join(e2.Game, "Mods", "Copy Me Local"))
	sid = scanNow(t, e2)
	wantKinds(t, eventsOf(t, e2, sid), map[string]int{identity.EvDiscovered: 2, identity.EvCopied: 1})
}

// A6: a copy carrying .rimarcho/origin.json (as copy-local writes it) — confirmed membership.
func TestA6OriginMarker(t *testing.T) {
	e := setup(t, false)
	src := e.AddWorkshop("1006", testkit.Sample("tester.origin", "Origin"))
	scanNow(t, e)
	orig := one(t, e, "Origin")
	dst := filepath.Join(e.Game, "Mods", "Origin (local)")
	e.CopyTree(src, dst)
	// The player edits the copy heavily: similarity alone would no longer link it.
	e.Remove(filepath.Join(dst, "Defs"))
	e.Remove(filepath.Join(dst, "Patches"))
	e.WriteFile(filepath.Join(dst, "Defs", "Mine.xml"), "<Defs/>\n")
	b, _ := json.Marshal(scan.Origin{InstanceID: orig.ID, LineageID: orig.LineageID, WorkshopID: "1006"})
	e.WriteFile(filepath.Join(dst, ".rimarcho", "origin.json"), string(b))
	sid := scanNow(t, e)
	ms := named(t, e, "Origin")
	if len(ms) != 2 {
		t.Fatalf("instances = %d, want 2", len(ms))
	}
	for _, m := range ms {
		if m.ID != orig.ID && (m.LineageID != orig.LineageID || m.LineageBasis != identity.BasisConfirmed) {
			t.Fatalf("copy lineage %s basis %q, want %s confirmed", m.LineageID, m.LineageBasis, orig.LineageID)
		}
	}
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvCopied: 1})
	// Renaming the marked copy is a move, not a second copy.
	e.Move(dst, filepath.Join(e.Game, "Mods", "Origin renamed"))
	sid = scanNow(t, e)
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvMoved: 1})
}

// A7: a local mod folder is renamed — same Instance, `moved`.
func TestA7LocalRename(t *testing.T) {
	e := setup(t, false)
	dir := filepath.Join(e.Game, "Mods", "My Mod")
	e.WriteMod(dir, testkit.Sample("me.mymod", "My Mod"))
	scanNow(t, e)
	before := one(t, e, "My Mod")
	e.Move(dir, filepath.Join(e.Game, "Mods", "My Mod v2"))
	sid := scanNow(t, e)
	after := one(t, e, "My Mod")
	if after.ID != before.ID || after.Rel != "My Mod v2" {
		t.Fatalf("got %s at %s, want %s at My Mod v2", after.ID, after.Rel, before.ID)
	}
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvMoved: 1})
	// Rename plus an edit in the same scan is still a move (similarity ≥ 0.85 on paths).
	e.WriteFile(filepath.Join(e.Game, "Mods", "My Mod v2", "Defs", "ThingDefs", "mymod.xml"), "<Defs><!-- edited --></Defs>" + nl)
	e.Move(filepath.Join(e.Game, "Mods", "My Mod v2"), filepath.Join(e.Game, "Mods", "My Mod v3"))
	sid = scanNow(t, e)
	if one(t, e, "My Mod").ID != before.ID {
		t.Fatal("identity lost on rename + edit")
	}
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvMoved: 1, identity.EvUpdated: 1})
	// Rename plus a new semantic file in a 4-file mod drops path similarity to 0.8: below
	// R3's 0.85, so RimArcho asks (R6) instead of guessing.
	e.WriteFile(filepath.Join(e.Game, "Mods", "My Mod v3", "Defs", "Extra.xml"), "<Defs/>" + nl)
	e.Move(filepath.Join(e.Game, "Mods", "My Mod v3"), filepath.Join(e.Game, "Mods", "My Mod v4"))
	sid = scanNow(t, e)
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvDiscovered: 1, identity.EvDisappeared: 1})
	if qs, _ := e.App.Store.Questions(mustEnv(t, e), false); len(qs) != 1 || qs[0].CandidateID != before.ID {
		t.Fatalf("questions = %+v", qs)
	}
}

// A8: unsubscribe, then re-subscribe within 30 days — same Instance, `reappeared`.
func TestA8Resubscribe(t *testing.T) {
	e := setup(t, false)
	dir := e.AddWorkshop("1008", testkit.Sample("tester.back", "Comes Back"))
	scanNow(t, e)
	before := one(t, e, "Comes Back")
	backup := filepath.Join(e.Root, "backup")
	e.Move(dir, backup)
	e.Unsubscribe("1008")
	e.Advance(24 * time.Hour)
	sid := scanNow(t, e)
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvDisappeared: 1})
	if m := one(t, e, "Comes Back"); m.Status != identity.Tombstone {
		t.Fatalf("status %s, want tombstone", m.Status)
	}
	e.Advance(29 * 24 * time.Hour)
	e.Move(backup, dir)
	e.Subscribe("1008")
	sid = scanNow(t, e)
	after := one(t, e, "Comes Back")
	if after.ID != before.ID || after.Status != identity.Live {
		t.Fatalf("got %s %s, want %s live", after.ID, after.Status, before.ID)
	}
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvReappeared: 1})
}

// A9: the same workshop ID in the Steam client folder and a SteamCMD folder — two Instances,
// one lineage.
func TestA9SteamAndSteamCMD(t *testing.T) {
	e := setup(t, true)
	m := testkit.Sample("tester.dual", "Dual")
	e.AddWorkshop("1009", m)
	e.WriteMod(filepath.Join(e.SteamCMD, "1009"), m)
	sid := scanNow(t, e)
	ms := named(t, e, "Dual")
	if len(ms) != 2 || ms[0].LineageID != ms[1].LineageID || ms[0].ID == ms[1].ID {
		t.Fatalf("want two instances in one lineage, got %d", len(ms))
	}
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvDiscovered: 2, identity.EvCopied: 1})
	sid = scanNow(t, e)
	wantKinds(t, eventsOf(t, e, sid), map[string]int{})
}

// A10: two unrelated mods declaring the same packageId — two Instances, two lineages; a
// pkg: reference lists both instead of picking one.
func TestA10SamePackageIDUnrelated(t *testing.T) {
	e := setup(t, false)
	e.AddWorkshop("1010", testkit.Sample("common.name", "First Author Mod"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Other"), testkit.Sample("common.name", "Second Author Mod"))
	scanNow(t, e)
	a, b := one(t, e, "First Author Mod"), one(t, e, "Second Author Mod")
	if a.LineageID == b.LineageID {
		t.Fatal("unrelated mods share a lineage")
	}
	out, code := e.Run("mod", "pkg:common.name")
	if code != 3 || !strings.Contains(out, "matches 2 instances") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}

// A11: one directory reachable from two sources through a junction — one Instance with one
// alias location.
func TestA11Junction(t *testing.T) {
	e := setup(t, false)
	target := e.AddWorkshop("1011", testkit.Sample("tester.linked", "Linked"))
	e.Junction(filepath.Join(e.Game, "Mods", "_Link_1011"), target)
	sid := scanNow(t, e)
	m := one(t, e, "Linked")
	if m.Kind != source.Workshop {
		t.Fatalf("primary location is %s, want the real (workshop) directory", m.SourceLabel)
	}
	al, err := e.App.Store.Aliases(m.ID)
	if err != nil || len(al) != 1 || al[0].SourceLabel != "local" || al[0].Rel != "_Link_1011" {
		t.Fatalf("aliases = %+v, %v", al, err)
	}
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvDiscovered: 2}) // + Core
	sid = scanNow(t, e)
	wantKinds(t, eventsOf(t, e, sid), map[string]int{})
	// A junction into a folder outside every source is just a local mod that links out.
	outside := filepath.Join(e.Root, "Elsewhere", "Deployed")
	e.WriteMod(outside, testkit.Sample("tester.deployed", "Deployed"))
	e.Junction(filepath.Join(e.Game, "Mods", "_Link_Deployed"), outside)
	scanNow(t, e)
	d := one(t, e, "Deployed")
	want, _ := source.RealPath(outside) // expands 8.3 names of the temp dir
	if d.Kind != source.Local || !strings.EqualFold(d.RealPath, want) {
		t.Fatalf("deployed: %s real %s", d.SourceLabel, d.RealPath)
	}
}

// Residue: an unsubscribed workshop folder with About.xml is an Instance flagged residue;
// one without About.xml is only counted.
func TestResidue(t *testing.T) {
	e := setup(t, false)
	e.AddWorkshop("2001", testkit.Sample("tester.sub", "Subscribed"))
	e.WriteMod(filepath.Join(e.Workshop, "2002"), testkit.Sample("tester.left", "Left Behind"))
	e.WriteFile(filepath.Join(e.Workshop, "2003", "Textures", "x.png"), "x")
	r, err := e.App.Scan(false)
	if err != nil {
		t.Fatal(err)
	}
	if !one(t, e, "Left Behind").Residue || one(t, e, "Subscribed").Residue {
		t.Fatal("residue flags wrong")
	}
	var ws struct{ Residue, ResidueNoAbout int }
	for _, s := range r.Stats.Sources {
		if s.Label == "workshop" {
			ws.Residue, ws.ResidueNoAbout = s.Residue, s.ResidueNoAbout
		}
	}
	if ws.Residue != 1 || ws.ResidueNoAbout != 1 {
		t.Fatalf("workshop stats %+v", ws)
	}
}

// R6 + identity resolve: a vanished mod reappears elsewhere under a new packageId — RimArcho
// asks instead of guessing; "same" restores the original identity.
func TestQuestionResolveSame(t *testing.T) {
	e := setup(t, false)
	dir := filepath.Join(e.Game, "Mods", "Gone")
	e.WriteMod(dir, testkit.Sample("tester.gone", "Gone"))
	scanNow(t, e)
	orig := one(t, e, "Gone")
	e.Remove(dir)
	scanNow(t, e)
	m := testkit.Sample("tester.renamed", "Gone")
	e.WriteMod(filepath.Join(e.Game, "Mods", "Elsewhere"), m)
	sid := scanNow(t, e)
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvDiscovered: 1})
	qs, _ := e.App.Store.Questions(mustEnv(t, e), false)
	if len(qs) != 1 || qs[0].CandidateID != orig.ID {
		t.Fatalf("questions = %+v", qs)
	}
	e.MustRun("identity", "resolve", fmt.Sprintf("q:%d", qs[0].ID), "same")
	ms := named(t, e, "Gone")
	if len(ms) != 1 || ms[0].ID != orig.ID || ms[0].Status != identity.Live || ms[0].Rel != "Elsewhere" {
		t.Fatalf("after resolve: %+v", ms)
	}
	if qs, _ := e.App.Store.Questions(mustEnv(t, e), false); len(qs) != 0 {
		t.Fatal("question still open")
	}
	// Further scans keep the merged identity.
	sid = scanNow(t, e)
	wantKinds(t, eventsOf(t, e, sid), map[string]int{})
}

const nl = string(rune(10))

func mustEnv(t *testing.T, e *testkit.Env) string {
	env, err := e.App.Env()
	if err != nil {
		t.Fatal(err)
	}
	return env.ID
}

// Lineage split and merge.
func TestLineageSplitMerge(t *testing.T) {
	e := setup(t, false)
	src := e.AddWorkshop("3001", testkit.Sample("tester.fam", "Family"))
	e.CopyTree(src, filepath.Join(e.Game, "Mods", "Family Copy"))
	scanNow(t, e)
	ms := named(t, e, "Family")
	if ms[0].LineageID != ms[1].LineageID {
		t.Fatal("copy not in lineage")
	}
	var local *store.Mod
	for _, m := range ms {
		if m.Kind == source.Local {
			local = m
		}
	}
	ix, _ := e.App.Index()
	e.MustRun("lineage", "split", ix.Ref(local.ID))
	ms = named(t, e, "Family")
	if ms[0].LineageID == ms[1].LineageID {
		t.Fatal("split did not separate")
	}
	ix, _ = e.App.Index()
	e.MustRun("lineage", "merge", ix.LRef(ms[0].LineageID), ix.Ref(ms[1].ID))
	ms = named(t, e, "Family")
	if ms[0].LineageID != ms[1].LineageID {
		t.Fatal("merge did not join")
	}
	// The manual decision survives rescans.
	scanNow(t, e)
	if ms = named(t, e, "Family"); ms[0].LineageID != ms[1].LineageID {
		t.Fatal("rescan undid merge")
	}
}

// Tombstones are kept for the configured retention, then gc removes them unless an open
// question refers to them.
func TestTombstoneGC(t *testing.T) {
	e := setup(t, false)
	a := filepath.Join(e.Game, "Mods", "A")
	b := filepath.Join(e.Game, "Mods", "B")
	e.WriteMod(a, testkit.Sample("tester.a", "Mod A"))
	e.WriteMod(b, testkit.Sample("tester.b", "Mod B"))
	scanNow(t, e)
	e.Remove(a)
	e.Remove(b)
	scanNow(t, e)
	// B's look-alike shows up and raises a question that keeps B's tombstone alive.
	e.WriteMod(filepath.Join(e.Game, "Mods", "B2"), testkit.Sample("tester.b2", "Mod B"))
	scanNow(t, e)
	e.Advance(89 * 24 * time.Hour)
	out := e.MustRun("gc")
	if !strings.Contains(out, "removed 0 tombstones") {
		t.Fatalf("gc before expiry:\n%s", out)
	}
	e.Advance(2 * 24 * time.Hour)
	out = e.MustRun("gc")
	if !strings.Contains(out, "removed 1 tombstone ") || !strings.Contains(out, "kept: 1") {
		t.Fatalf("gc after expiry:\n%s", out)
	}
	if len(named(t, e, "Mod A")) != 0 {
		t.Fatal("Mod A tombstone survived gc")
	}
	if len(named(t, e, "Mod B")) != 2 {
		t.Fatal("Mod B tombstone removed despite an open question")
	}
}

// 02 §4.3 (revised): files moved into a version folder with unchanged content still count
// as the same mod (J(hash)); a content-only edit keeps J(path) = 1.
func TestSimilarityRestructureAndContentOnly(t *testing.T) {
	e := setup(t, false)
	dir := filepath.Join(e.Game, "Mods", "Restructure")
	e.WriteMod(dir, testkit.Sample("tester.re", "Restructure"))
	scanNow(t, e)
	before := one(t, e, "Restructure")
	// Rename the folder and move Defs/ and Patches/ under 1.6/ in the same scan.
	nd := filepath.Join(e.Game, "Mods", "Restructure 1.6")
	e.Move(dir, nd)
	e.Move(filepath.Join(nd, "Defs"), filepath.Join(nd, "1.6", "Defs"))
	e.Move(filepath.Join(nd, "Patches"), filepath.Join(nd, "1.6", "Patches"))
	sid := scanNow(t, e)
	if one(t, e, "Restructure").ID != before.ID {
		t.Fatal("restructure + rename lost identity")
	}
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvMoved: 1, identity.EvUpdated: 1})
	// Rename plus every semantic file edited: paths still match.
	for _, f := range []string{"1.6/Defs/ThingDefs/restructure.xml", "1.6/Defs/RecipeDefs/restructure.xml", "1.6/Patches/restructure.xml"} {
		e.WriteFile(filepath.Join(nd, filepath.FromSlash(f)), "<Defs><!-- rewritten --></Defs>")
	}
	e.Move(nd, filepath.Join(e.Game, "Mods", "Restructure final"))
	sid = scanNow(t, e)
	if one(t, e, "Restructure").ID != before.ID {
		t.Fatal("content-only change + rename lost identity")
	}
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvMoved: 1, identity.EvUpdated: 1})
}

// 02 §4.1: RimCrow disables a workshop mod by renaming About.xml and deploys a copy through
// a junction outside every source. The disabled folder is an unloadable Instance, and the
// deployed copy joins its lineage.
func TestAboutDisabled(t *testing.T) {
	e := setup(t, false)
	ws := e.AddWorkshop("4001", testkit.Sample("tester.crow", "Crow Managed"))
	deployed := filepath.Join(e.Root, "RimCrow", "mods", "4001")
	e.CopyTree(ws, deployed)
	e.Move(filepath.Join(ws, "About", "About.xml"), filepath.Join(ws, "About", "About.xml.disabled"))
	e.Junction(filepath.Join(e.Game, "Mods", "_Link_4001"), deployed)
	sid := scanNow(t, e)
	ms := named(t, e, "Crow Managed")
	if len(ms) != 2 || ms[0].LineageID != ms[1].LineageID {
		t.Fatalf("want 2 instances in one lineage, got %d: %s", len(ms), e.MustRun("changes"))
	}
	for _, m := range ms {
		want := ""
		if m.Kind == source.Workshop {
			want = scan.UnloadableAboutDisabled
		}
		if m.Unloadable != want || m.PackageID != "tester.crow" {
			t.Fatalf("%s: unloadable %q pkg %q", m.SourceLabel, m.Unloadable, m.PackageID)
		}
	}
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvDiscovered: 2, identity.EvCopied: 1})
	out := e.MustRun("list", "--filter", "unloadable")
	if !strings.Contains(out, "unloadable: about-disabled") {
		t.Fatal(out)
	}
}

func TestEnvRemoveAndSourceRemove(t *testing.T) {
	e := setup(t, true)
	e.WriteMod(filepath.Join(e.SteamCMD, "5001"), testkit.Sample("tester.cmd", "Via SteamCMD"))
	scanNow(t, e)
	if _, code := e.Run("env", "set", "main", "--remove-source", "src:local"); code == 0 {
		t.Fatal("removing the local source must be refused")
	}
	e.MustRun("env", "set", "main", "--remove-source", "src:steamcmd")
	sid := scanNow(t, e)
	wantKinds(t, eventsOf(t, e, sid), map[string]int{identity.EvDisappeared: 1})
	if _, code := e.Run("env", "remove", "main"); code != 2 {
		t.Fatal("env remove without --yes must be refused")
	}
	e.MustRun("env", "remove", "main", "--yes")
	if out := e.MustRun("env", "list"); !strings.Contains(out, "environments: 0") {
		t.Fatal(out)
	}
	var n int
	e.App.Store.DB.QueryRow(`SELECT (SELECT count(*) FROM instance) + (SELECT count(*) FROM snapshot_file) + (SELECT count(*) FROM source)`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d rows left after env remove", n)
	}
}
