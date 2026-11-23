package app_test

import (
	"context"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// 20 §2 "Game-invisible Mods": without the Steam client the game cannot list
// subscriptions, so a test run links the list's Workshop Mods into Mods/ under the names a
// launch without Steam gives them (09 §3), and takes them away after the run. With Steam
// running they load from the Workshop and nothing is linked. (VM 2026-11-22: with Steam
// closed a test run loaded 8 of 25 Mods and said NOT TESTED.)
func TestTestRunWithoutSteamLinksWorkshopMods(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	steam := false
	e.App.SteamRunning = func() bool { return steam }
	harmony, a := workshopLink(t, e.App, "brrainz.harmony"), workshopLink(t, e.App, "tester.a")
	var during map[string]string
	start := fakeTestGame(t, "menu", "")
	e.App.TestGameStart = func(cmd *exec.Cmd) error {
		during = archoLinks(t, e)
		return start(cmd)
	}
	missing := func(r *app.TestRunReport) []string {
		var out []string
		for _, m := range r.Missing {
			if m.PackageID != "gone.mod" {
				out = append(out, m.PackageID)
			}
		}
		return out
	}

	r, err := e.App.TestRun(context.Background(), app.TestRunOptions{Timeout: 5 * time.Second, Settle: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if m := missing(r); len(m) != 0 || !slices.Contains(r.Linked, harmony) || !slices.Contains(r.Linked, a) || during[harmony] == "" || during[a] == "" {
		t.Fatalf("without Steam: missing %v, linked %v, links during the run %v", m, r.Linked, during)
	}
	if left := archoLinks(t, e); len(left) != 0 {
		t.Fatalf("links left after the run: %v", left)
	}

	steam = true
	r, err = e.App.TestRun(context.Background(), app.TestRunOptions{Timeout: 5 * time.Second, Settle: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if m := missing(r); len(m) != 0 || len(r.Linked) != 0 || len(during) != 0 {
		t.Fatalf("with Steam: missing %v, linked %v, links during the run %v", m, r.Linked, during)
	}
}
