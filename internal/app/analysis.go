package app

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/oplog"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// AnalyzeMemoryLimit is the soft heap cap of the `archo analyze` child (18 §2 target:
// peak ≤ 400 MB). M3 measurement: 310 MB peak working set on 482 active mods.
const AnalyzeMemoryLimit = 300 << 20

// ModDir is where an instance's files are (links resolved).
func (ix *Index) ModDir(m *store.Mod) string {
	if m.RealPath != "" {
		return m.RealPath
	}
	if s := ix.Source(m.SourceID); s != nil {
		return filepath.Join(s.Root, m.Rel)
	}
	return m.Rel
}

// ModIsLink checks the registered entry before resolving it to its target.
// ModDir normally returns RealPath, which no longer identifies a junction itself.
func (ix *Index) ModIsLink(m *store.Mod) bool {
	if s := ix.Source(m.SourceID); s != nil && source.IsLink(filepath.Join(s.Root, m.Rel)) {
		return true
	}
	return source.IsLink(ix.ModDir(m))
}

// LinkEntry is the link a linked copy is seen through (the entry in its source, such as
// Mods/_Link_<id>), "" when the copy is a real folder.
func (ix *Index) LinkEntry(m *store.Mod) string {
	if s := ix.Source(m.SourceID); s != nil {
		if p := filepath.Join(s.Root, m.Rel); source.IsLink(p) {
			return p
		}
	}
	if p := ix.ModDir(m); source.IsLink(p) {
		return p
	}
	return ""
}

// LinkMaker names the tool a link's name says made it: RimCrow for `_Link_`, RimArcho for
// its own prefixes (09 §3), "" when the name says nothing.
func LinkMaker(link string) string {
	n := strings.ToLower(filepath.Base(link))
	switch {
	case strings.HasPrefix(n, "_link_"):
		return "RimCrow"
	case strings.HasPrefix(n, "_archo_"), strings.HasPrefix(n, "!archopack_"):
		return "RimArcho"
	}
	return ""
}

// analysisMods: the enabled list in order, plus every other live instance the list could
// load: registered, or deployable through a link on save (SteamCMD and extra folders).
// Inactive ones are definers for no-match reasons and the missing-parent provider index
// (05 §4.3). pos is 1-based; 0 = inactive.
func analysisMods(st *LoadoutState) ([]*store.Mod, map[string]int) {
	pos := map[string]int{}
	for i, e := range st.List {
		if e.Instance != "" {
			if _, dup := pos[e.Instance]; !dup {
				pos[e.Instance] = i + 1
			}
		}
	}
	var all []*store.Mod
	for _, m := range st.Ix.Mods {
		if !m.TombstoneAt.IsZero() || m.SnapID == 0 {
			continue
		}
		if pos[m.ID] == 0 && (m.Residue || m.Unloadable != "" || !st.Reg.Loadable(m.ID) && !st.Deployable(m)) {
			continue
		}
		all = append(all, m)
	}
	return all, pos
}

// AnalysisKey identifies what an analysis depends on: analyzer and Def content versions
// (a change to the stored content fields alone runs it again), game version, and
// every included instance with its snapshot and position. Cheap (no declarations loaded).
func (a *App) AnalysisKey(st *LoadoutState, mm string) string {
	if mm == "" {
		mm = st.GameMM()
	}
	all, pos := analysisMods(st)
	h := sha256.New()
	fmt.Fprintf(h, "v%d|c%d|p%d|%s\n", analyze.Version, analyze.DefContentVersion, analyze.PatchVersion, mm)
	for _, m := range all {
		fmt.Fprintf(h, "%s %x %d\n", m.ID, m.SnapHash, pos[m.ID])
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:24]
}

