package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func envLinksFixture(t *testing.T) (*testkit.Env, string) {
	t.Helper()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("enable", "pkg:tester.b")
	e.App.MonitorBundle = monitorBundle(t, e, "0.9.0")
	e.MustRun("monitor", "install")
	e.MustRun("save")
	g2 := secondGame(e, "OtherGame")
	e.MustRun("env", "add", "other", "--game", g2)
	return e, g2
}

func TestEnvUseLinksLocalAndMonitorBeforeImport(t *testing.T) {
	t.Parallel()
	e, g2 := envLinksFixture(t)
	before := e.ReadModsConfig()
	main := active(t, e)
	// No switch into a game that is running; the refused switch changes nothing checked below.
	probe := e.App.GameProbe
	e.App.GameProbe = func(root string) *app.GameRun {
		if root == g2 {
			return &app.GameRun{PID: 42}
		}
		return nil
	}
	if _, err := e.App.EnvUse("other"); err == nil || !strings.Contains(err.Error(), "running") {
		t.Fatalf("switch to running game: %v", err)
	}
	e.App.GameProbe = probe
	e.MustRun("env", "use", "other")
	if got := active(t, e); !slices.Equal(got, main) {
		t.Fatalf("switch lost installed mods: %v; want %v", got, main)
	}
	if got := e.ReadModsConfig(); got != before {
		t.Fatal("switch rewrote ModsConfig.xml")
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"B", monName} {
		var found bool
		for _, c := range e.App.ModsCopies(st.Ix) {
			if c.Mod != nil && c.Mod.Name == name {
				found = c.Link && source.IsLink(filepath.Join(g2, "Mods", c.Name))
			}
		}
		if !found {
			t.Fatalf("%s is not linked into the destination", name)
		}
	}
	// Saving and switching again retain the local link and do not duplicate packages.
	e.MustRun("save")
	scanNow(t, e)
	if got := active(t, e); !slices.Equal(got, main) {
		t.Fatalf("save removed an inherited local mod: %v", got)
	}
	e.MustRun("env", "use", "main")
	e.MustRun("env", "use", "other")
	st, _ = e.App.Loadout()
	if dups := app.ModsDuplicates(e.App.ModsCopies(st.Ix)); len(dups) != 0 {
		t.Fatalf("repeated switch duplicated mods: %+v", dups)
	}
}

func TestEnvUseKeepsDestinationLocalCopy(t *testing.T) {
	t.Parallel()
	e, g2 := envLinksFixture(t)
	e.WriteMod(filepath.Join(g2, "Mods", "MyB"), testkit.Sample("tester.b", "Destination B"))
	e.MustRun("env", "use", "other")
	got := active(t, e)
	if !slices.Contains(got, "Destination B") || slices.Contains(got, "B") {
		t.Fatalf("did not retain destination's local copy: %v", got)
	}
	st, _ := e.App.Loadout()
	if dups := app.ModsDuplicates(e.App.ModsCopies(st.Ix)); len(dups) != 0 {
		t.Fatalf("destination has duplicates: %+v", dups)
	}
}

