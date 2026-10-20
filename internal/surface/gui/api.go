package gui

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/bbcode"
	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// API is the service the page calls (08 §1, §3): every method returns view models from
// internal/view, or runs a view verb. No business rule lives here.
//
// The app is not safe for concurrent use and Wails calls methods concurrently, so each
// method holds mu; the slow reads (the detail's description, which may fetch the Workshop
// page) run outside it on a snapshot of the index, as the TUI's loaders do.
type API struct {
	mu      sync.Mutex
	a       *app.App
	emit    func(name string, data any) // Wails events; nil in tests
	open    func(url string) error      // the system browser
	board   *view.Board
	needs   *view.Needs
	needsAt string // the database state the needs were built at
	content view.ContentCache
	events  map[int64]*store.Event // the changes list's events, by id
	busy    map[string]bool        // scan, analyze
	live    view.LiveTracker       // the running game's log (10 §3)
	// Exe runs the scan and analysis children; "" is this executable (the development
	// harness points it at archo.exe).
	Exe string
}

func newAPI(a *app.App) *API {
	return &API{a: a, open: openURL, events: map[int64]*store.Event{}, busy: map[string]bool{}}
}

func (s *API) send(name string, data any) {
	if s.emit != nil {
		s.emit(name, data)
	}
}

// ---- start ----

// processStart is when this process began (close enough: package initialisation).
var processStart = time.Now()

// Ready is called once the page has drawn the list; it logs the start-up time to
// <data>/logs/gui.log (08 §10: cold start to an interactive list).
func (s *API) Ready() {
	ms := time.Since(processStart).Milliseconds()
	dir := filepath.Join(s.a.Home, "logs")
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "gui.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s ready %d ms (%s)\n", time.Now().Format(time.RFC3339), ms, buildinfo.String())
}

// Start is what the page needs once: the language, its text table, the commands.
type Start struct {
	Lang     string            `json:"lang"`
	Text     map[string]string `json:"text"`
	Commands []view.Command    `json:"commands"`
	Version  string            `json:"version"`
}

// Init returns the page's start data.
func (s *API) Init() *Start {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &Start{Lang: view.Lang(), Text: view.Table(), Commands: view.Commands, Version: buildinfo.String()}
}

// ---- the board ----

// Board is the three columns with what surrounds them.
type Board struct {
	*view.Board
	Status    map[string]int `json:"status"` // row icons (06 §3)
	Needs     int            `json:"needs"`  // the top bar's number
	Stamp     string         `json:"stamp"`  // the database state: the page reloads when it changes
	Envs      []string       `json:"envs"`
	Error     string         `json:"error,omitempty"`
	Scanning  bool           `json:"scanning"`
	Analyzing bool           `json:"analyzing"`
	Never     bool           `json:"neverScanned"`
}

// Board builds the columns (06 §5) and the counts from the workspace.
func (s *API) Board() *Board {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.boardLocked()
}

func (s *API) boardLocked() *Board {
	b, err := view.BuildBoard(s.a)
	s.board = b
	out := &Board{Board: b, Stamp: s.a.Store.ChangeKey(), Scanning: s.busy["scan"], Analyzing: s.busy["analyze"]}
	if envs, err := s.a.Store.Envs(); err == nil {
		for _, e := range envs {
			out.Envs = append(out.Envs, e.Name)
		}
	}
	if err != nil {
		out.Error = view.Humanize(nil, err.Error())
		return out
	}
	if e, err := s.a.Env(); err == nil {
		sc, _, _ := s.a.LastScan(e.ID)
		out.Never = sc == nil
	}
	n := s.needsLocked(false)
	out.Status, out.Needs = n.Status, n.Count
	return out
}

// Stamp is the database state; the page asks on focus and reloads when it changed (G8).
// PackList is the environment's packs and how many belong to other environments.
type PackList struct {
	Rows   []view.PackRow `json:"rows"`
	Others int            `json:"others"`
}

// Packs lists the current environment's packs (10 §8).
func (s *API) Packs() (*PackList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, others, err := view.PackRows(s.a)
	return &PackList{Rows: rows, Others: others}, err
}

// PackPreview is a dry-run build of the current list as a pack.
func (s *API) PackPreview(name string) (*view.PackPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.PreviewPack(s.a, name)
}

