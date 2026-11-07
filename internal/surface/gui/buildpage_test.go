package gui

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func buildPage(t *testing.T, api *API, tex int) (*view.BuildPage, error) {
	t.Helper()
	id, err := api.BuildPage(tex)
	if err != nil {
		return nil, err
	}
	return waitBuildPage(t, api, id)
}

func waitBuildPage(t *testing.T, api *API, id uint64) (*view.BuildPage, error) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		p, err := api.BuildPageResult(id)
		if p != nil || err != nil {
			return p, err
		}
		time.Sleep(time.Millisecond)
	}
	return nil, fmt.Errorf("build page %d did not finish", id)
}

// 16 §3.1: a slow old reader cannot block input or replace the latest texture choice.
func TestBuildPageLatestMeasurementWins(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	e.MustRun("enable", "pkg:brrainz.harmony")
	started, release := make(chan struct{}), make(chan struct{})
	now := e.App.Now
	var once sync.Once
	e.App.Now = func() time.Time { once.Do(func() { close(started) }); <-release; return now() }
	released := false
	defer func() {
		if !released {
			close(release)
		}
		api.buildRead.workers.Wait()
	}()
	returned := make(chan uint64, 1)
	go func() {
		id, err := api.BuildPage(100)
		if err != nil {
			t.Error(err)
		}
		returned <- id
	}()
	var old uint64
	select {
	case old = <-returned:
	case <-time.After(time.Second):
		t.Fatal("BuildPage waited for the background reader")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background reader did not start")
	}
	if p, err := api.BuildPageResult(old); p != nil || err != nil {
		t.Fatalf("pending result = %+v, %v", p, err)
	}
	stampResponds(t, api)
	e.App.Now = now
	latest, err := api.BuildPage(50)
	if err != nil {
		t.Fatal(err)
	}
	p, err := waitBuildPage(t, api, latest)
	if err != nil || p.Tex != 50 {
		t.Fatalf("latest = %+v, %v", p, err)
	}
	close(release)
	released = true
	api.buildRead.workers.Wait()
	got, err := api.BuildPageResult(latest)
	if err != nil || got != p || got.Tex != 50 {
		t.Fatalf("old reader replaced latest = %+v, %v", got, err)
	}
	if _, err := api.BuildPageResult(old); !errors.Is(err, context.Canceled) {
		t.Fatalf("old ticket remained valid: %v", err)
	}
}
