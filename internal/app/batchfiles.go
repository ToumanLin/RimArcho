package app

import (
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/source"
)

// The files lane's batches (06 §2.16): every item is checked first, the folders then move
// several at a time, and one scan follows the whole batch instead of one per folder.

// TrashPlan checks that ref may go to the recycle bin and returns its real folder; nothing
// is moved (Trash's checks and the recycle bin's allow-list, 08 §7).
func (a *App) TrashPlan(ref string) (string, error) {
	op, err := a.Trash(ref, true)
	if err != nil {
		return "", err
	}
	check := a.trashAllow
	if check == nil {
		check = a.trashAllowed
	}
	if err := check(op.Path); err != nil {
		return "", err
	}
	return op.Path, nil
}

// TrashPath moves a folder TrashPlan returned to the recycle bin. It touches no other state
// of the app, so several can run at once.
func (a *App) TrashPath(path string) error {
	trash := a.TrashFunc
	if trash == nil {
		trash = source.Trash
	}
	return trash(path)
}

// UnhidePlan checks that ref is hidden and returns the rename that unhides it.
func (a *App) UnhidePlan(ref string) (from, to string, err error) {
	op, err := a.Unhide(ref, true)
	if err != nil {
		return "", "", err
	}
	if err := a.checkRoots(filepath.Dir(filepath.Dir(op.Path))); err != nil {
		return "", "", err
	}
	return op.Path, op.To, nil
}
