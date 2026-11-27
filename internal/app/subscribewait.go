package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// After a subscription through the Steam client (11 §2.2): the task follows Steam's
// download until each item is installed, reads the Workshop folder again, and adds what
// arrived to the plan it was asked for. SteamCMD downloads are installed when they end.

// downloadKind classifies a failed watch result for the task's reason (16 §2.16).
func downloadKind(result string) string {
	switch result {
	case "failed:download-stalled":
		return "stalled"
	case "failed:download-limit":
		return "slow"
	}
	return ""
}

// waitInstalled follows the download of op's confirmed items (Result "ok") until Steam has
// installed them, with the stall and overall limits of steamworks.Watch. Items still not
// installed get a failure; a stop leaves them unfinished. When something was installed,
// the Workshop source is read again and the items take their instance.
func (a *App) waitInstalled(op *SubscribeOp, w *workshopTransfer) []error {
	var ids []string
	at := map[string]int{}
	for i, it := range op.Items {
		if it.Result == "ok" && it.Record != nil {
			ids = append(ids, it.Record.WorkshopID)
			at[it.Record.WorkshopID] = i
			// until Steam says otherwise; a stop turns it into "cancelled"
			op.Items[i].Result = "failed: " + stillDownloading
		}
	}
	if len(ids) == 0 {
		return nil
	}
	item := func(id string) *TaskItem {
		for i := range w.items {
			if w.items[i].Ref == "ws:"+id {
				return &w.items[i]
			}
		}
		return nil
	}
	for _, id := range ids {
		if t := item(id); t != nil {
			t.State, t.Steam, t.Got = ItemRun, true, 0
		}
	}
	w.publish()
	ctx, cancel := context.WithTimeout(w.ctx, steamworks.DownloadLimit+time.Minute)
	defer cancel()
	var last time.Time
	installed := 0
	_, err := a.runSteamStreaming(ctx, steamworks.Watch, ids, func(line string) {
		if p, ok := steamworks.ParseProgress(line); ok {
			if t := item(strconv.FormatUint(p.ID, 10)); t != nil && t.State == ItemRun {
				t.Got = p.Got
				if p.Total > 0 {
					t.Size = p.Total
				}
				if now := time.Now(); now.Sub(last) >= 500*time.Millisecond {
					last = now
					w.publish()
				}
			}
			return
		}
		for _, x := range steamworks.Parse(line).Items {
			id := strconv.FormatUint(x.ID, 10)
			i, ok := at[id]
			if !ok {
				continue
			}
			it, t := &op.Items[i], item(id)
			if x.Result == "ok" {
				it.Result = "ok"
				installed++
				if t != nil {
					t.State, t.Why, t.Got = ItemOK, "", t.Size
				}
			} else {
				it.Result = "failed: " + downloadFailure(x.Result).Error()
				it.FailureKind = downloadKind(x.Result)
				if t != nil {
					t.State, t.Why, t.FailureKind = ItemFail, strings.TrimPrefix(it.Result, "failed: "), it.FailureKind
				}
			}
			w.publish()
		}
	})
	if err != nil && w.ctx.Err() == nil {
		for _, id := range ids {
			if it := &op.Items[at[id]]; it.Result == "failed: "+stillDownloading {
				it.Result = "failed: " + err.Error()
			}
		}
	}
	var errs []error
	for _, id := range ids {
		if it := op.Items[at[id]]; it.Result != "ok" && w.ctx.Err() == nil {
			errs = append(errs, fmt.Errorf("Workshop %s: %s; %s", id, strings.TrimPrefix(it.Result, "failed: "), workshopURL(id)))
		}
	}
	if installed > 0 {
		if err := a.scanWorkshop(); err != nil {
			return append(errs, err)
		}
		a.takeInstances(op)
	}
	return errs
}

// stillDownloading is an item's result while its download is followed.
const stillDownloading = "Steam did not finish downloading it; check Steam"

