package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type stagedDataFile struct{ tmp, dst string }

func stageDataFile(dst string, b []byte) (stagedDataFile, error) {
	f, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".*.tmp")
	if err != nil {
		return stagedDataFile{}, err
	}
	_, writeErr := f.Write(b)
	if err := errors.Join(writeErr, f.Close()); err != nil {
		os.Remove(f.Name())
		return stagedDataFile{}, err
	}
	return stagedDataFile{f.Name(), dst}, nil
}

// All files are staged first. Keep the previous set until publication and the
// generation record succeed; restore it on errors (08 §2.3). This is rollback,
// not a transaction across process crashes or power loss.
func replaceDataFiles(files []stagedDataFile, rename func(string, string) error, commit func() error) error {
	if len(files) == 0 {
		return nil
	}
	dir, err := os.MkdirTemp(filepath.Dir(files[0].dst), ".community-data-backup-*")
	if err != nil {
		return err
	}
	backups := make([]stagedDataFile, 0, len(files))
	var published []string
	rollback := func(cause error) error {
		var failures []error
		for _, dst := range published {
			if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, err)
			}
		}
		for i := len(backups) - 1; i >= 0; i-- {
			file := backups[i]
			if err := rename(file.tmp, file.dst); err != nil {
				failures = append(failures, err)
			}
		}
		if len(failures) != 0 {
			// Retain anything not restored for manual recovery.
			return errors.Join(cause, fmt.Errorf("community data rollback failed; backups kept at %s: %w", dir, errors.Join(failures...)))
		}
		os.RemoveAll(dir)
		return cause
	}
	for _, file := range files {
		info, err := os.Lstat(file.dst)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return rollback(err)
		}
		if !info.Mode().IsRegular() {
			return rollback(fmt.Errorf("community data destination is not a regular file: %s", file.dst))
		}
		backup := filepath.Join(dir, filepath.Base(file.dst))
		if err := rename(file.dst, backup); err != nil {
			return rollback(err)
		}
		backups = append(backups, stagedDataFile{backup, file.dst})
	}
	for _, file := range files {
		if err := rename(file.tmp, file.dst); err != nil {
			return rollback(err)
		}
		published = append(published, file.dst)
	}
	if err := commit(); err != nil {
		return rollback(err)
	}
	os.RemoveAll(dir)
	return nil
}
