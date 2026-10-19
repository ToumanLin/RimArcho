package app

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/rimarcho/rimarcho/internal/analyze"
	"github.com/rimarcho/rimarcho/internal/store"
)

// AnalyzeMemoryLimit is the soft heap cap of the `archo analyze` child (01 §5.2 target:
// peak ≤ 400 MB). M3 measurement: 310 MB peak working set on 482 active mods.
const AnalyzeMemoryLimit = 300 << 20

// ModDir is where an instance's files are (links resolved).
func (ix *Index) ModDir(m *store.Mod) string {
	if m.RealPath != "" {
		return m.RealPath
	}
	if s := ix.Source(m.ID); s != nil {
		return filepath.Join(s.Root, m.Rel)
	}
	return m.Rel
}

// analysisMods: the enabled list in order, plus every other live, loadable instance
// (inactive definers, for no-match reasons). pos is 1-based; 0 = inactive.
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
		if pos[m.ID] == 0 && (m.Residue || m.Unloadable != "" || !st.Reg.Loadable(m.ID)) {
			continue
		}
		all = append(all, m)
	}
	return all, pos
}

// AnalysisKey identifies what an analysis depends on: analyzer version, game version, and
// every included instance with its snapshot and position. Cheap (no declarations loaded).
func (a *App) AnalysisKey(st *LoadoutState, mm string) string {
	if mm == "" {
		mm = st.GameMM()
	}
	all, pos := analysisMods(st)
	h := sha256.New()
	fmt.Fprintf(h, "v%d|%s\n", analyze.Version, mm)
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
// the `archo analyze` child does (03 §3); other callers go through EnsureAnalysis.
func (a *App) Analyze(progress func(string)) (*AnalyzeReport, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	in, key, err := a.AnalyzeInput(st, "")
	if err != nil {
		return nil, err
	}
	in.Progress = progress
	started := a.Now()
	out, err := analyze.Run(in)
	if err != nil {
		return nil, err
	}
	in = analyze.Input{} // release the declarations before writing
	id, err := a.Store.SaveAnalysis(st.Ix.Env.ID, key, st.GameMM(), out, started, a.Now())
	if err != nil {
		return nil, err
	}
	return &AnalyzeReport{ID: id, Key: key, MM: st.GameMM(), Stats: out.Stats}, nil
}

// AnalysisState says whether the stored analysis matches the current workspace.
type AnalysisState struct {
	Run   *store.Analysis // nil: never analysed
	Fresh bool
}

// Analysis returns the stored run and whether it is current.
func (a *App) Analysis(st *LoadoutState) (*AnalysisState, error) {
	r, err := a.Store.LatestAnalysis(st.Ix.Env.ID)
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
func ChildAnalyze(home, envName string, progress func(string)) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"analyze"}
	if envName != "" {
		args = append(args, "--env", envName)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "RIMARCHO_HOME="+home)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	var last string
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		last = sc.Text()
		if progress != nil {
			progress(strings.TrimSpace(last))
		}
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("archo analyze: %v (%s)", err, last)
	}
	return nil
}

// InProcessAnalyze runs the analysis in this process (tests, and `archo analyze` itself).
func InProcessAnalyze(a *App) func(string, func(string)) error {
	return func(_ string, progress func(string)) error {
		_, err := a.Analyze(progress)
		return err
	}
}

// EnsureAnalysis brings the stored analysis up to date (03 §3: on demand), using the
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
		run = func(envName string, p func(string)) error { return ChildAnalyze(a.Home, envName, p) }
	}
	if err := run(st.Ix.Env.Name, progress); err != nil {
		return s, err
	}
	st, err = a.Loadout()
	if err != nil {
		return nil, err
	}
	return a.Analysis(st)
}

// SetAnalyzeLimits applies the analysis child's memory settings.
func SetAnalyzeLimits() { debug.SetMemoryLimit(AnalyzeMemoryLimit) }
