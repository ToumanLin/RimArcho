package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/oplog"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

// ErrNotInTrashList: a path outside the recycle bin's allow-list (02 §3).
var ErrNotInTrashList = errors.New("not something RimArcho moves to the recycle bin")

// ToTrash is the one way into the recycle bin (02 §3). Whoever calls it, the path must be
// on the allow-list; the caller's own rules (enabled, used by a plan, subscribed) come first.
// Every call is a row in the operation log (13 §1.3): ok, refused (not on the allow-list,
// gone) or failed.
func (a *App) ToTrash(path string) error { return a.toTrash(nil, path) }

// toTrash is ToTrash whose row belongs to the parent row h (a batch of saves, uninstall).
func (a *App) toTrash(h *oplog.Handle, path string) error {
	if err := a.trashCheck(path); err != nil {
		a.logTrash(h, path, err, true)
		return err
	}
	err := a.withIncomingLinks(path, "", func() error { return a.recycleChecked(path) })
	a.logTrash(h, path, err, false)
	return err
}

// recycleTarget is used inside an already journalled lifecycle. It retains the
// existing trash allow-list; detaching a link never goes through this function.
func (a *App) recycleTarget(path string) error { return a.recycleTargetIn(nil, path) }

// recycleTargetIn is recycleTarget whose row belongs to the parent row h (a subscription
// removal).
func (a *App) recycleTargetIn(h *oplog.Handle, path string) error {
	err := a.recycleChecked(path)
	a.logTrash(h, path, err, refusedRemoval(err))
	return err
}

// deleteChecked deletes a Mod folder for good (quick delete, skipping the recycle bin),
// inside an already journalled lifecycle: the path must be on the recycle bin's allow-list
// all the same (02 §3). Every call is a delete row (13 §1.3), the refusals too.
func (a *App) deleteChecked(h *oplog.Handle, path string) error {
	if err := a.trashAllowed(path); err != nil {
		a.logDelete(h, path, err, true)
		return err
	}
	return a.deleteTree(h, path)
}

// deleteTree is the only caller of removeModTree: every folder RimArcho deletes without
// the recycle bin is a delete row (13 §1.3; TestArchOpLog checks the callers).
func (a *App) deleteTree(h *oplog.Handle, path string) error {
	err := removeModTree(path)
	a.logDelete(h, path, err, errors.Is(err, os.ErrNotExist))
	return err
}

// refusedRemoval: a recycle or delete turned down before anything changed.
func refusedRemoval(err error) bool {
	return errors.Is(err, ErrNotInTrashList) || errors.Is(err, os.ErrNotExist)
}

func (a *App) recycleChecked(path string) error {
	if err := a.trashCheck(path); err != nil {
		return err
	}
	return a.trash(path)
}

func (a *App) trashCheck(path string) error {
	check := a.trashAllow
	if check == nil {
		check = a.trashAllowed
	}
	return check(path)
}

// trash moves path to the recycle bin without the allow-list. Only a caller that has its
// own reason uses it: MoveDirectory, once a managed directory's checked copy is in place.
func (a *App) trash(path string) error {
	trash := a.TrashFunc
	if trash == nil {
		trash = source.Trash
	}
	return trash(path)
}

// trashAllowed checks a path against the allow-list of 02 §3 (02 §1):
//   - a mod folder: inside a source folder (workshop, SteamCMD, Mods/, the lock library or
//     another mod folder), not the source folder itself;
//   - a pack: a folder directly in the pack folder;
//   - saves: inside a game's Saves/, an own save folder (<data>/userdata/<env>), or the save
//     backups (<data>/backups/saves);
//   - a game folder downloaded with SteamCMD (uninstall, when the player chose to delete it);
//   - a Mod settings file Mod_*.xml directly in an environment's user data Config/.
//
// A link is never moved, whatever it points at.
func (a *App) trashAllowed(path string) error {
	if path == "" {
		return fmt.Errorf("%w: no path", ErrNotInTrashList)
	}
	if source.IsLink(path) {
		return fmt.Errorf("%w: %s is a link, not a folder", ErrNotInTrashList, path)
	}
	if _, err := os.Lstat(path); err != nil {
		return err
	}
	p := realOrAbs(path)
	inside := func(root string) bool { // strictly below root
		if root == "" {
			return false
		}
		r := realOrAbs(root)
		return source.Within(p, r) && !source.Within(r, p)
	}
	child := func(root string) bool { // directly in root
		return root != "" && samePath(filepath.Dir(p), root)
	}

	// Mod folders: every source of every environment, and each game's Mods/.
	if srcs, err := a.Store.AllSources(); err == nil {
		for _, s := range srcs {
			if s.Status != source.StatusRetired && (s.Label == SubscribedLabel || s.Kind == source.SteamCMD) {
				if child(s.Root) {
					if s.Kind == source.SteamCMD {
						return nil
					}
					if _, err := subscribe.Read(path); err == nil {
						return nil
					}
				}
				continue
			}
			if s.Status != source.StatusRetired && s.Kind != source.Official && inside(s.Root) {
				return nil
			}
		}
	}
	if inside(a.LockDir()) || child(a.PacksDir()) || child(a.OwnSavesRoot()) {
		return nil
	}
	if bk := filepath.Join(a.Home, "backups", "saves"); inside(bk) || samePath(p, bk) {
		return nil
	}
	// A save in a pack's own folder: <packs>/<pack>/userdata/Saves/<file> (12 §4.4).
	if dir := filepath.Dir(p); strings.EqualFold(filepath.Base(dir), "Saves") && strings.EqualFold(filepath.Base(filepath.Dir(dir)), "userdata") &&
		samePath(filepath.Dir(filepath.Dir(filepath.Dir(dir))), realOrAbs(a.PacksDir())) {
		return nil
	}
	envs, err := a.Store.Envs()
	if err != nil {
		return err
	}
	for _, e := range envs {
		switch {
		case e.GameRoot != "" && inside(filepath.Join(e.GameRoot, "Mods")):
			return nil
		case e.UserDataRoot != "" && inside(filepath.Join(e.UserDataRoot, "Saves")):
			return nil
		case e.UserDataRoot == "" && a.DefaultUserData() != "" && inside(filepath.Join(a.DefaultUserData(), "Saves")):
			return nil // the game's default user data folder
		case downloadedGame(e.GameRoot) && samePath(p, e.GameRoot):
			return nil
		}
	}
	if a.settingsFileTrashable(p) {
		return nil // a Mod settings file in a Game Config's Config/ (16 §2.9)
	}
	return fmt.Errorf("%w: %s is not a mod folder, a pack, saves, a Mod settings file or a downloaded game", ErrNotInTrashList, path)
}

// realOrAbs resolves links where the path exists, else makes it absolute.
func realOrAbs(p string) string {
	if r, err := source.RealPath(p); err == nil {
		return r
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

func samePath(p, q string) bool {
	p = realOrAbs(p)
	return source.Within(p, realOrAbs(q)) && source.Within(realOrAbs(q), p)
}
