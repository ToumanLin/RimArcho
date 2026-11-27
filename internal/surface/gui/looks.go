//go:build windows || cgo

package gui

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/looks"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The plan editor's looks view (16 §2.5, 21 §6). Its index is the expensive part (seconds
// on a large plan the first time): it is built once per app.LooksKey, outside the page lock,
// and kept in memory; the choices are resolved against it again whenever the database
// changed. The page polls LooksProgress while a read waits for the build. Texture pictures
// come from /looktex/, which finds a file only through this index. The index is dropped
// when the page leaves the view (LooksLeave) and when nothing read it for readerIdle
// (16 §10: no page's data stays in memory for good).

// looksCache is the plan's index in memory, guarded by its own mutex: the page lock is not
// held while the index is built, and the picture route never takes it.
type looksCache struct {
	mu   sync.Mutex
	key  string       // app.LooksKey the index was built for
	idx  *looks.Index // nil: none yet, or dropped
	ls   *app.LooksState
	lsAt string // the database state and plan ls was resolved at
	// the build on its way
	building    bool
	done, total int
	use         idleUse // the last read and the expiry timer
	freed       func()  // returns the dropped index's memory to the OS (nil: tests of the cache alone)
}

// touch records a read and arms the expiry; call with c.mu held.
func (c *looksCache) touch() { c.use.touch(c.expire) }

// expire drops an index nothing read for readerIdle.
func (c *looksCache) expire() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.use.due() || c.building {
		return
	}
	c.forget()
}

// forget drops the index and stops the expiry; call with c.mu held.
func (c *looksCache) forget() {
	c.use.stop()
	had := c.idx != nil
	c.key, c.idx, c.ls, c.lsAt = "", nil, nil, ""
	if had && c.freed != nil {
		c.freed()
	}
}

// LooksProgress is the index build as it goes.
type LooksProgress struct {
	Building bool `json:"building"`
	Done     int  `json:"done"` // Mods read
	Total    int  `json:"total"`
}

// LooksProgress reports the build a looks read is waiting for (the page polls it).
func (s *API) LooksProgress() LooksProgress {
	c := &s.looks
	c.mu.Lock()
	defer c.mu.Unlock()
	return LooksProgress{Building: c.building, Done: c.done, Total: c.total}
}

// get returns the index kept for key.
func (c *looksCache) get(key string) *looks.Index {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.idx != nil && c.key == key {
		c.touch()
		return c.idx
	}
	return nil
}

func (c *looksCache) put(key string, idx *looks.Index) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.key, c.idx, c.ls, c.lsAt = key, idx, nil, ""
	c.touch()
}

// current is the index the picture route reads, with its key.
func (c *looksCache) current() (*looks.Index, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.idx != nil {
		c.touch()
	}
	return c.idx, c.key
}

// drop forgets the index: the player asked for a fresh read, the page left the view, or
// the window closes.
func (c *looksCache) drop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.forget()
}

// looksBuild is the app's LooksCache: an apply, undo or rebuild reuses the index the page
// already has, and keeps the one it had to build.
func (s *API) looksBuild(key string, build func() (*looks.Index, error)) (*looks.Index, error) {
	if idx := s.looks.get(key); idx != nil {
		return idx, nil
	}
	idx, err := build()
	if err == nil {
		s.looks.put(key, idx)
	}
	return idx, err
}

