package gui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/savefile"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Saves (16 §2.11, 12 §4). The page reads on its own session (savesState.read), whose App
// keeps the save heads cached by path, size and time (12 §1.7 L0), so coming back costs a
// stat per save. Listing a cold library reads every head, which can take longer than a
// call may (16 §8.4): SavesRead returns at once and the listing arrives as a "saves" event.
// Selecting a save checks it against its list (SaveDetail, milliseconds). Actions run on
// the window's session: fixes edit the save's list (one undo step), file actions change
// the save's folder (rename, copy) or copy and recycle after the player confirmed.
//
// The full read (12 §1.7 L1): after a listing that has saves not read yet, the background
// task "saves" runs `archo saves index` in a child process, the selected save first, and
// the page hears of what it read through "saves-read" events (relisted at most once a
// second, so rows get their day and colonies regroup by world). The selected save never
// waits for that run: SaveDetail of a save not read yet returns at once with skeletons
// and reads that one save on its own session in a goroutine (readSelected), then sends
// "saves-read" with its ref. The page holds the facts of the selected save and of its
// colony's previous read save only (savesState.facts); listings carry summaries.

// savesState is the page's reader and its last listing. Lock order: read.mu, then mu.
type savesState struct {
	read logReader
	mu   sync.Mutex
	list *app.SaveList // the last listing: the saves that checks and actions name by ref
	seq  uint64        // the newest listing asked for
	// visit counts the times the page was left: a listing asked for before the player left
	// neither stores itself nor starts the full read when it finishes after.
	visit uint64
	asked atomic.Uint64 // SaveDetail calls, numbered as they come (the selection's order)

	// The full read.
	stop      context.CancelFunc         // the background run's stop; nil when none runs
	closed    bool                       // the window is closing: nothing new starts or is sent
	jobs      sync.WaitGroup             // the run, the selected save's read and a pending event
	facts     map[string]*savefile.Facts // by factsKey: the selected save's and its previous one's
	failed    map[string]string          // by factsKey: why the selected save could not be read
	reading   string                     // factsKey of the save readSelected reads now ("" none)
	readingAt uint64                     // the SaveDetail call that asked for it (asked)
	next      *app.Save                  // the newest selection waiting for readSelected
	nextAt    uint64                     // the SaveDetail call that asked for next
	fresh     []string                   // refs read since the last "saves-read" event
	line      string                     // the run's newest progress, for the status line
	dirty     bool                       // saves were read since the page was last relisted
	timer     *time.Timer                // the next "saves-read" event, when one is due
	sent      time.Time                  // when the last event went
	listed    time.Time                  // when the page was last listed
	// Events are numbered when their news is taken (flushes) and sent in that order: an
	// event taken before one already sent drops its progress line, so a late flush never
	// puts "reading the saves n/m" back after the run's end (sendMu: one send at a time).
	flushes, lastSent uint64
	sendMu            sync.Mutex
	// runner replaces the background run's child process (tests); set before use.
	runner func(ctx context.Context, first []string, progress func(string)) error
}

// Throttles of the "saves-read" event (16 §2.16.1 G20): news at most every half second
// (the selected save's own read at once), the listing at most once a second.
const (
	savesNewsEvery = 500 * time.Millisecond
	savesListEvery = time.Second
)

// SavesReadEvent is the full read's news for the page: the saves read since the last
// event (Refs), the run's progress for the status line (Line), the listing again with
// their days and colonies (Page; nil when not relisted this time), and the run's end.
type SavesReadEvent struct {
	Refs  []string        `json:"refs,omitempty"`
	Line  string          `json:"line,omitempty"`
	Page  *view.SavesPage `json:"page,omitempty"`
	Done  bool            `json:"done,omitempty"`
	Text  string          `json:"text,omitempty"`
	Error string          `json:"error,omitempty"`
}