// PackBuild builds the pack.
func (s *API) PackBuild(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.a.BuildPack(name, false, false); err != nil {
		return "", err
	}
	return view.Tf("built pack %s", name), nil
}

// PackLaunch plays a pack.
func (s *API) PackLaunch(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.a.LaunchPack(name, false); err != nil {
		return "", err
	}
	return view.Tf("pack %s: the game is starting", name), nil
}

// PackRemove moves a pack to the recycle bin.
func (s *API) PackRemove(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.a.RemovePack(name, false); err != nil {
		return "", err
	}
	return view.Tf("pack %s is in the recycle bin", name), nil
}

// Settings is the settings page (10 §7).
func (s *API) Settings() []view.SettingGroup {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.SettingsPage(s.a, s.boardState())
}

// Environment is the environment page: the environments, the current one's folders and
// launch.
func (s *API) Environment() *view.EnvPage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.BuildEnvPage(s.a, s.boardState())
}

// ApplySetting runs one of the settings page's commands, with value in place of {}.
func (s *API) ApplySetting(argv []string, value string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := view.SettingCommand(argv); err != nil {
		return nil, err
	}
	run := make([]string, len(argv))
	for i, x := range argv {
		if x == "{}" {
			x = value
		}
		run[i] = x
	}
	text, code := cmds.Execute(s.a, run)
	s.board, s.needs = nil, nil
	if len(run) == 4 && run[0] == "config" && run[2] == "ui.lang" && code == cmds.ExitOK {
		view.SetLang(run[3])
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if code != cmds.ExitOK {
		return nil, fmt.Errorf("%s", strings.Join(lines, " "))
	}
	return lines, nil
}

// History is the history area (08 §14): unsaved steps around now, then the versions.
func (s *API) History() ([]view.HistItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.board == nil || s.board.St == nil {
		return nil, nil
	}
	return view.HistoryItems(s.a, s.board.St)
}

// HistoryGo moves through the unsaved steps (n > 0 undoes n, n < 0 redoes -n) or, with
// version > 0, loads that version into the list (loadout checkout).
func (s *API) HistoryGo(steps, version int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if version > 0 {
		if _, _, err := s.a.Checkout(fmt.Sprintf("v%d", version)); err != nil {
			return "", err
		}
		return view.Tf("loaded v%d into the list (not saved yet)", version), nil
	}
	r, err := s.a.StepTo(steps)
	if err != nil {
		return "", err
	}
	return view.Tf("%d changes", len(r.Changes)), nil
}

// Live reports the running game and the errors its log gained (10 §3.4); the page asks
// every two seconds.
func (s *API) Live() (*view.Live, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.board == nil || s.board.St == nil {
		return &view.Live{}, nil
	}
	return s.live.Update(s.a, s.board.St)
}

func (s *API) Stamp() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.a.Store.ChangeKey()
}

// Filter returns the indexes of a column's rows that match the query (06 §5.5), and how
// many rows match in each column (for "other lists: …", 06 §13.8).
func (s *API) Filter(col int, query string) (*Filtered, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.board == nil || s.board.St == nil || col < 0 || col > 2 {
		return &Filtered{}, nil
	}
	q := view.ParseQuery(query)
	f := view.MatchFacts{Status: s.needsLocked(false).Status}
	if q.NeedsContent() {
		c, err := s.content.Get(s.a, s.board.St)
		if err != nil {
			return nil, err
		}
		f.Content = c
	}
	out := &Filtered{}
	for k := range s.board.Cols {
		ix := q.Filter(s.board.Cols[k], f)
		out.Counts[k] = len(ix)
		if k == col {
			out.Rows = ix
		}
	}
	return out, nil
}

// Filtered is the result of a column search.
type Filtered struct {
	Rows   []int  `json:"rows"`
	Counts [3]int `json:"counts"`
}

// ---- the detail ----

