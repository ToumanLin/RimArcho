package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Pack revisions (10 §1 "id", §2 "new revision", §4 "update"). A pack keeps its id across
// revisions. Building a new revision of a pack, or importing a newer revision of a pack the
// player has, replaces the pack's Mods and manifest in place and keeps its own userdata/
// (saves, settings) and state.json.

// PackDiff is what a revision changes: Mods by name.
type PackDiff struct {
	Added   []string `json:"added,omitempty"`
	Removed []string `json:"removed,omitempty"`
	Changed []string `json:"changed,omitempty"` // other content (semantic hash or digest)
	// Settings: Mods whose carried settings the new revision brings (missing files only).
	Settings int `json:"settings,omitempty"`
}

// Empty: the two revisions carry the same Mods.
func (d *PackDiff) Empty() bool {
	return d == nil || len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0
}

// diffPacks compares two revisions' carried Mods by package.
func diffPacks(old, new *Pack) *PackDiff {
	d := &PackDiff{Settings: new.Settings.Mods()}
	key := func(e PackEntry) string { return packageBase(strings.ToLower(e.PackageID)) }
	was := map[string]PackEntry{}
	for _, e := range old.Entries {
		if !e.Official {
			was[key(e)] = e
		}
	}
	now := map[string]bool{}
	for _, e := range new.Entries {
		if e.Official {
			continue
		}
		now[key(e)] = true
		o, ok := was[key(e)]
		switch {
		case !ok:
			d.Added = append(d.Added, e.Name)
		case o.Hash != e.Hash || o.SHA256 != "" && e.SHA256 != "" && o.SHA256 != e.SHA256:
			d.Changed = append(d.Changed, e.Name)
		}
	}
	for _, e := range old.Entries {
		if !e.Official && !now[key(e)] {
			d.Removed = append(d.Removed, e.Name)
		}
	}
	return d
}

// PackUpdate is an import that meets a newer revision of a pack the player has.
type PackUpdate struct {
	Name    string    `json:"name"` // the pack it updates
	From    int       `json:"from"`
	To      int       `json:"to"`
	Diff    *PackDiff `json:"diff"`
	Applied bool      `json:"applied"` // false: offered only (import again with update)
}

// packByID is the installed pack with id, if any.
func (a *App) packByID(id string) (string, *Pack) {
	if id == "" {
		return "", nil
	}
	ents, err := os.ReadDir(a.PacksDir())
	if err != nil {
		return "", nil
	}
	for _, de := range ents {
		if !de.IsDir() || strings.HasSuffix(de.Name(), TmpSuffix) || strings.HasSuffix(de.Name(), oldSuffix) {
			continue
		}
		if p, err := a.ReadPack(de.Name()); err == nil && !p.Light && p.ID == id {
			return de.Name(), p
		}
	}
	return "", nil
}

// oldSuffix names the folder an update moves a pack's previous revision to, on its way to
// the recycle bin.
const oldSuffix = ".archo-old"

// replacePackContent puts the pack assembled in tmp (mods/ and pack.json) in place of pack
// name's, keeping name's userdata/ and state.json. The previous mods/ and pack.json go to
// the recycle bin. Refused while a game runs: its links point into mods/.
func (a *App) replacePackContent(name, tmp string) error {
	if err := a.checkNoGame(); err != nil {
		return err
	}
	dir := a.packPath(name)
	aside := dir + oldSuffix
	if err := os.RemoveAll(aside); err != nil {
		return err
	}
	if err := os.MkdirAll(aside, 0o755); err != nil {
		return err
	}
	moved := []string{}
	undo := func() {
		for i := len(moved) - 1; i >= 0; i-- {
			n := moved[i]
			os.RemoveAll(filepath.Join(dir, n))
			os.Rename(filepath.Join(aside, n), filepath.Join(dir, n))
		}
		os.RemoveAll(aside)
	}
	for _, n := range []string{"mods", "pack.json"} {
		err := os.Rename(filepath.Join(dir, n), filepath.Join(aside, n))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			undo()
			return fmt.Errorf("pack %s: %w", name, err)
		}
		moved = append(moved, n)
	}
	for _, n := range []string{"mods", "pack.json"} {
		err := os.Rename(filepath.Join(tmp, n), filepath.Join(dir, n))
		if errors.Is(err, os.ErrNotExist) && n == "mods" {
			continue
		}
		if err != nil {
			undo()
			return fmt.Errorf("pack %s: %w", name, err)
		}
	}
	os.RemoveAll(tmp)
	// the previous revision is RimArcho's own copy: the recycle bin when it can, else gone
	if err := a.ToTrash(aside); err != nil {
		a.deleteTree(nil, aside)
	}
	return nil
}

// renamePackManifest rewrites the manifest in dir to name the pack name (an update keeps
// the name the player has).
func renamePackManifest(dir, name string, p *Pack) error {
	if p.Name == name {
		return nil
	}
	q := *p
	q.Name = name
	b, err := json.MarshalIndent(&q, "", "  ")
	if err != nil {
		return err
	}
	p.Name = name
	return os.WriteFile(filepath.Join(dir, "pack.json"), append(b, '\n'), 0o644)
}