// withLooks runs fn with the plan's looks under the page lock. A missing index is built
// first, outside the lock and once for all waiting reads (single-flight on its key).
func withLooks[T any](s *API, fn func(ls *app.LooksState) (T, error)) (T, error) {
	var zero T
	for try := 0; ; try++ {
		s.mu.Lock()
		st, err := s.a.LoadoutSnapshot()
		if err != nil {
			s.mu.Unlock()
			return zero, errors.New(view.Humanize(nil, err.Error()))
		}
		key := s.a.LooksKey(st)
		if idx := s.looks.get(key); idx != nil {
			ls, err := s.looksStateLocked(st, key, idx)
			var out T
			if err == nil {
				out, err = fn(ls)
			}
			s.mu.Unlock()
			if err != nil {
				return zero, errors.New(view.Humanize(st, err.Error()))
			}
			return out, nil
		}
		job, err := s.a.LooksIndexJob(st)
		s.mu.Unlock()
		if err != nil {
			return zero, errors.New(view.Humanize(st, err.Error()))
		}
		if try == 2 {
			return zero, errors.New(view.T("the list changed while its looks were read; try again"))
		}
		if _, err := join(&s.flight, "LooksIndex|"+key, func() (*looks.Index, error) { return s.looksIndex(key, job) }); err != nil {
			return zero, errors.New(view.Humanize(st, err.Error()))
		}
	}
}

// looksIndex builds the index for key and keeps it; the page lock is not held.
func (s *API) looksIndex(key string, job func(context.Context, func(done, total int)) (*looks.Index, error)) (*looks.Index, error) {
	defer s.free.soon()
	c := &s.looks
	c.mu.Lock()
	c.building, c.done, c.total = true, 0, 0
	c.mu.Unlock()
	idx, err := job(s.branchCtx, func(done, total int) {
		c.mu.Lock()
		c.done, c.total = done, total
		c.mu.Unlock()
	})
	c.mu.Lock()
	c.building = false
	if err == nil {
		c.key, c.idx, c.ls, c.lsAt = key, idx, nil, ""
		c.touch()
	}
	c.mu.Unlock()
	return idx, err
}

// looksStateLocked resolves the choices against idx, or returns the state resolved at the
// same database state. Call with s.mu held.
func (s *API) looksStateLocked(st *app.LoadoutState, key string, idx *looks.Index) (*app.LooksState, error) {
	at := key + "|" + strconv.FormatInt(st.Plan.ID, 10) + "|" + s.a.Store.ChangeKey()
	c := &s.looks
	c.mu.Lock()
	ls := c.ls
	if c.lsAt != at || c.idx != idx {
		ls = nil
	}
	c.mu.Unlock()
	if ls != nil {
		return ls, nil
	}
	ls, err := s.a.LooksStateOf(st, idx)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.idx == idx {
		c.ls, c.lsAt = ls, at
	}
	c.mu.Unlock()
	return ls, nil
}

// LooksOverview is the looks view's Mods, totals and RimArcho Looks Preference card.
func (s *API) LooksOverview() (*view.LooksOverview, error) {
	return withLooks(s, func(ls *app.LooksState) (*view.LooksOverview, error) { return view.BuildLooksOverview(s.a, ls) })
}

// LooksRefresh reads the Mod folders again (the gallery's refresh): the index kept in
// memory is dropped; unchanged files still come from the header cache.
func (s *API) LooksRefresh() (*view.LooksOverview, error) {
	s.looks.drop()
	return s.LooksOverview()
}

// LooksLeave drops the index when the page leaves the looks view (16 §10); the next read
// builds it again from the header cache.
func (s *API) LooksLeave() {
	s.looks.drop()
}

// LooksGallery lists the looks of one Mod (mod: its position, as "12"), of the choices
// (mod "chosen") or of the whole plan (mod ""), narrowed by a filter and a search.
func (s *API) LooksGallery(mod, filter, search string) (*view.LooksGallery, error) {
	return withLooks(s, func(ls *app.LooksState) (*view.LooksGallery, error) {
		return view.BuildLooksGallery(s.a, ls, mod, filter, search)
	})
}

// LookDetail is one look for the looks editor; mod is the selected Mod's position (-1 none).
func (s *API) LookDetail(lookID string, mod int, scope string) (*view.LookDetail, error) {
	return withLooks(s, func(ls *app.LooksState) (*view.LookDetail, error) {
		return view.BuildLookDetail(s.a, ls, lookID, mod, scope)
	})
}

