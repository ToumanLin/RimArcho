package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func environmentResult(t *testing.T, events <-chan string, request uint64) *EnvironmentResult {
	t.Helper()
	timeout := time.After(30 * time.Second)
	for {
		select {
		case event := <-events:
			if !strings.HasPrefix(event, "environment ") {
				continue
			}
			var r EnvironmentResult
			if err := json.Unmarshal([]byte(strings.TrimPrefix(event, "environment ")), &r); err != nil {
				t.Fatal(err)
			}
			if r.Request == request {
				return &r
			}
		case <-timeout:
			t.Fatal("no environment result")
		}
	}
}

// An in-flight switch may finish preparing links, but cannot publish its destination
// after the player picked another game (16 §3.1).
func TestEnvironmentLatePreparationCannotSelectOldGame(t *testing.T) {
	t.Parallel()
	e, api, events := fixture(t)
	game := e.OtherGame("Other", "1.6.4633 rev1261")
	e.MustRun("env", "add", "other", "--game", game, "--saves", "own")
	other, _ := e.App.Store.EnvByName("other")
	config, err := os.ReadFile(filepath.Join(e.UserData, "Config", "ModsConfig.xml"))
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(filepath.Join(other.UserDataRoot, "Config", "ModsConfig.xml"), string(config))
	entered, release := make(chan struct{}), make(chan struct{})
	var enterOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	e.App.GameProbe = func(root string) *app.GameRun {
		if root == game {
			enterOnce.Do(func() { close(entered); <-release })
		}
		return nil
	}
	if !api.UseEnv(1, "other") {
		t.Fatal("switch did not start")
	}
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("switch never prepared destination")
	}
	if !api.UseEnv(2, "main") {
		t.Fatal("second switch was refused")
	}
	// Reading the current page is independent of the slow destination preparation.
	read := make(chan *Board, 1)
	go func() { read <- api.Board() }()
	select {
	case b := <-read:
		if b.EnvName != "main" {
			t.Fatalf("premature destination %s", b.EnvName)
		}
	case <-time.After(time.Second):
		unblock()
		t.Fatal("preparation held the page mutex")
	}
	unblock()
	first := environmentResult(t, events, 1)
	if !first.Stale {
		t.Fatalf("old result was published: %+v", first)
	}
	last := environmentResult(t, events, 2)
	if last.Stale || last.Error != "" || last.Board.EnvName != "main" {
		t.Fatalf("latest result: %+v", last)
	}
	current, _ := e.App.Store.CurrentEnv()
	if current.Name != "main" {
		t.Fatalf("old result selected %s", current.Name)
	}
}

func TestEnvironmentOutOfOrderRequestsAndQueuedWork(t *testing.T) {
	t.Parallel()
	_, api, events := fixture(t)
	api.environment.gate.Lock()
	api.UseEnv(2, "main")
	// A lower client sequence arriving later cannot replace a newer request.
	api.UseEnv(1, "missing")
	if r := environmentResult(t, events, 1); !r.Stale {
		t.Fatalf("older request: %+v", r)
	}
	api.environment.gate.Unlock()
	if r := environmentResult(t, events, 2); r.Stale || r.Error != "" || r.Board.EnvName != "main" {
		t.Fatalf("latest request: %+v", r)
	}
}

func TestHomeApplyReturnsPreparedWorkspace(t *testing.T) {
	t.Parallel()
	_, api, events := fixture(t)
	board := api.Board()
	if !api.HomeApply(1, view.RowPlan, "main", board.St.Plan.ID, "") {
		t.Fatal("apply did not start")
	}
	result := environmentResult(t, events, 1)
	if result.Error != "" || result.Stale || result.Board == nil || result.Shell.Plan == nil || result.Shell.Plan.ID != board.St.Plan.ID {
		t.Fatalf("apply result: %+v", result)
	}
	if !api.Workspace(2) {
		t.Fatal("workspace did not start")
	}
	refreshed := environmentResult(t, events, 2)
	if refreshed.Error != "" || refreshed.Stale || refreshed.Board.EnvName != "main" || refreshed.Shell.Plan.ID != board.St.Plan.ID {
		t.Fatalf("workspace result: %+v", refreshed)
	}
}
