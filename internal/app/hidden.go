package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Kinds of hidden mods (10 §2.1): folders another manager hid by renaming About.xml.
const (
	HiddenPinned  = "pinned"  // a loadable copy of the same lineage is what the game uses
	HiddenResidue = "residue" // an unsubscribed workshop folder
	HiddenAlone   = "hidden"  // hidden on its own
)

// Hidden is one about-disabled instance and what it means.
type Hidden struct {
	Mod  *store.Mod
	Kind string
	// Copy is the loadable instance that pins it (HiddenPinned); Upstream reports that the
	// hidden (workshop) copy has changed since the copy was made.
	Copy     *store.Mod
	Upstream bool
}

// Hidden classifies the environment's about-disabled instances.
func (st *LoadoutState) Hidden() []*Hidden {
	var out []*Hidden
	for _, m := range st.Ix.Mods {
		if m.Status != identity.Live || m.Unloadable != scan.UnloadableAboutDisabled {
			continue
		}
		h := &Hidden{Mod: m, Kind: HiddenAlone}
		if c := st.pinCopy(m); c != nil {
			h.Kind, h.Copy = HiddenPinned, c
			h.Upstream = m.SnapID != c.SnapID && string(m.SnapHash) != string(c.SnapHash) &&
				m.ACFTimeUpdated > 0 && m.ACFTimeUpdated > c.FirstSeen.Unix()
		} else if m.Residue {
			h.Kind = HiddenResidue
		}
		out = append(out, h)
	}
	return out
}

// HiddenOf returns the classification of one instance (nil if it is not hidden).
func (st *LoadoutState) HiddenOf(id string) *Hidden {
	for _, h := range st.Hidden() {
		if h.Mod.ID == id {
			return h
		}
	}
	return nil
}

// pinCopy is the loadable non-workshop instance of m's lineage, preferring an enabled one.
func (st *LoadoutState) pinCopy(m *store.Mod) *store.Mod {
	var best *store.Mod
	for _, o := range st.Ix.ByLineage[m.LineageID] {
		if o.ID == m.ID || o.Status != identity.Live || o.Unloadable != "" || o.Residue || o.Kind == source.Workshop {
			continue
		}
		if !st.Reg.Known(o.ID) && !st.Deployable(o) {
			continue
		}
		if best == nil || st.List.Index(o.ID) >= 0 && st.List.Index(best.ID) < 0 {
			best = o
		}
	}
	return best
}

// FileOp is a change RimArcho makes to a mod folder at the player's request (10 §2.2).
type FileOp struct {
	Op     string // "rename", "trash"
	Path   string
	To     string // rename target
	DryRun bool
	Swap   *EditResult // unpin: the workspace now uses the workshop copy
	Scan   *ScanReport // the rescan afterwards
}

func aboutDisabledPath(m *store.Mod) (string, error) {
	about := filepath.Join(m.RealPath, "About")
	ents, err := os.ReadDir(about)
	if err != nil {
		return "", err
	}
	for _, e := range ents {
		if strings.EqualFold(e.Name(), "About.xml") {
			return "", fmt.Errorf("%s already has %s; RimArcho does not overwrite it", m.RealPath, filepath.Join("About", e.Name()))
		}
	}
	for _, e := range ents {
		if strings.EqualFold(e.Name(), "About.xml.disabled") {
			return filepath.Join(about, e.Name()), nil
		}
	}
	return "", fmt.Errorf("%s has no About/About.xml.disabled", m.RealPath)
}

// Unhide renames About.xml.disabled back to About.xml so the game sees the folder again
// (10 §2.2), then rescans.
func (a *App) Unhide(ref string, dryRun bool) (*FileOp, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	m, err := st.Ix.ResolveOne(ref)
	if err != nil {
		return nil, err
	}
	if m.Unloadable != scan.UnloadableAboutDisabled || m.Status != identity.Live {
		return nil, fmt.Errorf("%s is not hidden (its About.xml is not renamed)", st.Ix.Ref(m.ID))
	}
	return a.unhide(st, m, dryRun)
}

func (a *App) unhide(st *LoadoutState, m *store.Mod, dryRun bool) (*FileOp, error) {
	from, err := aboutDisabledPath(m)
	if err != nil {
		return nil, err
	}
	op := &FileOp{Op: "rename", Path: from, To: filepath.Join(filepath.Dir(from), "About.xml"), DryRun: dryRun}
	if dryRun {
		return op, nil
	}
	if err := a.checkRoots(m.RealPath); err != nil {
		return nil, err
	}
	if err := os.Rename(op.Path, op.To); err != nil {
		return nil, err
	}
	op.Scan, err = a.Scan(false)
	return op, err
}