// scanWorkshop reads the Workshop folders again, not the other sources (04 §1.1).
func (a *App) scanWorkshop() error {
	e, err := a.Env()
	if err != nil {
		return err
	}
	srcs, err := a.Store.Sources(e.GameID)
	if err != nil {
		return err
	}
	var ids []string
	for _, s := range srcs {
		if s.Kind == source.Workshop && s.Status != source.StatusRetired {
			ids = append(ids, s.ID)
		}
	}
	if len(ids) == 0 {
		_, err = a.Scan(false)
	} else {
		_, err = a.ScanWith(ScanOptions{Sources: ids})
	}
	return err
}

// takeInstances gives op's installed Workshop items their live instance.
func (a *App) takeInstances(op *SubscribeOp) {
	ix, err := a.Index()
	if err != nil {
		return
	}
	for i := range op.Items {
		it := &op.Items[i]
		if it.Record == nil || it.Result != "ok" {
			continue
		}
		for _, m := range ix.Mods {
			if m.Kind == source.Workshop && m.Status == identity.Live && m.WorkshopID() == it.Record.WorkshopID {
				it.ID, it.Ref, it.Folder = m.ID, ix.Ref(m.ID), ix.ModDir(m)
				break
			}
		}
	}
}

// addSubscribed adds the installed items ids of a subscription to the plan c.AddTo asked
// for (WorkshopCalls): in its collection's order with c.Collection, else where the sort
// constraints put them.
func (a *App) addSubscribed(op *SubscribeOp, c WorkshopCalls, ids []string) error {
	if len(c.Add) > 0 {
		keep := map[string]bool{}
		for _, id := range c.Add {
			keep[id] = true
		}
		var only []string
		for _, id := range ids {
			if keep[id] {
				only = append(only, id)
			}
		}
		ids = only
	}
	if len(ids) == 0 {
		return nil
	}
	var r *SyncResult
	var err error
	if c.Collection != "" {
		r, err = a.CatchUpDownloaded(c.AddTo, c.Collection, ids)
	} else {
		r, err = a.AddInstalled(c.AddTo, ids)
	}
	if err != nil {
		return fmt.Errorf("could not add them to %s: %w", c.AddTo, err)
	}
	op.AddedTo, op.Added = r.Plan.Name, r.Enabled
	return nil
}

// AddInstalled enables in plan planName the installed copies of the Workshop items ids that
// it does not hold in any copy, where the sort constraints put them (07 §2.1); a missing
// entry of the same package takes the copy in its place. One undoable edit, "add
// downloaded"; with nothing to add, nothing changes.
func (a *App) AddInstalled(planName string, ids []string) (*SyncResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	p, err := a.findPlan(planName)
	if err != nil {
		return nil, err
	}
	ps, err := a.PlanState(st, p)
	if err != nil {
		return nil, err
	}
	r := &SyncResult{Plan: p, Before: ps.List.Clone()}
	r.After = r.Before
	have := map[string]bool{}
	for _, e := range ps.List {
		if m := st.Ix.ByID[e.Instance]; m != nil {
			have[strings.ToLower(m.PackageID)] = true
		}
	}
	ready := installedFor(st)
	var refs []string
	var mods []*store.Mod
	for _, id := range ids {
		m := ready[id]
		if m == nil || have[strings.ToLower(m.PackageID)] {
			continue
		}
		have[strings.ToLower(m.PackageID)] = true
		refs, mods = append(refs, st.Ix.Ref(m.ID)), append(mods, m)
	}
	if len(refs) == 0 {
		return r, nil
	}
	after, _, err := a.enableList(ps, ps.List.Clone(), refs, Placement{})
	if err != nil {
		return nil, err
	}
	if err := a.editPlan(p, ps.WS, r.Before, after, "add downloaded"); err != nil {
		return nil, err
	}
	if ps == st {
		st.List = after
	}
	r.After, r.Enabled = after, mods
	return r, nil
}
