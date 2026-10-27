package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// SubscribeWorkshopBatch changes no existing subscription policy. Only explicitly
// forced, non-preview SteamCMD bulk downloads create a credit batch.
func (a *App) SubscribeWorkshopBatch(inputs []string, steamcmd, batch, dryRun bool, calls ...WorkshopCalls) (*SubscribeOp, error) {
	op, err := a.SubscribeWorkshop(inputs, steamcmd, dryRun, calls...)
	if !steamcmd || !batch || dryRun || op == nil {
		return op, err
	}
	var items []store.CreditItem
	for _, it := range op.Items {
		if it.Result != "downloaded with SteamCMD" || it.ID == "" || it.Record == nil {
			continue
		}
		m, e := a.Store.Mod(it.ID)
		if e != nil {
			return op, errors.Join(err, e)
		}
		if m != nil && m.Kind == source.SteamCMD && m.Status == identity.Live {
			items = append(items, store.CreditItem{WorkshopID: it.Record.WorkshopID, InstanceID: m.ID, Name: m.DisplayName()})
		}
	}
	_, e := a.Store.CreateCreditBatch(a.Now(), items)
	return op, errors.Join(err, e)
}

// CreditPromptData is nil when asking must be deferred. Querying never consumes it;
// only a committed answer leaves prompt state.
type CreditPromptData struct {
	BatchIDs []int64  `json:"batchIDs"`
	ModCount int      `json:"modCount"`
	Examples []string `json:"examples"`
}

func (a *App) CreditPrompt() (*CreditPromptData, error) {
	if g, err := a.RunningGame(); err != nil || g != nil {
		return nil, err
	}
	bs, err := a.Store.CreditBatches("prompt")
	if err != nil {
		return nil, err
	}
	p := &CreditPromptData{BatchIDs: []int64{}, Examples: []string{}}
	seen := map[string]bool{}
	var probe string
	for _, b := range bs {
		items, err := a.Store.CreditItems(b.ID)
		if err != nil {
			return nil, err
		}
		p.BatchIDs = append(p.BatchIDs, b.ID)
		for _, it := range items {
			m, err := a.Store.Mod(it.InstanceID)
			if err != nil {
				return nil, err
			}
			if m == nil || m.Kind != source.SteamCMD || m.Status != identity.Live || seen[it.WorkshopID] {
				continue
			}
			seen[it.WorkshopID] = true
			probe = it.WorkshopID
			p.ModCount++
			if len(p.Examples) < 3 {
				p.Examples = append(p.Examples, m.DisplayName())
			}
		}
	}
	if len(p.BatchIDs) == 0 || probe == "" {
		return nil, nil
	}
	if _, err := a.runSteam(steamworks.State, []string{probe}); err != nil {
		if errors.Is(err, ErrSteamUnavailable) {
			return nil, nil
		}
		return nil, err
	}
	if g, err := a.RunningGame(); err != nil || g != nil {
		return nil, err
	}
	return p, nil
}

func (a *App) CreditAnswer(ids []int64, replace bool) error {
	return a.Store.AnswerCredit(ids, replace)
}