func TestEnvUseDoesNotInheritIntoEmptyOwnSaves(t *testing.T) {
	t.Parallel()
	e, g2 := envLinksFixture(t)
	e.MustRun("env", "set", "other", "--saves", "own")
	e.MustRun("env", "use", "other")
	entries, err := os.ReadDir(filepath.Join(g2, "Mods"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("empty separate list inherited links: %v, %v", entries, err)
	}
}

func TestEnvUseRollsBackFailedLinks(t *testing.T) {
	e, g2 := envLinksFixture(t)
	create := app.CreateLink
	t.Cleanup(func() { app.CreateLink = create })
	var calls int
	app.CreateLink = func(link, target string) error {
		calls++
		if calls == 2 {
			return errors.New("test link failure")
		}
		return create(link, target)
	}
	if _, err := e.App.EnvUse("other"); err == nil || !strings.Contains(err.Error(), "test link failure") {
		t.Fatalf("switch error: %v", err)
	}
	if cur, _ := e.App.Store.CurrentEnv(); cur == nil || cur.Name != "main" {
		t.Fatalf("failed switch changed current environment: %+v", cur)
	}
	entries, err := os.ReadDir(filepath.Join(g2, "Mods"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed switch left links: %v, %v", entries, err)
	}
	if !source.HasAbout(filepath.Join(e.Game, "Mods", "B")) {
		t.Fatal("rollback touched source files")
	}
}

func TestEnvUseKeepsSelectedMonitorVersion(t *testing.T) {
	t.Parallel()
	e, g2 := envLinksFixture(t)
	e.App.MonitorBundle = monitorBundle(t, e, "0.10.0")
	e.MustRun("monitor", "install")
	e.MustRun("undo") // deliberately keep 0.9.0 in the source plan
	e.MustRun("save")
	e.MustRun("env", "use", "other")
	if !source.IsLink(filepath.Join(g2, "Mods", "_Archo_monitor_ArchoMonitor-0.9.0")) {
		t.Fatal("switch did not preserve the selected Monitor version")
	}
	if _, err := os.Lstat(filepath.Join(g2, "Mods", "_Archo_monitor_ArchoMonitor-0.10.0")); !os.IsNotExist(err) {
		t.Fatalf("switch linked a different Monitor version: %v", err)
	}
}

func TestEnvUsePreservesLocalWorkshopTokens(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("5100", testkit.Sample("tester.same", "Workshop Same"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Same"), testkit.Sample("tester.same", "Local Same"))
	e.AddEnv(false)
	e.WriteModsConfig("1.6", "ludeon.rimworld", "tester.same", "tester.same_steam")
	scanNow(t, e)
	g2 := secondGame(e, "OtherGame")
	e.MustRun("env", "add", "other", "--game", g2)
	e.MustRun("env", "use", "other")
	if got := active(t, e); !slices.Equal(got, []string{"Core", "Local Same", "Workshop Same"}) {
		t.Fatalf("local and Workshop tokens changed their meaning: %v", got)
	}
}

func TestEnvUseScansExistingDestinationWithoutNewLinks(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	scanNow(t, e)
	g2 := secondGame(e, "OtherGame")
	e.WriteMod(filepath.Join(g2, "Mods", "OnlyHere"), testkit.Sample("tester.here", "Only Here"))
	e.MustRun("env", "add", "other", "--game", g2)
	e.WriteModsConfig("1.6", "ludeon.rimworld", "tester.here")
	e.MustRun("env", "use", "other")
	if got := active(t, e); !slices.Equal(got, []string{"Core", "Only Here"}) {
		t.Fatalf("existing destination mod is missing before its first scan: %v", got)
	}
}

func TestEnvUseMonitorKeepsSharedIdentityAcrossSwitches(t *testing.T) {
	t.Parallel()
	e, _ := envLinksFixture(t)
	monitorID := refID(t, e, monName, source.Extra)
	for _, name := range []string{"other", "main", "other", "main", "other"} {
		e.MustRun("env", "use", name)
		st, err := e.App.Loadout()
		if err != nil {
			t.Fatal(err)
		}
		var enabled string
		for _, entry := range st.List {
			if app.IsMonitor(st.Ix.ByID[entry.Instance]) {
				enabled = entry.Instance
			}
		}
		if enabled != monitorID {
			t.Fatalf("%s selected Monitor #%s instead of shared #%s", name, enabled, monitorID)
		}
		m := st.Ix.ByID[monitorID]
		if m == nil || m.Status != identity.Live || !st.Reg.Loadable(monitorID) {
			t.Fatalf("%s cannot load shared Monitor: %+v", name, m)
		}
		e.MustRun("save")
		link := filepath.Join(st.Ix.Env.GameRoot, "Mods", app.DeployName(m))
		if !source.IsLink(link) {
			t.Fatalf("%s save removed Monitor's link", name)
		}
		scanNow(t, e)
	}
	aliases, err := e.App.Store.Aliases(monitorID)
	if err != nil || len(aliases) != 2 {
		t.Fatalf("scanning one environment lost the other's Monitor alias: %+v, %v", aliases, err)
	}
	st, _ := e.App.Loadout()
	link := filepath.Join(st.Ix.Env.GameRoot, "Mods", app.DeployName(st.Ix.ByID[monitorID]))
	if err := source.RemoveLink(link); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.ScanWith(app.ScanOptions{GameOnly: true}); err != nil {
		t.Fatal(err)
	}
	aliases, err = e.App.Store.Aliases(monitorID)
	if err != nil || len(aliases) != 1 || aliases[0].SourceID == sourcesOf(t, e)["local"].ID {
		t.Fatalf("partial scan did not remove just the vanished alias: %+v, %v", aliases, err)
	}
	e.MustRun("save")
	if !source.IsLink(link) {
		t.Fatal("saving did not restore the missing Monitor link")
	}
}

func TestGameOnlyScanKeepsLinkedWorkshopMetadata(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	target := e.AddWorkshop("1011", testkit.Sample("tester.linked", "Linked"))
	e.Junction(filepath.Join(e.Game, "Mods", "_Link_1011"), target)
	scanNow(t, e)
	before := one(t, e, "Linked")
	r, err := e.App.ScanWith(app.ScanOptions{GameOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	after := one(t, e, "Linked")
	if after.ID != before.ID || after.Kind != source.Workshop || after.ACFManifest != before.ACFManifest || after.ACFSize != before.ACFSize || after.Residue != before.Residue {
		t.Fatalf("partial scan changed the linked Workshop instance: before %+v; after %+v", before, after)
	}
	for _, s := range r.Stats.Sources {
		if s.Kind == string(source.Workshop) {
			t.Fatal("game-only scan enumerated the Workshop folder")
		}
	}
}

func TestEnvUseRepairsLegacyLocalMonitorReferences(t *testing.T) {
	t.Parallel()
	for _, status := range []string{identity.Live, identity.Tombstone} {
		t.Run(status, func(t *testing.T) {
			e, g2 := envLinksFixture(t)
			monitorID := refID(t, e, monName, source.Extra)
			e.MustRun("env", "use", "other")
			st, err := e.App.Loadout()
			if err != nil {
				t.Fatal(err)
			}
			m := st.Ix.ByID[monitorID]
			legacyID := e.App.NewID()
			local := sourcesOf(t, e)["local"]
			// Reproduce the record created by the old game-only scan. The same
			// directory has a second identity, selected by its environment's plans.
			if _, err := e.App.Store.DB.Exec(`INSERT INTO instance(id,environment_id,lineage_id,source_id,rel_path,real_path,status,current_snapshot_id,first_seen,last_seen)
				VALUES(?,?,?,?,?,?,?,?,?,?)`, legacyID, st.Ix.Env.ID, m.LineageID, local.ID, app.DeployName(m), m.RealPath, status, m.SnapID, m.FirstSeen.Unix(), m.LastSeen.Unix()); err != nil {
				t.Fatal(err)
			}
			list := st.List.Clone()
			list[list.Index(monitorID)] = loadout.Entry{Instance: legacyID}
			st.WS.Entries = list.Encode()
			if err := e.App.Store.SetWorkspace(st.WS); err != nil {
				t.Fatal(err)
			}
			if err := e.App.Store.PushStep(st.Plan.ID, "undo", list.Encode(), "legacy Monitor"); err != nil {
				t.Fatal(err)
			}
			if _, err := e.App.Store.AddVersion(st.Plan.ID, &store.Version{Created: e.Now(), Source: "save", Entries: list.Encode()}); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(g2, "Mods", app.DeployName(m))
			if err := source.RemoveLink(link); err != nil {
				t.Fatal(err)
			}
			e.MustRun("env", "use", "main")
			e.MustRun("env", "use", "other")
			st, err = e.App.Loadout()
			if err != nil || st.List.Index(monitorID) < 0 || st.List.Index(legacyID) >= 0 || !st.Reg.Loadable(monitorID) {
				t.Fatalf("legacy Monitor did not heal: %+v, %v", st, err)
			}
			if old, err := e.App.Store.Mod(legacyID); err != nil || old != nil {
				t.Fatalf("duplicate local Monitor still exists: %+v, %v", old, err)
			}
			if err := e.App.Store.MergeLinkedInstances(map[string]string{legacyID: monitorID}); err != nil {
				t.Fatalf("repeating the same repair failed: %v", err)
			}
			v, err := e.App.Store.LatestVersion(st.Plan.ID)
			if err != nil || v == nil || loadout.Decode(v.Entries).Index(legacyID) >= 0 {
				t.Fatalf("history still references local Monitor: %+v, %v", v, err)
			}
			e.MustRun("undo")
			st, _ = e.App.Loadout()
			if st.List.Index(monitorID) < 0 || st.List.Index(legacyID) >= 0 {
				t.Fatal("undo resurrected the obsolete Monitor identity")
			}
			e.MustRun("save")
			if !source.IsLink(link) || !sameTarget(t, link, m.RealPath) {
				t.Fatal("repaired Monitor link did not survive save")
			}
		})
	}
}

func sameTarget(t *testing.T, a, b string) bool {
	t.Helper()
	x, err := source.RealPath(a)
	if err != nil {
		t.Fatal(err)
	}
	y, err := source.RealPath(b)
	if err != nil {
		t.Fatal(err)
	}
	return strings.EqualFold(x, y)
}
