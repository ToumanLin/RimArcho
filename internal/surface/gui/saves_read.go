package gui

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/savefile"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The Saves page's full read (16 §2.11 "Loading", 12 §1.7 L1); saves.go says how it fits.

// factsKey names a save's file as it is now: its facts are valid for this key only.
func factsKey(sv *app.Save) string {
	return sv.Path + "|" + strconv.FormatInt(sv.Size, 10) + "|" + strconv.FormatInt(sv.ModTime.UnixNano(), 10)
}

// savePlay fills the right card of d: the play facts when the save is read, else skeletons
// while readSelected reads it, else why there are none. Called with the page's reader
// locked; the facts of other saves are dropped. asked numbers the SaveDetail call.
func (s *API) savePlay(r *app.App, sv *app.Save, d *view.SaveDetail, asked uint64) {
	st := &s.saves
	if sv.Writing {
		d.PlayNote = view.T("the game is writing this file: its play facts are read once it has saved")
		return
	}
	f, err := s.factsOf(r, sv)
	if err != nil {
		d.PlayNote = view.Humanize(nil, err.Error())
		return
	}
	if f == nil {
		st.mu.Lock()
		why := st.failed[factsKey(sv)]
		st.mu.Unlock()
		if why == "" { // read before and failed; unchanged since (12 §1.7)
			if stored, _ := r.SaveReadFailure(sv); stored != "" {
				why = view.Humanize(nil, stored)
			}
		}
		if why != "" {
			d.PlayNote = view.Tf("the save could not be read: %s", why)
			return
		}
		d.Reading = true
		s.readSelected(sv, asked)
		return
	}
	if m := f.Malformed; m != nil && f.TicksGame == 0 { // broke off before the game's time: nothing to show
		d.PlayNote = view.Tf("the file breaks off at line %d, before its play facts", m.Line)
		return
	}
	keep := map[string]*savefile.Facts{factsKey(sv): f}
	st.mu.Lock()
	if st.list != nil { // still on the page
		st.facts = keep
	}
	st.mu.Unlock()
	d.Play = view.SavePlayOf(sv, f)
}

// factsOf is a save's full read from the page's kept facts or the cache; nil when it
// is not read. It never reads the file.
func (s *API) factsOf(r *app.App, sv *app.Save) (*savefile.Facts, error) {
	k := factsKey(sv)
	s.saves.mu.Lock()
	f := s.saves.facts[k]
	s.saves.mu.Unlock()
	if f != nil {
		return f, nil
	}
	cp := *sv // SaveFacts sets the summary: the listing is shared with other calls
	return r.SaveFacts(&cp, false)
}

// readSelected reads the selected save on its own session, off the call path (16 §8.4):
// one at a time, a newer selection replacing one that waits. Newer means asked for later
// (asked numbers the SaveDetail and SaveContent calls as they come, not as they take the
// reader's lock): an older selection never replaces a newer one. The background run may
// read the same save; the second read only rewrites the same cache row.
func (s *API) readSelected(sv *app.Save, asked uint64) {
	st := &s.saves
	k := factsKey(sv)
	st.mu.Lock()
	defer st.mu.Unlock()
	switch {
	case st.closed || st.list == nil || asked < max(st.readingAt, st.nextAt):
		return
	case st.reading == k: // the newest selection is the one being read
		st.next, st.nextAt = nil, asked
		return
	case st.reading != "":
		st.next, st.nextAt = sv, asked
		return
	}
	st.reading, st.readingAt = k, asked
	st.jobs.Add(1)
	go s.readSelectedLoop(sv)
}

func (s *API) readSelectedLoop(sv *app.Save) {
	st := &s.saves
	defer st.jobs.Done()
	r, rerr := s.detachedReader()
	if r != nil {
		defer r.Close()
	}
	for sv != nil {
		why := ""
		var read *savefile.Facts
		cp := *sv
		if rerr != nil {
			why = rerr.Error()
		} else {
			f, err := r.SaveFacts(&cp, true)
			switch {
			case err != nil:
				why = view.Humanize(nil, err.Error())
			case f == nil:
				why = view.T("the file changed while it was read; the game may be writing it")
			default:
				read = f
			}
		}
		st.mu.Lock()
		if why != "" && st.list != nil {
			if st.failed == nil {
				st.failed = map[string]string{}
			}
			st.failed[factsKey(sv)] = why
		}
		// The file changed since it was listed (the game saved over it): the cache now
		// holds it by its new size and time, which the listing does not know until the
		// page is relisted. Keep its facts under the listing's key meanwhile, so the page
		// shows them instead of reading the file again and again.
		if read != nil && st.list != nil && factsKey(&cp) != factsKey(sv) {
			if st.facts == nil {
				st.facts = map[string]*savefile.Facts{}
			}
			st.facts[factsKey(sv)] = read
		}
		ref := sv.Ref
		sv, st.next = st.next, nil
		if st.closed || st.list == nil {
			sv = nil
		}
		st.reading, st.readingAt = "", 0
		if sv != nil {
			st.reading, st.readingAt = factsKey(sv), st.nextAt
		}
		st.mu.Unlock()
		s.savesNews([]string{ref}, "", true)
	}
}