// Detail is one mod's detail (06 §6.1), in the order the page shows it.
type Detail struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Authors    string         `json:"authors,omitempty"`
	Kind       string         `json:"kind"`
	Header     []string       `json:"header"`
	Status     int            `json:"status"`
	StatusText string         `json:"statusText"`
	Facts      []view.Fact    `json:"facts"`
	Deps       []view.Dep     `json:"deps"`
	Show       string         `json:"show"`
	Note       string         `json:"note,omitempty"`
	State      string         `json:"state,omitempty"` // the Workshop page's state
	About      []bbcode.Block `json:"about"`
	Workshop   []bbcode.Block `json:"workshop"`
	History    string         `json:"history,omitempty"`
	Missing    bool           `json:"missing,omitempty"`
	URL        string         `json:"url,omitempty"` // the Workshop page
	Pos        int            `json:"pos,omitempty"`
}

// Detail returns the detail of an instance; fetch allows the network for the Workshop
// page (the page asks again after the cursor rested, 06 §6.4 item 3), refresh ignores the
// cache.
func (s *API) Detail(id string, staged, fetch, refresh bool) (*Detail, error) {
	s.mu.Lock()
	st := s.boardState()
	if st == nil {
		s.mu.Unlock()
		return nil, errors.New("no list")
	}
	mod := st.Ix.ByID[id]
	if mod == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("no mod %s", id)
	}
	n := s.needsLocked(false)
	pos := st.Position(id)
	item := view.ModItem(st, mod)
	s.mu.Unlock()

	data := view.LoadDetail(s.a, st.Ix, mod, fetch, refresh) // off the lock: may use the network
	d := &Detail{ID: id, Name: mod.DisplayName(), Authors: strings.TrimSpace(mod.Authors), Kind: view.SourceKind(mod), Pos: pos,
		Header: view.HeaderFacts(mod, pos, staged), Facts: view.Facts(mod, data), Deps: view.Deps(st, data),
		Show: data.Desc.Show, Note: view.DescNote(data), State: data.Desc.WorkshopState,
		About: data.About, Workshop: data.Workshop, History: view.HistoryLine(data.Events)}
	d.Status = view.RowStatus(&item, n.Status)
	switch d.Status {
	case view.StatusNone:
		d.StatusText = view.T("no problems found")
	case view.StatusUnloadable:
		d.StatusText = view.UnloadableWhy(st, mod)
	default:
		d.StatusText = view.T("needs attention")
	}
	if ws := mod.WorkshopID(); view.IsNumber(ws) {
		d.URL = view.WorkshopURL(ws)
	}
	return d, nil
}

// Sections are the full detail's sub-sections (06 §6.3), #refs replaced by names.
func (s *API) Sections(id string) ([]view.Section, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.boardState()
	if st == nil || st.Ix.ByID[id] == nil {
		return nil, fmt.Errorf("no mod %s", id)
	}
	secs := view.Sections(s.a, st.Ix, st.Ix.ByID[id])
	for i := range secs {
		for j, l := range secs[i].Lines {
			secs[i].Lines[j] = view.Humanize(st, l)
		}
	}
	return secs, nil
}

func (s *API) boardState() *app.LoadoutState {
	if s.board == nil || s.board.St == nil {
		s.boardLocked()
	}
	if s.board == nil {
		return nil
	}
	return s.board.St
}

// ---- verbs (06 §13.6) ----

func (s *API) object(ids, refs []string) view.Object {
	st := s.boardState()
	o := view.Object{IDs: ids, Refs: refs}
	if st != nil {
		if len(o.Refs) == 0 {
			o = view.ObjectOf(st, ids)
		} else {
			o.What = view.Named(st, ids)
			if len(ids) == 0 && len(refs) == 1 {
				o.What = strings.TrimPrefix(refs[0], "pkg:")
			}
		}
	}
	return o
}

func (s *API) after(r *view.Result, err error) (*view.Result, error) {
	s.board = nil
	if err != nil {
		return nil, errors.New(view.Humanize(s.boardState(), err.Error()))
	}
	return r, nil
}

// Toggle is space: enable, disable, or put back (by the column the object is in).
func (s *API) Toggle(col int, ids, refs []string) (*view.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.after(view.Toggle(s.a, view.Column(col), s.object(ids, refs)))
}

// Stage is t: stage, or put back from staging.
func (s *API) Stage(col int, ids []string) (*view.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.after(view.Stage(s.a, view.Column(col), s.object(ids, nil)))
}

// Unstage takes mods out of staging (a drop on Disabled).
func (s *API) Unstage(ids []string) (*view.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.after(view.Unstage(s.a, s.object(ids, nil)))
}

