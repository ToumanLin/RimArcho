package app

import (
	"errors"
	"runtime"
	"sync"
)

// Each worker owns its App/connection. No filesystem operation is inside a SQL
// transaction, and a failure/cancellation leaves already committed items intact.
func (a *App) removeSubscriptionsParallel(refs []string, opts RemoveSubscriptionOptions, calls BatchCalls) (*SubscribeOp, error) {
	workers := min(opts.Workers, 4, len(refs))
	if workers == 0 {
		return nil, errors.New("give at least one subscription reference")
	}
	// A link lifecycle journals the whole incoming set. Do not let another
	// worker mistake this batch's in-flight detach for interrupted recovery.
	links, err := a.readIncomingLinks()
	if err != nil {
		return nil, err
	}
	if len(links.links) > 0 {
		ix, err := a.Index()
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			if m, err := ix.ResolveOne(ref); err == nil && len(links.affected(ix.ModDir(m), false)) > 0 {
				workers = 1
				break
			}
		}
	}
	var sessions []*App
	for range workers {
		r, err := a.ReadSession()
		if err != nil {
			for _, r := range sessions {
				r.Close()
			}
			return nil, err
		}
		r.WorkspaceSnapshot()
		sessions = append(sessions, r)
	}
	result := &SubscribeOp{Action: "remove", DryRun: opts.DryRun, Items: make([]SubscribeItem, len(refs))}
	failures := make([]error, len(refs))
	jobs := make(chan int)
	var wg sync.WaitGroup
	opts.Workers = 0
	for _, r := range sessions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer r.Close()
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			for i := range jobs {
				if calls.Stopped != nil && calls.Stopped() {
					continue
				}
				reported := false
				op, err := r.RemoveSubscriptionsWith(refs[i:i+1], opts, BatchCalls{
					Stopped: calls.Stopped,
					Start: func(int) {
						if calls.Start != nil {
							calls.Start(i)
						}
					},
					Done: func(_ int, err error) {
						reported = true
						if calls.Done != nil {
							calls.Done(i, err)
						}
					},
				})
				if op != nil && len(op.Items) > 0 {
					result.Items[i] = op.Items[0]
				}
				failures[i] = err
				if !reported && err != nil && calls.Done != nil {
					calls.Done(i, err)
				}
			}
		}()
	}
	for i := range refs {
		if calls.Stopped != nil && calls.Stopped() {
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return result, errors.Join(failures...)
}
