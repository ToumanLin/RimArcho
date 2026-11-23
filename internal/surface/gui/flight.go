//go:build windows || cgo

package gui

import "sync"

// flights joins repeated page reads (16 §8.9): a call made while the same read (method and
// arguments) is on its way waits for that read and gets its answer instead of starting
// another, so rapid page switches and repeated clicks do not pile up runs behind a page's
// lock or in parallel. A call made after the read ended starts a new one; reads are
// change-keyed, so a page that changed is read again by its change event.
type flights struct {
	mu    sync.Mutex
	calls map[string]*flightCall
	runs  map[string]int // reads started per key (tests)
}

type flightCall struct {
	done chan struct{}
	val  any
	err  error
}

// join runs fn under key unless a run of key is on its way, which it waits for instead.
func join[T any](f *flights, key string, fn func() (T, error)) (T, error) {
	f.mu.Lock()
	if c := f.calls[key]; c != nil {
		f.mu.Unlock()
		<-c.done
		v, _ := c.val.(T)
		return v, c.err
	}
	c := &flightCall{done: make(chan struct{})}
	if f.calls == nil {
		f.calls, f.runs = map[string]*flightCall{}, map[string]int{}
	}
	f.calls[key] = c
	f.runs[key]++
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		delete(f.calls, key)
		f.mu.Unlock()
		close(c.done)
	}()
	v, err := fn()
	c.val, c.err = v, err
	return v, err
}

// started is how many reads of key began (tests).
func (f *flights) started(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runs[key]
}
