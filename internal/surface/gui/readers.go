package gui

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
)

type logReader struct {
	mu sync.Mutex
	a  *app.App
}

// detachedReader snapshots invocation options without building the board. It also works
// before the first environment exists (settings, detection and uninstall previews).
// The caller owns the connection and closes it after constructing the view model.
func (s *API) detachedReader() (*app.App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.a.ReadSession()
	if err != nil {
		return nil, err
	}
	if e, err := s.a.Env(); err == nil {
		r.EnvName = e.Name
	}
	r.AnalyzeExe, r.AnalyzeWait = s.Exe, 200*time.Millisecond
	return r, nil
}

// pageReader is called with s.mu held, only to snapshot the invocation settings.
// All slow work then uses this separate connection without the shared API mutex.
func (s *API) pageReader() (*app.App, error) {
	st := s.boardState()
	if st == nil {
		return nil, errors.New("no list")
	}
	r, err := s.a.ReadSession()
	if err != nil {
		return nil, err
	}
	r.EnvName = st.Ix.Env.Name
	r.AnalyzeExe, r.AnalyzeWait = s.Exe, 200*time.Millisecond
	return r, nil
}

// logSession is called with logRead.mu held; only snapshotting options needs s.mu.
func (s *API) logSession() (*app.App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, err := s.a.Env(); err != nil {
		return nil, err
	} else if s.board != nil && s.board.St != nil && s.board.St.Ix.Env.ID != e.ID {
		s.board, s.logLinks, s.logPath = nil, nil, ""
	}
	st := s.boardState()
	if st == nil {
		return nil, errors.New("no list")
	}
	if s.logRead.a == nil {
		var err error
		s.logRead.a, err = s.pageReader()
		if err != nil {
			return nil, err
		}
	}
	r := s.logRead.a
	r.ReadOptions(s.a)
	r.EnvName, r.AnalyzeExe, r.AnalyzeWait = st.Ix.Env.Name, s.Exe, 200*time.Millisecond
	return r, nil
}

// Close releases the page reader's database connection (also used by the dev harness).
func (s *API) Close() {
	s.buildRead.workers.Wait()
	s.tasks.Close() // what still runs or waits is stopped with the window (06 §2.16)
	s.mu.Lock()
	if s.texCancel != nil {
		s.texCancel()
		s.texCancel = nil
	}
	s.mu.Unlock()
	s.logRead.mu.Lock()
	defer s.logRead.mu.Unlock()
	if s.logRead.a != nil {
		s.logRead.a.Close()
		s.logRead.a = nil
	}
	s.texRead.mu.Lock()
	defer s.texRead.mu.Unlock()
	if s.texRead.a != nil {
		s.texRead.a.Close()
		s.texRead.a = nil
	}
}

// beginTextureRead supersedes an older request before it can queue more disk reads.
func (s *API) beginTextureRead() (context.Context, context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.texCancel != nil {
		s.texCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.texCancel = cancel
	return ctx, cancel
}

// textureSession is called with texRead.mu held. Capture small immutable targets from
// the current index; the reader keeps its own connection but never loads a second index.
func (s *API) textureSession(scope string) (*app.App, []app.TextureTarget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.texRead.a == nil {
		var err error
		s.texRead.a, err = s.pageReader()
		if err != nil {
			return nil, nil, err
		}
	}
	r := s.texRead.a
	r.ReadOptions(s.a)
	st := s.boardState()
	if st == nil {
		return nil, nil, errors.New("no list")
	}
	r.EnvName = st.Ix.Env.Name
	targets, err := s.a.TextureTargets(nil, scope)
	return r, targets, err
}
