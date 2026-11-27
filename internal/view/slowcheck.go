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

// SlowCheckOf is the prompt after a check that began at began and ended wall later, or nil
// (fast enough, not the window or the terminal, asked or sent within SlowCheckEvery, turned
// off, no relay, no check row). With feedback.timings always it sends at once, in the
// background, and returns nil. A prompt counts toward SlowCheckEvery when it is made.
func SlowCheckOf(a *app.App, began time.Time, wall time.Duration) *SlowCheck {
	if a == nil || a.Store == nil || wall < SlowCheckAfter {
		return nil
	}
	if o := a.Ops.Origin(); o != oplog.GUI && o != oplog.TUI {
		return nil
	}
	mode := cmds.Setting(a, "feedback.timings")
	if mode == "off" || feedback.TimingsURL() == "" || slowCheckRecent(a) {
		return nil
	}
	t, ok := slowCheckTimings(a, began, wall)
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
// process) that started with this check, and the machine facts RimArcho already knows.
func slowCheckTimings(a *app.App, began time.Time, wall time.Duration) (feedback.Timings, bool) {
	rows, err := a.Ops.Recent(oplog.Query{Op: oplog.Check, Since: began.Add(-2 * time.Second), Limit: 1})
	if err != nil || len(rows) == 0 || rows[0].Result != oplog.OK {
		return feedback.Timings{}, false
	}
	r := rows[0]
	p, ok := app.ParseCheckPhases(r.Detail)
	if !ok {
		return feedback.Timings{}, false
	}
	t := feedback.Timings{Version: buildinfo.Version, OS: feedback.System(), Game: p.Game, Active: p.Active, Total: p.Total,
		Automatic: p.Automatic, WallMs: wall.Milliseconds(), Phases: []feedback.TimingPhase{}, Threads: runtime.NumCPU()}
	for _, ph := range p.Phases {
		t.Phases = append(t.Phases, feedback.TimingPhase{Name: ph.Name, Ms: ph.Ms})
	}
	// starting the check's process, opening the database, and drawing the findings
	if rest := wall.Milliseconds() - r.Ms; r.Ms > 0 && rest > 0 {
		t.Phases = append(t.Phases, feedback.TimingPhase{Name: "other", Ms: rest})
	}
	if m := machine.Cached(); m.TotalRAM > 0 {
		t.RAMGB = int((m.TotalRAM + 1<<29) >> 30)
	}
	if st, err := a.LoadoutSnapshot(); err == nil && st != nil && st.Ix != nil && st.Ix.Env != nil {
		t.Disk = fsscan.DiskType(st.Ix.Env.GameRoot)
	}
	return t, true
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
