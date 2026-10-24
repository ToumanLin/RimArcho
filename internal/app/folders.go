package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Mod folders (02 §0): the folders every environment shares — the Workshop folder (found
// from Steam), SteamCMD folders, other folders, and RimArcho's own Archo Monitor folder and
// lock library. Their instances, and everything the player says about them, are the same
// whichever game is played. They are managed in one place: `archo folder`, the settings
// page's "Mod 文件夹".

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// SharedKind reports whether sources of this kind are shared mod folders.
func SharedKind(k source.Kind) bool {
	return k == source.Workshop || k == source.SteamCMD || k == source.Extra
}

// OwnFolder reports whether a shared folder is one RimArcho keeps itself (the monitor
// folder, the lock library and its older per-environment folders): the player does not
// add or remove those.
func OwnFolder(s *source.Source) bool {
	return s.Kind == source.Extra && (s.Label == MonitorLabel || s.Label == SubscribedLabel || s.Label == LockLabel || strings.HasPrefix(s.Label, LockLabel+"-"))
}

// Folders lists the shared mod folders, retired ones last.
func (a *App) Folders() ([]*source.Source, error) {
	all, err := a.Store.SharedSources()
	if err != nil {
		return nil, err
	}
	var live, gone []*source.Source
	for _, s := range all {
		if s.Status == source.StatusRetired {
			gone = append(gone, s)
		} else {
			live = append(live, s)
		}
	}
	return append(live, gone...), nil
}

func (a *App) workshopFolder() (*source.Source, error) {
	srcs, err := a.Store.SharedSources()
	if err != nil {
		return nil, err
	}
	for _, s := range srcs {
		if s.Kind == source.Workshop && s.Status != source.StatusRetired {
			return s, nil
		}
	}
	return nil, nil
}

// DetectWorkshop is the Workshop folder Steam has for RimWorld ("" when none is found).
func (a *App) DetectWorkshop() string {
	if ws := env.WorkshopFolders(a.Detect); len(ws) > 0 {
		return ws[0]
	}
	return ""
}

// overlapping returns an active source of any environment, or shared, whose folder
// contains root or is inside it.
func (a *App) overlapping(root string, skip func(*source.Source) bool) (*source.Source, error) {
	all, err := a.Store.AllSources()
	if err != nil {
		return nil, err
	}
	for _, s := range all {
		if s.Status == source.StatusRetired || (skip != nil && skip(s)) {
			continue
		}
		if source.Within(root, s.Root) || source.Within(s.Root, root) {
			return s, nil
		}
	}
	return nil, nil
}

// FolderAdd adds a shared mod folder. There is one Workshop folder: naming another one
// moves it (re-bound as in 02 §3.2). A folder already known is left as it is.
func (a *App) FolderAdd(kind source.Kind, root string) (SourceChange, error) {
	if !SharedKind(kind) {
		return SourceChange{}, fmt.Errorf("a mod folder is workshop, steamcmd or extra, not %s (a game's Data/ and Mods/ come with its environment)", kind)
	}
	root = strings.Trim(strings.TrimSpace(root), `"'`)
	if root == "" {
		return SourceChange{}, fmt.Errorf("no folder given")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return SourceChange{}, err
	}
	if err := a.checkRoots(root); err != nil {
		return SourceChange{}, err
	}
	shared, err := a.Store.SharedSources()
	if err != nil {
		return SourceChange{}, err
	}
	for _, s := range shared {
		if s.Status != source.StatusRetired && env.PathKey(s.Root) == env.PathKey(root) {
			if s.Kind != kind {
				return SourceChange{}, fmt.Errorf("%s is already src:%s (a %s folder)", root, s.Label, s.Kind)
			}
			return SourceChange{Source: s, Action: "unchanged"}, nil
		}
	}
	var ws *source.Source
	if kind == source.Workshop {
		if ws, err = a.workshopFolder(); err != nil {
			return SourceChange{}, err
		}
	}
	if o, err := a.overlapping(root, func(s *source.Source) bool { return ws != nil && s.ID == ws.ID }); err != nil {
		return SourceChange{}, err
	} else if o != nil {
		where := "a shared folder"
		if o.EnvID != store.Shared {
			if e, _ := a.Store.EnvByID(o.EnvID); e != nil {
				where = "env:" + e.Name
			}
		}
		return SourceChange{}, fmt.Errorf("%s overlaps src:%s (%s, %s); mod folders must not contain each other", root, o.Label, o.Root, where)
	}
	if ws != nil {
		return a.rebind(ws, root)
	}
	all, err := a.Store.AllSources()
	if err != nil {
		return SourceChange{}, err
	}
	s := &source.Source{ID: a.NewID(), EnvID: store.Shared, Kind: kind, Label: nextLabel(all, kind), Root: root, Status: source.StatusOK}
	if !dirExists(root) {
		s.Status = source.StatusMissing
	}
	if err := a.Store.InsertSource(s); err != nil {
		return SourceChange{}, err
	}
	return SourceChange{Source: s, Action: "added"}, nil
}

// FolderRemove forgets a shared mod folder: it is retired, renamed <label>-removed, and its
// instances become tombstones on the next scan. The files stay where they are.
func (a *App) FolderRemove(ref string) (SourceChange, error) {
	label := strings.TrimPrefix(strings.TrimSpace(ref), "src:")
	all, err := a.Store.AllSources()
	if err != nil {
		return SourceChange{}, err
	}
	var s *source.Source
	for _, x := range all {
		if x.Label == label && x.Status != source.StatusRetired && (x.EnvID == store.Shared || x.Kind == source.Official || x.Kind == source.Local) {
			if s == nil || x.EnvID == store.Shared {
				s = x
			}
		}
	}
	switch {
	case s == nil:
		return SourceChange{}, fmt.Errorf("no mod folder src:%s (archo folder list)", label)
	case s.EnvID != store.Shared:
		return SourceChange{}, fmt.Errorf("src:%s is a game's %s folder; it comes and goes with its environment (change --game instead)", label, s.Kind)
	case OwnFolder(s):
		return SourceChange{}, fmt.Errorf("src:%s is RimArcho's own folder (%s); it is not removed", label, s.Root)
	}
	s.Status = source.StatusRetired
	s.Label = nextLabel(all, source.Kind(label+"-removed"))
	if err := a.Store.UpdateSource(s); err != nil {
		return SourceChange{}, err
	}
	return SourceChange{Source: s, Action: "removed"}, nil
}

// sharedSource registers one of RimArcho's own folders (the monitor folder, the lock
// library) as a shared extra source with the given label, or returns the one there is.
func (a *App) sharedSource(label, root string) (*source.Source, error) {
	shared, err := a.Store.SharedSources()
	if err != nil {
		return nil, err
	}
	for _, s := range shared {
		if s.Kind == source.Extra && s.Status != source.StatusRetired && env.PathKey(s.Root) == env.PathKey(root) {
			return s, nil
		}
	}
	if err := a.checkRoots(root); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	all, err := a.Store.AllSources()
	if err != nil {
		return nil, err
	}
	s := &source.Source{ID: a.NewID(), EnvID: store.Shared, Kind: source.Extra, Label: nextLabel(all, source.Kind(label)), Root: root, Status: source.StatusOK}
	return s, a.Store.InsertSource(s)
}

// SharedUnread reports whether a shared mod folder has never been read (no instance was
// ever recorded from it): adding a game then scans the shared folders too, otherwise only
// the game's own Data/ and Mods/ (02 §0 item 8).
func (a *App) SharedUnread() bool {
	fs, err := a.Folders()
	if err != nil {
		return true
	}
	var n int
	for _, s := range fs {
		if s.Status != source.StatusOK {
			continue
		}
		if err := a.Store.DB.QueryRow(`SELECT count(*) FROM instance WHERE source_id=?`, s.ID).Scan(&n); err != nil || n == 0 {
			return true
		}
	}
	return false
}

// adoptGameWorkshop registers the Workshop folder a SteamCMD game keeps inside its own
// folder (steamapps\workshop\content\294100) as a shared SteamCMD folder (02 §0). It is
// never the Workshop folder: that one is the Steam client's, and a game's own folder must
// not move it. Nothing happens when the folder is missing, is the Steam client's, or is
// known already, retired included: a folder the player removed stays removed.
func (a *App) adoptGameWorkshop(e *env.Environment) (*SourceChange, error) {
	w := env.GameWorkshopFor(e.GameRoot)
	if !dirExists(w) {
		return nil, nil
	}
	for _, s := range env.WorkshopFolders(a.Detect) {
		if env.PathKey(s) == env.PathKey(w) {
			return nil, nil
		}
	}
	all, err := a.Store.AllSources()
	if err != nil {
		return nil, err
	}
	for _, s := range all {
		if env.PathKey(s.Root) == env.PathKey(w) {
			return nil, nil
		}
	}
	if a.checkRoots(w) != nil {
		return nil, nil
	}
	if o, err := a.overlapping(w, nil); err != nil || o != nil {
		return nil, err
	}
	ch, err := a.FolderAdd(source.SteamCMD, w)
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

// adoptGameWorkshops runs adoptGameWorkshop for every environment: environments added
// before RimArcho knew these folders get theirs on the next scan.
func (a *App) adoptGameWorkshops() error {
	envs, err := a.Store.Envs()
	if err != nil {
		return err
	}
	for _, e := range envs {
		if _, err := a.adoptGameWorkshop(e); err != nil {
			return err
		}
	}
	return nil
}
