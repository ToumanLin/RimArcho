package store_test

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestUninstallModsInputsStayFresh(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Sample("test.package", "A"))
	e.AddEnv(false)
	e.MustRun("scan")
	env, err := e.App.Env()
	if err != nil {
		t.Fatal(err)
	}
	before, err := e.App.Store.UninstallMods(env.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range before {
		if m.PackageID == "test.package" {
			found = true
		}
	}
	if !found {
		t.Fatal("current package missing")
	}
	// Current snapshot changes must affect the very next uninstall plan's inputs.
	if _, err := e.App.Store.DB.Exec(`UPDATE snapshot SET package_id='test.changed' WHERE package_id='test.package'`); err != nil {
		t.Fatal(err)
	}
	after, err := e.App.Store.UninstallMods(env.ID)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, m := range after {
		if m.PackageID == "test.package" {
			t.Fatal("stale package returned")
		}
		if m.PackageID == "test.changed" {
			found = true
		}
	}
	if !found {
		t.Fatal("new package missing")
	}
}
