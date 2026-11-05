package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// ManagedDirectory is a lightweight, live check of one configured directory.
// Status is ok, missing, changed, unavailable, unused or rebind-pending. Notes
// describe warnings only; checking directories never prevents the app opening.
type ManagedDirectory struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Path   string `json:"path"`
	Status string `json:"status"`
	Note   string `json:"note"`
}

// ManagedDirectories reads directory metadata only. It never scans mods, creates
// directories, writes markers or changes the database. Call again after overrides
// such as LockBase have been installed; Open's check does not cache these paths.
func (a *App) ManagedDirectories() ([]ManagedDirectory, error) {
	es, err := a.Store.Envs()
	if err != nil {
		return nil, err
	}
	ss, err := a.Store.AllSources()
	if err != nil {
		return nil, err
	}
	var out []ManagedDirectory
	for _, e := range es {
		d := a.directoryState("game:"+e.ID, "game", e.Name, e.GameRoot, true)
		if d.Status == "ok" && !env.LooksLikeGameRoot(e.GameRoot) {
			d.Status, d.Note = "changed", "This directory no longer looks like a RimWorld installation. Choose its current location."
		}
		for _, s := range ss {
			if s.EnvID == e.ID && s.Status == source.StatusRebindPending {
				d.Status, d.Note = "rebind-pending", "A game folder change is waiting for confirmation. Set its current location to keep the same records."
			}
		}
		out = append(out, d)
	}
	used := map[string]bool{}
	for _, s := range ss {
		if s.Status == source.StatusRetired || s.EnvID != store.Shared {
			continue
		}
		special := ""
		switch {
		case s.Label == LockLabel || a.IsLockSource(s):
			special = "lock"
		case s.Label == MonitorLabel:
			special = "monitor"
		case s.Label == SubscribedLabel:
			special = SubscribedLabel
		}
		if special != "" {
			used[special] = true
			continue
		}
		if !SharedKind(s.Kind) {
			continue
		}
		d := a.directoryState("source:"+s.ID, string(s.Kind), s.Label, s.Root, true)
		if s.Status == source.StatusRebindPending {
			d.Status, d.Note = "rebind-pending", "A folder change is waiting for confirmation. Set its current location to keep the same records."
		}
		out = append(out, d)
	}
	for _, x := range []struct{ id, label, path string }{{"lock", "Lock library", a.LockDir()}, {"source", "Source library", a.SourceDir()}, {"packs", "Packs", a.PacksDir()}, {"monitor", "Archo Monitor", a.MonitorDir()}, {SubscribedLabel, "Subscriptions", a.SubscribedDir()}} {
		configuredPack := x.id == "packs" && a.Store.Meta("cfg:pack.dir") != ""
		d := a.directoryState(x.id, x.id, x.label, x.path, used[x.id] || configuredPack || a.Store.Meta(directoryKey(x.id)) != "")
		for _, s := range ss {
			if s.Status == source.StatusRetired {
				continue
			}
			if (x.id == "lock" && s.Label == LockLabel || x.id == "monitor" && s.Label == MonitorLabel || x.id == SubscribedLabel && s.Label == SubscribedLabel) && env.PathKey(s.Root) != env.PathKey(x.path) {
				d.Status, d.Note = "changed", "The configured location differs from the registered source. Set the current location to keep the same records."
			}
			if (x.id == "lock" && (s.Label == LockLabel || a.IsLockSource(s)) || x.id == "monitor" && s.Label == MonitorLabel || x.id == SubscribedLabel && s.Label == SubscribedLabel) && s.Status == source.StatusRebindPending {
				d.Status, d.Note = "rebind-pending", "A folder change is waiting for confirmation. Set its current location to keep the same records."
			}
		}
		out = append(out, d)
	}
	tool := a.directoryState("steamcmd-tool", "tool", "SteamCMD", a.SteamCMDDir(), a.Store.Meta("cfg:steamcmd.path") != "" || a.Store.Meta(directoryKey("steamcmd-tool")) != "")
	if tool.Status == "ok" && steamCMDIn(tool.Path) == "" {
		tool.Status, tool.Note = "unused", "SteamCMD has not been installed in this directory yet."
		if a.Store.Meta("cfg:steamcmd.path") != "" || a.Store.Meta(directoryKey("steamcmd-tool")) != "" {
			tool.Status, tool.Note = "changed", "The SteamCMD program is missing from this directory. Choose its current location."
		}
	}
	out = append(out, tool)
	for i := range out {
		if a.Store.Meta("directory.links:"+out[i].ID) != "" {
			out[i].Status, out[i].Note = "changed", "Some managed links still need updating. Set this directory again to finish the location change."
		}
	}
	return out, nil
}

