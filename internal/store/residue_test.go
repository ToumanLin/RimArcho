package store_test

import (
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// A workshop manifest that cannot be parsed (Steam rewriting it) keeps each instance's stored
// residue; a readable one updates it again.
func TestUnreadableACFKeepsResidue(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("4301", testkit.Sample("test.kept", "Kept"))
	e.AddWorkshop("4302", testkit.Sample("test.gone", "Gone"))
	e.Unsubscribe("4302")
	e.AddEnv(false)
	residue := func(when string, want map[string]int) {
		t.Helper()
		for pkg, w := range want {
			var r int
			if err := e.App.Store.DB.QueryRow(`SELECT i.residue FROM instance i JOIN snapshot s ON s.id=i.current_snapshot_id
				WHERE s.package_id=?`, pkg).Scan(&r); err != nil {
				t.Fatal(err)
			}
			if r != w {
				t.Fatalf("%s: %s residue %d, want %d", when, pkg, r, w)
			}
		}
	}
	e.MustRun("scan")
	residue("first scan", map[string]int{"test.kept": 0, "test.gone": 1})

	acf := filepath.Join(e.Steam, "steamapps", "workshop", "appworkshop_294100.acf")
	e.WriteFile(acf, "\"AppWorkshop\"\n{\n\t\"WorkshopItemsInstalled\"\n")
	e.MustRun("scan")
	residue("unreadable manifest", map[string]int{"test.kept": 0, "test.gone": 1})

	e.Subscribe("4302")
	e.MustRun("scan")
	residue("resubscribed", map[string]int{"test.kept": 0, "test.gone": 0})
}
