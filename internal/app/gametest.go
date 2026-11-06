package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/gametest"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/patterns"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// TestRunOptions selects a plan to run without writing the player's files (20 §2).
type TestRunOptions struct {
	Env, Plan string
	Map       bool
	// Probe: a probe Mod folder to add to the tested list (map mode implied, 20 §3);
	// ProbeAfter or ProbeBefore name the packageId it goes next to (default: last).
	Probe, ProbeAfter, ProbeBefore string
	Timeout, Settle                time.Duration
	Progress                       func(string)
}

type TestRunMissing struct {
	PackageID string `json:"package_id"`
	Name      string `json:"name"`
	Reason    string `json:"reason,omitempty"` // why it could not be linked for the run
}

type TestRunCluster struct {
	key        string
	Class      string   `json:"class"`
	Message    string   `json:"message"`
	Count      int      `json:"count"`
	Mods       []string `json:"mods"`
	Confidence string   `json:"confidence"`
	Delta      string   `json:"delta,omitempty"`  // against the previous run: new / present
	Before     string   `json:"before,omitempty"` // in the player's log: present / absent
	// Note: "common kind, no source found: …" for a cluster no mod is named for whose kind is in more
	// than 5% of public logs (patterns.CommonUnattributed, 06 §8).
	Note string `json:"note,omitempty"`
	// Consequences: clusters that follow from this one's failed def file, folded into it and
	// not listed (06 §3); they still count in counts and clusters_total.
	Consequences int `json:"consequences,omitempty"`
}

type TestRunReport struct {
	Links         []*GameLink      `json:"links,omitempty"`
	Env           string           `json:"env"`
	Plan          string           `json:"plan"`
	GameVersion   string           `json:"game_version"`
	Status        string           `json:"status"`
	LoadSeconds   float64          `json:"load_seconds"`
	TotalSeconds  float64          `json:"total_seconds"`
	PeakMemoryMB  int64            `json:"peak_memory_mb"`
	Requested     int              `json:"requested"`
	Missing       []TestRunMissing `json:"missing"`
	Linked        []string         `json:"linked,omitempty"` // Mods/ links made for this run and removed after it
	RunDir        string           `json:"run_dir"`
	LogPath       string           `json:"log_path"`
	Args          []string         `json:"args,omitempty"` // saved in report.json (map mode of a baseline); not in --json output
	Attribution   string           `json:"attribution"`
	AnalysisError string           `json:"analysis_error,omitempty"` // attributed without a fresh analysis
	Counts        map[string]int   `json:"counts"`                   // clusters per class, before the limit
	Clusters      []TestRunCluster `json:"clusters"`
	// Info: the informational clusters (mod reports, other, known noise), one line each with
	// before, so an agent need not reread the log for them (MCP review round 2 P24).
	Info          []TestRunCluster  `json:"info,omitempty"`
	ClustersTotal int               `json:"clusters_total"`
	PlayerLog     *TestRunPlayerLog `json:"player_log,omitempty"`
	Fixed         []TestRunCluster  `json:"fixed,omitempty"`
	Baseline      *TestRunBaseline  `json:"baseline,omitempty"`
	Gone          []TestRunCluster  `json:"gone,omitempty"`
	Probe         *TestRunProbe     `json:"probe,omitempty"` // a probe run's lines and result (20 §3)
	clusterKeys   []string
	// playerClusters: the player's log's error clusters by key, attributed (comparePlayerLog)
	playerClusters map[string]TestRunCluster
}

// Success is the CLI/MCP outcome: the target was reached without error clusters. A probe
// run's target is its RESULT line (20 §3).
func (r *TestRunReport) Success() bool {
	if !r.reached() || (r.Status == "probe" && (r.Probe == nil || r.Probe.Outcome != "result")) {
		return false
	}
	for class, n := range r.Counts {
		if n > 0 && !logs.Informational(class) {
			return false
		}
	}
	return true
}

