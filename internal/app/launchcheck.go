package app

import (
	"fmt"
	"slices"

	"github.com/ToumanLin/RimArcho/internal/finding"
)

// LaunchCheck is one row of the pre-launch list (04 §8): a problem code whose mods were error
// sources in the game's log much more often than chance in the log corpus (decisions-corpus
// §3, G4/G8). The list never blocks a launch; it says what is likely to go wrong and what to do.
//
// To add a code (for example a new missing-dependency finding), add a row here: the launch
// output, the TUI and GUI previews (which show `launch --dry-run`) pick it up.
type LaunchCheck struct {
	Code string
	// Why is one plain line: what tends to go wrong, with the corpus measurement.
	Why string
	// Many describes several problems of the code: n problems over mods subject mods.
	Many func(n, mods int) string
	// NoInfo leaves out the code's info-level problems (the weaker corpus observations).
	NoInfo bool
}

// LaunchChecks in the order they are shown: most predictive first (observed / chance).
var LaunchChecks = []LaunchCheck{
	{Code: "patch.no-match",
		Why: "patches that find nothing usually fail with red errors: such mods were error sources in 37% of logs (4.5% by chance)",
		Many: func(n, mods int) string {
			return fmt.Sprintf("%s in %s find nothing", count(n, "patch", "patches"), count(mods, "mod", "mods"))
		}},
	{Code: "def.parent-missing",
		Why: "a definition built on a missing parent does not load: such mods were error sources in 16% of logs (4.1% by chance)",
		Many: func(n, mods int) string {
			return fmt.Sprintf("%s in %s build on a parent no enabled mod has", count(n, "definition", "definitions"), count(mods, "mod", "mods"))
		}},
	{Code: "incompat.active",
		Why: "their authors say these mods do not work together: they were error sources in 26% of logs (7.3% by chance)",
		Many: func(n, _ int) string {
			return fmt.Sprintf("%s of incompatible mods are enabled", count(n, "pair", "pairs"))
		}},
	{Code: "dep.inactive",
		Why: "a mod without its dependency fails in parts: such mods were error sources in 18% of logs (7.5% by chance)",
		Many: func(n, mods int) string {
			return fmt.Sprintf("%d dependencies of %s are installed but not enabled", n, count(mods, "mod", "mods"))
		}},
	{Code: "dep.missing",
		Why: "a mod without its dependency fails in parts: such mods were error sources in 10% of logs (4.4% by chance)",
		Many: func(n, mods int) string {
			return fmt.Sprintf("%d dependencies of %s are not installed", n, count(mods, "mod", "mods"))
		}},
	// G10 (decisions-corpus §2): learned from shared logs, warning level only. When the next
	// upload changed the mods involved, the error was gone in every case (8/8 and 48/48).
	{Code: "dep.observed-missing", NoInfo: true,
		Why: "the mod uses content from a mod that is not enabled, without declaring it: in shared logs it then errored in at least half of the cases",
		Many: func(n, mods int) string {
			return fmt.Sprintf("%s use content from %s that are not enabled", count(mods, "mod", "mods"), count(n, "mod", "mods"))
		}},
	{Code: "combo.known-bad", NoInfo: true,
		Why: "in shared logs this mod errored in at least half of the lists with the other mod (or this version), far less without it",
		Many: func(n, _ int) string {
			return fmt.Sprintf("%s of mods or versions are known to error", count(n, "combination", "combinations"))
		}},
}

// LaunchWarning is the problems of one LaunchCheck code in the enabled list.
type LaunchWarning struct {
	Check    *LaunchCheck
	Problems []*Problem // not ignored, in problem order
	Mods     []string   // distinct subject instances, in problem order
}

// LaunchWarnings returns the current problems of the LaunchChecks codes, in table order. It
// uses the stored analysis as it is (a launch does not wait for a new one): the returned
// state says whether the Def and Patch layers are current.
func (a *App) LaunchWarnings() ([]*LaunchWarning, *AnalysisState, error) {
	ps, err := a.Problems(false, nil)
	if err != nil {
		return nil, nil, err
	}
	by := map[string]*LaunchWarning{}
	var out []*LaunchWarning
	for i := range LaunchChecks {
		w := &LaunchWarning{Check: &LaunchChecks[i]}
		by[w.Check.Code] = w
		out = append(out, w)
	}
	for _, p := range ps.List {
		w := by[p.Code]
		if w == nil || p.Log != nil || (w.Check.NoInfo && p.Severity == finding.Info) {
			continue
		}
		w.Problems = append(w.Problems, p)
		if id := p.SubjectInstance(); id != "" && !slices.Contains(w.Mods, id) {
			w.Mods = append(w.Mods, id)
		}
	}
	kept := out[:0]
	for _, w := range out {
		if len(w.Problems) > 0 {
			kept = append(kept, w)
		}
	}
	return kept, ps.Analysis, nil
}

// count is "1 patch", "2 patches".
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
