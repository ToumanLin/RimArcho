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
	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// API is the service the page calls (06 §1, §3): every method returns view models from
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
	openDir func(dir string) error      // the file manager (nil: the system's)
	lib     *app.Library                // Mod 库存, as of libAt
	libAt   string
	board   *view.Board
	needs   *view.Needs
	needsAt string // the database state the needs were built at
	content view.ContentCache
	busy    map[string]bool // scan, analyze, pack
	// building is the pack this window's build child is making ("" none).
	building string
	live     view.LiveTracker // the running game's log (03 §6.6)
	logPath  string           // the log page's Player.log and its clusters' first lines
	logLinks map[string]string
	x        exchange // import and export (exchange.go)
	free     releaser // returns memory to the OS after heavy work (memory.go)
	// bgs are the environment bgsEnv's backgrounds; bgPick the image rotation chose for
	// this window (background.go)
	bgs    []app.Background
	bgsEnv string
	bgPick string
	// Exe runs the scan, analysis and pack build children; "" is this executable (the
	// development harness points it at archo.exe).
	Exe string

	monitorOnce sync.Once
	quit        func() // closes the window (nil in the harness and in tests)
	// PackRunner builds a pack in place of the child process (tests).
	PackRunner func(name string) error
	// TexRunner runs `archo textures optimize` in place of the child process (tests).
	TexRunner func(args []string, progress func(string)) ([]string, error)
	texLast   TextureEvent // the running conversion's last progress
	// ScanRunner scans an environment in place of the child process (tests).
	ScanRunner func(env string) (string, error)
}