// reached: the run got to its target: the menu, a quiet map, or a probe's last line.
func (r *TestRunReport) reached() bool {
	return r.Status == "menu" || r.Status == "map" || r.Status == "probe"
}

// TestRun starts the executable directly, including for a Steam-launch environment.
// The current plan and the player's config, saves and logs are left alone (20 §2).
func (a *App) TestRun(ctx context.Context, o TestRunOptions) (_ *TestRunReport, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if o.Timeout < 0 || o.Settle < 0 {
		return nil, fmt.Errorf("test run: durations must not be negative")
	}
	prevEnv := a.EnvName
	if o.Env != "" {
		a.EnvName = o.Env
		defer func() { a.EnvName = prevEnv }()
	}
	if err := a.checkNoGame(); err != nil {
		return nil, err
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	if o.Plan != "" {
		p, err := a.FindPlan(o.Plan)
		if err != nil {
			return nil, err
		}
		if st, err = a.PlanState(st, p); err != nil {
			return nil, err
		}
	}
	if st.Plan == nil {
		return nil, fmt.Errorf("test run: no current plan (archo plan list)")
	}
	e := st.Ix.Env
	links, err := a.CheckGameLinks(modsDir(st.Ix))
	if err != nil {
		return nil, err
	}
	if len(links) > 0 {
		return &TestRunReport{Env: e.Name, Plan: PlanRef(st.Plan), GameVersion: e.GameVersion, Status: "links-blocked", Links: links, Counts: map[string]int{}, Clusters: []TestRunCluster{}}, nil
	}
	base := filepath.Join(a.Home, "tests", e.ID)
	data := filepath.Join(base, "userdata")
	runs := filepath.Join(base, "runs")
	protected := []string{e.GameRoot, e.UserDataRoot, env.DefaultUserDataRoot()}
	for _, path := range []string{base, data, filepath.Join(data, "Config"), runs} {
		if err := testRunPath(a.Home, path, protected); err != nil {
			return nil, err
		}
	}
	if o.Progress != nil {
		o.Progress("preparing test data for " + PlanRef(st.Plan))
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, err
	}
	// Links an interrupted run left, then this run's: the plan's Mods the game cannot see.
	if err := a.removeTestLinks(base); err != nil {
		return nil, err
	}
	// After the old links are gone, so a probe linked by an interrupted run is linked anew.
	probe, err := a.checkProbe(st, o)
	if err != nil {
		return nil, err
	}
	mods, add, blocked := a.testRunLinks(st)
	if probe != nil && probe.link != nil {
		mods = modsDir(st.Ix)
		add = append(add, *probe.link)
	}
	unlink, err := a.addTestLinks(base, mods, add)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, unlink()) }() // a no-op once removed below
	if err := a.prepareTestConfig(st, data, protected, probe); err != nil {
		return nil, err
	}
	if probe != nil {
		if err := clearProbeShot(data); err != nil {
			return nil, err
		}
	}
	missing := testRunMissing(st, blocked)
	if err := os.MkdirAll(runs, 0o755); err != nil {
		return nil, err
	}
	// Reserve a timestamp without replacing an earlier run in the same second.
	stamp := a.Now().UTC()
	var runDir string
	for {
		runDir = filepath.Join(runs, stamp.Format("20060102-150405"))
		if err := os.Mkdir(runDir, 0o755); os.IsExist(err) {
			stamp = stamp.Add(time.Second)
			continue
		} else if err != nil {
			return nil, err
		}
		break
	}
	if o.Progress != nil {
		o.Progress("starting test run; waiting for the game to load")
	}
	raw, runErr := gametest.Run(ctx, gametest.Options{
		GameRoot: e.GameRoot, GameVersion: e.GameVersion, DataDir: data, RunDir: runDir,
		Map: o.Map, Probe: probe != nil, Timeout: o.Timeout, Settle: o.Settle, Start: a.TestGameStart,
	})
	// The game has exited (gametest.Run reaps it): take the links away before anything else.
	unlinkErr := unlink()
	if raw == nil {
		return nil, errors.Join(runErr, unlinkErr)
	}
	r := &TestRunReport{
		Env: e.Name, Plan: PlanRef(st.Plan), GameVersion: e.GameVersion, Status: raw.Status,
		LoadSeconds: raw.LoadSeconds, TotalSeconds: raw.TotalSeconds, PeakMemoryMB: raw.PeakMemoryMB,
		Requested: len(st.List), Missing: missing, Linked: linkNames(add), RunDir: runDir, LogPath: raw.LogPath,
		Args: raw.Args, Attribution: "tested-plan", Counts: map[string]int{}, Clusters: []TestRunCluster{},
	}
	var probeErr error
	if probe != nil {
		r.Probe = probe.report
		probeErr = finishProbe(r.Probe, raw.LogPath, data, runDir)
	}
	// Attribution reads assembly types and Harmony IDs from the analysis (06 §2); a
	// test run is often the first command in a fresh environment. After the game exits,
	// so the measured load is not slowed by it.
	if err := a.ensureTestAnalysis(ctx, st, o.Progress); err != nil {
		r.AnalysisError = err.Error()
	}
	if o.Progress != nil {
		o.Progress("analysing test log")
	}
	analysisErr := a.testRunLog(st, r)
	if o.Progress != nil {
		o.Progress("comparing with the player's log")
	}
	a.comparePlayerLog(st, r)
	compareTestRun(r)
	b, saveErr := json.MarshalIndent(r, "", "  ")
	if saveErr == nil {
		saveErr = os.WriteFile(filepath.Join(runDir, "report.json"), append(b, '\n'), 0o644)
	}
	pruneErr := pruneTestRuns(runs, runDir)
	return r, errors.Join(runErr, unlinkErr, probeErr, analysisErr, saveErr, pruneErr)
}

