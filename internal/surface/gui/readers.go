package gui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

type logReader struct {
	mu      sync.Mutex
	a       *app.App
	board   *view.Board
	boardAt string
}

const subscriptionIdle = 5 * time.Minute

type subscriptionReader struct {
	inventoryReader
	idle *time.Timer
	used time.Time
}

// Called with the subscription reader locked, including by the expiry callback.
func (w *subscriptionReader) release() {
	if w.idle != nil {
		w.idle.Stop()
		w.idle = nil
	}
	if w.a != nil {
		w.a.Close()
		w.a = nil
	}
	w.lib, w.key = nil, ""
}

func (s *API) expireSubscriptions() {
	w := &s.subRead
	w.mu.Lock()
	defer w.mu.Unlock()
	if time.Since(w.used) >= subscriptionIdle {
		w.release()
	}
}

// Called after each read while subRead.mu is held. A callback already waiting for
// that lock checks used again, so it cannot discard a newly refreshed cache.
func (s *API) retainSubscriptions() {
	w := &s.subRead
	w.used = time.Now()
	if w.idle == nil {
		w.idle = time.AfterFunc(subscriptionIdle, s.expireSubscriptions)
	} else {
		w.idle.Reset(subscriptionIdle)
	}
}

// Environment publication calls this after releasing API.mu (reader lock order).
func (s *API) releaseSubscriptionsForEnvironment() {
	w := &s.subRead
	w.mu.Lock()
	defer w.mu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, err := s.a.Env(); w.a != nil && (err != nil || w.a.EnvName != e.Name) {
		w.release()
	}
}

