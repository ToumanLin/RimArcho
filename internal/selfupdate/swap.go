package selfupdate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var rename = os.Rename

// Swap records a replacement so the new files and previous installation can be restored.
// With a journal (ApplyRecorded) the record is also on disk, so a process killed mid-swap
// leaves enough for Resume to finish or undo it at the next start.
type Swap struct {
	Program, Stage, Old string
	// Version is the release being installed, kept in the journal for the next start's check.
	Version string
	journal string
	plan    []journalEntry
	steps   []swapStep
}

type swapStep struct {
	from, to string
	undone   bool
}

// Journal states: "replacing" until the caller confirms the new files (Commit), then the
// journal is removed; "rolling-back" from the first undo until the old files are back.
const (
	SwapReplacing   = "replacing"
	SwapRollingBack = "rolling-back"
)

// swapJournal is the file form of a Swap. Paths are relative to the journal's folder so a
// moved program folder still resolves. Each entry renames Program/Name to Old/Name (when
// HadOld) and then Stage/Name to Program/Name; because Old starts empty and every staged
// entry exists when the journal is written, Old/Name existing means the first rename
// happened and Stage/Name missing means the second did.
type swapJournal struct {
	State   string         `json:"state"`
	Version string         `json:"version,omitempty"`
	Program string         `json:"program"`
	Stage   string         `json:"stage"`
	Old     string         `json:"old"`
	Entries []journalEntry `json:"entries"`
}

type journalEntry struct {
	Name   string `json:"name"`
	HadOld bool   `json:"hadOld,omitempty"`
}

// Apply moves old files aside before installing staged entries, undoing failed replacements.
func Apply(program, stage, old string) (*Swap, error) {
	return ApplyRecorded(program, stage, old, "", "")
}

// ApplyRecorded is Apply that first writes journal (when not empty): the planned renames
// and the version being installed. The caller removes it with Commit once the new files
// are confirmed; Rollback removes it once the old files are back.
func ApplyRecorded(program, stage, old, journal, version string) (*Swap, error) {
	for _, dir := range []string{program, stage} {
		info, err := os.Stat(dir)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("update path %q is not a directory", dir)
		}
	}
	var entries []journalEntry
	for _, name := range Files {
		if _, err := os.Lstat(filepath.Join(stage, name)); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		_, err := os.Lstat(filepath.Join(program, name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		entries = append(entries, journalEntry{Name: name, HadOld: err == nil})
	}
	if err := os.Mkdir(old, 0o755); err != nil {
		return nil, err
	}
	s := &Swap{Program: program, Stage: stage, Old: old, Version: version, journal: journal, plan: entries}
	if err := s.record(SwapReplacing); err != nil {
		os.Remove(old)
		return nil, err
	}
	for _, e := range entries {
		if e.HadOld {
			s.steps = append(s.steps, swapStep{from: filepath.Join(program, e.Name), to: filepath.Join(old, e.Name), undone: true})
		}
		s.steps = append(s.steps, swapStep{from: filepath.Join(stage, e.Name), to: filepath.Join(program, e.Name), undone: true})
	}
	if err := s.forward(); err != nil {
		return nil, errors.Join(err, s.Rollback())
	}
	return s, nil
}

// forward performs the steps not done yet, in order.
func (s *Swap) forward() error {
	for i := range s.steps {
		step := &s.steps[i]
		if !step.undone {
			continue
		}
		if _, err := os.Lstat(step.to); err == nil {
			return fmt.Errorf("update destination %q already exists", step.to)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := rename(step.from, step.to); err != nil {
			return err
		}
		step.undone = false
	}
	return nil
}

// record writes the journal (no-op without one): a temporary file renamed into place.
func (s *Swap) record(state string) error {
	if s.journal == "" {
		return nil
	}
	dir := filepath.Dir(s.journal)
	rel := func(p string) (string, error) { return filepath.Rel(dir, p) }
	j := swapJournal{State: state, Version: s.Version, Entries: s.plan}
	var err error
	if j.Program, err = rel(s.Program); err != nil {
		return err
	}
	if j.Stage, err = rel(s.Stage); err != nil {
		return err
	}
	if j.Old, err = rel(s.Old); err != nil {
		return err
	}
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	tmp := s.journal + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	if err = errors.Join(err, f.Close()); err == nil {
		err = os.Rename(tmp, s.journal)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// Commit confirms the new files: the journal is removed, the old files stay in Old for
// the caller's later clean-up.
func (s *Swap) Commit() error {
	if s.journal == "" {
		return nil
	}
	if err := os.Remove(s.journal); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Rollback reverses completed renames and keeps trying after individual failures.
func (s *Swap) Rollback() error {
	if err := s.record(SwapRollingBack); err != nil {
		return fmt.Errorf("cannot record the rollback, nothing was undone: %w", err)
	}
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
	if len(errs) == 0 {
		errs = append(errs, s.Commit())
	}
	return errors.Join(errs...)
}

// Resume reads a journal a killed process left and returns its swap with the renames done
// so far worked out from the files. A swap that was "replacing" is carried forward to the
// end (the staged files were verified before the journal was written), so the caller
// checks the new program and then calls Commit or Rollback; one that was "rolling-back" is
// returned for Rollback. No journal: nil, "", nil.
func Resume(journal string) (*Swap, string, error) {
	b, err := os.ReadFile(journal)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", nil
	} else if err != nil {
		return nil, "", err
	}
	var j swapJournal
	if err := json.Unmarshal(b, &j); err != nil {
		return nil, "", fmt.Errorf("unreadable update journal %s: %w", journal, err)
	}
	if j.State != SwapReplacing && j.State != SwapRollingBack {
		return nil, "", fmt.Errorf("update journal %s has unknown state %q", journal, j.State)
	}
	dir := filepath.Dir(journal)
	s := &Swap{Program: filepath.Join(dir, j.Program), Stage: filepath.Join(dir, j.Stage), Old: filepath.Join(dir, j.Old), Version: j.Version, journal: journal, plan: j.Entries}
	exists := func(p string) (bool, error) {
		_, err := os.Lstat(p)
		if err == nil {
			return true, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	for _, e := range j.Entries {
		if e.Name == "" || filepath.Base(e.Name) != e.Name {
			return nil, "", fmt.Errorf("update journal %s names %q", journal, e.Name)
		}
		if e.HadOld {
			moved, err := exists(filepath.Join(s.Old, e.Name))
			if err != nil {
				return nil, "", err
			}
			s.steps = append(s.steps, swapStep{from: filepath.Join(s.Program, e.Name), to: filepath.Join(s.Old, e.Name), undone: !moved})
		}
		staged, err := exists(filepath.Join(s.Stage, e.Name))
		if err != nil {
			return nil, "", err
		}
		s.steps = append(s.steps, swapStep{from: filepath.Join(s.Stage, e.Name), to: filepath.Join(s.Program, e.Name), undone: staged})
	}
	if j.State == SwapReplacing {
		if err := os.MkdirAll(s.Old, 0o755); err != nil {
			return nil, "", err
		}
		if err := s.forward(); err != nil {
			return s, j.State, err
		}
	}
	return s, j.State, nil
}
