package view

import (
	"context"
	"errors"
	"log"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/feedback"
	"github.com/ToumanLin/RimArcho/internal/fsscan"
	"github.com/ToumanLin/RimArcho/internal/machine"
	"github.com/ToumanLin/RimArcho/internal/oplog"
)

// Slow checks (16 §2.14 "Slow checks", 16 §10): after a can-it-run check in the window or the
// terminal that took longer than SlowCheckAfter, one quiet prompt offers to send its timings
// (feedback.Timings) to the relay, which appends them to one fixed issue. Never from an agent
// or MCP, at most once every SlowCheckEvery (a prompt or a send), never without the player's
// yes unless feedback.timings is always.

// SlowCheckAfter is the wall time over which a check offers its timings. The environment
// variable RIMARCHO_SLOWCHECK_AFTER (a Go duration such as 0s) lowers it for the screenshot
// harness, so the prompt shows without a real 30-second check.
var SlowCheckAfter = func() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("RIMARCHO_SLOWCHECK_AFTER")); err == nil && d >= 0 {
		return d
	}
	return 30 * time.Second
}()

// SlowCheckEvery: at most one prompt or send in this time, per install.
const SlowCheckEvery = 24 * time.Hour

// slowCheckMeta holds when the last prompt was shown or timings were sent (Unix ms).
const slowCheckMeta = "feedback:timings_at"

// SlowCheck is the prompt.
type SlowCheck struct {
	Took    string `json:"took"`    // "48 s"
	Text    string `json:"text"`    // the one line, for screen readers
	Ask     string `json:"ask"`     // the line without the time, beside the time as a number
	Preview string `json:"preview"` // exactly what is sent (feedback.Timings.Body)
}

// SlowCheckOf is the prompt after the check clock timed, which ended wall after it began,
// or nil (fast enough, not the window or the terminal, asked or sent within
// SlowCheckEvery, turned off, no relay, no check row). With feedback.timings always it
// sends at once, in the background, and returns nil. A prompt counts toward
// SlowCheckEvery when it is made.
func SlowCheckOf(a *app.App, clock *app.CheckClock, wall time.Duration) *SlowCheck {
	if a == nil || a.Store == nil || clock == nil || wall < SlowCheckAfter {
		return nil
	}
	if o := a.Ops.Origin(); o != oplog.GUI && o != oplog.TUI {
		return nil
	}
	mode := cmds.Setting(a, "feedback.timings")
	if mode == "off" || feedback.TimingsURL() == "" || slowCheckRecent(a) {
		return nil
	}
	t, ok := slowCheckTimings(a, clock, wall)
	if !ok {
		return nil
	}
	body := t.Body()
	if mode == "always" {
		go SendSlowCheck(a, body)
		return nil
	}
	markSlowCheck(a)
	took := oplog.Dur(wall.Milliseconds())
	return &SlowCheck{Took: took, Text: Tf("This check took %s. Send the timings to help make it faster?", took),
		Ask: T("Send the timings to help make it faster?"), Preview: body}
}

func slowCheckRecent(a *app.App) bool {
	ms, err := strconv.ParseInt(a.Store.Meta(slowCheckMeta), 10, 64)
	return err == nil && a.Now().Sub(time.UnixMilli(ms)) < SlowCheckEvery
}

func markSlowCheck(a *app.App) {
	if err := a.Store.SetMeta(slowCheckMeta, strconv.FormatInt(a.Now().UnixMilli(), 10)); err != nil {
		log.Printf("slow check: %v", err)
	}
}

