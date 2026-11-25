//go:build windows || cgo

package gui

import "time"

// Expiring page readers share Mod index contents and keep their own SQLite connection,
// list state and problems (16 §2.16.1 M07, 17 §7.3). After readerIdle, they close and
// drop what only they built. The editor/sidebar and log readers stay open for
// published objects and cached reports. Updates has its own expiry.

// readerIdle is how long an unused page reader keeps its connection and copies.
const readerIdle = 5 * time.Minute

// idleUse is a reader's last use and expiry timer, guarded by that reader's mutex.
type idleUse struct {
	timer *time.Timer
	last  time.Time
}

// touch records a use and arms the expiry again.
func (u *idleUse) touch(expire func()) {
	u.last = time.Now()
	if u.timer == nil {
		u.timer = time.AfterFunc(readerIdle, expire)
	} else {
		u.timer.Reset(readerIdle)
	}
}

// due reports whether an expiry callback may release the reader: still armed, and unused
// for readerIdle. A callback that waited for the mutex behind a newer read finds it recent.
func (u *idleUse) due() bool { return u.timer != nil && time.Since(u.last) >= readerIdle }

func (u *idleUse) stop() {
	if u.timer != nil {
		u.timer.Stop()
		u.timer = nil
	}
}

// release closes the reader's connection and drops its copies; call with w.mu held.
func (w *logReader) release() {
	w.stop()
	if w.a != nil {
		w.a.Close()
		w.a = nil
	}
	w.board, w.boardAt = nil, ""
}

// retainReader marks an expiring page reader used; call with its mutex held, after the
// read (a deferred call runs before the deferred unlock).
func (s *API) retainReader(w *logReader) {
	w.touch(func() { s.expireReader(w) })
}

func (s *API) expireReader(w *logReader) {
	lib := w == &s.libRead
	if lib { // the library page's lock order: libGate, libRead.mu, mu
		s.libGate.Lock()
		defer s.libGate.Unlock()
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.due() {
		return
	}
	w.release()
	if lib {
		s.mu.Lock()
		s.lib, s.libAt = nil, "" // built on the closed reader
		s.mu.Unlock()
	}
	s.free.soon()
}

// release closes a copy reader and drops its library; call with w.mu held.
func (w *inventoryReader) release() {
	w.stop()
	if w.a != nil {
		w.a.Close()
		w.a = nil
	}
	w.lib, w.key, w.ref = nil, "", ""
}

// retainCopy marks the library's copy-detail reader used; call with copyRead.mu held.
func (s *API) retainCopy() { s.copyRead.touch(s.expireCopy) }

func (s *API) expireCopy() {
	w := &s.copyRead
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.due() {
		return
	}
	w.release()
	s.free.soon()
}
