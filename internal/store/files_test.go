package store_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Log attribution looks for the installed mod that ships a missing texture or DLL (03 §6.2).
// Since v14 the shared folders' instances have an empty environment_id (02 §0), so the lookup must
// cover them as well as the environment's own Mods/; another game's Mods/ stays out.
func TestInstanceFilesSharedFolders(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	ship := func(name string) testkit.Mod {
		return testkit.Mod{PackageID: "tester." + name, Name: name, Author: "Tester", Files: map[string]string{
			"Textures/UI/Lib/Badge.png": "png",
			"Assemblies/Extra.dll":      "MZ",
		}}
	}
	e.AddWorkshop("1001", ship("workshop"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Private"), ship("private"))
	e.AddEnv(true)
	e.WriteMod(filepath.Join(e.SteamCMD, "2002"), ship("steamcmd"))
	e.MustRun("scan")

	g2 := e.OtherGame("Game15", "1.5.4409 rev1118")
	e.WriteMod(filepath.Join(g2, "Mods", "Elsewhere"), ship("elsewhere"))
	e.MustRun("env", "add", "old", "--game", g2)
	e.MustRun("env", "use", "old")
	e.MustRun("scan", "--game-only")
	e.MustRun("env", "use", "main")

	ids := map[string]string{} // rel_path → instance ID
	rows, err := e.App.Store.DB.Query(`SELECT id, rel_path FROM instance`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id, rel string
		if err := rows.Scan(&id, &rel); err != nil {
			t.Fatal(err)
		}
		ids[rel] = id
	}
	rows.Close()
	envs := map[string]string{} // name → environment ID
	rows, err = e.App.Store.DB.Query(`SELECT id, name FROM environment`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatal(err)
		}
		envs[name] = id
	}
	rows.Close()

	for _, c := range []struct {
		env, like string
		want      []string // rel paths
	}{
		{"main", "%Textures/%", []string{"1001", "2002", "Private"}},
		{"main", "%Extra.dll", []string{"1001", "2002", "Private"}},
		{"old", "%Textures/%", []string{"1001", "2002", "Elsewhere"}},
		{"old", "%Extra.dll", []string{"1001", "2002", "Elsewhere"}},
	} {
		files, err := e.App.Store.InstanceFiles(envs[c.env], c.like)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for rel, id := range ids {
			if len(files[id]) > 0 {
				got = append(got, rel)
			}
		}
		slices.Sort(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s %s: instances %v, want %v (files %v)", c.env, c.like, got, c.want, files)
		}
	}
}
