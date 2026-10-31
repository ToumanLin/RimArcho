package selfupdate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var rename = os.Rename

// Swap records a replacement so the new files and previous installation can be restored.
type Swap struct {
	Program, Stage, Old string
	steps               []swapStep
}

type swapStep struct {
	from, to string
	undone   bool
}

// Apply moves old files aside before installing staged entries, undoing failed replacements.
func Apply(program, stage, old string) (*Swap, error) {
	for _, dir := range []string{program, stage} {
		info, err := os.Stat(dir)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("update path %q is not a directory", dir)
		}
	}
	if err := os.Mkdir(old, 0o755); err != nil {
		return nil, err
	}
	s := &Swap{Program: program, Stage: stage, Old: old}
	fail := func(err error) (*Swap, error) {
		undoErr := s.Rollback()
		return nil, errors.Join(err, undoErr)
	}
	move := func(from, to string) error {
		if err := rename(from, to); err != nil {
			return err
		}
		s.steps = append(s.steps, swapStep{from: from, to: to})
		return nil
	}
	for _, name := range Files {
		staged := filepath.Join(stage, name)
		if _, err := os.Lstat(staged); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return fail(err)
		}
		installed := filepath.Join(program, name)
		if _, err := os.Lstat(installed); err == nil {
			if err := move(installed, filepath.Join(old, name)); err != nil {
				return fail(err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fail(err)
		}
		if err := move(staged, installed); err != nil {
			return fail(err)
		}
	}
	return s, nil
}

// Rollback reverses completed renames and keeps trying after individual failures.
func (s *Swap) Rollback() error {
	var errs []error
	for i := len(s.steps) - 1; i >= 0; i-- {
		step := &s.steps[i]
		if step.undone {
			continue
		}
		// A failed earlier undo may leave a new file here; never overwrite it on Unix.
		if _, err := os.Lstat(step.from); err == nil {
			errs = append(errs, fmt.Errorf("rollback destination %q already exists", step.from))
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
			continue
		}
		if err := rename(step.to, step.from); err != nil {
			errs = append(errs, err)
		} else {
			step.undone = true
		}
	}
	if len(errs) == 0 {
		if err := os.Remove(s.Old); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