// AnalyzeInput builds the analysis input from the workspace. mm overrides the game version
// ("" = the environment's).
func (a *App) AnalyzeInput(st *LoadoutState, mm string) (analyze.Input, string, error) {
	if mm == "" {
		mm = st.GameMM()
	}
	in := analyze.Input{MM: mm}
	all, pos := analysisMods(st)
	if err := a.Store.LoadDecls(all); err != nil {
		return in, "", err
	}
	for _, m := range all {
		in.Mods = append(in.Mods, &analyze.Mod{
			Instance: m.ID, Lineage: m.LineageID, SnapID: m.SnapID, SnapHash: m.SnapHash,
			Dir: st.Ix.ModDir(m), Pkg: strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam"),
			Name: m.DisplayName(), Decl: m.Decl(), Pos: pos[m.ID],
		})
	}
	in.Cached = a.Store.CachedDefs
	in.Workers = a.AnalyzeWorkers
	return in, a.AnalysisKey(st, mm), nil
}

// AnalyzeReport is the outcome of Analyze.
type AnalyzeReport struct {
	ID    int64
	Key   string
	MM    string
	Stats analyze.Stats
}

// Analyze runs the Def and Patch layers in this process and stores the result. It is what
// the `archo analyze` child does (05 §3); other callers go through EnsureAnalysis.
func (a *App) Analyze(progress func(string)) (*AnalyzeReport, error) {
	return a.AnalyzeContext(context.Background(), progress)
}

// AnalyzeContext allows cancellation before publishing the completed analysis. It is the
// can-it-run check's primitive: one check row (oplog.Check, 13 §1.3) records its duration and
// phases (CheckPhases), whichever surface started it.
func (a *App) AnalyzeContext(ctx context.Context, progress func(string)) (rep *AnalyzeReport, err error) {
	h := a.Ops.Begin(oplog.Row{Op: oplog.Check, EnvID: a.opEnv()})
	var ph CheckPhases
	ph.Automatic = a.AnalyzeBackground
	mark := time.Now()
	gc0 := gcCPU()
	var report *CheckReport // for the slow-check report (16 §2.14), saved with the row
	lap := func() int64 {
		now := time.Now()
		d := now.Sub(mark).Milliseconds()
		mark = now
		return d
	}
	defer func() {
		if h == nil {
			return
		}
		result := oplog.OK
		if err != nil {
			result = oplog.Failed
		}
		if report != nil {
			report.GCMs = (gcCPU() - gc0).Milliseconds()
			a.saveCheckReport(*report)
		}
		h.End(result, joinDetail(ph.Detail(), errText(err)))
	}()
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	if st.Plan != nil && h != nil {
		h.SetSubject(st.Plan.Name)
	}
	in, key, err := a.AnalyzeInput(st, "")
	if err != nil {
		return nil, err
	}
	ph.Game = st.GameMM()
	for _, m := range in.Mods {
		ph.Total++
		if m.Pos > 0 {
			ph.Active++
		}
	}
	list := "new"
	if prev, _ := a.analysisOf(st, false); prev != nil && prev.Run != nil {
		list = map[bool]string{true: "same", false: "changed"}[prev.Run.RunKey == key]
	}
	ph.Add("read", lap())
	in.Progress = progress
	cache := a.assemblyCache()
	if cache != nil {
		in.Assemblies = cache
	}
	started := a.Now()
	out, err := analyze.Run(in)
	if err != nil {
		return nil, err
	}
	cache.prune()
	run := lap()
	ws := map[string]string{}
	for _, m := range st.Ix.Mods {
		if id := m.WorkshopID(); id != "" {
			ws[m.ID] = id
		}
	}
	rp := checkReport(h.ID(), out.Stats, func(inst string) string { return ws[inst] })
	rp.OpenMs, rp.List, rp.Workers = a.OpenedIn.Milliseconds(), list, in.Workers
	if rp.Workers <= 0 {
		rp.Workers = min(max(2, runtime.NumCPU()), 8) // analyze.Run's default
	}
	report = &rp
	ph.Add("defs", out.Stats.DefsMs)
	ph.Add("patches", out.Stats.PatchMs)
	ph.Add("harmony", out.Stats.HarmonyMs)
	if rest := run - out.Stats.DefsMs - out.Stats.PatchMs - out.Stats.HarmonyMs; rest > 0 {
		ph.Add("cache", rest)
	}
	in = analyze.Input{} // release the declarations before writing
	id, err := a.Store.SaveAnalysisContext(ctx, st.Ix.Env.GameID, SetFingerprint(st.List), key, st.GameMM(), a.planSetKeys(st), out, started, a.Now())
	if err != nil {
		return nil, err
	}
	a.Store.TruncateWAL() // a first analysis writes ~100 MB through the WAL (04 §4)
	ph.Add("save", lap())
	// The log attribution's reference index for this set (06 §5), so the first query on a
	// new log does not read every mod file.
	if progress != nil {
		progress("indexing def references")
	}
	a.RefIndex(st)
	ph.Add("refs", lap())
	return &AnalyzeReport{ID: id, Key: key, MM: st.GameMM(), Stats: out.Stats}, nil
}