func (a *App) CreditPending() ([]store.CreditItem, error) {
	bs, err := a.Store.CreditBatches("replacing")
	if err != nil {
		return nil, err
	}
	out := []store.CreditItem{}
	for _, b := range bs {
		items, err := a.Store.CreditItems(b.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

// CreditCallbacks report checkpoint progress and stop at each safe item boundary.
type CreditCallbacks struct {
	Context  context.Context
	Stopped  func() bool
	Progress func(store.CreditItem)
}

// CreditStep performs a bounded pass. Network failures and backoff are durable;
// shutdown loses no obligation. replace lets the GUI serialize mutations with launches
// while Steam workers run without its page lock. nil uses the same core implementation.
func (a *App) CreditStep(replace func(store.CreditItem) (string, error), callbacks ...CreditCallbacks) error {
	items, err := a.CreditPending()
	if err != nil {
		return err
	}
	if replace == nil {
		replace = a.ReplaceCreditItem
	}
	var hooks CreditCallbacks
	if len(callbacks) > 0 {
		hooks = callbacks[0]
	}
	stop := func() bool { return hooks.Stopped != nil && hooks.Stopped() }
	progress := func(it store.CreditItem) {
		if hooks.Progress != nil {
			hooks.Progress(it)
		}
	}
	ctx := hooks.Context
	if ctx == nil {
		ctx = context.Background()
	}
	processed := 0
	for _, it := range items {
		if stop() {
			return nil
		}
		if it.Terminal() || it.NextAttempt > a.Now().Unix() {
			continue
		}
		if processed >= 10 {
			break
		}
		processed++
		// Check each pass, rather than retaining the process-table snapshot in probe().
		if g, err := a.RunningGame(); err != nil {
			return err
		} else if g != nil {
			return nil
		}
		progress(it)
		// Refresh shared confirmation even after this item's subscribed checkpoint:
		// another batch may have completed the same physical handoff since the last pass.
		peer, err := a.Store.CreditPeer(it)
		if err != nil {
			return err
		}
		if peer != nil {
			changed := false
			if it.State == "pending" {
				it.State, it.Result, changed = "subscribed", "waiting for Steam installation", true
				if peer.State == "failed" {
					it.State, it.Result = "failed", peer.Result
				}
			}
			if peer.InstanceID == it.InstanceID && peer.State == "done" && peer.TargetID != "" {
				old, err := a.Store.Mod(it.InstanceID)
				if err != nil {
					return err
				}
				if old == nil || old.Status != identity.Live {
					it.State, it.Result, it.TargetID, it.NextAttempt, changed = "done", "already replaced with Workshop", peer.TargetID, 0, true
				}
			}
			if changed {
				if err := a.Store.SaveCreditItem(it); err != nil {
					return err
				}
				progress(it)
				if it.Terminal() {
					continue
				}
			}
		}
		if it.State == "pending" {
			rep, callErr := a.runSteamContext(ctx, steamworks.Subscribe, []string{it.WorkshopID})
			result := "unconfirmed"
			if callErr == nil {
				for _, r := range rep.Items {
					if strconv.FormatUint(r.ID, 10) == it.WorkshopID {
						result = r.Result
					}
				}
			}
			switch {
			case callErr != nil:
				creditRetry(&it, a.Now(), callErr.Error())
			case result == "ok":
				it.State, it.Result, it.NextAttempt = "subscribed", "waiting for Steam installation", 0
			case creditPermanent(result):
				it.State, it.Result = "failed", result+"; SteamCMD copy retained; "+workshopURL(it.WorkshopID)
			default:
				creditRetry(&it, a.Now(), result)
			}
			if err := a.Store.SaveCreditItem(it); err != nil {
				return err
			}
			progress(it)
			if stop() || errors.Is(callErr, ErrSteamUnavailable) {
				return nil
			}
			if it.State != "subscribed" {
				continue
			}
		}
		if it.State == "subscribed" || it.State == "switching" {
			rep, callErr := a.runSteamContext(ctx, steamworks.State, []string{it.WorkshopID})
			ready := false
			if callErr == nil {
				for _, r := range rep.Items {
					if strconv.FormatUint(r.ID, 10) == it.WorkshopID {
						ready = creditInstalled(r.State)
					}
				}
			}
			if callErr != nil {
				creditRetry(&it, a.Now(), callErr.Error())
			} else if !ready {
				it.Result = "waiting for Steam installation"
				it.NextAttempt = a.Now().Add(30 * time.Second).Unix()
			} else {
				result, e := replace(it)
				// ReplaceCreditItem journals target and deployments before any plan edits.
				// Reload that checkpoint before saving a retry, including after partial failure.
				saved, e2 := a.Store.CreditItems(it.BatchID)
				if e2 != nil {
					return e2
				}
				for _, x := range saved {
					if x.WorkshopID == it.WorkshopID {
						it = x
						break
					}
				}
				if e != nil {
					var wait CreditWait
					if errors.As(e, &wait) {
						it.NextAttempt, it.Result = a.Now().Add(5*time.Second).Unix(), e.Error()
					} else {
						creditRetry(&it, a.Now(), e.Error())
					}
				} else {
					it.State, it.Result, it.NextAttempt = "done", result, 0
					if strings.HasPrefix(result, "retained:") {
						it.State = "retained"
					}
				}
			}
			if err := a.Store.SaveCreditItem(it); err != nil {
				return err
			}
			progress(it)
			if errors.Is(callErr, ErrSteamUnavailable) {
				return nil
			}
		}
	}
	return a.Store.FinishCreditBatches()
}

func creditInstalled(state uint32) bool {
	return state&(steamworks.ItemSubscribed|steamworks.ItemInstalled) == steamworks.ItemSubscribed|steamworks.ItemInstalled &&
		state&(steamworks.ItemNeedsUpdate|steamworks.ItemDownloading|steamworks.ItemDownloadPending) == 0
}

func creditPermanent(result string) bool {
	code, _ := strconv.Atoi(strings.TrimPrefix(result, "failed:"))
	switch code {
	case 9, 15, 17, 24, 26, 41:
		return true
	}
	return false
}
func creditRetry(i *store.CreditItem, now time.Time, why string) {
	i.Attempts++
	delay := min(30*time.Minute, 5*time.Second*time.Duration(1<<min(i.Attempts-1, 9)))
	i.NextAttempt = now.Add(delay).Unix()
	i.Result = fmt.Sprintf("waiting to retry: %s", why)
}

// CreditWait is a scheduler/readiness wait, not a failed subscription attempt.
// Recheck soon without growing the durable exponential retry counter.
type CreditWait string

func (w CreditWait) Error() string { return string(w) }
