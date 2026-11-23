//go:build windows || cgo

package gui_test

import (
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/surface/gui"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Add Game Config's copy way proposes the folder of the name the copy gets (03 §3.5): with a
// "main-copy" Game Config already there, "RimWorld main-copy-2", not the first copy's folder,
// which the plan refused with nothing on the page (VM round 2A 2026-11-23).
func TestGuideCopyPlanFollowsTheFreeName(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld")
	e.AddEnv(false)
	e.MustRun("env", "add", "main-copy", "--like", "main", "--saves", "own")
	api := gui.NewAPI(e.App, nil)
	defer api.Close()
	p, err := api.GuideCopyPlan("main", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "main-copy-2" || filepath.Base(p.Dir) != "RimWorld main-copy-2" {
		t.Fatalf("name %q dir %q", p.Name, p.Dir)
	}
}