// detachedReader snapshots invocation options without building the board. It also works
// before the first environment exists (settings, detection and uninstall previews).
// The caller owns the connection and closes it after constructing the view model.
func (s *API) detachedReader() (*app.App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.a.Uninstalled {
		return nil, errors.New("RimArcho 已卸载，账户操作已停用")
	}
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

// subscriptionSession is called with subRead.mu held. The reader owns its connection
// and lazy declarations; only invocation options and a detached index cross API.mu.
func (s *API) subscriptionSession() (*app.App, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.a.Uninstalled {
		return nil, "", errors.New("RimArcho 已卸载，账户操作已停用")
	}
	w := &s.subRead
	if e, err := s.a.Env(); w.a != nil && (err != nil || w.a.EnvName != e.Name) {
		w.release()
	}
	var err error
	if w.a == nil {
		w.a, err = s.readInventorySession()
	} else {
		s.refreshInventorySession(w.a)
	}
	if err != nil {
		return nil, "", err
	}
	w.a.AnalyzeExe, w.a.AnalyzeWait = s.Exe, 200*time.Millisecond
	return w.a, fmt.Sprintf("%s|%d", s.inventoryKey(), s.subRevision.Load()), nil
}

// pageReader is called with s.mu held, only to snapshot the invocation settings.
// All slow work then uses this separate connection without the shared API mutex.
func (s *API) pageReader() (*app.App, error) {
	r, err := s.readInventorySession()
	if err != nil {
		return nil, err
	}
	r.AnalyzeExe, r.AnalyzeWait = s.Exe, 200*time.Millisecond
	return r, nil
}

// inventorySession retains a page's connection and detached index. Caller holds
// both the page reader mutex and API.mu; view construction runs after API.mu is freed.
func (s *API) inventorySession(w *logReader) (*app.App, error) {
	if w.a == nil {
		var err error
		w.a, err = s.pageReader()
		if err != nil {
			return nil, err
		}
	} else {
		s.refreshInventorySession(w.a)
	}
	return w.a, nil
}

// Coalesce a scan/removal's index reload across pages. The shared index is never
// published: every page owns a detached copy and its lazy declarations. Neither
// this gate nor the full index query is taken by editor writes or API.mu.
func (s *API) refreshPageIndex(a *app.App) error {
	w := &s.indexRead
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.a == nil {
		var err error
		w.a, err = a.ReadSession()
		if err != nil {
			return err
		}
	}
	w.a.ReadOptions(a)
	release, err := w.a.Store.ReadSnapshot()
	if err != nil {
		return err
	}
	defer release()
	if _, err := w.a.Index(); err != nil {
		return err
	}
	a.ReadIndex(w.a)
	return nil
}

func pinPageRead(a *app.App) (func(), error) {
	// First use or an external ModsConfig change may require guarded writes.
	if _, err := a.Loadout(); err != nil {
		return nil, err
	}
	return a.Store.ReadSnapshot()
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
	if s.logRead.a == nil {
		var err error
		s.logRead.a, err = s.pageReader()
		if err != nil {
			return nil, err
		}
	}
	r := s.logRead.a
	r.ReadOptions(s.a)
	if e, err := s.a.Env(); err == nil {
		r.EnvName = e.Name
	}
	r.AnalyzeExe, r.AnalyzeWait = s.Exe, 200*time.Millisecond
	return r, nil
}

// Close releases the page reader's database connection (also used by the dev harness).
func (s *API) Close() {
	if s.branchCancel != nil {
		s.branchCancel()
	}
	autoDone := make(chan struct{})
	go func() { s.autoWG.Wait(); close(autoDone) }()
	select {
	case <-autoDone:
	case <-time.After(3 * time.Second):
	}
	s.mu.Lock()
	s.creditClosed = true
	s.needsClosed = true
	if s.creditCancel != nil {
		s.creditCancel()
	}
	s.mu.Unlock()
	s.needsJobs.Wait()
	creditDone := make(chan struct{})
	go func() { s.creditWG.Wait(); close(creditDone) }()
	select {
	case <-creditDone:
	case <-time.After(3 * time.Second):
		// A buffered runner may ignore cancellation. It owns its detached App until
		// return and cannot mutate GUI state after creditClosed.
	}
	s.accountMu.Lock()
	s.accountClosed = true
	var accountFiles map[string]loverDownload
	if !s.accountActive {
		accountFiles, s.llFiles, s.ll = s.llFiles, nil, nil
	}
	s.accountMu.Unlock()
	for _, p := range accountFiles {
		_ = os.Remove(p.path)
	}
	s.cancelLoadCost(true)
	s.costMeasure.workers.Wait()
	s.mu.Lock()
	s.bgRead.closed = true
	if s.bgRead.cancel != nil {
		s.bgRead.cancel()
	}
	s.mu.Unlock()
	s.bgRead.workers.Wait()
	s.buildRead.workers.Wait()
	s.environment.mu.Lock()
	s.environment.closed = true
	s.environment.seq++
	s.environment.mu.Unlock()
	s.environment.wg.Wait()
	s.tasks.Close() // what still runs or waits is stopped with the window (06 §2.16)
	s.mu.Lock()
	if s.texCancel != nil {
		s.texCancel()
		s.texCancel = nil
	}
	s.mu.Unlock()
	s.libGate.Lock()
	s.libGate.Unlock()
	for _, w := range []*logReader{&s.libRead, &s.plansRead, &s.workspaceReader, &s.indexRead} {
		w.mu.Lock()
		if w.a != nil {
			w.a.Close()
			w.a = nil
		}
		w.mu.Unlock()
	}
	s.detailRead.mu.Lock()
	if s.detailRead.a != nil {
		s.detailRead.a.Close()
		s.detailRead.a = nil
	}
	s.detailRead.mu.Unlock()
	s.subRead.mu.Lock()
	s.subRead.release()
	s.subRead.mu.Unlock()
	s.copyRead.mu.Lock()
	if s.copyRead.a != nil {
		s.copyRead.a.Close()
		s.copyRead.a = nil
	}
	s.copyRead.lib = nil
	s.copyRead.mu.Unlock()
	s.uninstallRead.mu.Lock()
	if s.uninstallRead.a != nil {
		s.uninstallRead.a.Close()
		s.uninstallRead.a = nil
	}
	s.uninstallRead.mu.Unlock()
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
	if s.texRead.a == nil {
		var err error
		s.texRead.a, err = s.pageReader()
		if err != nil {
			s.mu.Unlock()
			return nil, nil, err
		}
	}
	r := s.texRead.a
	r.ReadOptions(s.a)
	if e, err := s.a.Env(); err == nil {
		r.EnvName = e.Name
	}
	s.mu.Unlock()
	targets, err := r.TextureTargets(nil, scope)
	return r, targets, err
}