// AnalysisState says whether the stored analysis matches the current workspace.
type AnalysisState struct {
	Run   *store.Analysis // nil: never analysed
	Fresh bool
}

// Analysis returns the stored run of st's slot — its enabled set (05 §3) — and whether it
// is current. Each plan's newest run is kept, so switching plans back and forth reuses it.
// A slot not analysed yet (the list just gained or lost a mod) shows the environment's
// newest run as stale, as before slots existed, until the analysis catches up.
func (a *App) Analysis(st *LoadoutState) (*AnalysisState, error) { return a.analysisOf(st, true) }

// analysisOf is Analysis; without newest an empty slot is "never analysed" (the per-plan
// rows: another plan's run says nothing about this one).
func (a *App) analysisOf(st *LoadoutState, newest bool) (*AnalysisState, error) {
	a.Store.MM = st.GameMM()
	r, err := a.Store.LatestAnalysis(st.Ix.Env.GameID, SetFingerprint(st.List))
	if err == nil && r == nil && newest {
		r, err = a.Store.NewestAnalysis(st.Ix.Env.GameID)
	}
	if err != nil {
		return nil, err
	}
	s := &AnalysisState{Run: r}
	if r != nil {
		s.Fresh = r.RunKey == a.AnalysisKey(st, "")
	}
	return s, nil
}

// ChildAnalyze runs `<this executable> analyze --env <name>` and streams its output lines.
func ChildAnalyze(home, envName, plan, mm string, progress func(string)) error {
	return ChildAnalyzeExeOf("", home, envName, plan, mm, false, progress)
}

// ChildAnalyzeExe is ChildAnalyze with an explicit archo executable. plan is the list a
// session opened ("" the current one, 07 §1.3).
func ChildAnalyzeExe(exe, home, envName, plan, mm string, progress func(string)) error {
	return ChildAnalyzeExeOf(exe, home, envName, plan, mm, false, progress)
}

// ChildAnalyzeExeOf is ChildAnalyzeExe; background starts an automatic check's child
// (`analyze --background`, SetAnalyzeBackground). exe "" is this executable.
func ChildAnalyzeExeOf(exe, home, envName, plan, mm string, background bool, progress func(string)) error {
	return ChildAnalyzeClocked(exe, home, envName, plan, mm, background, nil, progress)
}

// ChildAnalyzeClocked is ChildAnalyzeExeOf that also records on clock (nil: none) when the
// child started and exited, for the slow-check report (16 §2.14).
func ChildAnalyzeClocked(exe, home, envName, plan, mm string, background bool, clock *CheckClock, progress func(string)) error {
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return err
		}
		if b := strings.ToLower(filepath.Base(exe)); strings.HasSuffix(b, ".test") || strings.HasSuffix(b, ".test.exe") {
			// A test binary would run its tests again instead of `analyze`, and those could
			// start it again: tests set App.AnalyzeRunner (testkit uses InProcessAnalyze).
			return errors.New("child analysis from a test binary: set App.AnalyzeRunner")
		}
	}
	args := []string{"analyze"}
	if background {
		args = append(args, "--background")
	}
	if envName != "" {
		args = append(args, "--env", envName)
	}
	if plan != "" {
		args = append(args, "--open-plan", plan)
	}
	if mm != "" {
		args = append(args, "--game-version", mm)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = ChildEnv(home)
	hideWindow(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	if clock != nil {
		clock.Started = time.Now()
	}
	var last string
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		last = sc.Text()
		if progress != nil {
			progress(strings.TrimSpace(last))
		}
	}
	err = cmd.Wait()
	if clock != nil {
		clock.Exited = time.Now()
	}
	if err != nil {
		return fmt.Errorf("archo analyze: %v (%s)", err, last)
	}
	return nil
}

