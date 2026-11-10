package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

const workshopMissing = "removed from the Workshop (the author deleted it or made it private)"

// WorkshopCalls observes one subscription operation, regardless of its internal batches.
type WorkshopCalls struct {
	Context context.Context
	Changed func([]TaskItem)
}

type workshopDetail struct {
	ID     string      `json:"publishedfileid"`
	Result int         `json:"result"`
	AppID  int         `json:"consumer_app_id"`
	Title  string      `json:"title"`
	Size   json.Number `json:"file_size"`
}

// A failed request supplies no classification. Missing entries are also unknown.
func (a *App) preflightWorkshop(ctx context.Context, ids []string) map[string]workshopDetail {
	details := map[string]workshopDetail{}
	endpoint := a.SteamAPI
	if endpoint == "" {
		endpoint = SteamAPI
	}
	for start := 0; start < len(ids); start += 100 {
		part := ids[start:min(start+100, len(ids))]
		form := url.Values{"itemcount": {strconv.Itoa(len(part))}}
		for i, id := range part {
			form.Set(fmt.Sprintf("publishedfileids[%d]", i), id)
		}
		chunk, cancel := context.WithTimeout(ctx, 3*time.Second)
		req, err := http.NewRequestWithContext(chunk, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
		if err != nil {
			cancel()
			break
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", UserAgent)
		resp, err := a.httpClient().Do(req)
		if err != nil {
			cancel()
			break
		}
		var body struct {
			Response struct {
				Details []workshopDetail `json:"publishedfiledetails"`
			} `json:"response"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&body)
		resp.Body.Close()
		cancel()
		if resp.StatusCode != http.StatusOK || err != nil {
			break
		}
		for _, d := range body.Response.Details {
			if slices.Contains(part, d.ID) {
				details[d.ID] = d
			}
		}
	}
	return details
}

type workshopTransfer struct {
	ctx     context.Context
	items   []TaskItem
	changed func([]TaskItem)
	// confirmed is told when SteamCMD's output confirms an item and moves on to the next
	// one (a success line can still be followed by an error for the same item), or when
	// its batch ends; downloadWorkshopGroup finishes the item then.
	confirmed func(id string)
	okPending string // the item whose success line came last, not yet passed on
	// settled receives each item downloadWorkshopGroup has finished, success or not.
	settled func(SubscribeItem)
}

func workshopObserver(transfers []*workshopTransfer) *workshopTransfer {
	if len(transfers) > 0 && transfers[0] != nil {
		return transfers[0]
	}
	return &workshopTransfer{ctx: context.Background()}
}

func (w *workshopTransfer) publish() {
	if w != nil && w.changed != nil {
		w.changed(slices.Clone(w.items))
	}
}

func (w *workshopTransfer) line(line string) {
	if w == nil {
		return
	}
	id, state, kind := "", ItemRun, ""
	if m := workshopSuccess.FindStringSubmatch(line); m != nil {
		id, state = m[1], ItemOK
	} else if m := workshopFailure.FindStringSubmatch(line); m != nil {
		id, state, kind = m[1], ItemFail, workshopErrorKind(strings.ToLower(m[0]))
	} else if m := workshopCurrent.FindStringSubmatch(line); m != nil {
		id = m[1]
	}
	if id == "" {
		return
	}
	if state == ItemRun {
		for i := range w.items {
			if w.items[i].State == ItemRun {
				w.items[i].State = ItemWait
			}
		}
	}
	for i := range w.items {
		it := &w.items[i]
		if it.Ref != "ws:"+id {
			continue
		}
		it.State = state
		it.FailureKind, it.Permanent = kind, workshopPermanent(kind)
		if state == ItemFail {
			it.Why = (workshopOutcome{kind: kind}).why()
		}
		if state == ItemOK {
			it.Why, it.Got = "", it.Size
		}
		if state != ItemRun {
			for j := i + 1; j < len(w.items); j++ {
				if w.items[j].State == ItemWait {
					w.items[j].State = ItemRun
					break
				}
			}
		}
		break
	}
	w.publish()
	switch {
	case state == ItemOK:
		w.passOn(id)
		w.okPending = id
	case state == ItemFail && w.okPending == id:
		w.okPending = ""
	case state == ItemRun:
		w.passOn(id)
	}
}

// passOn confirms the item whose success line came last unless it is next (the same item
// again); its output is complete once SteamCMD reports another one.
func (w *workshopTransfer) passOn(next string) {
	if id := w.okPending; id != "" && id != next {
		w.okPending = ""
		if w.confirmed != nil {
			w.confirmed(id)
		}
	}
}

func (w *workshopTransfer) outcome(id string, outcome workshopOutcome) {
	for i := range w.items {
		it := &w.items[i]
		if it.Ref != "ws:"+id {
			continue
		}
		it.State, it.Why = ItemFail, outcome.why()
		if outcome.confirmed {
			it.State, it.Why, it.Got = ItemOK, "", it.Size
		}
		it.FailureKind, it.Permanent = outcome.kind, workshopPermanent(outcome.kind)
		break
	}
	w.publish()
}

func workshopPermanent(kind string) bool {
	return kind == "missing" || kind == "permission" || kind == "ownership" || kind == "disk"
}

// SubscribeWorkshop prechecks both download routes and reports titles and live results.
// The observer receives provisional output; the returned operation uses final parsing.
func (a *App) SubscribeWorkshop(inputs []string, forceSteamCMD, dryRun bool, calls ...WorkshopCalls) (*SubscribeOp, error) {
	ids, err := workshopIDs(inputs)
	if err != nil {
		return nil, err
	}
	if _, err = a.Env(); err != nil {
		return nil, err
	}
	if dryRun {
		return a.subscribeWorkshop(ids, forceSteamCMD, true, nil)
	}
	w := &workshopTransfer{ctx: context.Background()}
	if len(calls) > 0 {
		w.changed = calls[0].Changed
		if calls[0].Context != nil {
			w.ctx = calls[0].Context
		}
	}
	details := a.preflightWorkshop(w.ctx, ids)
	op := &SubscribeOp{Action: "workshop"}
	var valid []string
	for _, id := range ids {
		it := SubscribeItem{Name: "Workshop " + id, Record: &subscribe.Record{WorkshopID: id}}
		ti := TaskItem{Ref: "ws:" + id, Name: it.Name, State: ItemWait}
		if d, ok := details[id]; ok {
			if strings.TrimSpace(d.Title) != "" {
				it.Name, ti.Name = d.Title, d.Title
			}
			ti.Size, _ = d.Size.Int64()
			if d.Result != 1 || d.AppID != 294100 {
				it.Result, it.FailureKind, it.Permanent = "failed: "+workshopMissing, "missing", true
				ti.State, ti.Why, ti.FailureKind, ti.Permanent = ItemFail, workshopMissing, "missing", true
			}
		}
		if !it.Permanent {
			valid = append(valid, id)
		}
		op.Items, w.items = append(op.Items, it), append(w.items, ti)
	}
	w.publish()
	var run *SubscribeOp
	if len(valid) > 0 && w.ctx.Err() == nil {
		run, err = a.subscribeWorkshop(valid, forceSteamCMD, false, w)
	}
	if w.ctx.Err() != nil {
		err = errors.Join(err, w.ctx.Err())
	}
	byID := map[string]SubscribeItem{}
	if run != nil {
		op.Notes = run.Notes
		for _, it := range run.Items {
			byID[it.Record.WorkshopID] = it
		}
	}
	var failures []error
	for i, id := range ids {
		it := &op.Items[i]
		name := it.Name
		if got, ok := byID[id]; ok {
			*it = got
			it.Name = name
		}
		ti := &w.items[i]
		switch {
		case it.Result == "ok" || it.Result == "downloaded with SteamCMD":
			ti.State, ti.Why, ti.Got = ItemOK, "", ti.Size
		case w.ctx.Err() != nil && !it.Permanent:
			ti.State, ti.Why = ItemSkip, "cancelled"
		default:
			if it.Result == "" {
				it.Result = "failed: download was not confirmed"
				if err != nil {
					it.Result = "failed: " + err.Error()
				}
			}
			ti.State, ti.Why = ItemFail, strings.TrimPrefix(it.Result, "failed: ")
			failures = append(failures, fmt.Errorf("Workshop %s: %s; %s", id, ti.Why, workshopURL(id)))
		}
		ti.FailureKind, ti.Permanent = it.FailureKind, it.Permanent
	}
	w.publish()
	if len(failures) > 0 {
		return op, errors.Join(append(failures, w.ctx.Err())...)
	}
	return op, err
}

// SetWorkshopProgress keeps one monotonic progress bar across batches and retries.
func (t *Task) SetWorkshopProgress(items []TaskItem) {
	// A continuation reports only its remaining IDs; retain earlier outcomes and
	// weights so it is still the same operation in the task page.
	if len(t.Items) > len(items) {
		merged := slices.Clone(t.Items)
		for _, next := range items {
			for i := range merged {
				if merged[i].Ref == next.Ref {
					merged[i] = next
					break
				}
			}
		}
		items = merged
	}
	previousDone := t.Done
	t.Items, t.Total, t.Done, t.Phase = slices.Clone(items), len(items), 0, ""
	var known, count float64
	for _, it := range items {
		if it.Size > 0 {
			known += float64(it.Size)
			count++
		}
	}
	unit := 1.0
	if count > 0 {
		unit = known / count
	}
	var done, total float64
	for _, it := range items {
		weight := unit
		if it.Size > 0 {
			weight = float64(it.Size)
		}
		total += weight
		// A transient failure may still be retried. Counting it as complete here
		// would jump the entire bar to 99% before the retry has downloaded it.
		if it.State == ItemOK || it.State == ItemFail && it.Permanent {
			t.Done++
			done += weight
		}
		if it.State == ItemRun {
			t.Phase = it.Name
		}
	}
	if total > 0 {
		t.Frac = max(t.Frac, done/total)
	}
	t.Done = max(t.Done, previousDone)
}
