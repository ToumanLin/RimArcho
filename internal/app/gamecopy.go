package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// CopyGameSpec copies a Game Config's game folder to Dir and adds the copy as a new
// Game Config (03 §3.5): a backup of a working game before the game updates, or a second
// game to try something in.
type CopyGameSpec struct {
	From string // the environment copied
	Dir  string // the new game folder: absent or empty
	Name string // the new environment's name; "" = <From>-copy (then -copy-2…)
	// Mods: the game's own Mods/ folder too (local Mods). RimArcho's links there are never
	// copied: launching makes them again for the copy.
	Mods bool
	// OwnSaves: the copy keeps its saves and settings in a folder of its own (SavesOwn);
	// otherwise both games share the game's default user data folder.
	OwnSaves bool
}

// CopyGamePlan is what a copy would do: where, what it is called and how much it copies.
type CopyGamePlan struct {
	From     *env.Environment
	Dir      string
	Name     string
	Bytes    int64 // what is copied
	ModBytes int64 // the game's Mods/ folder, copied only with Mods
	Free     int64 // on Dir's drive; -1 unknown
}

// CopyGameProgress is how far a copy is: bytes of Total, files so far.
type CopyGameProgress struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
	Total int64 `json:"total"`
}

// ErrCopyStopped ends a copy the player stopped; nothing is left behind.
var ErrCopyStopped = errors.New("the copy was stopped")

// PlanGameCopy checks a copy without writing: the source game exists, the folder is free
// (CheckDownloadDir's rules: not RimArcho's data, no other game or mod folder, empty), the
// name is free, and how much it copies.
func (a *App) PlanGameCopy(spec CopyGameSpec) (*CopyGamePlan, error) {
	from, err := a.Store.EnvByName(spec.From)
	if err != nil {
		return nil, err
	}
	if from == nil {
		return nil, fmt.Errorf("no Game Config named %q (archo env list)", spec.From)
	}
	dir, err := a.CheckDownloadDir(spec.Dir)
	if err != nil {
		return nil, err
	}
	if source.Within(dir, from.GameRoot) || source.Within(from.GameRoot, dir) {
		return nil, fmt.Errorf("%s is inside the game it copies (or holds it); choose a folder of its own", dir)
	}
	if ents, err := os.ReadDir(dir); err == nil && len(ents) > 0 {
		return nil, fmt.Errorf("%s is not empty; choose an empty or new folder", dir)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		name = a.freeEnvName(from.Name + "-copy")
	} else if err := ValidEnvName(name); err != nil {
		return nil, err
	} else if e, err := a.Store.EnvByName(name); err != nil {
		return nil, err
	} else if e != nil {
		return nil, fmt.Errorf("a Game Config named %s is already added; give the copy another name", name)
	}
	p := &CopyGamePlan{From: from, Dir: dir, Name: name, Free: DiskFree(dir)}
	if err := walkGameCopy(from.GameRoot, true, func(rel string, fi fs.FileInfo) error {
		if inMods(rel) {
			p.ModBytes += fi.Size()
			if !spec.Mods {
				return nil
			}
		}
		p.Bytes += fi.Size()
		return nil
	}); err != nil {
		return nil, err
	}
	return p, nil
}

// CopyGame copies the game folder (03 §3.5) into a folder next to Dir and renames it into
// place, so a failure or a stop leaves nothing; then it adds the copy as a Game Config that
// selects the copied config's list and starts with the game's own exe (Steam only starts
// its own install). progress is called after each file; cancelling ctx stops the copy.
func (a *App) CopyGame(ctx context.Context, spec CopyGameSpec, progress func(CopyGameProgress)) (*SourceChangeEnv, error) {
	p, err := a.PlanGameCopy(spec)
	if err != nil {
		return nil, err
	}
	if p.Free >= 0 && p.Free < p.Bytes {
		return nil, fmt.Errorf("%s has %s free and the copy needs %s", p.Dir, sizeText(p.Free), sizeText(p.Bytes))
	}
	tmp := p.Dir + ".archo-tmp"
	os.RemoveAll(tmp)
	got := CopyGameProgress{Total: p.Bytes}
	err = walkGameCopy(p.From.GameRoot, spec.Mods, func(rel string, fi fs.FileInfo) error {
		if ctx.Err() != nil {
			return ErrCopyStopped
		}
		dst := filepath.Join(tmp, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := copyFile(filepath.Join(p.From.GameRoot, rel), dst, fi); err != nil {
			return err
		}
		got.Files++
		got.Bytes += fi.Size()
		if progress != nil {
			progress(got)
		}
		return nil
	})
	if err == nil {
		// the game finds Mods/ even when it is empty, and launching links into it
		err = os.MkdirAll(filepath.Join(tmp, "Mods"), 0o755)
	}
	if err == nil {
		os.Remove(p.Dir) // an empty folder the player chose; Rename needs it gone
		err = os.Rename(tmp, p.Dir)
	}
	if err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	saves := SavesShared
	if spec.OwnSaves {
		saves = SavesOwn
	}
	// the copy plays the list its game plays (lists are global)
	spec2 := EnvSpec{GameRoot: p.Dir, Launch: "direct", Saves: saves}
	if cur, err := a.Store.CurrentPlan(p.From.ID); err == nil && cur != nil {
		spec2.List = cur.Name
	}
	e, changes, err := a.EnvAdd(p.Name, spec2)
	if err != nil {
		return nil, err
	}
	return &SourceChangeEnv{Env: e, Changes: changes}, nil
}

// walkGameCopy calls file for each plain file under root that a copy takes, by its path
// relative to root. Links are never followed or copied: in Mods/ they are RimArcho's
// (09 §3), elsewhere they point outside the game. withMods false skips Mods/.
func walkGameCopy(root string, withMods bool, file func(rel string, fi fs.FileInfo) error) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == "." {
			return err
		}
		if source.IsLink(p) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if !withMods && strings.EqualFold(rel, "Mods") {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		return file(rel, fi)
	})
}

func inMods(rel string) bool {
	first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
	return strings.EqualFold(first, "Mods")
}

// sizeText is a byte count for an error message.
func sizeText(n int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	f, i := float64(n), 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}
