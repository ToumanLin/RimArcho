package app

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/oplog"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
)

// The operation log (02 §1, 04 §4, 13 §1.3). The App owns one *oplog.Log (Ops); the
// primitives that change the player's files write it, never the surfaces. A nil Ops does
// nothing.

// SetOrigin sets this process's origin (gui, tui, cli, mcp): the surfaces call it once in
// main. An origin inherited from a parent process (oplog.EnvVar) wins, so the scans and
// helpers the window starts write gui rows and Archo's MCP server writes agent rows; the
// variable is then set for this process's own children.
func (a *App) SetOrigin(origin string) {
	origin = oplog.OriginFromEnv(origin)
	a.Ops.SetOrigin(origin)
	os.Setenv(oplog.EnvVar, origin)
}

// PruneOps trims the log to its retention (oplog.MaxAge, oplog.MaxRows). The GUI and TUI
// call it in the background once the first page has drawn.
func (a *App) PruneOps() {
	if n, err := a.Ops.Prune(); err != nil {
		log.Printf("operation log: prune: %v", err)
	} else if n > 0 {
		log.Printf("operation log: pruned %d rows", n)
	}
}

// Operations reads the log for `archo ops` and the lists (13 §1.3).
func (a *App) Operations(q oplog.Query) ([]oplog.Row, error) {
	if a.Ops == nil {
		return nil, errors.New("this RimArcho has no operation log")
	}
	return a.Ops.Recent(q)
}

// opResult is the result a primitive records for err: refused when the action was turned
// down before anything changed, failed otherwise.
func opResult(err error, refused bool) string {
	switch {
	case err == nil:
		return oplog.OK
	case errors.Is(err, errPartial):
		return oplog.Partial
	case refused:
		return oplog.Refused
	}
	return oplog.Failed
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// opEnv is the current environment's id for a row ("" when there is none).
func (a *App) opEnv() string {
	if a.Ops == nil || a.Store == nil {
		return ""
	}
	if a.EnvName != "" {
		if e, err := a.Store.EnvByName(a.EnvName); err == nil && e != nil {
			return e.ID
		}
		return ""
	}
	if e, err := a.Store.CurrentEnv(); err == nil && e != nil {
		return e.ID
	}
	return ""
}

// trashSubject names what a recycled path held: its folder or file name.
func trashSubject(path string) string { return filepath.Base(filepath.Clean(path)) }

// logTrash records one recycle-bin move.
func (a *App) logTrash(h *oplog.Handle, path string, err error, refused bool) {
	r := oplog.Row{Op: oplog.Trash, Subject: trashSubject(path), Target: path, Result: opResult(err, refused), Detail: errText(err)}
	a.opAdd(h, r)
}

// logUnsubscribe records a Workshop unsubscribe, one row per item: the Steam result from rep,
// or err (refused before Steam was asked, or the call failed). why marks the unsubscribe
// half of a resubscribe.
func (a *App) logUnsubscribe(items []WorkshopItem, rep *steamworks.Report, err error, refused bool, why string) {
	by := map[string]string{}
	if rep != nil {
		for _, x := range rep.Items {
			by[strconv.FormatUint(x.ID, 10)] = x.Result
		}
	}
	env := a.opEnv()
	for _, it := range items {
		r := oplog.Row{Op: oplog.Unsubscribe, EnvID: env, Subject: it.Name, Target: "workshop:" + it.WorkshopID, Result: opResult(err, refused), Detail: errText(err)}
		if rep != nil {
			switch res, ok := by[it.WorkshopID]; {
			case !ok:
				r.Result, r.Detail = oplog.Failed, "Steam did not answer (timeout)"
			case res != "ok":
				r.Result, r.Detail = oplog.Failed, "Steam: "+res
			}
		}
		r.Detail = joinDetail(why, r.Detail)
		a.Ops.Add(r)
	}
}

// joinDetail joins the non-empty parts with "; ".
func joinDetail(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "; ")
}

// opAdd writes r as a child of h, or as a row of its own when h is nil.
func (a *App) opAdd(h *oplog.Handle, r oplog.Row) int64 {
	if h != nil {
		return h.Child(r)
	}
	return a.Ops.Add(r)
}

// opRun is one action's row while it runs (launch, uninstall, update): refused until it
// began changing something, then failed or ok. A nil opRun (a dry run) records nothing.
type opRun struct {
	a     *App
	row   oplog.Row
	h     *oplog.Handle
	began bool
	done  bool
}

// opStart starts the row of an action; dry runs are not logged (02 §1: a preview writes
// nothing).
func (a *App) opStart(dryRun bool, row func() oplog.Row) *opRun {
	if dryRun || a.Ops == nil {
		return nil
	}
	return &opRun{a: a, row: row()}
}

// parent writes the row now (result started), so the steps can hang under it.
func (o *opRun) parent() *oplog.Handle {
	if o == nil {
		return nil
	}
	if o.h == nil {
		o.h = o.a.Ops.Begin(o.row)
	}
	return o.h
}