func newAPI(a *app.App) *API {
	return &API{a: a, open: openURL, busy: map[string]bool{}}
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
// <data>/logs/gui.log (06 §10: cold start to an interactive list).
func (s *API) Ready() {
	s.monitorOnce.Do(s.checkMonitor)
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

// checkMonitor installs or updates Archo Monitor in a child process once per window (08 §9)
// and, when that changed something, refreshes the page the way a scan does.
func (s *API) checkMonitor() {
	home, env, exe := s.a.Home, s.a.EnvName, s.Exe
	go func() {
		var out string
		var err error
		if exe != "" {
			out, err = app.ChildMonitorExe(exe, home, env)
		} else {
			out, err = app.ChildMonitor(home, env)
		}
		if err != nil || !strings.Contains(out, "installed ") && !strings.Contains(out, "plan:") && !strings.Contains(out, "replaced ") {
			return
		}
		s.mu.Lock()
		s.board, s.needs = nil, nil
		s.mu.Unlock()
		s.send("scan", map[string]any{"ok": true, "text": view.T("Archo Monitor is ready")})
	}()
}

// Start is what the page needs once: the language, its text table, the commands.
type Start struct {
	Lang     string            `json:"lang"`
	Text     map[string]string `json:"text"`
	Commands []view.Command    `json:"commands"`
	Version  string            `json:"version"`
	// Ops is the op table both pages' menus, hint lines and keys come from (06 §8.5).
	Ops []view.Op `json:"ops"`
}

// Init returns the page's start data.
func (s *API) Init() *Start {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &Start{Lang: view.Lang(), Text: view.Table(), Commands: view.Commands, Version: buildinfo.String(), Ops: view.OpTable()}
}

// ---- the board ----

// Board is the three columns with what surrounds them.
type Board struct {
	*view.Board
	Status    map[string]int `json:"status"` // row icons (06 §7)
	Needs     int            `json:"needs"`  // the top bar's number
	Stamp     string         `json:"stamp"`  // the database state: the page reloads when it changes
	Envs      []string       `json:"envs"`
	Error     string         `json:"error,omitempty"`
	Scanning  bool           `json:"scanning"`
	Analyzing bool           `json:"analyzing"`
	Never     bool           `json:"neverScanned"`
}

// Board builds the columns (06 §2.5) and the counts from the workspace.
func (s *API) Board() *Board {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.free.soon()
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
// PackList is the environment's packs, how many belong to other environments, and the
// builds running or cut short.
type PackList struct {
	Rows       []view.PackRow       `json:"rows"`
	Others     int                  `json:"others"`
	Unfinished []view.UnfinishedRow `json:"unfinished"`
	Building   string               `json:"building,omitempty"` // the build this window started
}

// Packs lists the current environment's packs (08 §5).
func (s *API) Packs() (*PackList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, others, err := view.PackRows(s.a)
	if err != nil {
		return nil, err
	}
	un, err := view.UnfinishedRows(s.a)
	return &PackList{Rows: rows, Others: others, Unfinished: un, Building: s.building}, err
}

// BuildPage is 编译成整合包 for the current plan (06 §2.6): checked and sorted under the
// lock, measured (every mod's folder walked) outside it; a light pack is not measured.
// tex is the textures choice (view.PackTextureOptions): 0 off, 100 预转换, less 精简.
func (s *API) BuildPage(light bool, tex int) (*view.BuildPage, error) {
	s.mu.Lock()
	plan, chip := "", ""
	if st := s.boardState(); st != nil {
		chip = view.EnvChip(s.board.EnvName, s.board.GameVersion)
		if st.Plan != nil {
			plan = st.Plan.Name
		}
	}
	name := view.DefaultPackName(s.a, plan)
	pl, err := s.a.PlanPack(name, light)
	if err == nil && !light {
		view.PlanTextures(s.a, pl, tex)
	}
	dir := s.a.PacksDir()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	pl.Measure(dir)
	p := view.BuildPageOf(pl, plan, chip, dir, light)
	p.Name = name
	return p, nil
}

// PackNameCheck says what is wrong with a new pack's name ("" when it can be used).
func (s *API) PackNameCheck(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return view.T("give the pack a name")
	}
	if app.CheckPackName(name) != nil {
		return view.T("letters, digits, spaces and . _ - only (up to 64)")
	}
	if _, err := s.a.ReadPack(name); err == nil {
		return view.T("a pack of this name exists")
	}
	return ""
}

// PackDryRun is 先看清单，不写磁盘: the text of archo pack build --dry-run (nothing is written).
func (s *API) PackDryRun(name string, light bool) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	argv := []string{"pack", "build", strings.TrimSpace(name), "--dry-run"}
	if light {
		argv = append(argv, "--light")
	}
	text, _ := cmds.Execute(s.a, argv)
	st := s.boardState()
	var out []string
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if strings.HasPrefix(l, "usage:") {
			break // a refused build ends with the command's usage: not for the page
		}
		out = append(out, view.Humanize(st, l))
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

// Pack is one pack's page (06 §2.7).
func (s *API) Pack(name string) (*view.PackPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.BuildPackPage(s.a, name)
}

// PackExport writes a pack to a file the player picks (pack export): the save dialog in
// the window; file is the typed path where there is none. "" when it was cancelled.
func (s *API) PackExport(name, file string, light bool) (string, error) {
	ext := ".archopack"
	if light {
		ext = ".archopack.json"
	}
	if file == "" {
		if s.x.native == nil {
			return "", errors.New(view.T("no file dialog here; type the path"))
		}
		f, err := s.x.native.saveFile(view.T("export the pack"), name+ext, [][2]string{{"*" + ext, "*" + ext}, {"All files (*.*)", "*.*"}})
		if err != nil || f == "" {
			return "", err
		}
		file = f
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err := s.a.ExportPack(name, file, light)
	if err != nil {
		return "", err
	}
	return view.Tf("exported pack %s to %s (%s)", name, file, view.SizeText(n)), nil
}

// PackBuild starts building the pack in an `archo pack build` child process and returns at
// once: the copy takes minutes and holds nothing here, so every page keeps working, and it
// goes on when the window closes. "pack" events carry the progress (view.PackProgress) and
// the end. One build at a time.
func (s *API) PackBuild(name string, light bool, tex int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := app.CheckPackName(name); err != nil {
		return "", err
	}
	if light {
		// The list only: nothing to copy, done at once.
		if _, err := s.a.BuildPack(name, true, false); err != nil {
			return "", err
		}
		return view.Tf("built pack %s", name), nil
	}
	if s.busy["pack"] {
		return "", errors.New(view.T("another pack is being built; wait for it to finish"))
	}
	if un, err := s.a.UnfinishedPacks(); err == nil {
		for _, u := range un {
			if u.Running {
				return "", errors.New(view.T("another pack is being built; wait for it to finish"))
			}
		}
	}
	if _, err := s.a.ReadPack(name); err == nil {
		return "", fmt.Errorf("pack %q exists; remove it first or pick another name", name)
	}
	s.busy["pack"], s.building = true, name
	tmp := filepath.Join(s.a.PacksDir(), name+app.TmpSuffix)
	run := s.PackRunner
	if run == nil {
		home, env, exe := s.a.Home, s.a.EnvName, s.Exe
		log := filepath.Join(home, "logs", "pack-build.log")
		extra := app.PackBuildArgs(view.PackTextureOptions(tex))
		run = func(name string) error {
			if exe != "" {
				return app.ChildPackBuildExe(exe, home, env, name, log, extra...)
			}
			return app.ChildPackBuild(home, env, name, log, extra...)
		}
	}
	go s.watchBuild(name, tmp, run)
	return view.Tf("build pack %s: it goes on in the background; you can use the other pages, or close the window", name), nil
}

// watchBuild runs the build and passes its progress file on as events until it ends.
func (s *API) watchBuild(name, tmp string, run func(string) error) {
	done := make(chan error, 1)
	go func() { done <- run(name) }()
	t := time.NewTicker(300 * time.Millisecond)
	defer t.Stop()
	var last view.PackProgress
	for {
		select {
		case err := <-done:
			s.mu.Lock()
			s.busy["pack"], s.building = false, ""
			s.mu.Unlock()
			ev := view.PackProgress{Name: name, Done: true, Ok: err == nil, Step: 5, Frac: 1, Text: view.Tf("built pack %s", name)}
			if err != nil {
				ev.Step, ev.Frac, ev.Text = last.Step, last.Frac, ""
				lines := strings.Split(view.Humanize(nil, err.Error()), "\n")
				for i, l := range lines {
					lines[i] = view.T(strings.TrimPrefix(strings.TrimSpace(l), "error: "))
				}
				ev.Error = strings.Join(lines, "\n")
			}
			s.send("pack", ev)
			return
		case <-t.C:
			if p := app.ReadBuildProgress(tmp); p != nil {
				if pp := view.ProgressOf(p); pp != last {
					last = pp
					s.send("pack", pp)
				}
			}
		}
	}
}

// PackStop asks a running build to stop (it removes what it copied).
func (s *API) PackStop(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.a.StopBuild(name); err != nil {
		return "", err
	}
	return view.Tf("asked the build of %s to stop", name), nil
}

// PackRemoveUnfinished deletes what a build cut short copied (its folder in the packs
// folder, nothing else).
func (s *API) PackRemoveUnfinished(name string) (string, error) {
	s.mu.Lock()
	dir, err := s.a.UnfinishedDir(name)
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	if err := os.RemoveAll(dir); err != nil { // off the lock: gigabytes take a while
		return "", err
	}
	return view.Tf("deleted the unfinished build of %s", name), nil
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

// Settings is the settings page (06 §2.14).
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

// History is the history area (06 §2.5): unsaved steps around now, then the versions.
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

// Live reports the running game and the errors its log gained (06 §2.1); the page asks
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

// Filter returns the indexes of a column's rows that match the query (06 §3.7), and how
// many rows match in each column (for "other lists: …", 06 §3.7).
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
	var tags []string
	for k := range s.board.Cols {
		for i := range s.board.Cols[k] {
			tags = append(tags, s.board.Cols[k][i].Tags...)
		}
	}
	out.Complete = view.CompleteQuery(query, tags)
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
	// Complete is the grey text after the query that tab accepts (06 §3.7).
	Complete string `json:"complete,omitempty"`
}

// ---- the detail ----

// Detail is one mod's detail (06 §4.1), in the order the page shows it.
type Detail struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Authors    string      `json:"authors,omitempty"`
	Kind       string      `json:"kind"`
	Header     []string    `json:"header"`
	Status     int         `json:"status"`
	StatusText string      `json:"statusText"`
	Facts      []view.Fact `json:"facts"`
	// Fields is the detail's one field list (06 §4.1): Archo ID, package, location, Workshop,
	// tags, languages, dependencies, conflicts.
	Fields   []view.Field   `json:"fields"`
	Deps     []view.Dep     `json:"deps"`
	Show     string         `json:"show"`
	Note     string         `json:"note,omitempty"`
	State    string         `json:"state,omitempty"` // the Workshop page's state
	About    []bbcode.Block `json:"about"`
	Workshop []bbcode.Block `json:"workshop"`
	History  string         `json:"history,omitempty"`
	Missing  bool           `json:"missing,omitempty"`
	URL      string         `json:"url,omitempty"` // the Workshop page
	Pos      int            `json:"pos,omitempty"`
}

// Detail returns the detail of an instance; fetch allows the network for the Workshop
// page (the page asks again after the cursor rested, 06 §4.3 item 3), refresh ignores the
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
		Header: view.HeaderFacts(mod, pos, staged), Facts: view.Facts(mod, data), Deps: view.Deps(st, data), Fields: view.ModFields(s.a, st, mod, data),
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

// Sections are the full detail's sub-sections (06 §4.2), #refs replaced by names.
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

// ---- verbs (06 §3.5) ----

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

// PosOptions are the position input's choices (06 §3.6).
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

// StageAll is T: every enabled mod but Core and the DLCs goes to staging (one undo step).
func (s *API) StageAll() (*view.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.after(view.StageAll(s.a))
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

// Preview is the dry run of sort, save or launch (the palette's 预览…; 06 §6).
func (s *API) Preview(what string) (*view.Preview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.DryRun(s.a, what)
}

// Run does sort, save or launch at once (S, w, L; 06 §6): no confirmation. A refusal comes
// back in Outcome.Refused with the commands that resolve it.
func (s *API) Run(what string) (*view.Outcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.needs = nil
	s.board = nil
	switch what {
	case view.ListSort, view.ListSave, view.ListLaunch:
	default:
		return nil, fmt.Errorf("no action %s", what)
	}
	return view.Run(s.a, what), nil
}

// ---- tags (06 §3.8) ----

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

// ---- 需要处理 (06 §5) ----

// Needs builds the screen. Opening it includes the newest log (parsing it if needed) and
// starts the files' check when it is stale, as the TUI does.
func (s *API) Needs() *view.Needs {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.free.soon()
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
		// The newest log was parsed into this list (03 §6.4): it has been seen.
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

// Inspect runs a read-only command an action names (view, 06 §5) and returns its text
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

// ---- changes (06 §2.9) ----

// Changes is the 变化 page: the current plan's mods since it was last played, or the whole
// library; of (an instance ID) narrows it to that mod's Lineage, every change by day.
func (s *API) Changes(library bool, of string) (*modchanges.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.boardState()
	if st == nil {
		return nil, errors.New("no list")
	}
	o := modchanges.Opts{Library: library}
	if of != "" {
		o.Of = view.LineageIDs(st.Ix, of)
		if len(o.Of) == 0 {
			return nil, errors.New(view.T("that mod is not known"))
		}
		o.OfName = st.Ix.ByID[of].DisplayName()
	}
	return view.BuildChanges(s.a, st, o)
}

// ChangeFiles is the raw file list behind a row: the files that differ between two
// snapshots.
func (s *API) ChangeFiles(before, after int64) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.ChangeFiles(s.a, before, after)
}

// OpenChangeNotes opens a Workshop item's change notes (ctrl+w on 变化).
func (s *API) OpenChangeNotes(ws string) (string, error) {
	if !view.IsNumber(ws) {
		return "", errors.New(view.T("no Workshop ID: not a Workshop mod"))
	}
	if err := s.open(view.ChangeNotesURL(ws)); err != nil {
		return "", err
	}
	return view.T("opened the change notes"), nil
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

// EnvRemoval is the dry run of removing an environment (06 §2.14): the confirmation's
// title and lines. Nothing changes.
type EnvRemoval struct {
	Title string   `json:"title"`
	Lines []string `json:"lines"`
}

func (s *API) EnvRemovePreview(name string) (*EnvRemoval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.a.EnvRemovePreview(name)
	if err != nil {
		return nil, err
	}
	return &EnvRemoval{Title: view.EnvRemovalTitle(r), Lines: view.EnvRemovalLines(r)}, nil
}

// RemoveEnv removes an environment from RimArcho's records; another one becomes current
// when it was in use. Refused while its game runs.
func (s *API) RemoveEnv(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.a.EnvName = ""
	_, err := s.a.EnvRemove(name)
	s.board, s.needs, s.lib = nil, nil, nil
	return s.text(view.Tf("removed environment %s", name), err)
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
	defer s.free.soon()
	st := s.boardState()
	if st == nil {
		return nil, errors.New("no list")
	}
	// A log of another game version needs an analysis for that version first (seconds):
	// it runs in the background and the page says so instead of holding every other call.
	s.a.AnalyzeExe = s.Exe
	s.a.AnalyzeWait = 200 * time.Millisecond
	defer func() { s.a.AnalyzeWait = 0 }()
	p, err := view.BuildLogPage(s.a, st, path, &s.live)
	if err == nil && p.Path != "" {
		s.logLinks, s.logPath = p.Links, p.Path
	}
	return p, err
}

// LogMonitor is the companion section of path as the running game writes it, for the log
// page's refresh on the live tick (nil when the game does not write path).
func (s *API) LogMonitor(path string) (*view.MonitorLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.boardState()
	if st == nil {
		return nil, errors.New("no list")
	}
	var links map[string]string
	if strings.EqualFold(s.logPath, path) {
		links = s.logLinks
	}
	return view.LiveMonitor(s.a, st, &s.live, path, links), nil
}

// AddMonitor puts Archo Monitor after Harmony in the current plan (the log page's action;
// one undoable step).
func (s *API) AddMonitor() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text(view.MonitorAdd(s.a))
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
		s.free.soon()
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

// Palette ranks commands, mods and the other environments for a query (06 §3.5).
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
	if rows, err := view.PlanRows(s.a); err == nil {
		all = append(all, view.PlanEntries(rows)...)
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

// Setting reads a GUI setting (column widths, 06 §7).
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