func steamCMDIn(path string) string {
	return deps.SteamCMDIn(path)
}

func directoryKey(id string) string { return "directory.used:" + id }

func (a *App) directoryState(id, kind, label, path string, used bool) ManagedDirectory {
	d := ManagedDirectory{ID: id, Kind: kind, Label: label, Path: path, Status: "ok"}
	st, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		d.Status, d.Note = "missing", "This directory is missing. Choose its current location if it was moved."
		if !used {
			d.Status, d.Note = "unused", "This directory has not been used yet. It will be created when needed."
		}
	case err != nil:
		d.Status, d.Note = "unavailable", "This directory cannot be accessed. Check the drive and access permissions."
	case !st.IsDir():
		d.Status, d.Note = "changed", "This path is no longer a directory. Choose its current location."
	}
	return d
}

func directoryPath(path string) (string, error) {
	path = strings.Trim(strings.TrimSpace(path), `"'`)
	if path == "" {
		return "", fmt.Errorf("no directory given")
	}
	return filepath.Abs(path)
}

// directoryReal resolves existing ancestors too, so a missing destination below
// a junction cannot bypass overlap checks.
func directoryReal(path string) string {
	if p, err := source.RealPath(path); err == nil {
		return p
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path
	}
	return filepath.Join(directoryReal(parent), filepath.Base(path))
}

func directoriesOverlap(a, b string) bool {
	a, b = directoryReal(a), directoryReal(b)
	return source.Within(a, b) || source.Within(b, a)
}

func (a *App) managedDirectory(id string) (*ManagedDirectory, error) {
	ds, err := a.ManagedDirectories()
	if err != nil {
		return nil, err
	}
	for _, d := range ds {
		if d.ID == id {
			return &d, nil
		}
	}
	return nil, fmt.Errorf("unknown managed directory %q", id)
}

func (a *App) validateDirectory(d *ManagedDirectory, path string) error {
	if d.ID == "packs" || d.ID == "source" {
		// These managed directories default to their own child of Home.
		if directoriesOverlap(path, a.Home) && env.PathKey(path) != env.PathKey(filepath.Join(a.Home, d.ID)) {
			return fmt.Errorf("the %s directory overlaps the data directory", d.ID)
		}
	} else if directoriesOverlap(path, a.Home) {
		return fmt.Errorf("the directory overlaps the data directory")
	}
	ds, err := a.ManagedDirectories()
	if err != nil {
		return err
	}
	for _, other := range ds {
		if other.ID == d.ID {
			continue
		}
		// A SteamCMD game's own workshop is allowed to travel with its game.
		if (d.Kind == "game" || d.ID == "steamcmd-tool") && source.Within(other.Path, d.Path) {
			continue
		}
		if directoriesOverlap(path, other.Path) {
			return fmt.Errorf("the directory overlaps %s (%s)", other.Label, other.Path)
		}
	}
	ss, err := a.Store.AllSources()
	if err != nil {
		return err
	}
	for _, s := range ss {
		if s.Status == source.StatusRetired || "source:"+s.ID == d.ID {
			continue
		}
		if (d.ID == "lock" && (s.Label == LockLabel || a.IsLockSource(s))) || (d.ID == "monitor" && s.Label == MonitorLabel) || (d.ID == SubscribedLabel && s.Label == SubscribedLabel) {
			continue
		}
		if d.Kind == "game" && s.EnvID == strings.TrimPrefix(d.ID, "game:") {
			continue
		}
		if (d.Kind == "game" || d.ID == "steamcmd-tool") && source.Within(s.Root, d.Path) {
			continue
		}
		if directoriesOverlap(path, s.Root) {
			return fmt.Errorf("the directory overlaps src:%s (%s)", s.Label, s.Root)
		}
	}
	return nil
}

// SetDirectory accepts the player's explicit identification of a moved directory.
// It keeps source and instance IDs even when content has changed, and clears any
// pending sample-based rebind. The target must already exist; nothing is created.
func (a *App) SetDirectory(id, path string) error {
	if id == "source" {
		release, err := a.sourceAccess(context.Background(), true, false)
		if err != nil {
			return err
		}
		defer release()
	}
	d, err := a.managedDirectory(id)
	if err != nil {
		return err
	}
	path, err = directoryPath(path)
	if err != nil {
		return err
	}
	if st, err := os.Stat(path); err != nil {
		return fmt.Errorf("directory %s: %w", path, err)
	} else if !st.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	if err := a.validateDirectory(d, path); err != nil {
		return err
	}
	es, err := a.Store.Envs()
	if err != nil {
		return err
	}
	for _, e := range es {
		if g := a.probe()(e.GameRoot); g != nil {
			return fmt.Errorf("a game is running (pid %d); close it before changing directories", g.PID)
		}
	}
	if d.Kind == "game" {
		if g := a.probe()(path); g != nil {
			return fmt.Errorf("the game at the new location is running (pid %d); close it before changing directories", g.PID)
		}
	}
	return a.setDirectory(d, path)
}

