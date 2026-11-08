package app_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func copyFixtureDLL(t *testing.T, e *testkit.Env, name, dst string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(testkit.DotnetBin(), name))
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(dst, string(b))
}

// A scan indexes the game's own assemblies for its version, rereads them only when they
// change, and keeps 1.6 and 1.7 games apart (05 §5.3).
func TestScanIndexesGameTypes(t *testing.T) {
	e := testkit.New(t)
	managed := filepath.Join(e.Game, "RimWorldWin64_Data", "Managed")
	main := filepath.Join(managed, store.GameMainAssembly)
	firstpass := filepath.Join(managed, "Assembly-CSharp-firstpass.dll")
	copyFixtureDLL(t, e, "Assembly-CSharp.dll", main) // the fake game: Verse.Thing, Verse.Pawn …
	copyFixtureDLL(t, e, "ModC.dll", firstpass)
	e.AddEnv(false)
	e.MustRun("scan")
	en, err := e.App.Env()
	if err != nil {
		t.Fatal(err)
	}
	st := e.App.Store
	st.MM = "1.6"
	want := []string{store.GameMainAssembly}
	if got, err := st.GameTypeOwners("Verse.Pawn"); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("GameTypeOwners = %v, %v", got, err)
	}
	if got, _ := st.GameClassOwners("Pawn"); !reflect.DeepEqual(got, want) {
		t.Errorf("GameClassOwners(Pawn) = %v", got)
	}
	have, err := st.GameAssemblies(en.ID)
	if err != nil || len(have) != 2 || have[store.GameMainAssembly].Types != 3 || have["Assembly-CSharp-firstpass.dll"].Types == 0 {
		t.Fatalf("GameAssemblies = %+v, %v", have, err)
	}

	// Same size and modification time: not read again (the bytes are now garbage).
	fi, err := os.Stat(main)
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(main, string(make([]byte, fi.Size())))
	if err := os.Chtimes(main, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	e.MustRun("scan")
	if got, _ := st.GameTypeOwners("Verse.Pawn"); !reflect.DeepEqual(got, want) {
		t.Fatalf("unchanged assembly was read again: %v", got)
	}

	// A new modification time: read again; an unreadable assembly leaves no types.
	later := fi.ModTime().Add(time.Hour)
	if err := os.Chtimes(main, later, later); err != nil {
		t.Fatal(err)
	}
	e.Remove(firstpass)
	e.MustRun("scan")
	have, _ = st.GameAssemblies(en.ID)
	if g := have[store.GameMainAssembly]; len(have) != 1 || g == nil || g.Err == "" || g.Types != 0 {
		t.Fatalf("after change: %+v", have)
	}
	if ok, _ := st.GameTypesIndexed(); ok {
		t.Error("an unreadable Assembly-CSharp.dll counts as indexed")
	}
	if got, _ := st.GameTypeOwners("Verse.Pawn"); got != nil {
		t.Errorf("stale types: %v", got)
	}

	// A 1.7 game keeps Assembly-CSharp.dll in its own folder; its types are 1.7's only.
	g17 := e.OtherGame("RimWorld17", "1.7.4985 rev1269")
	copyFixtureDLL(t, e, "Assembly-CSharp.dll", filepath.Join(g17, store.GameMainAssembly))
	e.MustRun("env", "add", "g17", "--game", g17, "--user-data", filepath.Join(e.Root, "ud17"))
	e.MustRun("env", "use", "g17")
	e.MustRun("scan", "--game-only")
	st.MM = "1.7"
	if got, _ := st.GameTypeOwners("Verse.Pawn"); !reflect.DeepEqual(got, want) {
		t.Errorf("1.7 GameTypeOwners = %v", got)
	}
	st.MM = "1.6"
	if got, _ := st.GameTypeOwners("Verse.Pawn"); got != nil {
		t.Errorf("1.7 types answer for 1.6: %v", got)
	}
}

// A type asked for under the wrong namespace is offered under the right one (read_source,
// "types with that name").
func TestTypesNamed(t *testing.T) {
	e := testkit.New(t)
	copyFixtureDLL(t, e, "Assembly-CSharp.dll", filepath.Join(e.Game, "RimWorldWin64_Data", "Managed", store.GameMainAssembly))
	e.AddEnv(false)
	e.MustRun("scan")
	st := e.App.Store
	st.MM = "1.6"
	if got, err := st.TypesNamed("RimWorld.Pawn", 5); err != nil || !reflect.DeepEqual(got, []string{"Verse.Pawn"}) {
		t.Fatalf("TypesNamed = %v, %v", got, err)
	}
	if got, _ := st.TypesNamed("Verse.Pawn", 5); len(got) != 0 {
		t.Errorf("the asked name itself offered: %v", got)
	}
	if got, _ := st.TypesNamed("Verse.P_wn", 5); len(got) != 0 {
		t.Errorf("_ matched as a wildcard: %v", got)
	}
}
