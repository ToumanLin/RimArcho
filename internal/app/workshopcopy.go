package app

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// WorkshopUpdateSource selects the installed Workshop (else SteamCMD) copy for an
// in-place update. It is independent of a lock's Updated flag: local edits can be
// discarded even when the upstream has not changed. Instance IDs break ties stably.
func (ix *Index) WorkshopUpdateSource(m *store.Mod) *store.Mod {
	if m == nil || m.Status != identity.Live || !m.Kind.Managed() || m.Kind.WorkshopLike() {
		return nil
	}
	var best *store.Mod
	for _, o := range ix.ByLineage[m.LineageID] {
		if o.ID == m.ID || o.Status != identity.Live || !o.Kind.WorkshopLike() ||
			o.Unloadable != "" && o.Unloadable != scan.UnloadableAboutDisabled {
			continue
		}
		if best == nil || o.Kind == source.Workshop && best.Kind != source.Workshop ||
			o.Kind == best.Kind && o.ID < best.ID {
			best = o
		}
	}
	return best
}

// WorkshopCopyUpdate describes an in-place replacement, not pin --update's new
// retained version. The target instance and all plan references stay unchanged.
type WorkshopCopyUpdate struct {
	From, Target string
	Copy         *CopyResult
	Links        *LinkLifecyclePlan
	Scan         *ScanReport
}

// UpdateToWorkshop replaces a managed local copy with the upstream's installed
// files. Stage before recycling; retain the staged replacement if installation
// fails after recycling, just like Workshop imports. Incoming links use the
// journalled replacement lifecycle, including foreign-link and running-game checks.
func (a *App) UpdateToWorkshop(ref string, dryRun bool) (*WorkshopCopyUpdate, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	m, err := st.Ix.ResolveOne(ref)
	if err != nil {
		return nil, err
	}
	up := st.Ix.WorkshopUpdateSource(m)
	if up == nil {
		return nil, fmt.Errorf("%s: no installed Workshop or SteamCMD copy to update this local copy from", st.Ix.Ref(m.ID))
	}
	if st.Ix.ModIsLink(m) {
		return nil, fmt.Errorf("%s is a link, not a folder; update its original copy instead", st.Ix.ModDir(m))
	}
	target, err := source.RealPath(st.Ix.ModDir(m))
	if err != nil {
		return nil, err
	}
	s := st.Ix.Source(m.SourceID)
	if s == nil {
		return nil, fmt.Errorf("%s: source folder is missing", target)
	}
	root, err := source.RealPath(s.Root)
	if err != nil {
		return nil, err
	}
	if !source.Within(target, root) || env0(target, root) {
		return nil, fmt.Errorf("%s is not inside its source folder %s", target, root)
	}
	from, err := source.RealPath(st.Ix.ModDir(up))
	if err != nil {
		return nil, err
	}
	if source.Within(from, target) || source.Within(target, from) {
		return nil, fmt.Errorf("update folders overlap: %s and %s", from, target)
	}
	if links := linksOutside(target); len(links) > 0 {
		return nil, fmt.Errorf("%s contains links outside the mod folder: %v", target, links)
	}
	r := &WorkshopCopyUpdate{From: from, Target: target, Copy: &CopyResult{Dir: target, dry: dryRun, generated: a.generatedIn(from)}}
	r.Links, err = a.PlanIncomingLinks(target, target)
	if err != nil {
		return nil, err
	}
	if dryRun {
		return r, copyTree(from, from, "", "", r.Copy)
	}
	tmp, err := os.MkdirTemp(filepath.Dir(target), ".archo-update-")
	if err != nil {
		return nil, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(tmp)
		}
	}()
	if err := copyTree(from, from, tmp, "", r.Copy); err != nil {
		return nil, err
	}
	// An upstream hidden by another manager is still usable. Restore only the
	// staged About.xml; leave the Workshop's hidden state untouched.
	if up.Unloadable == scan.UnloadableAboutDisabled {
		about, err := aboutDisabledPath(&store.Mod{RealPath: tmp})
		if err != nil {
			return nil, err
		}
		if err := os.Rename(about, filepath.Join(filepath.Dir(about), "About.xml")); err != nil {
			return nil, err
		}
	}
	// Refresh an existing lock marker, but do not turn an ordinary local copy into
	// a lock merely because its files were updated.
	if hasOrigin(target) {
		o := scan.Origin{InstanceID: up.ID, LineageID: m.LineageID, WorkshopID: up.WorkshopID(), SnapshotHash: hex.EncodeToString(up.SnapHash), CopiedAt: a.Now().UTC().Format("2006-01-02T15:04:05Z")}
		b, _ := json.MarshalIndent(o, "", "  ")
		if err := os.MkdirAll(filepath.Join(tmp, ".rimarcho"), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(tmp, filepath.FromSlash(scan.OriginFile)), append(b, '\n'), 0o644); err != nil {
			return nil, err
		}
	}
	err = a.withIncomingLinks(target, target, func() error {
		if err := a.recycleTarget(target); err != nil {
			return err
		}
		if err := os.Rename(tmp, target); err != nil {
			cleanup = false
			return fmt.Errorf("old folder is in the recycle bin; replacement remains at %s: %w", tmp, err)
		}
		r.Scan, err = a.scanAfterFileOp(st.Ix, m)
		return err
	})
	return r, err
}