func (a *App) prepareTestConfig(st *LoadoutState, data string, protected []string, probe *probeSetup) error {
	cfg := filepath.Join(data, "Config")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		return err
	}
	if root := st.Ix.Env.UserDataRoot; root != "" {
		files, err := os.ReadDir(filepath.Join(root, "Config"))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		for _, f := range files {
			if f.IsDir() || !strings.EqualFold(filepath.Ext(f.Name()), ".xml") ||
				strings.EqualFold(f.Name(), "ModsConfig.xml") || strings.EqualFold(f.Name(), "Knowledge.xml") || gametest.PreferenceFile(f.Name()) {
				continue
			}
			dst := filepath.Join(cfg, f.Name())
			if err := testRunPath(data, dst, protected); err != nil {
				return err
			}
			b, err := os.ReadFile(filepath.Join(root, "Config", f.Name()))
			if err != nil {
				return err
			}
			if err := os.WriteFile(dst, b, 0o644); err != nil {
				return err
			}
		}
		// Settings follow the Mod (09 §3): what a launch would copy, written here only.
		job := a.listSettings(st, testRunSeen(st), filepath.Join(root, "Config"))
		job.write = cfg
		for _, c := range carryModSettings(job, func(path string) error { return testRunPath(data, path, protected) }) {
			if c.Err != "" {
				return fmt.Errorf("test run: copying %s's settings: %s", c.Mod, c.Err)
			}
		}
	}
	var known []string
	for _, m := range st.Ix.Mods {
		if m.Kind == source.Official && m.PackageID != "ludeon.rimworld" && m.Status == identity.Live {
			known = append(known, m.PackageID)
		}
	}
	tokens := probe.probeTokens(testRunTokens(st))
	b, err := loadout.Render(nil, st.Ix.Env.GameVersion, tokens, known)
	if err != nil {
		return err
	}
	path := filepath.Join(cfg, "ModsConfig.xml")
	if err := testRunPath(data, path, protected); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// shortMessage is a message's first line cut to 100 characters, for the lists whose entries