func (a *App) setDirectory(d *ManagedDirectory, path string) error {
	p, err := a.PlanIncomingLinks(d.Path, path)
	if err != nil {
		return err
	}
	p.Directory = d.ID
	if err := a.beginLinkLifecycle(p); err != nil {
		return err
	}
	if err := a.setDirectoryRecords(d, path); err != nil {
		return a.abortLinkLifecycle(p, err)
	}
	return a.finishLinkLifecycle(p)
}

func (a *App) setDirectoryRecords(d *ManagedDirectory, path string) error {
	if d.ID == "steamcmd-tool" && steamCMDIn(path) == "" {
		return fmt.Errorf("%s contains no SteamCMD program", path)
	}
	ss, err := a.Store.AllSources()
	if err != nil {
		return err
	}
	linkKey := "directory.links:" + d.ID
	linkRoot := a.Store.Meta(linkKey)
	if linkRoot == "" {
		linkRoot = d.Path
		if err := a.Store.SetMeta(linkKey, linkRoot); err != nil {
			return err
		}
	}
	if d.Kind == "game" {
		e, err := a.Store.EnvByID(strings.TrimPrefix(d.ID, "game:"))
		if err != nil {
			return err
		}
		if !env.LooksLikeGameRoot(path) {
			return fmt.Errorf("%s does not look like a RimWorld installation", path)
		}
		if _, _, err := a.EnvSet(e.Name, EnvSpec{GameRoot: path, directoryOnly: true}); err != nil {
			return err
		}
		for _, s := range ss {
			if s.Status == source.StatusRetired {
				continue
			}
			root := ""
			if s.EnvID == e.ID && s.Kind == source.Official {
				root = filepath.Join(path, "Data")
			}
			if s.EnvID == e.ID && s.Kind == source.Local {
				root = filepath.Join(path, "Mods")
			}
			if root == "" && source.Within(s.Root, d.Path) {
				rel, _ := filepath.Rel(d.Path, s.Root)
				root = filepath.Join(path, rel)
			}
			if root != "" {
				if err := a.Store.RebindSource(s, root); err != nil {
					return err
				}
			}
		}
	} else {
		for _, s := range ss {
			if s.Status == source.StatusRetired {
				continue
			}
			match := "source:"+s.ID == d.ID || d.ID == "lock" && (s.Label == LockLabel || a.IsLockSource(s)) || d.ID == "monitor" && s.Label == MonitorLabel || d.ID == SubscribedLabel && s.Label == SubscribedLabel || d.ID == "steamcmd-tool" && source.Within(s.Root, d.Path)
			if !match {
				continue
			}
			root := path
			if (d.ID == "lock" && s.Label != LockLabel || d.ID == "steamcmd-tool") && source.Within(s.Root, d.Path) {
				rel, _ := filepath.Rel(d.Path, s.Root)
				root = filepath.Join(path, rel)
			}
			if err := a.Store.RebindSource(s, root); err != nil {
				return err
			}
		}
		if d.ID == "steamcmd-tool" {
			if err := a.Store.SetMeta("cfg:steamcmd.path", steamCMDIn(path)); err != nil {
				return err
			}
		}
		key := map[string]string{"lock": "lock.dir", "source": "source.dir", "packs": "pack.dir", "monitor": "monitor.dir", SubscribedLabel: "subscribed.dir"}[d.ID]
		if key != "" {
			if err := a.Store.SetMeta("cfg:"+key, path); err != nil {
				return err
			}
		}
	}
	if err := a.Store.SetMeta(directoryKey(d.ID), path); err != nil {
		return err
	}
	a.ixCache, a.stCache, a.prCache, a.linkInst = nil, nil, nil, nil
	forgetLinks(a.Store)
	return a.Store.SetMeta(linkKey, "")
}

// PlanDirectoryMove previews the common incoming-link plan without writing files.
func (a *App) PlanDirectoryMove(id, path string) (*LinkLifecyclePlan, error) {
	d, err := a.managedDirectory(id)
	if err != nil {
		return nil, err
	}
	path, err = directoryPath(path)
	if err != nil {
		return nil, err
	}
	if source.IsLink(d.Path) || directoriesOverlap(d.Path, path) {
		return nil, fmt.Errorf("source is a link or old and new directories overlap")
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("destination is occupied or inaccessible: %s", path)
	}
	if err := a.validateDirectory(d, path); err != nil {
		return nil, err
	}
	p, err := a.PlanIncomingLinks(d.Path, path)
	if err == nil {
		p.Directory = d.ID
	}
	return p, err
}

