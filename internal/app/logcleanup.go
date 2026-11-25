package app

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

// LogCleanup is the preview of an irreversible change to a managed oversized log (06 §4).
type LogCleanup struct {
	Path      string
	Size      int64
	Omitted   int64
	Remaining int64
	Token     string
}

var (
	ErrLogCleanupTarget  = errors.New("Choose an oversized Player.log or Player-prev.log for this game.")
	ErrLogCleanupChanged = errors.New("The log changed. Review its new size and confirm again.")
)

func (a *App) openLogCleanup(path string) (*os.File, os.FileInfo, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, nil, err
	}
	cur, prev := a.LogPaths(st)
	switch {
	case sameLog(path, cur):
		path = cur
	case sameLog(path, prev):
		path = prev
	default:
		return nil, nil, ErrLogCleanupTarget
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !fi.Mode().IsRegular() || fi.Size() <= LogSizeLimit {
		return nil, nil, ErrLogCleanupTarget
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	if opened, err := f.Stat(); err != nil || !sameLogFile(fi, opened) {
		f.Close()
		return nil, nil, ErrLogCleanupChanged
	}
	return f, fi, nil
}

func sameLogFile(before, after os.FileInfo) bool {
	return os.SameFile(before, after) && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime())
}

func logCleanupPreview(f *os.File, fi os.FileInfo, trim bool) (LogCleanup, logSample, error) {
	s, err := logSections(f, fi.Size())
	if err != nil {
		return LogCleanup{}, logSample{}, err
	}
	marker := logs.TrimMarker(s.omitted())
	token := fmt.Sprintf("%s|%t|%d|%d|%d|%d|%d", f.Name(), trim, fi.Size(), fi.ModTime().UnixNano(),
		logCreationTime(f.Name(), fi).UnixNano(), s.headEnd, s.tailStart)
	return LogCleanup{
		Path: f.Name(), Size: fi.Size(), Omitted: s.omitted(),
		Remaining: fi.Size() - s.omitted() + int64(len(marker)),
		Token:     fmt.Sprintf("%x", sha256.Sum256([]byte(token))),
	}, s, nil
}

// PreviewLogCleanup calculates the original, remaining and omitted sizes from the same
// line boundaries that analysis and trimming use.
func (a *App) PreviewLogCleanup(path string, trim bool) (LogCleanup, error) {
	f, fi, err := a.openLogCleanup(path)
	if err != nil {
		return LogCleanup{}, err
	}
	defer f.Close()
	p, _, err := logCleanupPreview(f, fi, trim)
	return p, err
}

// ApplyLogCleanup refuses changes while a game runs and rechecks the confirmed file
// immediately before replacing or deleting it.
func (a *App) ApplyLogCleanup(path, token string, trim bool) error {
	unlock, err := a.lockLaunch()
	if err != nil {
		return err
	}
	defer unlock()
	if err := a.refuseWhileRunning(); err != nil {
		return err
	}
	f, fi, err := a.openLogCleanup(path)
	if err != nil {
		return err
	}
	defer f.Close()
	p, s, err := logCleanupPreview(f, fi, trim)
	if err != nil {
		return err
	}
	if p.Token != token {
		return ErrLogCleanupChanged
	}
	var temp string
	if trim {
		temp, err = stageLogTrim(f, fi, s)
		if err != nil {
			return err
		}
		defer os.Remove(temp)
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := a.refuseWhileRunning(); err != nil {
		return err
	}
	if latest, err := os.Lstat(p.Path); err != nil || !sameLogFile(fi, latest) {
		return ErrLogCleanupChanged
	}
	if trim {
		err = os.Rename(temp, p.Path)
	} else {
		err = os.Remove(p.Path)
	}
	if err == nil {
		a.logCache = nil
	}
	return err
}

func stageLogTrim(f *os.File, fi os.FileInfo, s logSample) (name string, err error) {
	tmp, err := os.CreateTemp(filepath.Dir(f.Name()), ".rimarcho-log-*")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	head, tail := s.readers(f)
	if _, err = io.CopyN(tmp, head, head.Size()); err != nil {
		return "", err
	}
	if _, err = io.WriteString(tmp, logs.TrimMarker(s.omitted())); err != nil {
		return "", err
	}
	if tail != nil {
		if _, err = io.CopyN(tmp, tail, tail.Size()); err != nil {
			return "", err
		}
	}
	if err = tmp.Chmod(fi.Mode().Perm()); err != nil {
		return "", err
	}
	if err = tmp.Sync(); err != nil {
		return "", err
	}
	if err = tmp.Close(); err != nil {
		return "", err
	}
	// Attribution compares the log's modification time with recorded launches.
	if err = os.Chtimes(tmp.Name(), fi.ModTime(), fi.ModTime()); err != nil {
		return "", err
	}
	return tmp.Name(), nil
}