// only identify a cluster (info, fixed, gone; MCP review round 2 P9).
func shortMessage(msg string) string {
	s := firstLine(msg)
	if r := []rune(s); len(r) > 100 {
		s = string(r[:97]) + "..."
	}
	return s
}

func linkNames(cs []LinkChange) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Name)
	}
	slices.Sort(out)
	return out
}

// testRunMissing lists the plan's Mods this run's game cannot see, after its links are made.
func testRunMissing(st *LoadoutState, blocked map[string]string) []TestRunMissing {
	seen := testRunSeen(st)
	missing := []TestRunMissing{}
	for _, entry := range st.List {
		m := st.Ix.ByID[entry.Instance]
		if m == nil {
			missing = append(missing, TestRunMissing{PackageID: entry.Missing, Name: entry.Missing})
			continue
		}
		if m.Kind != source.Official && seen[m.ID] == "" {
			missing = append(missing, TestRunMissing{PackageID: m.PackageID, Name: m.DisplayName(), Reason: blocked[m.ID]})
		}
	}
	return missing
}

// testRunSeen maps each listed Mod a test run's game sees to the folder it sees it under.
// It is the Mod's folder in Data/, Mods/ or the Workshop, or the name of a link in Mods/
// to it (including the links a test run makes, 20 §2).
func testRunSeen(st *LoadoutState) map[string]string {
	parents, linked := map[string]bool{}, map[string]string{}
	roots := []string{filepath.Join(st.Ix.Env.GameRoot, "Data"), filepath.Join(st.Ix.Env.GameRoot, "Mods")}
	for _, s := range st.Ix.Sources {
		if s.Kind == source.Workshop && s.Status != source.StatusRetired {
			roots = append(roots, s.Root)
		}
	}
	for _, root := range roots {
		if real, err := source.RealPath(root); err == nil {
			parents[env.PathKey(real)] = true
		}
	}
	modsDir := filepath.Join(st.Ix.Env.GameRoot, "Mods")
	files, _ := os.ReadDir(modsDir)
	for _, f := range files {
		path := filepath.Join(modsDir, f.Name())
		if !source.IsLink(path) {
			continue
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			if real, err := source.RealPath(path); err == nil {
				linked[env.PathKey(real)] = f.Name()
			}
		}
	}
	seen := map[string]string{}
	for _, entry := range st.List {
		m := st.Ix.ByID[entry.Instance]
		if m == nil || m.Kind == source.Official {
			continue
		}
		real, err := source.RealPath(st.Ix.ModDir(m))
		if err != nil {
			continue
		}
		if parents[env.PathKey(filepath.Dir(real))] {
			seen[m.ID] = filepath.Base(real)
		} else if name := linked[env.PathKey(real)]; name != "" {
			seen[m.ID] = name
		}
	}
	return seen
}

