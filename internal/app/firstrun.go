package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

// SetManagedRoot chooses defaults before managed content exists. Existing per-folder
// overrides stay effective; moving existing content uses the folder move commands.
func (a *App) SetManagedRoot(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("choose an absolute folder path")
	}
	path = filepath.Clean(path)
	current := a.LocalDir()
	if !strings.EqualFold(directoryReal(current), directoryReal(path)) {
		if entries, err := os.ReadDir(current); err == nil && len(entries) > 0 {
			return errors.New("managed folders are already in use; move them individually in Settings")
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		if entries, err := os.ReadDir(path); err == nil && len(entries) > 0 {
			return errors.New("choose an empty folder for RimArcho's managed files")
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}

		program, err := a.SelfProgramDir()
		if err != nil {
			return err
		}
		if directoriesOverlap(path, program) {
			return errors.New("choose a separate install folder")
		}
		if directoriesOverlap(path, a.Home) {
			return errors.New("the managed folder overlaps RimArcho's data directory")
		}
		if _, ok := selfupdate.InWorkshopFolder(directoryReal(path)); ok {
			return errors.New("choose a folder outside the Steam Workshop")
		}
		ds, err := a.ManagedDirectories()
		if err != nil {
			return err
		}
		for _, d := range ds {
			if d.Kind == "game" || strings.HasPrefix(d.ID, "source:") {
				if directoriesOverlap(path, d.Path) {
					return fmt.Errorf("the managed folder overlaps %s", d.Path)
				}
			}
		}
	}
	return a.Store.SetMeta("cfg:local.dir", path)
}

const WorkshopCleanupEnv = "RIMARCHO_WORKSHOP_CLEANUP"

// RelocateWorkshop copies only release entries and launches the new GUI after a
// confirmed preview. A failed launch restores an existing installation.
func (a *App) RelocateWorkshop(target string) error {
	from, err := a.SelfProgramDir()
	if err != nil {
		return err
	}
	if _, ok := selfupdate.InWorkshopFolder(from); !ok {
		return errors.New("RimArcho is not running from a Workshop folder")
	}
	target, err = a.CheckRelocationTarget(target)
	if err != nil {
		return err
	}
	if len(a.OtherArchoIn(target)) > 0 {
		return errors.New("close the RimArcho running in the destination folder first")
	}

	size, err := selfupdate.ReleaseSize(from)
	if err != nil {
		return err
	}
	if free := DiskFree(target); free >= 0 && free < size*2 {
		return errors.New("not enough free space")
	}
	swap, err := selfupdate.CopyRelease(from, target)
	if err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(target, "RimArcho.exe"), os.Args[1:]...)
	cmd.Dir = target
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, WorkshopCleanupEnv+"=") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, WorkshopCleanupEnv+"="+from)

	if err := cmd.Start(); err != nil {
		undo := swap.Rollback()
		if undo == nil {
			return errors.Join(err, os.RemoveAll(swap.Stage))
		}
		return errors.Join(err, undo)
	}

	cmd.Process.Release()
	os.RemoveAll(swap.Stage)
	os.RemoveAll(swap.Old)
	return nil
}

func (a *App) CheckRelocationTarget(target string) (string, error) {
	if !filepath.IsAbs(target) {
		return "", errors.New("choose an absolute install folder")
	}
	target = directoryReal(filepath.Clean(target))
	from, err := a.SelfProgramDir()
	if err != nil {
		return "", err
	}
	if _, ok := selfupdate.InWorkshopFolder(target); ok {
		return "", errors.New("choose a folder outside the Steam Workshop")
	}
	if directoriesOverlap(target, from) || directoriesOverlap(target, a.Home) || directoriesOverlap(target, a.LocalDir()) {
		return "", errors.New("choose a separate install folder")
	}
	ds, err := a.ManagedDirectories()
	if err != nil {
		return "", err
	}
	for _, d := range ds {
		if directoriesOverlap(target, d.Path) {
			return "", fmt.Errorf("the install folder overlaps %s", d.Path)
		}
	}
	_, installed := os.Stat(filepath.Join(target, "RimArcho.exe"))
	if installed != nil {
		for _, name := range selfupdate.Files {
			if _, err := os.Lstat(filepath.Join(target, name)); err == nil {
				return "", fmt.Errorf("%s already contains %s; choose an empty install folder", target, name)
			}
		}
	}
	return target, nil
}

// FinishWorkshopRelocation carries the previous process's confirmed cleanup into
// the new copy. Wait for every old-folder process, and never clean from that folder.
func (a *App) FinishWorkshopRelocation() error {
	old := os.Getenv(WorkshopCleanupEnv)
	os.Unsetenv(WorkshopCleanupEnv)
	if old == "" {
		return nil
	}
	current, err := a.SelfProgramDir()
	if err != nil {
		return err
	}
	if _, ok := selfupdate.InWorkshopFolder(current); ok {
		return errors.New("relocation cleanup requires the installed copy")
	}
	if _, ok := selfupdate.InWorkshopFolder(old); !ok || directoriesOverlap(current, old) {
		return errors.New("invalid Workshop cleanup folder")
	}
	deadline := time.Now().Add(30 * time.Second)
	for len(a.OtherArchoIn(old)) > 0 {
		if time.Now().After(deadline) {
			return errors.New("Workshop release cleanup is waiting for another RimArcho to close")
		}
		time.Sleep(200 * time.Millisecond)
	}
	return selfupdate.CleanWorkshopRelease(old)
}
