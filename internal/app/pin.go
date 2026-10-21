package app

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Locked is a locked version RimArcho created (04 §7.1): a copy in the local (or an extra) source with
// .rimarcho/origin.json whose origin instance (the upstream) is still installed and not
// hidden. RimCrow-style locks (a hidden workshop copy) are Hidden with HiddenPinned.
type Locked struct {
	Copy     *store.Mod
	Upstream *store.Mod
	Origin   scan.Origin
	// Updated: the upstream's current snapshot differs from origin_snapshot (04 §7.3).
	Updated bool
}

// Locked lists the environment's RimArcho-created locked versions.
func (st *LoadoutState) Locked() []*Locked {
	var out []*Locked
	for _, m := range st.Ix.Mods {
		if l := st.lockedCopy(m); l != nil {
			out = append(out, l)
		}
	}
	return out
}

// lockedCopy returns the lock m is the copy of (nil if m is not one).
func (st *LoadoutState) lockedCopy(m *store.Mod) *Locked {
	if m.Origin == "" || m.Status != identity.Live || m.Unloadable != "" || m.Kind != source.Local && m.Kind != source.Extra {
		return nil
	}
	var og scan.Origin
	if json.Unmarshal([]byte(m.Origin), &og) != nil || og.InstanceID == "" || og.InstanceID == m.ID {
		return nil
	}
	up := st.Ix.ByID[og.InstanceID]
	if up == nil || up.Status != identity.Live || up.Unloadable != "" {
		return nil // upstream gone, or hidden (then the RimCrow-style rule applies)
	}
	return &Locked{Copy: m, Upstream: up, Origin: og,
		Updated: og.SnapshotHash != "" && !strings.EqualFold(hex.EncodeToString(up.SnapHash), og.SnapshotHash)}
}

// lockFor finds the lock a reference names: the copy itself, or an upstream (preferring
// the copy the current plan uses).
func (st *LoadoutState) lockFor(m *store.Mod) (*Locked, error) {
	if l := st.lockedCopy(m); l != nil {
		return l, nil
	}
	var cands []*Locked
	for _, l := range st.Locked() {
		if l.Upstream.ID == m.ID {
			if st.List.Index(l.Copy.ID) >= 0 {
				return l, nil
			}
			cands = append(cands, l)
		}
	}
	switch len(cands) {
	case 0:
		return nil, nil
	case 1:
		return cands[0], nil
	}
	mods := make([]*store.Mod, len(cands))
	for i, l := range cands {
		mods[i] = l.Copy
	}
	return nil, &Ambiguous{Ref: st.Ix.Ref(m.ID), Mods: mods}
}

// PinResult is the outcome of Pin and PinUpdate (04 §7.2, §7.4).
type PinResult struct {
	From *store.Mod  // the instance copied
	Copy *CopyResult // the files copied (counted only on a dry run)
	// CopyMod is the new instance after the rescan (nil on a dry run).
	CopyMod *store.Mod
	// Replaced is the instance the copy replaces in the current plan (nil: the plan is
	// unchanged because neither it nor the old copy is enabled).
	Replaced *store.Mod
	Swap     *EditResult
	Scan     *ScanReport
	// Update only: the previous copy (left where it is) and the semantic files that differ
	// from it to the upstream.
	Old    *store.Mod
	Diff   []string
	DryRun bool
	// Blocked: why the copy was not swapped in although Replaced is enabled (the game
	// would not load it).
	Blocked string
	// Moved: update only, the old copy was in Mods/ and moves to the lock library first.
	Moved *MoveResult
}

// Pin creates a locked version (04 §7.2): copy into the lock library with an origin
// marker, rescan, and swap the copy in for the original in the current plan when the
// original is enabled. The game sees the copy only through an _Archo_ link (08 §3).
func (a *App) Pin(ref, name string, git, dryRun bool) (*PinResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	m, err := st.Ix.ResolveOne(ref)
	if err != nil {
		return nil, err
	}
	res := &PinResult{From: m, DryRun: dryRun}
	if st.List.Index(m.ID) >= 0 {
		res.Replaced = m
	}
	return res, a.makeLock(st, res, name, git)
}