// InProcessAnalyze is an App.AnalyzeRunner that runs the analysis in this process (tests),
// on a session of its own like the `archo analyze` child: never on from itself, which the
// caller's page or request may use at the same time.
func InProcessAnalyze(from *App, progress func(string)) error {
	r, err := from.ReadSession()
	if err != nil {
		return err
	}
	defer r.Close()
	_, err = r.Analyze(progress)
	return err
}

// AnalyzeFrom is a copy of a's options for an AnalyzeRunner on another goroutine, with
// envName pinned: the runner reads it there while a goes on changing (another list opened,
// another Game Config), as the child's arguments are fixed when it starts.
func (a *App) AnalyzeFrom(envName string) *App {
	b := &App{Store: a.Store}
	b.ReadOptions(a)
	b.OpenPlan, b.PlanName = 0, a.OpenPlanName()
	b.EnvName = envName
	b.AnalyzeBackground = a.AnalyzeBackground
	return b
}

// ErrStillComputing: the analysis did not finish within App.AnalyzeWait; it goes on in
// the background and a later call picks up the result.
var ErrStillComputing = errors.New("the Def/Patch/Harmony analysis is still computing in the background; call again in a few seconds")

type pendingAnalysis struct {
	key  string
	done chan error
}

// AnalysisRunning reports whether this session's background EnsureAnalysis still runs.
func (a *App) AnalysisRunning() bool {
	return a.analyzing != nil && len(a.analyzing.done) == 0
}

// EnsureAnalysis brings the stored analysis up to date (05 §3: on demand), using the
// configured runner — by default the `archo analyze` child process.
func (a *App) EnsureAnalysis(progress func(string)) (*AnalysisState, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	s, err := a.Analysis(st)
	if err != nil || s.Fresh {
		return s, err
	}
	run := a.AnalyzeRunner
	if run == nil {
		run = func(from *App, p func(string)) error {
			return ChildAnalyzeExeOf(from.AnalyzeExe, from.Home, from.EnvName, from.PlanName, from.GameVersion, from.AnalyzeBackground, p)
		}
	}
	// the options are read now: with AnalyzeWait the runner goes on in a goroutine, after
	// EnsureLogAnalysis may have put GameVersion back
	from := a.AnalyzeFrom(st.Ix.Env.Name)
	if a.AnalyzeWait <= 0 {
		if err := run(from, progress); err != nil {
			return s, err
		}
	} else {
		key := st.Ix.Env.ID + "|" + st.GameMM() + "|" + SetFingerprint(st.List)
		if a.analyzing == nil || a.analyzing.key != key {
			p := &pendingAnalysis{key: key, done: make(chan error, 1)}
			a.analyzing = p
			go func() { p.done <- run(from, nil) }()
		}
		select {
		case err := <-a.analyzing.done:
			a.analyzing = nil
			if err != nil {
				return s, err
			}
		case <-time.After(a.AnalyzeWait):
			return s, ErrStillComputing
		}
	}
	st, err = a.Loadout()
	if err != nil {
		return nil, err
	}
	return a.Analysis(st)
}

// SetAnalyzeLimits applies the analysis child's memory settings.
func SetAnalyzeLimits() { debug.SetMemoryLimit(AnalyzeMemoryLimit) }

// BackgroundWorkers is how many threads an automatic check uses (16 §8.9).
const BackgroundWorkers = 2

// SetAnalyzeBackground makes this process an automatic check's (`analyze --background`,
// 16 §8.9): below normal priority with EcoQoS on Windows (nice 10 elsewhere), at most
// BackgroundWorkers threads for Go and the file readers. The result is the same; it only
// takes longer while it stays out of the way of the game and the window.
func (a *App) SetAnalyzeBackground() {
	lowerPriority()
	runtime.GOMAXPROCS(BackgroundWorkers)
	a.AnalyzeWorkers = BackgroundWorkers
	a.AnalyzeBackground = true // its check row says automatic
}