// SavesEvent is a listing as the "saves" event carries it.
type SavesEvent struct {
	Request uint64          `json:"request"`
	Page    *view.SavesPage `json:"page,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// savesSession is the page's reader, made on first use; call with read.mu held.
func (s *API) savesSession() (*app.App, error) {
	w := &s.saves.read
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.a.Uninstalled {
		return nil, errors.New("RimArcho was uninstalled; account operations are disabled")
	}
	if s.needsClosed { // the window is closing: Close has released the readers
		return nil, errors.New(view.T("the window is closing"))
	}
	if w.a == nil {
		r, err := s.pageReader()
		if err != nil {
			return nil, err
		}
		w.a = r
	} else {
		s.refreshInventorySession(w.a)
	}
	w.a.AnalyzeExe, w.a.AnalyzeWait = s.Exe, 200*time.Millisecond
	return w.a, nil
}

// SavesRead starts reading every save's head; the result is a "saves" event with this
// request number (an older one is dropped by the page). It returns at once. When saves
// are not read in full yet, the background read starts with first (the page's selected
// save; "" or gone: the save the page selects, the first colony's newest).
func (s *API) SavesRead(request uint64, first string) bool {
	s.saves.mu.Lock()
	if request > s.saves.seq {
		s.saves.seq = request
	}
	visit := s.saves.visit
	s.saves.mu.Unlock()
	go func() {
		ev, l := s.listSaves(request, visit)
		s.send("saves", ev)
		if l != nil {
			s.startSavesIndex(l, first, visit)
		}
	}()
	return true
}

// listSaves reads the listing on the page's reader. A request older than the newest one
// asked for is not read at all, and is not kept when a newer one came or the player left
// the page since it was asked for (visit: the page's visit then, savesState.visit).
func (s *API) listSaves(request, visit uint64) (*SavesEvent, *app.SaveList) {
	ev := &SavesEvent{Request: request}
	w := &s.saves.read
	w.mu.Lock()
	defer w.mu.Unlock()
	defer s.retainReader(w)
	s.saves.mu.Lock()
	stale := request < s.saves.seq || visit != s.saves.visit
	s.saves.mu.Unlock()
	if stale {
		ev.Error = "stale"
		return ev, nil
	}
	r, err := s.savesSession()
	if err != nil {
		ev.Error = err.Error()
		return ev, nil
	}
	l, err := r.ListSaves()
	if err != nil {
		ev.Error = err.Error()
		return ev, nil
	}
	s.saves.mu.Lock()
	stale = request < s.saves.seq || visit != s.saves.visit
	if !stale {
		s.saves.list = l
		s.saves.listed = time.Now()
	}
	s.saves.mu.Unlock()
	if stale {
		ev.Error = "stale"
		return ev, nil
	}
	ev.Page = view.SavesRows(l)
	return ev, l
}

// findSave is a save of the last listing.
func (s *API) findSave(ref string) (*app.Save, error) {
	s.saves.mu.Lock()
	l := s.saves.list
	s.saves.mu.Unlock()
	if l == nil {
		return nil, errors.New(view.T("the saves are still being read"))
	}
	sv, err := l.Find(ref)
	if errors.Is(err, app.ErrNoSave) {
		return nil, errors.New(view.T("that save is gone; the list is read again"))
	}
	return sv, err
}

// SaveDetail is the selected save against its list (12 §4.3): the middle and right cards.
func (s *API) SaveDetail(ref string) (*view.SaveDetail, error) {
	asked := s.saves.asked.Add(1) // before the reader's lock: calls may take it out of order
	sv, err := s.findSave(ref)
	if err != nil {
		return nil, err
	}
	w := &s.saves.read
	w.mu.Lock()
	defer w.mu.Unlock()
	defer s.retainReader(w)
	r, err := s.savesSession()
	if err != nil {
		return nil, err
	}
	target, err := appliedTarget(r, sv)
	if err != nil {
		return nil, err
	}
	c, err := r.CheckSave(target)
	if err != nil {
		return nil, err
	}
	// The save's own game and list or pack, when the applied ones are others: the bar's
	// Switch and launch line says how it loads with them.
	var own *app.SaveCheck
	if target != sv && (sv.Plan != nil || sv.Pack != "") {
		if own, err = r.CheckSave(sv); err != nil {
			return nil, err
		}
	}
	g, _ := r.RunningGame()
	current := target.Plan != nil && target.Env != nil && r.Store.CurrentPlanID(target.Env.ID) == target.Plan.ID
	d := view.SaveDetailAgainst(c, g != nil, current, sv, own)
	s.savePlay(r, sv, d, asked)
	return d, nil
}

// appliedTarget is what the Saves page checks a save against (16 §2.11): what the Launch
// page applied, the current game's current list or its applied pack. The save itself when
// that is its own game and list or pack, else a copy (app.SaveAgainstCurrent).
func appliedTarget(r *app.App, sv *app.Save) (*app.Save, error) {
	e, err := r.Env()
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, app.ErrNoEnv
	}
	pack := view.AppliedPack(r.Store.Meta(view.FeaturedKey), e.Name, r.Store.CurrentPlanID(e.ID))
	if pack != "" {
		if _, err := r.ReadPack(pack); err != nil {
			pack = "" // removed since: the current list is what launches
		}
	}
	cur, err := r.SaveAgainstCurrent(sv, pack)
	if err != nil {
		return nil, err
	}
	if app.SameTarget(sv, cur) {
		return sv, nil
	}
	return cur, nil
}

// SaveFixReply is what a fix did: the status sentence, whether u undoes it, and the
// Workshop items the page subscribes to next (through Subscribe, as missing Mods are).
type SaveFixReply struct {
	Text        string   `json:"text"`
	Undo        bool     `json:"undo"`
	WorkshopIDs []string `json:"workshopIds"`
	Plan        int64    `json:"plan"`
	Page        string   `json:"page"`
}

// SaveFix applies a save's Mod fixes to its list as one undoable edit (12 §4.4): mods
// names the Mods to enable or subscribe (none: all of them), disableAdded and order the
// other two fixes. The list opens in the editor first (not current, 07 §1.3), so the
// status line's undo undoes this edit.
func (s *API) SaveFix(ref string, mods []string, disableAdded, order bool) (*SaveFixReply, error) {
	sv, err := s.findSave(ref)
	if err != nil {
		return nil, err
	}
	// the list the page checked the save against: the applied one (SaveDetail), found on
	// the window's session under its lock, so it is still the applied one when fixed
	s.mu.Lock()
	defer s.mu.Unlock()
	if sv, err = appliedTarget(s.a, sv); err != nil {
		return nil, err
	}
	if sv.Plan == nil {
		if sv.Pack != "" {
			return nil, errors.New(view.Tf("%s is a save of pack %s, whose Mods do not change; make a list from this save instead", sv.Name, sv.Pack))
		}
		return nil, errors.New(view.T("this save matches no list; make a list from it instead"))
	}
	s.board, s.needs = nil, nil
	if _, err := view.OpenPlan(s.a, sv.Plan.ID); err != nil {
		return nil, err
	}
	r, err := s.a.FixSave(sv, app.SaveFixOptions{Mods: mods, DisableAdded: disableAdded, Order: order}, false)
	if err != nil {
		return nil, errors.New(view.Humanize(nil, err.Error()))
	}
	return &SaveFixReply{Text: view.SaveFixText(r), Undo: r.Edit != nil, WorkshopIDs: append([]string{}, r.WorkshopIDs()...),
		Plan: sv.Plan.ID, Page: view.PlanPage(sv.Plan.ID)}, nil
}

// SaveFileReply is a rename or copy: the status sentence and the save to select.
type SaveFileReply struct {
	Text string `json:"text"`
	Ref  string `json:"ref"`
}

// SaveNameProblem is what is wrong with a new name while the player types ("" none).
func (s *API) SaveNameProblem(ref, name string) string {
	sv, err := s.findSave(ref)
	if err != nil {
		return err.Error()
	}
	return view.SaveNameProblem(sv, name)
}

// SaveRename renames a save in its folder, keeping a RimSaves prefix (12 §4.4); refused
// while a game runs.
func (s *API) SaveRename(ref, name string) (*SaveFileReply, error) {
	sv, err := s.findSave(ref)
	if err != nil {
		return nil, err
	}
	if p := view.SaveNameProblem(sv, name); p != "" {
		return nil, errors.New(p)
	}
	s.mu.Lock()
	to, err := s.a.RenameSave(sv, name)
	s.mu.Unlock()
	if err != nil {
		return nil, saveErr(err)
	}
	_, shown := app.SplitSaveName(filepath.Base(to))
	return &SaveFileReply{Text: view.Tf("renamed %s to %s", sv.Name, shown), Ref: app.SaveRef(to)}, nil
}

// ColonyRename names a colony in RimArcho (12 §4.1); "" gives it back its saves' name.
// The page lists the saves again to show it.
func (s *API) ColonyRename(key, name string) (string, error) {
	s.saves.mu.Lock()
	l := s.saves.list
	s.saves.mu.Unlock()
	if l == nil {
		return "", errors.New(view.T("the saves are still being read"))
	}
	var c *app.Colony
	for _, x := range l.Colonies {
		if x.Key == key {
			c = x
		}
	}
	if c == nil {
		return "", errors.New(view.T("that colony is gone; the list is read again"))
	}
	game := c.GameName
	if game == "" {
		game = c.Name
	}
	s.mu.Lock()
	err := s.a.RenameColony(c, name)
	s.mu.Unlock()
	if err != nil {
		return "", errors.New(view.Humanize(nil, err.Error()))
	}
	if name = strings.TrimSpace(name); name == "" || name == game {
		return view.Tf("%s has its saves' name again", game), nil
	}
	return view.Tf("renamed colony %s to %s", c.Name, name), nil
}

// SaveCopy copies a save next to itself as "<name> (copy)" (12 §4.4).
func (s *API) SaveCopy(ref string) (*SaveFileReply, error) {
	sv, err := s.findSave(ref)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	to, err := s.a.CopySave(sv)
	s.mu.Unlock()
	if err != nil {
		return nil, saveErr(err)
	}
	_, shown := app.SplitSaveName(filepath.Base(to))
	return &SaveFileReply{Text: view.Tf("copied %s to %s", sv.Name, shown), Ref: app.SaveRef(to)}, nil
}

// SaveFilesReply is a backup's or recycling's preview (Preview) or result (Text).
type SaveFilesReply struct {
	Preview *view.SaveFilesPreview `json:"preview,omitempty"`
	Text    string                 `json:"text,omitempty"`
	Failed  bool                   `json:"failed,omitempty"`
}

func (s *API) savesOf(refs []string) ([]*app.Save, error) {
	var out []*app.Save
	for _, ref := range refs {
		sv, err := s.findSave(ref)
		if err != nil {
			return nil, err
		}
		out = append(out, sv)
	}
	if len(out) == 0 {
		return nil, errors.New(view.T("no save chosen"))
	}
	return out, nil
}

// SaveBackup copies saves to <data>/backups/saves/<time>/ (12 §4.4). Without yes it only
// lists the files and their sizes for the page's confirmation (16 §6).
func (s *API) SaveBackup(refs []string, yes bool, token string) (*SaveFilesReply, error) {
	saves, err := s.savesOf(refs)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	r, err := s.a.BackupSaves(saves, yes, token)
	s.mu.Unlock()
	if err != nil {
		return nil, saveErr(err)
	}
	if !yes {
		return &SaveFilesReply{Preview: view.SaveBackupPreview(r)}, nil
	}
	return &SaveFilesReply{Text: view.SaveFilesText(r, true), Failed: len(r.Errs) > 0}, nil
}

// SaveTrash moves saves to the recycle bin (02 §3, 12 §4.4), refused while a game runs.
// Without yes it only lists the files and their sizes.
func (s *API) SaveTrash(refs []string, yes bool, token string) (*SaveFilesReply, error) {
	saves, err := s.savesOf(refs)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	r, err := s.a.TrashSaves(saves, yes, token)
	s.mu.Unlock()
	if err != nil {
		return nil, saveErr(err)
	}
	if !yes {
		return &SaveFilesReply{Preview: view.SaveTrashPreview(r)}, nil
	}
	return &SaveFilesReply{Text: view.SaveFilesText(r, false), Failed: len(r.Errs) > 0}, nil
}

// SaveNewList makes a new list from the save's Mods, as All lists' "from a save" does
// (07 §7, 12 §4.4): named after the colony, opened in the editor, not made current. With
// analyze it also starts the analysis of the new list (background task, 16 §2.16) and
// the page does not move.
func (s *API) SaveNewList(ref string, analyze bool) (*PlanDone, error) {
	sv, err := s.findSave(ref)
	if err != nil {
		return nil, err
	}
	name := sv.Name
	if sv.Colony != nil {
		name = sv.Colony.Name
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, t, err := view.NewPlan(s.a, view.FreePlanName(s.a, name), sv.Path)
	if p == nil {
		return s.planDone(0, t, err)
	}
	d, err := s.planDone(p.ID, t, err)
	// Analysing the new list indexes the save's Mods, so Can it load? can check what they
	// own (12 §4.6); the page stays and checks the save again when it is done.
	if err == nil && analyze {
		if s.startAnalyzeOf(p.Name) {
			d.Text += "  " + view.Tf("analysing %s…", p.Name)
		} else {
			d.Text += "  " + view.T("an analysis is already running; analyse the new list once it is done")
		}
		d.Page = ""
	}
	return d, err
}

// OpenSaveFolder shows the save's folder with its file selected where the system can.
func (s *API) OpenSaveFolder(ref string) error {
	sv, err := s.findSave(ref)
	if err != nil {
		return err
	}
	if s.openDir != nil {
		return s.openDir(sv.Folder.Dir)
	}
	return openDir(sv.Folder.Dir)
}

// saveErr words the core's refusals for the status line.
func saveErr(err error) error {
	if errors.Is(err, app.ErrGameRunning) {
		return errors.New(view.T("the game is running: close it first"))
	}
	return errors.New(view.Humanize(nil, err.Error()))
}

// leaveSaves forgets the listing and the facts when the player leaves the page (17 §7.3);
// the reader and its head cache stay until it is idle (readerIdle). A background read
// goes on (it is a task, 16 §2.16); a selected save waiting to be read is dropped, and a
// listing still being read when the player leaves is not kept (visit).
func (s *API) leaveSaves() {
	s.saves.mu.Lock()
	s.saves.list, s.saves.facts, s.saves.failed, s.saves.next = nil, nil, nil, nil
	s.saves.visit++
	s.saves.mu.Unlock()
}