// PinUpdate remakes a locked version from the upstream's current content (04 §7.4): a
// fresh copy under a new folder name replaces the old copy (or the upstream) in the
// current plan; the old copy stays in the library. An old copy still in Mods/ (made before
// the library) is moved into the library first. ref may name the copy or the upstream.
func (a *App) PinUpdate(ref, name string, git, dryRun bool) (*PinResult, error) {
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
		if h := st.HiddenOf(m.ID); h != nil && h.Kind == HiddenPinned || st.pinnedBy(m) != nil {
			return nil, fmt.Errorf("%s is locked by another manager (About.xml renamed); archo unpin it, then archo pin it again", st.Ix.Ref(m.ID))
		}
		return nil, fmt.Errorf("%s is not a locked version RimArcho made (archo pin %s creates one)", st.Ix.Ref(m.ID), st.Ix.Ref(m.ID))
	}
	if !l.Updated {
		return nil, fmt.Errorf("%s has not changed since %s was copied from it; nothing to update", st.Ix.Ref(l.Upstream.ID), st.Ix.Ref(l.Copy.ID))
	}
	res := &PinResult{From: l.Upstream, Old: l.Copy, DryRun: dryRun}
	if res.Diff, err = a.DiffInstances(l.Copy, l.Upstream); err != nil {
		return nil, err
	}
	if l.Copy.Kind == source.Local {
		if res.Moved, err = a.moveToLibrary(st, l.Copy, dryRun); err != nil {
			return nil, err
		}
		if !dryRun {
			if st, err = a.Loadout(); err != nil {
				return nil, err
			}
			if res.Moved.After == nil {
				return res, fmt.Errorf("moved %s to %s, but the scan did not find it there", st.Ix.Ref(l.Copy.ID), res.Moved.To)
			}
			if l = st.lockedCopy(res.Moved.After); l == nil {
				return res, fmt.Errorf("moved to %s, but it is no longer recognised as a locked version", res.Moved.To)
			}
			res.Old, res.From = l.Copy, l.Upstream
		}
	}
	switch {
	case st.List.Index(l.Copy.ID) >= 0:
		res.Replaced = l.Copy
	case st.List.Index(l.Upstream.ID) >= 0:
		res.Replaced = l.Upstream
	}
	return res, a.makeLock(st, res, name, git)
}

// pinnedBy is the hidden workshop instance that m (a loadable copy) pins, RimCrow-style.
func (st *LoadoutState) pinnedBy(m *store.Mod) *Hidden {
	for _, h := range st.Hidden() {
		if h.Kind == HiddenPinned && h.Copy.ID == m.ID {
			return h
		}
	}
	return nil
}

// freeName is base, or base2, base3 … whichever does not exist under root.
func freeName(root, base string) string {
	name := base
	for i := 2; ; i++ {
		if _, err := os.Lstat(filepath.Join(root, name)); err != nil {
			return name
		}
		name = base + strconv.Itoa(i)
	}
}

// makeLock copies res.From, rescans and swaps the copy in for res.Replaced.
func (a *App) makeLock(st *LoadoutState, res *PinResult, name string, git bool) error {
	root := a.lockRoot(st.Ix.Env)
	if !res.DryRun {
		if _, err := a.lockSource(st.Ix, true); err != nil {
			return err
		}
	}
	var err error
	if res.Copy, err = a.copyLocal(st, res.From, root, name, git, res.DryRun); err != nil || res.DryRun {
		return err
	}
	// Scan so the copy has a ref (and its lineage via the origin marker) right away.
	if res.Scan, err = a.Scan(false); err != nil {
		return err
	}
	ix, err := a.Index()
	if err != nil {
		return err
	}
	res.CopyMod = ix.ModAt(res.Copy.Dir)
	if res.Replaced == nil || res.CopyMod == nil {
		return nil
	}
	// The copy is deployable (an _Archo_ link on save or launch); should the game still not
	// be able to load it, the plan is left as it is.
	st2, err := a.Loadout()
	if err != nil {
		return err
	}
	if why := swapBlocker(st2, res.CopyMod, res.Replaced); why != "" {
		res.Blocked = why
		return nil
	}
	res.Swap, err = a.Swap(ix.Ref(res.Replaced.ID), ix.Ref(res.CopyMod.ID))
	return err
}

// ModAt is the live instance whose folder is dir (nil if none).
func (ix *Index) ModAt(dir string) *store.Mod {
	want := []string{filepath.Clean(dir)}
	if real, err := source.RealPath(dir); err == nil {
		want = append(want, filepath.Clean(real))
	}
	for _, x := range ix.Mods {
		if !x.TombstoneAt.IsZero() {
			continue
		}
		for _, w := range want {
			if strings.EqualFold(filepath.Clean(ix.ModDir(x)), w) {
				return x
			}
		}
	}
	return nil
}

// swapBlocker is why in cannot replace out in the list (whyNotLoadable), judged as the game
// would see it after the swap: out's own registration (e.g. its _Archo_ link, which the
// next save removes) does not count against in.
func swapBlocker(st *LoadoutState, in, out *store.Mod) string {
	why := whyNotLoadable(st, in)
	if why == "" || !strings.HasPrefix(why, "pkg.duplicate-installed") || !st.Deployed(out) {
		return why
	}
	var cs []loadout.Candidate
	for _, c := range st.cands {
		if c.ID != out.ID && c.ID != in.ID {
			cs = append(cs, c)
		}
	}
	if st.Reg.Known(in.ID) {
		for _, c := range st.cands {
			if c.ID == in.ID {
				cs = append(cs, c)
			}
		}
	} else if st.Deployable(in) {
		cs = append(cs, deployCandidate(in))
	}
	if loadout.NewRegistry(cs).Loadable(in.ID) {
		return ""
	}
	return why
}
