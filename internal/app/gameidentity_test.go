package app_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Production registration still creates equal IDs. Deliberately separate them to
// catch an environment ID accidentally reaching an inventory/analysis query.
func TestGameIdentityDistinctFromEnvironment(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	old, err := e.App.Env()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.Store.DB.Exec(`UPDATE environment SET id='profile' WHERE id=?`, old.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.App.Store.SetCurrentEnv("profile"); err != nil {
		t.Fatal(err)
	}
	e.WriteMod(filepath.Join(e.Game, "Mods", "Local"), testkit.Sample("synthetic.local", "Local"))
	e.MustRun("scan")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if st.Ix.Env.ID != "profile" || st.Ix.Env.GameID != old.ID {
		t.Fatalf("identities: %+v", st.Ix.Env)
	}
	if len(st.Ix.Mods) < 2 {
		t.Fatal("game inventory missing")
	}
	for _, m := range st.Ix.Mods {
		if st.Ix.Source(m.SourceID) == nil {
			t.Fatal("source missing")
		}
	}
	sc, _, err := e.App.LastScan(old.GameID)
	if err != nil || sc == nil || sc.EnvID != old.GameID {
		t.Fatalf("scan scope: %+v %v", sc, err)
	}
	if _, err := e.App.Analyze(nil); err != nil {
		t.Fatal(err)
	}
	e.App.Store.MM = st.GameMM()
	if run, err := e.App.Store.NewestAnalysis(old.GameID); err != nil || run == nil {
		t.Fatalf("analysis scope: %+v %v", run, err)
	}
	if err := e.App.Store.SetIgnore("profile", &store.Ignore{Code: "synthetic", Key: "key"}); err != nil {
		t.Fatal(err)
	}
	if ignores, err := e.App.Store.Ignores(old.GameID); err != nil || len(ignores) != 0 {
		t.Fatalf("ignore leaked into game scope: %v %v", ignores, err)
	}
	if ignores, err := e.App.Store.Ignores("profile"); err != nil || len(ignores) != 1 {
		t.Fatalf("Game Config ignore: %v %v", ignores, err)
	}
	if err := e.App.Store.SetMeta("env:profile:launch_args", "-popupwindow"); err != nil {
		t.Fatal(err)
	}
	if err := e.App.Store.AddLaunchRecord(&store.LaunchRecord{EnvID: "profile", Started: testkit.Start, Method: "direct", Command: "synthetic", PlanID: e.App.Store.CurrentPlanID("profile")}); err != nil {
		t.Fatal(err)
	}
	if launches, err := e.App.Store.Launches(old.GameID, 10); err != nil || len(launches) != 0 {
		t.Fatalf("launch leaked into game scope: %v %v", launches, err)
	}
	b, err := json.Marshal(st.Ix.Env)
	if err != nil || strings.Contains(string(b), "GameID") {
		t.Fatalf("new field in interface output: %s %v", b, err)
	}
	e.MustRun("env", "list")
	e.MustRun("status")
	e.MustRun("library")
}
