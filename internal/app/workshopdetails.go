package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// workshopBatch is how many items one GetPublishedFileDetails request asks about, the
// same 100 as the subscribe preflight and collection titles.
const workshopBatch = 100

// workshopRequests is how many detail requests run at once.
const workshopRequests = 4

// workshopRetryPause is the wait before a failed detail request is sent again.
const workshopRetryPause = time.Second

// workshopAnswer is what Steam said about one item: its details, or why there are none.
type workshopAnswer struct {
	d   *workshopCache
	err error
}

// workshopDetails reads the details of many items, workshopBatch per request (11 §1).
// A request that fails is sent once more after a pause, and so are the items Steam
// answered with a passing error (busy, timed out, rate limited). Every ID gets an answer.
func (a *App) workshopDetails(ctx context.Context, ids []string) map[string]workshopAnswer {
	seen := map[string]bool{}
	var unique []string
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	out := make(map[string]workshopAnswer, len(unique))
	var mu sync.Mutex
	ask := func(part []string) {
		got, err := a.postWorkshopDetails(ctx, part)
		if err != nil && pause(ctx) {
			got, err = a.postWorkshopDetails(ctx, part)
		}
		mu.Lock()
		defer mu.Unlock()
		for _, id := range part {
			d, ok := got[id]
			switch {
			case err != nil:
				out[id] = workshopAnswer{err: err}
			case !ok:
				out[id] = workshopAnswer{err: errors.New("Steam returned no details")}
			default:
				out[id] = workshopAnswer{d: d}
			}
		}
	}
	inBatches(unique, ask)
	var again []string
	for _, id := range unique {
		if d := out[id].d; d != nil && passingResult(d.Result) {
			again = append(again, id)
		}
	}
	if len(again) > 0 && pause(ctx) {
		inBatches(again, ask)
	}
	return out
}

// inBatches calls ask for each batch of ids, workshopRequests at a time.
func inBatches(ids []string, ask func([]string)) {
	slots := make(chan struct{}, workshopRequests)
	var wg sync.WaitGroup
	for i := 0; i < len(ids); i += workshopBatch {
		part := ids[i:min(i+workshopBatch, len(ids))]
		slots <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-slots; wg.Done() }()
			ask(part)
		}()
	}
	wg.Wait()
}

// pause waits before a retry; false when the check was cancelled meanwhile.
func pause(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(workshopRetryPause):
		return true
	}
}

// passingResult: Steam result codes worth asking about once more.
func passingResult(code int) bool {
	switch code {
	case 2, 16, 20, 25, 84: // failed, timed out, unavailable, limit exceeded, rate limited
		return true
	}
	return false
}

// postWorkshopDetails sends one GetPublishedFileDetails request.
func (a *App) postWorkshopDetails(ctx context.Context, ids []string) (map[string]*workshopCache, error) {
	api := a.SteamAPI
	if api == "" {
		api = SteamAPI
	}
	form := url.Values{"itemcount": {strconv.Itoa(len(ids))}}
	for i, id := range ids {
		form.Set(fmt.Sprintf("publishedfileids[%d]", i), id)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", api, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", UserAgent)
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("Steam unreachable: %v", shortErr(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Steam answered %s", resp.Status)
	}
	var r struct {
		Response struct {
			Details []struct {
				ID          string `json:"publishedfileid"`
				Result      int    `json:"result"`
				Title       string `json:"title"`
				Description string `json:"description"`
				TimeUpdated int64  `json:"time_updated"`
				Tags        []struct {
					Tag string `json:"tag"`
				} `json:"tags"`
			} `json:"publishedfiledetails"`
		} `json:"response"`
	}
	// Descriptions make up most of an answer: 100 long ones stay well under 32 MiB.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&r); err != nil {
		return nil, fmt.Errorf("unexpected answer from Steam: %v", err)
	}
	out := map[string]*workshopCache{}
	now := a.Now().Unix()
	for _, x := range r.Response.Details {
		out[x.ID] = &workshopCache{ID: x.ID, Title: x.Title, Description: x.Description, TimeUpdated: x.TimeUpdated, Fetched: now, Result: x.Result}
		for _, tag := range x.Tags {
			out[x.ID].Tags = append(out[x.ID].Tags, tag.Tag)
		}
	}
	return out, nil
}
