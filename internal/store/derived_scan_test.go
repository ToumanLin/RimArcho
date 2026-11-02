package store_test

import (
	"reflect"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// A scan writes the derived columns with each snapshot; a snapshot copied for a new external
// state (same files, new ACF manifest) carries them over.
func TestScanWritesDerivedContent(t *testing.T) {
	e := testkit.New(t)
	m := testkit.Sample("test.derived", "Derived")
	m.Files["1.6/Assemblies/Derived.dll"] = "MZ"
	m.Files["1.6/Languages/ChineseSimplified (简体中文)/Keyed/k.xml"] = "<LanguageData/>\n"
	e.AddWorkshop("4242", m)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "test.derived")
	e.AddEnv(false)
	e.MustRun("scan")
	db := e.App.Store.DB
	current := func() (string, int64) {
		t.Helper()
		var id string
		var snap int64
		if err := db.QueryRow(`SELECT i.id, i.current_snapshot_id FROM instance i JOIN snapshot s ON s.id=i.current_snapshot_id
			WHERE s.package_id='test.derived'`).Scan(&id, &snap); err != nil {
			t.Fatal(err)
		}
		return id, snap
	}
	check := func(when string) int64 {
		t.Helper()
		id, snap := current()
		kinds, err := e.App.Store.ContentKinds(map[string]int64{id: snap})
		if err != nil || kinds[id] != store.HasDLL|store.HasDefsPatch|store.HasLanguages {
			t.Fatalf("%s: kinds %v, %v", when, kinds, err)
		}
		png, err := e.App.Store.SnapshotsWithPNG()
		if err != nil || !png[snap] {
			t.Fatalf("%s: PNG %v, %v", when, png, err)
		}
		ls, err := e.App.Store.Languages(snap)
		if want := []string{"ChineseSimplified (简体中文)", "English"}; err != nil || !reflect.DeepEqual(ls, want) {
			t.Fatalf("%s: languages %q, %v", when, ls, err)
		}
		return snap
	}
	first := check("first scan")
	e.SetManifest("4242", 777)
	e.Advance(1)
	e.MustRun("scan")
	if again := check("new manifest"); again == first {
		t.Fatal("the new manifest made no new snapshot: the copy path is not covered")
	}
}