// slowCheckTimings reads the newest check row (written by the analysis, maybe in its child
// process) that started with this check, the report written with it (app.CheckReport),
// the clock, and the machine facts RimArcho already knows.
func slowCheckTimings(a *app.App, clock *app.CheckClock, wall time.Duration) (feedback.Timings, bool) {
	rows, err := a.Ops.Recent(oplog.Query{Op: oplog.Check, Since: clock.Began.Add(-2 * time.Second), Limit: 1})
	if err != nil || len(rows) == 0 || rows[0].Result != oplog.OK {
		return feedback.Timings{}, false
	}
	r := rows[0]
	p, ok := app.ParseCheckPhases(r.Detail)
	if !ok {
		return feedback.Timings{}, false
	}
	t := feedback.Timings{Version: buildinfo.Version, OS: feedback.System(), Game: p.Game, Active: p.Active, Total: p.Total,
		Automatic: p.Automatic, Priority: "normal", WallMs: wall.Milliseconds(), Phases: feedback.Phases{},
		CPU: machine.CPUName(), Threads: runtime.NumCPU()}
	if p.Automatic {
		t.Priority = "low"
	}
	rep, hasReport := a.CheckReportOf(r.ID)
	t.Phases = checkTimeline(clock, r, p, rep, wall)
	if hasReport {
		t.Run = &feedback.TimingRun{Cold: clock.First || rep.List != "same", First: clock.First, List: rep.List,
			ModsRead: rep.ModsRead, ModsCached: rep.ModsCached, Workers: rep.Workers, GCMs: rep.GCMs, PeakHeapMB: rep.PeakHeapMB}
		pt := &feedback.TimingPatches{Ops: rep.Ops, Nodes: rep.Nodes, TooSlow: rep.TooSlow, ParseMs: rep.PatchParseMs,
			SimMs: rep.PatchSimMs, FactsMs: max(0, p.Ms("patches")-rep.PatchParseMs-rep.PatchSimMs)}
		for _, o := range rep.SlowestOps {
			if o.Us < 50 { // rounds to 0.0 ms: says nothing
				continue
			}
			pt.Slowest = append(pt.Slowest, feedback.TimingOp{Ms: usMs(o.Us), Pkg: o.Pkg, WS: o.WS, Class: o.Class,
				XPath: o.XPath, File: o.File, TooSlow: o.TooSlow})
		}
		t.Patches = pt
		h := &feedback.TimingHarmony{DLLs: rep.DLLs, Cached: rep.DLLsCached, Patches: rep.HarmonyPatches, Methods: rep.HarmonyTargets}
		for _, d := range rep.SlowestDLLs {
			if d.Us < 50 {
				continue
			}
			h.Slowest = append(h.Slowest, feedback.TimingDLL{Ms: usMs(d.Us), Pkg: d.Pkg, WS: d.WS, File: d.File, Cached: d.Cached})
		}
		t.Harmony = h
	}
	if m := machine.Cached(); m.TotalRAM > 0 {
		t.RAMGB = int((m.TotalRAM + 1<<29) >> 30)
	}
	if st, err := a.LoadoutSnapshot(); err == nil && st != nil && st.Ix != nil && st.Ix.Env != nil {
		t.Disk = fsscan.DiskType(st.Ix.Env.GameRoot)
	}
	t.Local(localFolders(a)...)
	return t, true
}

// localFolders are the folders on this computer a Mod's text could name (its data folder,
// every game and user data folder, every Mod folder): Body removes them, also where no
// drive or home shows they are local (a Linux game under /opt).
func localFolders(a *app.App) []string {
	dirs := []string{a.Home}
	if es, err := a.Store.Envs(); err == nil {
		for _, e := range es {
			dirs = append(dirs, e.GameRoot, e.UserDataRoot)
		}
	}
	if ss, err := a.Store.AllSources(); err == nil {
		for _, s := range ss {
			dirs = append(dirs, s.Root)
		}
	}
	return dirs
}

// usMs is microseconds as milliseconds with one decimal.
func usMs(us int64) float64 { return float64((us+50)/100) / 10 }

// checkTimeline is the check's phases in order, tiling its wall time: with a child
// process, spawn (starting it), boot (until the check row began, less open: opening the
// data folder), the row's phases, exit (from the row's end to the child's exit), then what
// the window or the terminal did after it (clock.After); other is what is left, the
// rounding and what the row's phases do not name.
func checkTimeline(clock *app.CheckClock, r oplog.Row, p app.CheckPhases, rep app.CheckReport, wall time.Duration) feedback.Phases {
	var out feedback.Phases
	add := func(name string, ms int64) { out = append(out, feedback.TimingPhase{Name: name, Ms: max(0, ms)}) }
	child := !clock.Started.IsZero() && !clock.Exited.IsZero() && r.Ms > 0
	if child {
		add("spawn", clock.Started.Sub(clock.Began).Milliseconds())
		boot := r.At.Sub(clock.Started).Milliseconds()
		add("boot", boot-rep.OpenMs)
		add("open", min(boot, rep.OpenMs))
	}
	for _, ph := range p.Phases {
		add(ph.Name, ph.Ms)
	}
	if child {
		add("exit", clock.Exited.Sub(r.At.Add(time.Duration(r.Ms)*time.Millisecond)).Milliseconds())
	}
	for _, ph := range clock.After {
		add(ph.Name, ph.Ms)
	}
	var sum int64
	for _, ph := range out {
		sum += ph.Ms
	}
	if rest := wall.Milliseconds() - sum; rest > 0 && (child || r.Ms > 0) {
		add("other", rest)
	}
	return out
}

// SendSlowCheck sends body (a SlowCheck's Preview, unchanged) and counts it toward
// SlowCheckEvery. A relay without the timings route is not an error the player hears of.
func SendSlowCheck(a *app.App, body string) error {
	markSlowCheck(a)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := feedback.SendTimings(ctx, feedback.TimingsURL(), "RimArcho/"+buildinfo.Version, body)
	if errors.Is(err, feedback.ErrNoTimingsRoute) {
		log.Printf("slow check: the feedback relay has no timings route")
		return nil
	}
	if err != nil {
		log.Printf("slow check: %v", err)
	}
	return err
}

// AlwaysSendSlowChecks sets feedback.timings to always (the prompt's "always send").
func AlwaysSendSlowChecks(a *app.App) error {
	return a.Store.SetMeta("cfg:feedback.timings", "always")
}