// begin marks the point after which a failure is "failed", not "refused".
func (o *opRun) begin() {
	if o != nil {
		o.began = true
	}
}

// end records the outcome; the first end wins.
func (o *opRun) end(err error, detail string) {
	if o == nil || o.done {
		return
	}
	o.done = true
	result := opResult(err, !o.began)
	if !errors.Is(err, errPartial) {
		detail = joinDetail(detail, errText(err))
	}
	if o.h != nil {
		o.h.End(result, detail)
	} else {
		o.row.Result, o.row.Detail = result, detail
		o.a.Ops.Add(o.row)
	}
}

// logModsConfig records a ModsConfig.xml write (07 §4) with the version it made or kept, so
// the row points to the plan's history.
func (a *App) logModsConfig(st *LoadoutState, r *SaveResult, err error, credit bool) {
	if st == nil || st.op == nil {
		return
	}
	row := oplog.Row{Op: oplog.ModsConfigWrite, Target: st.FilePath, Result: opResult(err, !st.op.wrote), Detail: errText(err)}
	if st.Ix != nil && st.Ix.Env != nil {
		row.EnvID = st.Ix.Env.ID
	}
	if st.Plan != nil {
		row.Subject = st.Plan.Name
	}
	if r != nil && err == nil {
		n, how := r.Version, r.Source
		if r.Unchanged && st.Latest != nil {
			n, how = st.Latest.N, "unchanged list"
		}
		if credit {
			how = "credit handoff"
		}
		row.Detail = fmt.Sprintf("v%d, %d mods, %s", n, len(st.List), how)
	}
	a.opAdd(st.op.h, row)
}

// applyLinks journals deployment links (09 §3); applyLinksIn is applyLinks whose rows
// belong to the parent row h. The log gets counts: one link.remove and one link.create row
// for what was done, and a failed row of its own for the link that failed.
func (a *App) applyLinksIn(h *oplog.Handle, mods string, changes []LinkChange) error {
	done := map[string]int{}
	err := a.applyLinkChanges(mods, changes, func(c LinkChange, err error) {
		op := oplog.LinkCreate
		if c.Op == "remove" {
			op = oplog.LinkRemove
		}
		if err == nil {
			done[op]++
			return
		}
		a.opAdd(h, oplog.Row{Op: op, Subject: c.Name, Target: filepath.Join(mods, c.Name), Result: oplog.Failed, Detail: err.Error()})
	})
	game := filepath.Base(filepath.Dir(mods))
	for _, op := range []string{oplog.LinkRemove, oplog.LinkCreate} {
		if n := done[op]; n > 0 {
			a.opAdd(h, oplog.Row{Op: op, Subject: game, Target: mods, Detail: fmt.Sprintf("%d links", n)})
		}
	}
	return err
}

// launchDetail is a started launch's method (09 §2).
func launchDetail(r *SaveResult) string {
	if r == nil || r.Launch == nil {
		return ""
	}
	return "method " + r.Launch.Method
}

// saveTo is a renamed or copied save's new file, for the detail.
func saveTo(to string) string {
	if to == "" {
		return ""
	}
	return "to " + filepath.Base(to)
}

// batchErr is a batch of saves' outcome: nil when every save went, an error when some or
// none did (opRun.end turns "some" into partial through its detail).
func batchErr(r *SaveFilesResult, n int) error {
	if r == nil || len(r.Errs) == 0 {
		return nil
	}
	if len(r.Saves) > 0 && len(r.Saves) < n {
		return errPartial
	}
	return errors.New(r.Errs[0])
}

// errPartial marks a batch where some items went and some did not.
var errPartial = errors.New("partly done")

// logRollback records putting the old program files back after a new release did not start
// (17 §5.1): ok when they are back, failed when the next start has to try again.
func (a *App) logRollback(program, back, failed string, why, err error) {
	subject := "RimArcho " + back
	if back == "" {
		subject = "RimArcho"
	}
	a.Ops.Add(oplog.Row{Op: oplog.UpdateRollback, Subject: subject, Target: program, Result: opResult(err, false),
		Detail: joinDetail("RimArcho "+failed+" did not start", errText(why), errText(err))})
}

// logLinkRepair records a confirmed link repair (09 §3): the count of links removed, and the
// error that stopped it (refused when nothing was removed yet).
func (a *App) logLinkRepair(plan *LinkRepairPlan, err error) {
	if n := len(plan.Removed); n > 0 {
		a.Ops.Add(oplog.Row{Op: oplog.LinkRemove, Subject: "link repair", Detail: fmt.Sprintf("%d links", n)})
	}
	if err != nil {
		a.Ops.Add(oplog.Row{Op: oplog.LinkRemove, Subject: "link repair", Result: opResult(err, len(plan.Removed) == 0), Detail: err.Error()})
	}
}