// startSavesIndex starts the background read of the saves in l that are not read yet,
// first the one named by first (16 §2.11, 12 §1.7): one run at a time, task kind "saves".
// A save whose read failed before and is unchanged is not counted (app.Save.NeedsRead),
// and nothing starts when the player left the page since the listing (visit).
func (s *API) startSavesIndex(l *app.SaveList, first string, visit uint64) {
	n := 0
	for _, sv := range l.Saves() {
		if sv.NeedsRead() {
			n++
		}
	}
	if n == 0 {
		return
	}
	if sv, err := l.Find(first); first == "" || err != nil || sv == nil {
		first = ""
		if len(l.Colonies) > 0 {
			first = l.Colonies[0].Latest().Ref
		}
	}
	st := &s.saves
	st.mu.Lock()
	if st.closed || st.stop != nil || st.visit != visit || st.list == nil {
		st.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	st.stop = cancel
	st.jobs.Add(1)
	st.mu.Unlock()
	task := s.beginSelf(view.TaskSaves, view.Tf("%d saves", n), func() error { cancel(); return nil })
	go func() {
		defer st.jobs.Done()
		read, failed, why := 0, 0, ""
		progress := func(lines string) {
			for _, line := range strings.Split(lines, "\n") {
				if _, reason, ok := app.ParseSavesFailLine(line); ok {
					if failed++; why == "" {
						why = view.Humanize(nil, reason)
					}
					continue
				}
				done, total, ref, name, ok := app.ParseSavesIndexLine(line)
				if !ok {
					continue
				}
				read = done
				// "n/m" makes the status line a progress bar (16 §11 A11)
				text := view.Tf("reading the saves %d/%d  %s", done, total, name)
				s.tasks.Update(task, func(t *app.Task) { t.Phase, t.Done, t.Total = text, done, total })
				s.savesNews([]string{ref}, text, false)
			}
		}
		var firsts []string
		if first != "" {
			firsts = []string{first}
		}
		err := s.runSavesIndex(ctx, firsts, progress)
		stopped := ctx.Err() != nil
		cancel()
		st.mu.Lock()
		st.stop = nil
		closed := st.closed
		st.mu.Unlock()
		if closed { // Close marks it stopped with the window (16 §2.16)
			return
		}
		// The child exits with an error when a save could not be read; each such save
		// was named on its own line and its failure is stored, so the run itself did
		// its work: it ends done, saying how many could not be read and why. A child that
		// failed on its own (its "error:" line) still ends failed.
		var child *app.SavesIndexError
		if failed > 0 && errors.As(err, &child) && !child.Fatal {
			err = nil
		}
		ev := &SavesReadEvent{Done: true}
		switch {
		case stopped:
			ev.Text = view.Tf("stopped reading the saves: %d read are kept", read)
			s.tasks.Finish(task, app.TaskStopped, ev.Text)
		case err != nil:
			reason := view.Humanize(nil, err.Error())
			ev.Error = failedText(view.TaskSaves, view.Tf("%d saves", n), reason)
			s.endSelf(task, errors.New(reason), "")
		case failed > 0:
			ev.Text = view.Tf("read %d saves; %d could not be read: %s", read-failed, failed, why)
			s.endSelf(task, nil, ev.Text)
		default:
			ev.Text = view.Tf("read %d saves", read)
			s.endSelf(task, nil, ev.Text)
		}
		s.savesFlush(ev)
	}()
}

// runSavesIndex is the background read: `archo saves index` as a child process (stopped
// with ctx), or with an in-process runner (tests) on its own session, never the window's.
func (s *API) runSavesIndex(ctx context.Context, first []string, progress func(string)) error {
	if run := s.saves.runner; run != nil {
		return run(ctx, first, progress)
	}
	s.mu.Lock()
	inProcess, exe, home := s.a.SavesIndexRunner != nil, s.Exe, s.a.Home
	s.mu.Unlock()
	if !inProcess {
		if exe == "" {
			r := &app.App{Home: home}
			return r.RunSavesIndexContext(ctx, "", first, progress)
		}
		return app.ChildSavesIndexContext(ctx, exe, home, first, progress)
	}
	r, err := s.detachedReader()
	if err != nil {
		return err
	}
	defer r.Close()
	return app.InProcessSavesIndex(r)(first, progress)
}

// savesNews records saves read and the run's progress, and sends them to the page:
// at once when now (the selected save), else at most every savesNewsEvery.
func (s *API) savesNews(refs []string, line string, now bool) {
	st := &s.saves
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.closed {
		return
	}
	st.fresh = append(st.fresh, refs...)
	st.dirty = st.dirty || len(refs) > 0
	if line != "" {
		st.line = line
	}
	wait := savesNewsEvery - time.Since(st.sent)
	if now {
		wait = 0
	}
	switch {
	case st.timer == nil:
	case !now || !st.timer.Stop():
		return // one is due already
	default:
		st.jobs.Done() // the stopped timer's
	}
	st.jobs.Add(1)
	st.timer = time.AfterFunc(max(0, wait), func() {
		defer st.jobs.Done()
		s.savesFlush(nil)
	})
}

// savesFlush sends a "saves-read" event with what is pending, relisting the page when
// saves were read and the last listing is older than savesListEvery (else a later event
// is set for it). end is the run's end: the listing then goes in any case.
func (s *API) savesFlush(end *SavesReadEvent) {
	st := &s.saves
	st.mu.Lock()
	if st.timer != nil && end != nil && st.timer.Stop() {
		st.jobs.Done()
	}
	st.timer = nil
	ev := end
	if ev == nil {
		ev = &SavesReadEvent{}
	}
	ev.Refs, ev.Line, st.fresh, st.line = st.fresh, st.line, nil, ""
	if end != nil {
		ev.Line = ""
	}
	relist := st.dirty && (end != nil || time.Since(st.listed) >= savesListEvery)
	onPage, closed := st.list != nil, st.closed
	if relist {
		st.dirty = false
	}
	st.sent = time.Now()
	st.flushes++
	ticket := st.flushes
	st.mu.Unlock()
	if closed {
		return
	}
	st.sendMu.Lock()
	if relist && onPage {
		ev.Page = s.relistSaves()
	}
	st.mu.Lock()
	if ticket < st.lastSent { // a newer event went first: this line is old news
		ev.Line = ""
	} else {
		st.lastSent = ticket
	}
	st.mu.Unlock()
	if ev.Refs != nil || ev.Line != "" || ev.Page != nil || ev.Done {
		s.send("saves-read", ev)
	}
	st.sendMu.Unlock()
	st.mu.Lock()
	later := st.dirty && st.timer == nil && !st.closed && st.list != nil
	if later {
		st.jobs.Add(1)
		st.timer = time.AfterFunc(max(0, savesListEvery-time.Since(st.listed)), func() {
			defer st.jobs.Done()
			s.savesFlush(nil)
		})
	}
	st.mu.Unlock()
}

// relistSaves lists the saves again on the page's reader (heads cached: a stat per save)
// so the rows get their day and colonies regroup; nil when the page was left.
func (s *API) relistSaves() *view.SavesPage {
	w := &s.saves.read
	w.mu.Lock()
	defer w.mu.Unlock()
	defer s.retainReader(w)
	s.saves.mu.Lock()
	onPage := s.saves.list != nil
	s.saves.mu.Unlock()
	if !onPage {
		return nil
	}
	r, err := s.savesSession()
	if err != nil {
		return nil
	}
	l, err := r.ListSaves()
	if err != nil {
		return nil
	}
	s.saves.mu.Lock()
	defer s.saves.mu.Unlock()
	if s.saves.list == nil {
		return nil
	}
	s.saves.list, s.saves.listed = l, time.Now()
	return view.SavesRows(l)
}

// closeSaves stops the full read with the window (16 §2.16): the child process is ended
// (each save it read is already stored), a selected save being read finishes, and nothing
// more is sent. Close calls it before it marks the tasks stopped and releases the readers.
func (s *API) closeSaves() {
	st := &s.saves
	st.mu.Lock()
	st.closed = true
	stop := st.stop
	if st.timer != nil && st.timer.Stop() {
		st.jobs.Done()
	}
	st.timer, st.next = nil, nil
	st.mu.Unlock()
	if stop != nil {
		stop()
	}
	st.jobs.Wait()
}