// MoveDirectory detaches incoming links before a rename or checked copy/recycle,
// updates the records, then recreates entries at their new locations.
func (a *App) MoveDirectory(id, path string) (copied bool, err error) {
	if id == "source" {
		release, err := a.sourceAccess(context.Background(), true, false)
		if err != nil {
			return false, err
		}
		defer release()
	}
	d, err := a.managedDirectory(id)
	if err != nil {
		return false, err
	}
	path, err = directoryPath(path)
	if err != nil {
		return false, err
	}
	if st, err := os.Stat(d.Path); err != nil {
		return false, fmt.Errorf("old directory: %w; move it manually, then set its new location (archo folder set)", err)
	} else if !st.IsDir() {
		return false, fmt.Errorf("old path is not a directory")
	}
	if source.IsLink(d.Path) {
		return false, fmt.Errorf("the old directory is a link; move it manually, then set its new location (archo folder set)")
	}
	if directoriesOverlap(d.Path, path) {
		return false, fmt.Errorf("old and new directories overlap")
	}
	if _, err := os.Lstat(path); err == nil {
		return false, fmt.Errorf("destination %s is occupied", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if !dirExists(filepath.Dir(path)) {
		return false, fmt.Errorf("destination parent directory must exist")
	}
	if err := a.validateDirectory(d, path); err != nil {
		return false, err
	}
	es, err := a.Store.Envs()
	if err != nil {
		return false, err
	}
	for _, e := range es {
		if g := a.probe()(e.GameRoot); g != nil {
			return false, fmt.Errorf("a game is running (pid %d); close it before moving directories", g.PID)
		}
	}
	if d.Kind == "game" && !env.LooksLikeGameRoot(d.Path) {
		return false, fmt.Errorf("the old directory does not look like a RimWorld installation")
	}
	if d.ID == "steamcmd-tool" && steamCMDIn(d.Path) == "" {
		return false, fmt.Errorf("the old directory contains no SteamCMD program")
	}
	same := a.SameVolume
	if same == nil {
		same = func(from, to string) bool {
			return strings.EqualFold(filepath.VolumeName(from), filepath.VolumeName(to))
		}
	}
	copied = !same(d.Path, path)
	links, err := a.PlanIncomingLinks(d.Path, path)
	if err != nil {
		return false, err
	}
	links.Directory = d.ID
	if err := a.beginLinkLifecycle(links); err != nil {
		return false, err
	}
	var trashErr error
	if copied {
		err = copyChecked(d.Path, path)
		if err != nil {
			return false, a.abortLinkLifecycle(links, fmt.Errorf("could not copy the directory: %w; move it manually, then set its new location (archo folder set)", err))
		}
		trashErr = a.trash(d.Path)
	} else if err := os.Rename(d.Path, path); err != nil {
		return false, a.abortLinkLifecycle(links, fmt.Errorf("could not safely rename the directory: %w; move it manually, then set its new location (archo folder set)", err))
	}
	if err := a.setDirectoryRecords(d, path); err != nil {
		return copied, a.abortLinkLifecycle(links, fmt.Errorf("the directory was moved to %s, but updating records or links failed: %w; set this location to finish (archo folder set)", path, err))
	}
	if err := a.finishLinkLifecycle(links); err != nil {
		return copied, err
	}
	if trashErr != nil {
		return copied, fmt.Errorf("the directory was copied to %s and RimArcho uses it there, but the original could not go to the recycle bin: %v (remove %s yourself)", path, trashErr, d.Path)
	}
	return copied, nil
}

// managedRootReady prevents a previously used managed root from silently being
// recreated at its old location. It is used only before writes, never launch checks.
func (a *App) managedRootReady(id, path string) error {
	ss, err := a.Store.AllSources()
	if err != nil {
		return err
	}
	used := a.Store.Meta(directoryKey(id)) != ""
	for _, s := range ss {
		if s.Status == source.StatusRetired {
			continue
		}
		if id == "lock" && (s.Label == LockLabel || a.IsLockSource(s)) || id == "monitor" && s.Label == MonitorLabel || id == SubscribedLabel && s.Label == SubscribedLabel {
			used = true
			if s.Status == source.StatusRebindPending || ((s.Label == LockLabel || s.Label == MonitorLabel || s.Label == SubscribedLabel) && env.PathKey(s.Root) != env.PathKey(path)) {
				return fmt.Errorf("the %s directory has an unresolved location change; locate it in settings before writing to it", id)
			}
		}
	}
	if used && !dirExists(path) {
		return fmt.Errorf("the %s directory %s is unavailable; enter its new location in settings, or reconnect the drive before writing to it", id, path)
	}
	return nil
}