func (a *App) testRunLog(st *LoadoutState, r *TestRunReport) error {
	fi, err := os.Stat(r.LogPath)
	if err != nil {
		return err
	}
	lf, _, err := readLog(r.LogPath, fi.Size())
	if err != nil {
		return err
	}
	// logState can reload the current plan; this run always uses the tested list.
	copy := *st
	copy.MM = env.MajorMinor(st.Ix.Env.GameVersion)
	prevMM := a.Store.MM
	defer func() { a.Store.MM = prevMM }()
	a.Store.MM = copy.GameMM()
	fails, err := analyze.ParseLogFailures(r.LogPath)
	if err != nil {
		return err
	}
	rep := &LogReport{Path: r.LogPath, Version: lf.Version, Lines: lf.Lines}
	if err := a.attribute(&copy, rep, lf, fails, nil); err != nil {
		return err
	}
	for _, lc := range rep.Clusters {
		class := lc.Class
		settings := gametest.SettingsError(lc.First)
		if settings {
			class = gametest.ClassSettings
		}
		// A probe's own messages are in probe.lines; as mod reports they would only repeat them (20 §3).
		if r.Probe != nil && logs.Informational(class) && strings.HasPrefix(lc.First.Message, "[Probe] ") {
			continue
		}
		r.Counts[class]++
		if logs.Informational(class) {
			if len(r.Info) < testRunListLimit {
				r.Info = append(r.Info, TestRunCluster{key: lc.Key, Class: class, Message: shortMessage(lc.First.Message), Count: lc.Count})
			}
			continue
		}
		r.ClustersTotal++
		r.clusterKeys = append(r.clusterKeys, lc.Key)
		if len(r.Clusters) == 30 || lc.ConsequenceOf != "" {
			continue
		}
		mods := []string{}
		confidence := lc.Confidence
		if settings {
			confidence = finding.Certain
		} else {
			mods = testRunMods(st, lc)
		}
		note := ""
		if !settings {
			note = patterns.Load().CommonUnattributed(class, lc.Template, len(lc.Attr) > 0)
		}
		if slices.Contains(lc.Details, ownArtNote) {
			note = ownArtNote
		}
		r.Clusters = append(r.Clusters, TestRunCluster{key: lc.Key, Class: class, Message: firstLine(lc.First.Message), Count: lc.Count, Mods: mods, Confidence: confidence, Note: note, Consequences: lc.Consequences})
	}
	return nil
}

// testRunMods names a cluster's candidate mods in attribution order.
func testRunMods(st *LoadoutState, lc *LogCluster) []string {
	mods := []string{}
	for _, at := range lc.Attr {
		if m := st.Ix.ByID[at.Instance]; m != nil && !slices.Contains(mods, m.DisplayName()) {
			mods = append(mods, m.DisplayName())
		}
	}
	return mods
}

// ensureTestAnalysis brings the tested plan's Def/Patch/Harmony analysis up to date,
// waiting without the MCP bound (14 §1): the test run's report depends on it.
func (a *App) ensureTestAnalysis(ctx context.Context, st *LoadoutState, progress func(string)) error {
	if p := a.analyzing; p != nil { // a background analysis from an earlier call
		select {
		case <-p.done:
		case <-ctx.Done():
			return ctx.Err()
		}
		a.analyzing = nil
	}
	if s, err := a.analysisOf(st, false); err == nil && s.Fresh {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if progress != nil {
		progress("analysing the plan's mods for attribution")
	}
	open, name := a.OpenPlan, a.PlanName
	a.OpenPlan, a.PlanName = st.Plan.ID, ""
	defer func() { a.OpenPlan, a.PlanName = open, name }()
	if a.AnalyzeRunner != nil {
		return a.AnalyzeRunner(st.Ix.Env.Name, progress)
	}
	if a.AnalyzeExe != "" {
		return ChildAnalyzeExe(a.AnalyzeExe, a.Home, st.Ix.Env.Name, st.Plan.Name, "", progress)
	}
	return ChildAnalyze(a.Home, st.Ix.Env.Name, st.Plan.Name, "", progress)
}

func testRunPath(root, path string, protected []string) error {
	root, path = directoryReal(realOrAbs(root)), directoryReal(realOrAbs(path))
	if !source.Within(path, root) {
		return fmt.Errorf("test run: path %q must stay inside %q", path, root)
	}
	for _, dir := range protected {
		if dir != "" && source.Within(path, directoryReal(realOrAbs(dir))) {
			return fmt.Errorf("test run: isolated path %q must be outside %q", path, dir)
		}
	}
	return nil
}

func pruneTestRuns(root, current string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() || source.IsLink(filepath.Join(root, entry.Name())) {
			continue
		}
		if _, err := time.Parse("20060102-150405", entry.Name()); err == nil {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	slices.Reverse(names)
	kept := 1 // The active run is kept even if the clock moved backwards.
	for _, name := range names {
		path := filepath.Join(root, name)
		if path == current {
			continue
		}
		if kept < 10 {
			kept++
			continue
		}
		if err := testRunPath(root, path, nil); err != nil {
			return err
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}