// MoveBy is J / K.
func (s *API) MoveBy(ids []string, d int) (*view.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.after(view.MoveBy(s.a, s.object(ids, nil), d))
}

// Place puts the mods at a 1-based place of the list without them (m, or a drop).
func (s *API) Place(col int, ids []string, at int) (*view.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.after(view.Place(s.a, view.Column(col), s.object(ids, nil), at))
}

// PosOptions are the position input's choices (06 §13.7).
func (s *API) PosOptions(ids []string, text string, before bool) ([]view.PosOpt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.boardState()
	if st == nil {
		return nil, errors.New("no list")
	}
	return view.PosOptions(s.a, st, ids, text, before), nil
}

func (s *API) text(t string, err error) (string, error) {
	s.board = nil
	if err != nil {
		return "", errors.New(view.Humanize(s.boardState(), err.Error()))
	}
	return t, nil
}

// Undo and Redo step the workspace (u, ctrl+r).
func (s *API) Undo() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text(view.UndoRedo(s.a, false))
}

// Redo steps the workspace forward.
func (s *API) Redo() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text(view.UndoRedo(s.a, true))
}

// KeepOfficial is T: only Core and the DLCs stay enabled (one undo step).
func (s *API) KeepOfficial() (*view.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.after(view.KeepOfficial(s.a))
}

// RestoreAll puts everything in staging back (the palette).
func (s *API) RestoreAll() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text(view.RestoreAll(s.a))
}

// Swap is x.
func (s *API) Swap(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.boardState()
	if st == nil {
		return "", errors.New("no list")
	}
	return s.text(view.Swap(s.a, st, id))
}

// Preview shows sort, save or launch before it happens (06 §9).
func (s *API) Preview(what string) (*view.Preview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.DryRun(s.a, what)
}

// Apply runs sort, save or launch after its preview.
func (s *API) Apply(what string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.needs = nil
	return s.text(view.Apply(s.a, what))
}

// ---- tags (06 §12.3) ----

// TagCandidates for the mods.
func (s *API) TagCandidates(ids []string) ([]view.TagCand, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.boardState()
	if st == nil {
		return nil, errors.New("no list")
	}
	var sel [][]string
	for _, id := range ids {
		if m := st.Ix.ByID[id]; m != nil {
			sel = append(sel, m.Tags)
		}
	}
	return view.TagCandidates(s.a, st.Ix.Env.ID, sel), nil
}

// SetTag adds a tag to the mods, or removes it.
func (s *API) SetTag(ids []string, tag string, remove bool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.boardState()
	if st == nil {
		return "", errors.New("no list")
	}
	o := view.ObjectOf(st, ids)
	_, err := s.a.SetTags(o.Refs, []string{tag}, remove)
	msg := view.Tf("added %s", tag)
	if remove {
		msg = view.Tf("removed %s", tag)
	}
	return s.text(view.Tf("tags of %s: %s", o.What, msg), err)
}

// ---- 需要处理 (06 §7) ----

// Needs builds the screen. Opening it includes the newest log (parsing it if needed) and
// starts the files' check when it is stale, as the TUI does.
func (s *API) Needs() *view.Needs {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.needsLocked(true)
	if (n.Stale || n.Never) && !s.busy["analyze"] {
		s.startAnalyze()
	}
	return n
}

func (s *API) needsLocked(withLog bool) *view.Needs {
	key := s.a.Store.ChangeKey()
	if !withLog && s.needs != nil && s.needsAt == key {
		return s.needs
	}
	if withLog {
		s.a.LazyLog, s.a.LogCacheOnly = false, false
	}
	st := s.boardState()
	ps, err := s.a.Problems(false, nil)
	if err != nil || st == nil {
		return &view.Needs{Status: map[string]int{}}
	}
	if withLog {
		// The newest log was parsed into this list (decisions-m4 §7.2): it has been seen.
		if cur, _ := s.a.LogPaths(st); cur != "" && s.a.ProblemLog(st) == cur {
			s.a.SelectLog(st, cur)
		}
	}
	s.needs, s.needsAt = view.BuildNeeds(s.a, st, ps), s.a.Store.ChangeKey()
	return s.needs
}

