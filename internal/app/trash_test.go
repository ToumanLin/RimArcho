package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// H8: the recycle bin's entry refuses any path outside the allow-list of 02 §3, whoever
// calls it; paths on the list reach the recycle bin.
func TestTrashAllowList(t *testing.T) {
	t.Parallel()
	e := setup(t, true)
	scanNow(t, e)
	var trashed []string
	e.App.TrashFunc = func(p string) error { trashed = append(trashed, p); return nil }

	refused := map[string]string{
		"a folder outside every list": filepath.Join(e.Root, "Elsewhere", "Stuff"),
		"the game folder":             e.Game,
		"Mods/ itself":                filepath.Join(e.Game, "Mods"),
		"the workshop folder itself":  e.Workshop,
		"Core":                        filepath.Join(e.Game, "Data", "Core"),
		"the user data folder":        e.UserData,
		"the data folder":             e.Home,
		"a pack's inner folder":       filepath.Join(e.Home, "packs", "first", "mods", "X"),
		"the pack folder itself":      filepath.Join(e.Home, "packs"),
	}
	for name, p := range refused {
		e.MkdirAll(p)
		if err := e.App.ToTrash(p); !errors.Is(err, app.ErrNotInTrashList) {
			t.Errorf("%s (%s): want refused, got %v", name, p, err)
		}
	}
	// A link in Mods/ never goes, even to a folder that would.
	target := filepath.Join(e.Game, "Mods", "Real")
	e.MkdirAll(target)
	link := filepath.Join(e.Game, "Mods", "_Archo_link")
	e.Junction(link, target)
	if err := e.App.ToTrash(link); !errors.Is(err, app.ErrNotInTrashList) {
		t.Errorf("a link: want refused, got %v", err)
	}
	if len(trashed) != 0 {
		t.Fatalf("refused paths reached the recycle bin: %v", trashed)
	}
	if _, err := os.Stat(filepath.Join(e.Root, "Elsewhere", "Stuff")); err != nil {
		t.Fatal("a refused folder was touched")
	}
	// A foreign incoming link blocks the target too, even on the trash allow-list.
	if err := e.App.ToTrash(target); err == nil {
		t.Fatal("trashed a foreign link's target")
	}
	if err := source.RemoveLink(link); err != nil {
		t.Fatal(err)
	}

	allowed := map[string]string{
		"a folder in Mods/":   target,
		"a workshop folder":   filepath.Join(e.Workshop, "1234"),
		"a SteamCMD folder":   filepath.Join(e.SteamCMD, "5678"),
		"a pack":              filepath.Join(e.Home, "packs", "first"),
		"a lock library copy": filepath.Join(e.App.LockDir(), "Copy"),
		"a save":              filepath.Join(e.UserData, "Saves", "Colony.rws"),
		"an own save folder":  filepath.Join(e.App.OwnSavesRoot(), "main"),
		"the save backups":    filepath.Join(e.Home, "backups", "saves"),
		"a dated save backup": filepath.Join(e.Home, "backups", "saves", "2026-10-22"),
	}
	for name, p := range allowed {
		if filepath.Ext(p) == ".rws" {
			e.WriteFile(p, "save")
		} else {
			e.MkdirAll(p)
		}
		if err := e.App.ToTrash(p); err != nil {
			t.Errorf("%s (%s): %v", name, p, err)
		}
	}
	if len(trashed) != len(allowed) {
		t.Fatalf("trashed %d of %d", len(trashed), len(allowed))
	}
}
