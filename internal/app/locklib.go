package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// LockLabel is the source label of the lock library (src:locked).
const LockLabel = "locked"

// LockDir is the lock library root (04 §7): config lock.dir, else
// %LOCALAPPDATA%\RimArcho\locked. It lives outside the data directory, so the rule that the
// data directory and source roots never contain each other still holds.
func (a *App) LockDir() string {
	if v := a.Store.Meta("cfg:lock.dir"); v != "" {
		return v
	}
	base := a.LockBase
	if base == "" {
		base = os.Getenv("LOCALAPPDATA")
	}
	if base == "" {
		base, _ = os.UserCacheDir()
	}
	return filepath.Join(base, "RimArcho", "locked")
}

// lockRoot is the environment's folder in the lock library: one per environment, so the
// copies of one installation never show up in another.
func (a *App) lockRoot(e *env.Environment) string { return filepath.Join(a.LockDir(), e.Name) }

// lockSource returns the environment's lock library source, creating the folder and
// registering it as an extra source (src:locked) when create is set. The game never
// looks there: copies reach it only through _Archo_ links (08 §3).
func (a *App) lockSource(ix *Index, create bool) (*source.Source, error) {
	root := a.lockRoot(ix.Env)
	for _, s := range ix.Sources {
		if s.Kind == source.Extra && s.Status != source.StatusRetired && env.PathKey(s.Root) == env.PathKey(root) {
			return s, nil
		}
	}
	if !create {
		return nil, nil
	}
	if err := a.checkRoots(root); err != nil {
		return nil, err
	}
	for _, s := range ix.Sources {
		if s.Status != source.StatusRetired && (source.Within(root, s.Root) || source.Within(s.Root, root)) {
			return nil, fmt.Errorf("the lock library %s overlaps src:%s (%s); choose another folder with archo config set lock.dir <dir>", root, s.Label, s.Root)
		}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	srcs, err := a.Store.Sources(ix.Env.ID)
	if err != nil {
		return nil, err
	}
	s := &source.Source{ID: a.NewID(), EnvID: ix.Env.ID, Kind: source.Extra, Label: nextLabel(srcs, source.Kind(LockLabel)), Root: root, Status: source.StatusOK}
	if err := a.Store.InsertSource(s); err != nil {
		return nil, err
	}
	return s, nil
}

// MoveResult is a locked copy moved from Mods/ into the lock library (04 §7).
type MoveResult struct {
	Mod    *store.Mod // the copy before the move
	From   string
	To     string
	Copied bool // another volume: copied, checked, then the original went to the recycle bin
	After  *store.Mod
	Swap   *EditResult // the plan entry moved to the instance at the new place
	DryRun bool
}

// MoveToLibrary moves a locked copy RimArcho made in Mods/ (one with .rimarcho/origin.json)
// into the lock library. ref may name the copy or its upstream.
func (a *App) MoveToLibrary(ref string, dryRun bool) (*MoveResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	m, err := st.Ix.ResolveOne(ref)
	if err != nil {
		return nil, err
	}
	l, err := st.lockFor(m)
	if err != nil {
		return nil, err
	}
	if l == nil {
		return nil, fmt.Errorf("%s is not a locked version RimArcho made (only folders with .rimarcho/origin.json are moved)", st.Ix.Ref(m.ID))
	}
	if l.Copy.Kind != source.Local {
		return nil, fmt.Errorf("%s is already in the lock library (%s)", st.Ix.Ref(l.Copy.ID), l.Copy.RealPath)
	}
	return a.moveToLibrary(st, l.Copy, dryRun)
}

func (a *App) moveToLibrary(st *LoadoutState, m *store.Mod, dryRun bool) (*MoveResult, error) {
	if m.Kind != source.Local || m.Origin == "" {
		return nil, fmt.Errorf("%s is not a copy RimArcho made in Mods/; RimArcho moves nothing else out of Mods/", st.Ix.Ref(m.ID))
	}
	from := st.Ix.ModDir(m)
	if source.IsLink(from) {
		return nil, fmt.Errorf("%s is a link, not a folder; RimArcho does not move it", from)
	}
	if real, err := source.RealPath(from); err == nil {
		from = real
	}
	root := a.lockRoot(st.Ix.Env)
	r := &MoveResult{Mod: m, From: from, To: filepath.Join(root, freeName(root, filepath.Base(from))), DryRun: dryRun,
		Copied: !strings.EqualFold(filepath.VolumeName(from), filepath.VolumeName(root))}
	if dryRun {
		return r, nil
	}
	if _, err := a.lockSource(st.Ix, true); err != nil {
		return nil, err
	}
	if err := a.checkRoots(from); err != nil {
		return nil, err
	}
	if !r.Copied {
		if err := os.Rename(from, r.To); err != nil {
			return nil, err
		}
	} else if err := a.copyMove(from, r.To); err != nil {
		return nil, err
	}
	if _, err := a.Scan(false); err != nil {
		return nil, err
	}
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	r.After = ix.ModAt(r.To)
	if r.After != nil && r.After.ID != m.ID {
		// The scan made the moved folder a new instance: keep the plan on it.
		if st2, err := a.Loadout(); err == nil && st2.List.Index(m.ID) >= 0 {
			if r.Swap, err = a.Swap(ix.Ref(m.ID), ix.Ref(r.After.ID)); err != nil {
				return r, err
			}
		}
	}
	return r, nil
}

// copyMove copies a folder to another volume, checks every file arrived, then moves the
// original to the recycle bin (RimArcho never deletes; 08 §7).
func (a *App) copyMove(from, to string) error {
	tmp := to + ".archo-tmp"
	os.RemoveAll(tmp)
	res := &CopyResult{keepAll: true}
	if err := copyTree(from, from, tmp, "", res); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	if len(res.Skipped) > 0 {
		os.RemoveAll(tmp)
		return fmt.Errorf("%s has links that point outside it (%s); move it yourself", from, strings.Join(res.Skipped, ", "))
	}
	a1, err1 := treeSizes(from)
	a2, err2 := treeSizes(tmp)
	if err1 != nil || err2 != nil || len(a1) != len(a2) {
		os.RemoveAll(tmp)
		return fmt.Errorf("the copy of %s is incomplete; nothing was moved", from)
	}
	for p, n := range a1 {
		if a2[p] != n {
			os.RemoveAll(tmp)
			return fmt.Errorf("the copy of %s differs at %s; nothing was moved", from, p)
		}
	}
	if err := os.Rename(tmp, to); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	trash := a.TrashFunc
	if trash == nil {
		trash = source.Trash
	}
	if err := trash(from); err != nil {
		return fmt.Errorf("copied to %s, but the original could not go to the recycle bin: %v (remove %s yourself)", to, err, from)
	}
	return nil
}

// treeSizes maps every file below root (relative, lower case) to its size.
func treeSizes(root string) (map[string]int64, error) {
	out := map[string]int64{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		fi, err := os.Stat(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		out[strings.ToLower(filepath.ToSlash(rel))] = fi.Size()
		return nil
	})
	return out, err
}
