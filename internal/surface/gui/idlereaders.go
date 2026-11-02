package gui

import "time"

// Page readers close when unused (06 §2.16.1 M07, 07 §8.3). Each keeps its own SQLite
// connection and a detached copy of the Mod index, list state and problems: about 5-12 MB
// per reader for a library of 6,574 copies. A reader unused for readerIdle closes, drops
// what only it built, and the window returns the memory (M01); the next visit reads again
// on a new connection.
//
// Mods read their declarations lazily through the connection of the reader that copied
// them, so a reader may close only when nothing it built stays published. That rules out
// the editor and sidebar reader (the window's board is its board) and the log reader
// (Can it run's saved problem sets are its objects; the launch page also reads on it
// every 30 seconds). The Updates reader has its own expiry (subscriptionReader).

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
