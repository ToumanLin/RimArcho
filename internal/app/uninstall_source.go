package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// Recheck the recorded root and refuse redirection before preview or deletion.
// The existing tree remover never enters link targets, even if a link appears later.
func (a *App) checkUninstallSource(path string) error {
	if !filepath.IsAbs(path) || env.PathKey(path) != env.PathKey(a.SourceDir()) {
		return fmt.Errorf("refusing source removal outside the recorded library: %s", path)
	}
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		if source.IsLink(p) {
			return fmt.Errorf("source library path is a link: %s; relocate it in settings before uninstalling", p)
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	if err := a.validateDirectory(&ManagedDirectory{ID: "source"}, path); err != nil {
		return fmt.Errorf("cannot remove source library %s: %w", path, err)
	}
	if fi, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	} else if !fi.IsDir() {
		return fmt.Errorf("source library is not a directory: %s", path)
	}
	return filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if source.IsLink(p) {
			return fmt.Errorf("source library contains a link: %s; remove the link before uninstalling", p)
		}
		return nil
	})
}

func (a *App) uninstallSource(path string) error {
	unlock, err := a.sourceAccess(context.Background(), true, false)
	if err != nil {
		return err
	}
	defer unlock()
	if err := a.checkUninstallSource(path); err != nil {
		return err
	}
	return removeTree(path)
}