// DoAction runs an item's data action (view.Do); option picks a choose-disable choice.
func (s *API) DoAction(act view.Action, option int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	done, err := view.Do(s.a, act, option)
	s.needs = nil
	if errors.Is(err, view.ErrNothing) {
		return done, nil
	}
	return s.text(done, err)
}

// inspectable are the read-only commands an item's "see what" may run.
var inspectable = map[string]bool{"problems": true, "patches": true, "mod": true, "def": true, "harmony": true}

// Inspect runs a read-only command an action names (view, 06 §7.2) and returns its text
// with names for #refs.
func (s *API) Inspect(argv []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(argv) == 0 || !inspectable[argv[0]] {
		return nil, fmt.Errorf("not a read-only command: %v", argv)
	}
	text, _ := cmds.Execute(s.a, argv)
	st := s.boardState()
	var out []string
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		out = append(out, view.Humanize(st, l))
	}
	return out, nil
}

// ---- changes (06 §4.1) ----

// Changes lists the last scans' changes.
func (s *API) Changes() ([]view.ChangeScan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.boardState()
	if st == nil {
		return nil, errors.New("no list")
	}
	cs, err := view.Changes(s.a, st)
	for _, c := range cs {
		for _, e := range c.Events {
			s.events[e.ID] = e.Event
		}
	}
	return cs, err
}

// FileDiff lists the files a change touched.
func (s *API) FileDiff(eventID int64) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.events[eventID]; e != nil {
		return view.FileDiff(s.a, e)
	}
	return nil
}

// ---- environments, logs, scans ----

// UseEnv switches the environment.
func (s *API) UseEnv(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.a.EnvName = ""
	_, err := s.a.EnvUse(name)
	s.needs = nil
	return s.text(view.Tf("switched to %s", name), err)
}

// Logs are the logs the player may choose (Player.log, Player-prev.log).
func (s *API) Logs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.boardState()
	if st == nil {
		return nil
	}
	var out []string
	cur, prev := s.a.LogPaths(st)
	for _, p := range []string{cur, prev} {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// LogPage analyses a log for the 日志 tab (the newest when path is empty).
func (s *API) LogPage(path string) (*view.LogPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.boardState()
	if st == nil {
		return nil, errors.New("no list")
	}
	// A log of another game version needs an analysis for that version first (seconds):
	// it runs in the background and the page says so instead of holding every other call.
	s.a.AnalyzeExe = s.Exe
	s.a.AnalyzeWait = 200 * time.Millisecond
	defer func() { s.a.AnalyzeWait = 0 }()
	return view.BuildLogPage(s.a, st, path)
}

// LogCluster is one cluster's evidence (archo log --cluster), names for #refs.
func (s *API) LogCluster(path, id string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if path == "" || id == "" || strings.HasPrefix(path, "-") || strings.HasPrefix(id, "-") {
		return nil, errors.New("no cluster")
	}
	text, _ := cmds.Execute(s.a, []string{"log", "--file", path, "--cluster", id})
	st := s.boardState()
	var out []string
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		out = append(out, view.Humanize(st, l))
	}
	return out, nil
}

// SelectLog picks the log whose errors join 需要处理.
func (s *API) SelectLog(path string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.boardState()
	if st == nil {
		return "", errors.New("no list")
	}
	s.needs = nil
	return s.text(view.T("the log is read"), s.a.SelectLog(st, path))
}

// Scan rescans in a child process; the "scan" event says when it is done.
func (s *API) Scan() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy["scan"] {
		return false
	}
	s.busy["scan"] = true
	home, env := s.a.Home, s.a.EnvName
	go func() {
		var out string
		var err error
		if s.Exe != "" {
			out, err = app.ChildScanExe(s.Exe, home, env)
		} else {
			out, err = app.ChildScan(home, env)
		}
		s.mu.Lock()
		s.busy["scan"], s.board, s.needs = false, nil, nil
		ev := map[string]any{"ok": err == nil}
		if err != nil {
			ev["error"] = view.Humanize(nil, firstLine(out)+" "+err.Error())
		} else if b := s.boardLocked(); b.Board != nil && b.Changes > 0 {
			ev["text"] = view.T("scan complete") + " · " + view.ChangeSummary(b.LastEvents)
		} else {
			ev["text"] = view.T("scan complete · nothing changed")
		}
		s.mu.Unlock()
		s.send("scan", ev)
	}()
	return true
}