// LookPreview predicts an apply without writing.
func (s *API) LookPreview(lookID string, mod int, slots []string, scope string) (*view.LookPreview, error) {
	return withLooks(s, func(ls *app.LooksState) (*view.LookPreview, error) {
		return view.BuildLookPreview(ls, lookID, mod, slots, scope)
	})
}

// looksWrite runs an apply, undo or rebuild under the page lock. The plan may have gained
// its RimArcho Looks Preference entry, so the board is dropped.
func (s *API) looksWrite(dryRun bool, fn func() (*view.LooksDone, error)) (*view.LooksDone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := fn()
	if !dryRun {
		s.board = nil
		s.looks.mu.Lock()
		s.looks.ls, s.looks.lsAt = nil, ""
		s.looks.mu.Unlock()
	}
	if err != nil {
		return nil, errors.New(view.Humanize(s.boardState(), err.Error()))
	}
	if d.Added {
		s.listEdits++
		s.needsAsked = false
	}
	return d, nil
}

// LooksApply chooses that a look, or the given slots of it, comes from the Mod modRef names.
func (s *API) LooksApply(lookID, modRef string, slots []string, scope string) (*view.LooksDone, error) {
	return s.looksWrite(false, func() (*view.LooksDone, error) {
		return view.LooksApply(s.branchCtx, s.a, []string{lookID}, modRef, slots, scope, false)
	})
}

// LooksApplyMany chooses that every look of lookIDs comes, whole, from the Mod modRef names
// (the gallery's selection): one undoable plan edit and one rebuild.
func (s *API) LooksApplyMany(lookIDs []string, modRef, scope string) (*view.LooksDone, error) {
	return s.looksWrite(false, func() (*view.LooksDone, error) {
		return view.LooksApply(s.branchCtx, s.a, lookIDs, modRef, nil, scope, false)
	})
}

// LooksApplyPreview is what an apply would write, for the first apply's confirmation: the
// folder, the files as links or copies, whether the plan gains the entry. It writes nothing.
func (s *API) LooksApplyPreview(lookIDs []string, modRef string, slots []string, scope string) (*view.LooksDone, error) {
	return s.looksWrite(true, func() (*view.LooksDone, error) {
		if len(lookIDs) != 1 {
			slots = nil
		}
		return view.LooksApply(s.branchCtx, s.a, lookIDs, modRef, slots, scope, true)
	})
}

// LooksUndo removes one look's choices in one scope.
func (s *API) LooksUndo(lookID, scope string) (*view.LooksDone, error) {
	return s.looksWrite(false, func() (*view.LooksDone, error) {
		return view.LooksUndo(s.branchCtx, s.a, []string{lookID}, scope, false)
	})
}

// LooksUndoMany removes the choices of several looks in one scope, with one rebuild.
func (s *API) LooksUndoMany(lookIDs []string, scope string) (*view.LooksDone, error) {
	return s.looksWrite(false, func() (*view.LooksDone, error) {
		return view.LooksUndo(s.branchCtx, s.a, lookIDs, scope, false)
	})
}

// LooksRebuild rebuilds RimArcho Looks Preference for the current plan.
func (s *API) LooksRebuild() (*view.LooksDone, error) {
	return s.looksWrite(false, func() (*view.LooksDone, error) { return view.LooksRebuild(s.branchCtx, s.a, false) })
}

// styleWrite runs a style add or remove under the page lock, like looksWrite: the plan may
// have gained its RimArcho Styles Preference entry, so the board is dropped.
func (s *API) styleWrite(dryRun bool, fn func() (*view.LookStyleDone, error)) (*view.LookStyleDone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := fn()
	if !dryRun {
		s.board = nil
		s.looks.mu.Lock()
		s.looks.ls, s.looks.lsAt = nil, ""
		s.looks.mu.Unlock()
	}
	if err != nil {
		return nil, errors.New(view.Humanize(s.boardState(), err.Error()))
	}
	if d.Added && !dryRun {
		s.listEdits++
		s.needsAsked = false
	}
	return d, nil
}