// Unpin restores the hidden workshop copy of a pinned mod and switches the workspace to
// it; the pinning copy stays where it is (10 §2.2). ref may name either copy.
func (a *App) Unpin(ref string, dryRun bool) (*FileOp, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	m, err := st.Ix.ResolveOne(ref)
	if err != nil {
		return nil, err
	}
	var h *Hidden
	for _, x := range st.Hidden() {
		if x.Kind == HiddenPinned && (x.Mod.ID == m.ID || x.Copy.ID == m.ID) {
			h = x
			break
		}
	}
	if h == nil {
		return nil, fmt.Errorf("%s is not pinned (no hidden workshop copy with a loadable copy beside it)", st.Ix.Ref(m.ID))
	}
	op, err := a.unhide(st, h.Mod, dryRun)
	if err != nil || dryRun || st.List.Index(h.Copy.ID) < 0 {
		return op, err
	}
	op.Swap, err = a.Swap(st.Ix.Ref(h.Copy.ID), st.Ix.Ref(h.Mod.ID))
	return op, err
}

// ErrNotTrashable: Trash only removes hidden or unsubscribed workshop and SteamCMD folders.
var ErrNotTrashable = errors.New("not trashable")

// Trash moves a hidden or unsubscribed workshop (or SteamCMD) folder to the recycle bin
// (10 §2.2, §2.4), then rescans.
func (a *App) Trash(ref string, dryRun bool) (*FileOp, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	m, err := st.Ix.ResolveOne(ref)
	if err != nil {
		return nil, err
	}
	switch {
	case m.Status != identity.Live:
		return nil, fmt.Errorf("%s is no longer on disk", st.Ix.Ref(m.ID))
	case m.Kind != source.Workshop && m.Kind != source.SteamCMD:
		return nil, fmt.Errorf("%w: %s is in src:%s; RimArcho only moves hidden or unsubscribed workshop folders to the recycle bin", ErrNotTrashable, st.Ix.Ref(m.ID), m.SourceLabel)
	case m.Unloadable != scan.UnloadableAboutDisabled && !m.Residue:
		return nil, fmt.Errorf("%w: %s is a subscribed, loadable mod; unsubscribe it in Steam instead", ErrNotTrashable, st.Ix.Ref(m.ID))
	case st.List.Index(m.ID) >= 0:
		return nil, fmt.Errorf("%s is enabled; disable it first", st.Ix.Ref(m.ID))
	}
	var root string
	for _, s := range st.Ix.Sources {
		if s.ID == m.SourceID {
			root = s.Root
		}
	}
	real, err := source.RealPath(m.RealPath)
	if err != nil {
		return nil, err
	}
	if r, err := source.RealPath(root); err == nil {
		root = r
	}
	if root == "" || !source.Within(real, root) || env0(real, root) {
		return nil, fmt.Errorf("%w: %s is not inside its source folder %s", ErrNotTrashable, real, root)
	}
	op := &FileOp{Op: "trash", Path: real, DryRun: dryRun}
	if dryRun {
		return op, nil
	}
	trash := a.TrashFunc
	if trash == nil {
		trash = source.Trash
	}
	if err := trash(real); err != nil {
		return nil, err
	}
	op.Scan, err = a.Scan(false)
	return op, err
}

// env0 reports whether p is the source root itself (never trashed).
func env0(p, root string) bool { return source.Within(root, p) }

// SortHidden orders hidden mods for display: by kind, then name.
func SortHidden(hs []*Hidden) {
	rank := map[string]int{HiddenAlone: 0, HiddenResidue: 1, HiddenPinned: 2}
	sort.SliceStable(hs, func(i, j int) bool {
		if rank[hs[i].Kind] != rank[hs[j].Kind] {
			return rank[hs[i].Kind] < rank[hs[j].Kind]
		}
		return hs[i].Mod.DisplayName() < hs[j].Mod.DisplayName()
	})
}

// DiffInstances lists the semantic files that differ from a to b: "+ only in b",
// "~ changed", "- only in a".
func (a *App) DiffInstances(x, y *store.Mod) ([]string, error) {
	xs, err := a.Store.SemanticSet(x.SnapID)
	if err != nil {
		return nil, err
	}
	ys, err := a.Store.SemanticSet(y.SnapID)
	if err != nil {
		return nil, err
	}
	// A hidden copy's About.xml.disabled is its About.xml.
	for _, set := range []scan.SemanticSet{xs, ys} {
		if h, ok := set["about/about.xml.disabled"]; ok {
			delete(set, "about/about.xml.disabled")
			set["about/about.xml"] = h
		}
	}
	var out []string
	for p, h := range ys {
		if old, ok := xs[p]; !ok {
			out = append(out, "+ "+p)
		} else if old != h {
			out = append(out, "~ "+p)
		}
	}
	for p := range xs {
		if _, ok := ys[p]; !ok {
			out = append(out, "- "+p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][2:] < out[j][2:] })
	return out, nil
}
