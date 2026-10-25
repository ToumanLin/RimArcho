package gui

import (
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestInventoryCacheSurvivesNavigationAndSeesExternalWrites(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	if _, err := api.Library("", nil, "name"); err != nil {
		t.Fatal(err)
	}
	first := api.lib
	api.Leave("library")
	if _, err := api.Library("", nil, "name"); err != nil {
		t.Fatal(err)
	}
	if api.lib != first {
		t.Fatal("navigation rebuilt unchanged inventory")
	}
	r, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.Store == e.App.Store || r.Store.DB == e.App.Store.DB {
		t.Fatal("reader shares database handle")
	}
	if err := r.Store.SetMeta("test:external", "changed"); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Library("", nil, "name"); err != nil {
		t.Fatal(err)
	}
	if api.lib == first {
		t.Fatal("external write did not invalidate inventory")
	}
}

func TestLogAnalysisDoesNotBlockOtherPagesOrRestartBetweenPolls(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	e.WriteFile(filepath.Join(e.UserData, "Player.log"), "RimWorld 1.5.4104 rev435\nNullReferenceException: fixture\n  at Fixture.Tick ()\n")
	started, release := make(chan struct{}, 4), make(chan struct{})
	var calls atomic.Int32
	e.App.AnalyzeRunner = func(string, func(string)) error {
		calls.Add(1)
		started <- struct{}{}
		<-release
		return errors.New("fixture analysis ended")
	}
	defer close(release)
	type result struct {
		pending string
		err     error
	}
	read := func() <-chan result {
		ch := make(chan result, 1)
		go func() {
			p, err := api.LogPage("")
			v := result{err: err}
			if p != nil {
				v.pending = p.Pending
			}
			ch <- v
		}()
		return ch
	}
	first := read()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("analysis did not start")
	}
	stamp := make(chan string, 1)
	go func() { stamp <- api.Stamp() }()
	select {
	case <-stamp:
	case <-time.After(150 * time.Millisecond):
		t.Fatal("log analysis holds shared API lock")
	}
	for i, ch := range []<-chan result{first, read()} {
		select {
		case v := <-ch:
			if v.err != nil || v.pending == "" {
				t.Fatalf("poll %d: %+v", i, v)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("pending log read did not return")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("analysis restarted: %d", calls.Load())
	}
}

func TestTextureReadSupersededAndCancelledOnLeave(t *testing.T) {
	t.Parallel()
	_, api, _ := fixture(t)
	first, cancel := api.beginTextureRead()
	defer cancel()
	second, cancel2 := api.beginTextureRead()
	defer cancel2()
	if first.Err() == nil || second.Err() != nil {
		t.Fatal("new request did not supersede only the old one")
	}
	api.Leave("textures")
	if second.Err() == nil {
		t.Fatal("abandoned read is still active")
	}
}

func TestLogPageSwitchesEnvironmentAndRejectsPreviousPath(t *testing.T) {
	t.Parallel()
	e, api, events := fixture(t)
	old := filepath.Join(e.UserData, "Player.log")
	e.WriteFile(old, "RimWorld 1.6.4633 rev1261\n")
	if p, err := api.LogPage(""); err != nil || p.Path != old {
		t.Fatalf("initial log: %+v, %v", p, err)
	}
	g2 := e.OtherGame("Other", "1.6.4633 rev1261")
	e.MustRun("env", "add", "other", "--game", g2, "--saves", "own")
	other, err := e.App.Store.EnvByName("other")
	if err != nil {
		t.Fatal(err)
	}
	next := filepath.Join(other.UserDataRoot, "Player.log")
	e.WriteFile(next, "RimWorld 1.6.4633 rev1261\n")
	api.UseEnv(1, "other")
	if r := environmentResult(t, events, 1); r.Error != "" {
		t.Fatal(r.Error)
	}
	// No intervening Board call: the backend itself must use the new environment.
	for _, path := range []string{"", old} {
		if p, err := api.LogPage(path); err != nil || p.Path != next {
			t.Fatalf("destination log for %q: %+v, %v", path, p, err)
		}
	}
	e.MustRun("env", "add", "empty", "--game", e.OtherGame("Empty", "1.6.4633 rev1261"), "--saves", "own")
	api.UseEnv(2, "empty")
	if r := environmentResult(t, events, 2); r.Error != "" {
		t.Fatal(r.Error)
	}
	if p, err := api.LogPage(next); err != nil || p.Path != "" || len(p.Files) != 0 {
		t.Fatalf("empty environment retained old log: %+v, %v", p, err)
	}
}
