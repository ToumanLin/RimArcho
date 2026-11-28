package app

import (
	"encoding/json"
	"log"
	"runtime/metrics"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ToumanLin/RimArcho/internal/analyze"
)

// The slow-check report (16 §2.14 "Slow checks") needs more than a check row's one line
// (CheckPhases): which operations and DLLs were slowest, whether the check was a repeat,
// and when the check's process started and ended. The analysis writes CheckReport beside
// its row; the window or the terminal keeps a CheckClock while the child runs.

// checkReportMeta holds the newest check's CheckReport (JSON); each check replaces it.
const checkReportMeta = "check:report"

// CheckReport is what one check measured beyond its row's phases. Paths in it are relative
// to a Mod's folder.
type CheckReport struct {
	Row int64 `json:"row"` // the check row (oplog.Check) it belongs to
	// OpenMs: opening the data folder (app.OpenWith) before the row began, in the process
	// that ran the check
	OpenMs int64 `json:"open_ms"`
	// List: "new" (no earlier check of this list), "changed" (its Mods, versions or order
	// changed since the last check) or "same" (a repeat)
	List       string `json:"list"`
	ModsRead   int    `json:"mods_read"`   // Mods whose files were read
	ModsCached int    `json:"mods_cached"` // inactive Mods whose facts came from the cache
	Workers    int    `json:"workers"`
	GCMs       int64  `json:"gc_ms"` // CPU time of the garbage collector during the check
	PeakHeapMB int    `json:"peak_heap_mb"`

	Ops            int        `json:"ops"`
	Nodes          int        `json:"nodes"`
	TooSlow        int        `json:"too_slow"`
	PatchParseMs   int64      `json:"patch_parse_ms"`
	PatchSimMs     int64      `json:"patch_sim_ms"`
	SlowestOps     []CheckOp  `json:"slowest_ops,omitempty"`
	DLLs           int        `json:"dlls"`
	DLLsCached     int        `json:"dlls_cached"`
	HarmonyPatches int        `json:"harmony_patches"`
	HarmonyTargets int        `json:"harmony_targets"`
	SlowestDLLs    []CheckDLL `json:"slowest_dlls,omitempty"`
}

// CheckOp is one of the slowest patch operations.
type CheckOp struct {
	Us      int64  `json:"us"` // microseconds
	Pkg     string `json:"pkg"`
	WS      string `json:"ws,omitempty"`
	Class   string `json:"class"`
	XPath   string `json:"xpath,omitempty"`
	File    string `json:"file"`
	TooSlow bool   `json:"too_slow,omitempty"`
}

// CheckDLL is one of the slowest DLLs of the Harmony layer.
type CheckDLL struct {
	Us     int64  `json:"us"`
	Pkg    string `json:"pkg"`
	WS     string `json:"ws,omitempty"`
	File   string `json:"file"`
	Cached bool   `json:"cached,omitempty"`
}

// checkReport builds the report of a finished analysis; ws maps an instance to its
// Workshop id.
func checkReport(row int64, s analyze.Stats, ws func(string) string) CheckReport {
	r := CheckReport{Row: row, ModsRead: s.InstalledMods - s.CachedMods, ModsCached: s.CachedMods,
		PeakHeapMB: s.PeakHeapMB, Ops: s.TopOps, Nodes: s.Nodes, TooSlow: s.SlowOps,
		PatchParseMs: s.PatchParseMs, PatchSimMs: s.PatchSimMs, DLLs: s.Assemblies, DLLsCached: s.DLLsCached,
		HarmonyPatches: s.HarmonyPatches, HarmonyTargets: s.HarmonyTargets}
	for _, o := range s.SlowestOps {
		r.SlowestOps = append(r.SlowestOps, CheckOp{Us: o.Dur.Microseconds(), Pkg: o.Pkg, WS: ws(o.Instance),
			Class: o.Class, XPath: o.XPath, File: slashed(o.File), TooSlow: o.TooSlow})
	}
	for _, d := range s.SlowestDLLs {
		r.SlowestDLLs = append(r.SlowestDLLs, CheckDLL{Us: d.Dur.Microseconds(), Pkg: d.Pkg, WS: ws(d.Instance),
			File: slashed(d.File), Cached: d.Cached})
	}
	return r
}

func slashed(p string) string { return strings.ReplaceAll(p, `\`, "/") }

func (a *App) saveCheckReport(r CheckReport) {
	b, err := json.Marshal(r)
	if err == nil {
		err = a.Store.SetMeta(checkReportMeta, string(b))
	}
	if err != nil {
		log.Printf("check report: %v", err)
	}
}

// CheckReportOf is the report written with the check row id, if it is the newest.
func (a *App) CheckReportOf(row int64) (CheckReport, bool) {
	var r CheckReport
	if a.Store == nil || json.Unmarshal([]byte(a.Store.Meta(checkReportMeta)), &r) != nil || r.Row != row || row == 0 {
		return CheckReport{}, false
	}
	return r, true
}

// gcCPU is the garbage collector's CPU time in this process so far.
func gcCPU() time.Duration {
	s := []metrics.Sample{{Name: "/cpu/classes/gc/total:cpu-seconds"}}
	metrics.Read(s)
	if s[0].Value.Kind() != metrics.KindFloat64 {
		return 0
	}
	return time.Duration(s[0].Value.Float64() * float64(time.Second))
}

// CheckClock is when a check's child process started and exited as the window or the
// terminal saw it, and what they did after it (the report's spawn, boot, exit and later
// phases, 16 §2.14). A check run in the same process (App.AnalyzeRunner) has no Started.
type CheckClock struct {
	Began, Started, Exited time.Time
	First                  bool // the first check since this process started
	// After: what the caller did between Exited and the report, in order (Lap)
	After []CheckPhase
	last  time.Time
}

var checksBegun atomic.Int32

// NewCheckClock starts the clock of a check that begins now.
func NewCheckClock() *CheckClock {
	return &CheckClock{Began: time.Now(), First: checksBegun.Add(1) == 1}
}

// Lap records name as the time since Exited or the previous Lap. Nil-safe; it does nothing
// before the child exited.
func (c *CheckClock) Lap(name string) {
	if c == nil || c.Exited.IsZero() {
		return
	}
	now := time.Now()
	from := c.last
	if from.IsZero() {
		from = c.Exited
	}
	c.After = append(c.After, CheckPhase{Name: name, Ms: max(0, now.Sub(from).Milliseconds())})
	c.last = now
}