// LookStyleAdd keeps the Mod modRef's version of a look for all plans (21 §5): kind
// "style" adds it as a new style in the style category target (random: new items may also
// receive it at random); kind "variant" adds it as a random variant of target, "default"
// or a style Def. The choices LookDetail carries (add.styles, add.variants) name what can
// be added.
func (s *API) LookStyleAdd(lookID, modRef, kind, target string, random bool) (*view.LookStyleDone, error) {
	return s.styleWrite(false, func() (*view.LookStyleDone, error) {
		return view.LookStyleAdd(s.branchCtx, s.a, lookID, modRef, kind, target, random, false)
	})
}

// LookStyleAddPreview is what LookStyleAdd would write, for the first add's confirmation
// (ui.gui.styles.confirmed): the folder, the textures as links or copies, the Def files,
// and whether the plan gains RimArcho Styles Preference. It writes nothing.
func (s *API) LookStyleAddPreview(lookID, modRef, kind, target string, random bool) (*view.LookStyleDone, error) {
	return s.styleWrite(true, func() (*view.LookStyleDone, error) {
		return view.LookStyleAdd(s.branchCtx, s.a, lookID, modRef, kind, target, random, true)
	})
}

// LookStyleRemove deletes one style or random variant entry (its id in LookDetail's
// add.entries); the generated Mod is rebuilt without it.
func (s *API) LookStyleRemove(id int) (*view.LookStyleDone, error) {
	return s.styleWrite(false, func() (*view.LookStyleDone, error) {
		return view.LookStyleRemove(s.branchCtx, s.a, int64(id), false)
	})
}

// lookFile finds a texture file through the index in memory: look, provider position and
// slot are keys of its maps, and nothing of them is ever part of a path.
func (s *API) lookFile(lookID string, pos int, slot string) (*looks.File, string) {
	idx, key := s.looks.current()
	if idx == nil {
		return nil, ""
	}
	l := idx.ByID[lookID]
	if l == nil {
		return nil, key
	}
	for _, p := range l.Providers {
		if p.Pos == pos {
			return p.Slots[slot], key
		}
	}
	return nil, key
}

// LookShow opens the folder of a look's texture file in the file manager.
func (s *API) LookShow(lookID string, mod int, slot string) error {
	f, _ := s.lookFile(lookID, mod, slot)
	if f == nil {
		return errors.New(view.T("the texture is no longer there; refresh the looks"))
	}
	if s.openDir != nil {
		return s.openDir(filepath.Dir(f.Path))
	}
	return openDir(filepath.Dir(f.Path))
}

// serveLookTex is /looktex/<look id>?p=<provider position>&s=<slot>&w=<0|96|192|512>&v=<stamp>:
// a look's texture as the Mod ships it (w 0; a DDS as PNG) or as a PNG thumbnail that fits
// a w×w box (16 §4.4). The file is found only through the index in memory. With v, the
// stamp of the looks views, the answer may be cached for good: a changed plan or Mod has
// another stamp. A format without a decoder (BC7, PSD) is 404; the view models say so.
func (s *API) serveLookTex(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pos, err := strconv.Atoi(q.Get("p"))
	width := 0
	if v := q.Get("w"); err == nil && v != "" {
		width, err = strconv.Atoi(v)
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, key := s.lookFile(strings.TrimPrefix(r.URL.Path, "/looktex/"), pos, q.Get("s"))
	v := q.Get("v")
	if f == nil || (v != "" && v != key) {
		http.NotFound(w, r)
		return
	}
	path, mime, err := s.a.LookTexture(f, width)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	fi, err := file.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if v != "" {
		w.Header().Set("Cache-Control", "max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, "", fi.ModTime(), file)
}