// Analyze checks the mods' files again in a child process; "analyze" events carry the
// progress lines and the end.
func (s *API) Analyze() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startAnalyze()
}

func (s *API) startAnalyze() bool {
	if s.busy["analyze"] {
		return false
	}
	s.busy["analyze"] = true
	run := s.a.AnalyzeRunner
	home, env, exe := s.a.Home, s.a.EnvName, s.Exe
	go func() {
		progress := func(l string) { s.send("analyze", map[string]any{"line": strings.TrimPrefix(l, "progress: ")}) }
		var err error
		switch {
		case run != nil:
			err = run(env, progress)
		case exe != "":
			err = app.ChildAnalyzeExe(exe, home, env, "", progress)
		default:
			err = app.ChildAnalyze(home, env, "", progress)
		}
		s.mu.Lock()
		s.busy["analyze"], s.needs = false, nil
		s.mu.Unlock()
		ev := map[string]any{"done": true, "ok": err == nil}
		if err != nil {
			ev["error"] = err.Error()
		}
		s.send("analyze", ev)
	}()
	return true
}

// ---- palette and the rest ----

// Palette ranks commands, mods and the other environments for a query (06 §8).
func (s *API) Palette(query string) []view.PalEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []view.PalEntry
	for _, c := range view.Commands {
		all = append(all, view.PalEntry{Kind: view.PalCommand, ID: c.ID, Name: c.Name(), Alias: c.En, Desc: c.Desc})
	}
	cur := ""
	if s.board != nil {
		cur = s.board.EnvName
	}
	if envs, err := s.a.Store.Envs(); err == nil {
		for _, e := range envs {
			if e.Name != cur {
				all = append(all, view.PalEntry{Kind: view.PalEnv, ID: e.Name, Name: view.Tf("switch to %s", e.Name), Alias: e.Name, Desc: "env"})
			}
		}
	}
	if s.board != nil {
		seen := map[string]bool{}
		for _, col := range s.board.Cols {
			for _, it := range col {
				if it.ID == "" || seen[it.ID] {
					continue
				}
				seen[it.ID] = true
				all = append(all, view.PalEntry{Kind: view.PalMod, ID: it.ID, Name: it.Name, Desc: it.Package + " " + it.Author})
			}
		}
	}
	out := view.PalRank(query, all)
	if len(out) > 50 {
		out = out[:50]
	}
	return out
}

// RunCLI runs a line the player typed as an archo command (the palette's command line).
func (s *API) RunCLI(line string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	argv := splitArgs(line)
	if len(argv) > 0 && argv[0] == "archo" {
		argv = argv[1:]
	}
	if len(argv) == 0 || argv[0] == "mcp" || argv[0] == "tui" || argv[0] == "gui" {
		return []string{view.T("not a command here")}
	}
	text, _ := cmds.Execute(s.a, argv)
	s.board, s.needs = nil, nil
	st := s.boardState()
	var out []string
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		out = append(out, view.Humanize(st, l))
	}
	return out
}

// OpenURL opens an http(s) link in the system browser (links in descriptions, the
// Workshop page).
func (s *API) OpenURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("not a web link: %s", raw)
	}
	return s.open(u.String())
}

// Setting reads a GUI setting (column widths, 08 §4).
func (s *API) Setting(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !strings.HasPrefix(key, "ui.gui.") {
		return ""
	}
	return s.a.Store.Meta(key)
}

// SetSetting stores a GUI setting.
func (s *API) SetSetting(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !strings.HasPrefix(key, "ui.gui.") {
		return fmt.Errorf("not a GUI setting: %s", key)
	}
	return s.a.Store.SetMeta(key, value)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// splitArgs splits a command line on spaces, keeping "quoted words" together.
func splitArgs(line string) []string {
	var out []string
	var cur strings.Builder
	in, has := false, false
	for _, r := range line {
		switch {
		case r == '"':
			in, has = !in, true
		case r == ' ' && !in:
			if has {
				out = append(out, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteRune(r)
			has = true
		}
	}
	if has {
		out = append(out, cur.String())
	}
	return out
}
