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

// LockDir is the lock library root (07 §5): config lock.dir, else
// %LOCALAPPDATA%\RimArcho\locked. It lives outside the data directory, so the rule that the
// data directory and source roots never contain each other still holds.
func (a *App) LockDir() string {
	if v := a.Store.Meta("cfg:lock.dir"); v != "" {
		return v
	}
	return filepath.Join(a.LocalDir(), "locked")
}

// lockRoot is the lock library's folder. The library is shared by every environment
// (03 §2 item 9): a copy is the same mod record whichever game plays it, and each plan links
// the copy it uses. e is kept for the callers.
func (a *App) lockRoot(e *env.Environment) string { return a.LockDir() }

// IsLockSource reports whether s is the lock library, or one of the per-environment
// library folders (<lock.dir>/<env>) RimArcho used before the library was shared.
func (a *App) IsLockSource(s *source.Source) bool {
	return s != nil && s.Kind == source.Extra && (env.PathKey(s.Root) == env.PathKey(a.LockDir()) || strings.HasPrefix(s.Label, LockLabel+"-"))
}

// lockSource returns the lock library source, creating the folder and registering it as a
// shared extra source (src:locked) when create is set. The game never looks there: copies
// reach it only through _Archo_ links (09 §3).
func (a *App) lockSource(ix *Index, create bool) (*source.Source, error) {
	root := a.LockDir()
	if create {
		if err := a.managedRootReady("lock", root); err != nil {
			return nil, err
		}
	}
	shared, err := a.Store.SharedSources()
	if err != nil {
		return nil, err
	}
	for _, s := range shared {
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
	// The older per-environment folders sit inside the library on purpose.
	if o, err := a.overlapping(root, func(s *source.Source) bool { return a.IsLockSource(s) }); err != nil {
		return nil, err
	} else if o != nil {
		return nil, fmt.Errorf("the lock library %s overlaps src:%s (%s); choose another folder with archo config set lock.dir <dir>", root, o.Label, o.Root)
	}
	return a.sharedSource(LockLabel, root)
}

// MoveResult is a locked copy moved from Mods/ into the lock library (07 §5).
type MoveResult struct {
	Mod    *store.Mod // the copy before the move
	From   string
	To     string
	Copied bool // another volume: copied, checked, then the original went to the recycle bin
	After  *store.Mod
	Swap   *EditResult // the plan entry moved to the instance at the new place
	DryRun bool
	// Origin: .rimarcho/origin.json is (or would be) written because the folder had none.
	Origin bool
	// Plans are the plans that use it; they keep using it (the instance is the same).
	Plans []string
	Links *LinkLifecyclePlan // affected games and entries, also in the existing preview
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
	if !l.Copy.Kind.GameMods() {
		return nil, fmt.Errorf("%s is already in the lock library (%s)", st.Ix.Ref(l.Copy.ID), l.Copy.RealPath)
	}
	return a.moveToLibrary(st, l.Copy, dryRun)
}

// moveToLibrary moves a folder of Mods/ into the lock library and points its instance at
// the new place, so the instance (and every plan's entry) stays the same (16 §2.9, 07 §5.4).
func (a *App) moveToLibrary(st *LoadoutState, m *store.Mod, dryRun bool) (result *MoveResult, moveErr error) {
	if !m.Kind.GameMods() {
		return nil, fmt.Errorf("%s is not in Mods/; RimArcho moves nothing else into the lock library", st.Ix.Ref(m.ID))
	}
	from := st.Ix.ModDir(m)
	if st.Ix.ModIsLink(m) {
		return nil, fmt.Errorf("%s is a link, not a folder; RimArcho does not move it", from)
	}
	if real, err := source.RealPath(from); err == nil {
		from = real
	}
	if out := linksOutside(from); len(out) > 0 {
		return nil, fmt.Errorf("%s has links that point outside it (%s); RimArcho does not move it, move it yourself", from, strings.Join(out, ", "))
	}
	root := a.lockRoot(st.Ix.Env)
	r := &MoveResult{Mod: m, From: from, To: filepath.Join(root, freeName(root, filepath.Base(from))), DryRun: dryRun,
		Copied: !strings.EqualFold(filepath.VolumeName(from), filepath.VolumeName(root)), Origin: !hasOrigin(from)}
	if a.SameVolume != nil {
		r.Copied = !a.SameVolume(from, root)
	}
	var err error
	r.Links, err = a.PlanIncomingLinks(from, r.To)
	if err != nil {
		return nil, err
	}
	if st.List.Index(m.ID) >= 0 {
		r.Links.ensureEntry(st.Ix.Env.GameRoot, modsDir(st.Ix), DeployPrefix+LockLabel+"_"+filepath.Base(r.To), r.To, m.ID)
	}
	if r.Plans, err = a.usedBy(st, m.ID); err != nil {
		return nil, err
	}
	if dryRun {
		return r, nil
	}
	src, err := a.lockSource(st.Ix, true)
	if err != nil {
		return nil, err
	}
	if err := a.checkRoots(from); err != nil {
		return nil, err
	}
	if err := a.beginLinkLifecycle(r.Links); err != nil {
		return nil, err
	}
	finished := false
	defer func() {
		if !finished {
			moveErr = a.abortLinkLifecycle(r.Links, moveErr)
		}
	}()
	if !r.Copied {
		if err := os.Rename(from, r.To); err != nil {
			return nil, err
		}
	} else if err := a.copyMove(from, r.To); err != nil {
		return nil, err
	}
	if r.Origin {
		if err := a.writeOrigin(st.Ix, m, r.To); err != nil {
			return r, fmt.Errorf("moved to %s, but .rimarcho/origin.json could not be written: %v", r.To, err)
		}
	}
	if err := a.Store.RelocateInstance(m.ID, src.ID, filepath.Base(r.To), r.To); err != nil {
		return r, err
	}
	finished = true
	if err := a.finishLinkLifecycle(r.Links); err != nil {
		return r, err
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
// original to the recycle bin (RimArcho never deletes; 02 §3).
func (a *App) copyMove(from, to string) error {
	if err := copyChecked(from, to); err != nil {
		return err
	}
	if err := a.recycleTarget(from); err != nil {
		return fmt.Errorf("copied to %s, but the original could not go to the recycle bin: %v (remove %s yourself)", to, err, from)
	}
	return nil
}

// copyChecked copies a folder to to (which must not exist) through a temporary folder and
// checks every file arrived before it takes the new name; from is left as it is. A link
// that points outside the folder stops the copy.
func copyChecked(from, to string) error {
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
