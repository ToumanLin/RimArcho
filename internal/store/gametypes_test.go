package store_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/store"
)

func openTemp(t *testing.T) (*store.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archo.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func saveGame(t *testing.T, s *store.Store, envID, file, mm string, types ...string) {
	t.Helper()
	g := &store.GameAssembly{EnvID: envID, File: file, MM: mm, Path: `C:\g\` + file, Size: 1, MTime: 1, Reader: 1}
	if err := s.SaveGameAssembly(g, types); err != nil {
		t.Fatal(err)
	}
	if g.Types != len(types) {
		t.Fatalf("Types = %d", g.Types)
	}
}

func TestGameTypeOwnersByVersion(t *testing.T) {
	s, _ := openTemp(t)
	saveGame(t, s, "env16", store.GameMainAssembly, "1.6",
		"Verse.Thing", "RimWorld.CompPowerTrader", "RimWorld.Outer+Inner", "Verse.AI.Group.LordJob_Siege",
		"Verse.AI.LordToil_X", "Verse.Pair`2", "GlobalType", "Other.Ns.Widget")
	saveGame(t, s, "env16", "Assembly-CSharp-firstpass.dll", "1.6", "UnityStandardAssets.ImageEffects.Bloom", "Verse.Thing")
	saveGame(t, s, "env17", store.GameMainAssembly, "1.7", "Verse.Thing", "RimWorld.OnlyIn17")
	main, both := []string{store.GameMainAssembly}, []string{"Assembly-CSharp-firstpass.dll", store.GameMainAssembly}

	s.MM = "1.6"
	for _, c := range []struct {
		name string
		want []string
	}{
		{"Verse.Thing", both},
		{"verse.THING", both}, // the game's lookup ignores case
		{"RimWorld.Outer+Inner", main},
		{"RimWorld.Outer.Inner", main}, // nested type printed with dots
		{"RimWorld.OnlyIn17", nil},     // another game version
		{"RimWorld.Missing", nil},
	} {
		got, err := s.GameTypeOwners(c.name)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("1.6 GameTypeOwners(%q) = %v, %v; want %v", c.name, got, err, c.want)
		}
	}
	s.MM = "1.7"
	if got, _ := s.GameTypeOwners("RimWorld.OnlyIn17"); !reflect.DeepEqual(got, main) {
		t.Errorf("1.7 GameTypeOwners = %v", got)
	}
	if got, _ := s.GameTypeOwners("RimWorld.CompPowerTrader"); got != nil {
		t.Errorf("1.6 type found for 1.7: %v", got)
	}

	for mm, want := range map[string]bool{"1.6": true, "1.7": true, "1.5": false} {
		s.MM = mm
		if ok, err := s.GameTypesIndexed(); err != nil || ok != want {
			t.Errorf("GameTypesIndexed(%s) = %v, %v", mm, ok, err)
		}
	}
}

func TestGameClassOwnersFollowsTheGame(t *testing.T) {
	s, _ := openTemp(t)
	saveGame(t, s, "env16", store.GameMainAssembly, "1.6",
		"RimWorld.CompPowerTrader", "RimWorld.Outer+Inner", "Verse.AI.Group.LordJob_Siege",
		"Verse.AI.LordToil_X", "Verse.Pair`2", "GlobalType", "Other.Ns.Widget")
	s.MM = "1.6"
	main := []string{store.GameMainAssembly}
	for _, c := range []struct {
		class string
		want  []string
	}{
		{"CompPowerTrader", main},                 // written without its ignored namespace
		{"Inner", main},                           // nested type: its own name
		{"LordJob_Siege", main},                   // Verse.AI.Group is ignored too
		{"GlobalType", main},                      // global namespace
		{"Other.Ns.Widget", main},                 // other namespaces are written in full
		{"AI.LordToil_X", main},                   // an ignored namespace prefixed
		{"Verse.Pair`2[Verse.Thing,Int32]", main}, // generic: its definition
		{"compPowerTrader", main},                 // the prefixed full-name retry ignores case
		{"Widget", nil},                           // not an ignored namespace
		{"CompMissing", nil},
	} {
		got, err := s.GameClassOwners(c.class)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("GameClassOwners(%q) = %v, %v; want %v", c.class, got, err, c.want)
		}
	}
}

func TestGameAssemblyReplacedAndDeleted(t *testing.T) {
	s, _ := openTemp(t)
	saveGame(t, s, "env16", store.GameMainAssembly, "1.6", "Verse.Old")
	saveGame(t, s, "env16", store.GameMainAssembly, "1.6", "Verse.New")
	s.MM = "1.6"
	if got, _ := s.GameTypeOwners("Verse.Old"); got != nil {
		t.Errorf("replaced types still found: %v", got)
	}
	have, err := s.GameAssemblies("env16")
	if err != nil || len(have) != 1 || have[store.GameMainAssembly].Types != 1 {
		t.Fatalf("GameAssemblies = %v, %v", have, err)
	}
	if err := s.DeleteEnv("env16"); err != nil {
		t.Fatal(err)
	}
	var n int
	s.DB.QueryRow(`SELECT (SELECT count(*) FROM game_assembly) + (SELECT count(*) FROM game_type)`).Scan(&n)
	if n != 0 {
		t.Errorf("%d game type rows left after removing the environment", n)
	}
}

// The steps from 29 (game types 29 → 30, save cache 30 → 31, tag notes 31 → 32) create the same tables as schema.sql.
func TestUpgradeGameTypes(t *testing.T) {
	s, path := openTemp(t)
	want := gameTypeDDL(t, s.DB)
	for _, q := range []string{`DROP TABLE game_type`, `DROP TABLE game_assembly`, `DROP TABLE save_cache`, `DROP TABLE tag_note`, `UPDATE meta SET value='29' WHERE key='schema_version'`} {
		if _, err := s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	var steps int
	u, err := store.OpenUpgrading(path, func(p store.UpgradeProgress) {
		if p.From != 29 || p.To != store.SchemaVersion {
			t.Errorf("progress %+v", p)
		}
		steps++
	})
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	if got := gameTypeDDL(t, u.DB); got != want {
		t.Errorf("upgraded schema:\n%s\nwant:\n%s", got, want)
	}
	if v := u.Meta("schema_version"); v != fmt.Sprint(store.SchemaVersion) || steps == 0 {
		t.Errorf("schema_version %s after %d progress calls", v, steps)
	}
}

func gameTypeDDL(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`SELECT name, sql FROM sqlite_master WHERE tbl_name IN ('game_assembly','game_type','save_cache','tag_note') AND sql IS NOT NULL ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := ""
	for rows.Next() {
		var n, q string
		rows.Scan(&n, &q)
		out += n + ": " + q + "\n"
	}
	if out == "" {
		t.Fatal("no game type tables")
	}
	return out
}
