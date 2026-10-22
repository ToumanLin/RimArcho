package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// fakeSteamWorker answers like the worker: every call succeeds, and the state follows it.
func fakeSteamWorker(calls *[][]string) func(dir string, args []string) (string, error) {
	return func(dir string, args []string) (string, error) {
		*calls = append(*calls, append([]string{dir}, args...))
		var out []string
		for _, s := range args[1:] {
			id, _ := strconv.ParseUint(s, 10, 64)
			switch args[0] {
			case steamworks.State:
				out = append(out, steamworks.StateLine(id, steamworks.ItemSubscribed|steamworks.ItemInstalled))
			case steamworks.Subscribe:
				out = append(out, steamworks.ItemLine(id, "ok", steamworks.ItemSubscribed|steamworks.ItemDownloadPending))
			default:
				out = append(out, steamworks.ItemLine(id, "ok", 0))
			}
		}
		out = append(out, steamworks.DoneLine(args[0], len(args)-1))
		return strings.Join(out, "\n"), nil
	}
}

func steamDLL(e *testkit.Env) {
	e.WriteFile(filepath.Join(e.Game, "RimWorldWin64_Data", "Plugins", "x86_64", "steam_api64.dll"), "not really a DLL")
}

// 06 §2.9: 退订 and 重新订阅 go through the Steam client, in a worker run in a runtime
// folder outside the game; unsubscribing is refused while the mod is enabled, in a plan or
// the game runs; without Steam the error carries the Workshop pages.
func TestWorkshopSubscribe(t *testing.T) {
	e := setup(t, false)
	e.AddWorkshop("7100", testkit.Sample("tester.keep", "Keep"))
	e.AddWorkshop("7200", testkit.Sample("tester.drop", "Drop"))
	e.AddWorkshop("7300", testkit.Sample("tester.left", "Left"))
	e.Unsubscribe("7300")
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.keep")
	scanNow(t, e)
	var calls [][]string
	e.App.SteamWorker = fakeSteamWorker(&calls)
	drop, keep, left := refID(t, e, "Drop", source.Workshop), refID(t, e, "Keep", source.Workshop), refID(t, e, "Left", source.Workshop)

	// no DLL in the game: Steam is out of reach, and the page is the way
	_, err := e.App.Workshop(steamworks.Unsubscribe, []string{drop}, false)
	var u *app.SteamUnavailable
	if !errors.As(err, &u) || !errors.Is(err, app.ErrSteamUnavailable) || len(u.URLs) != 1 || !strings.HasSuffix(u.URLs[0], "?id=7200") {
		t.Fatalf("no DLL: %v", err)
	}
	steamDLL(e)

	op, err := e.App.Workshop(steamworks.Unsubscribe, []string{drop}, true)
	if err != nil || len(calls) != 0 || !op.DryRun || !strings.HasSuffix(op.Items[0].Folder, "7200") {
		t.Fatalf("dry run: %+v %v %v", op, err, calls)
	}
	if _, err := e.App.Workshop(steamworks.Unsubscribe, []string{keep}, true); err == nil || !strings.Contains(err.Error(), "enabled") {
		t.Fatalf("enabled: %v", err)
	}
	if _, err := e.App.Workshop(steamworks.Unsubscribe, []string{left}, true); err == nil || !strings.Contains(err.Error(), "not subscribed") {
		t.Fatalf("leftover: %v", err)
	}
	if _, err := e.App.Workshop(steamworks.Subscribe, []string{drop}, true); err == nil || !strings.Contains(err.Error(), "subscribed already") {
		t.Fatalf("subscribed: %v", err)
	}
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42} }
	if _, err := e.App.Workshop(steamworks.Unsubscribe, []string{drop}, false); err == nil || !strings.Contains(err.Error(), "game is running") {
		t.Fatalf("game running: %v", err)
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }

	op, err = e.App.Workshop(steamworks.Unsubscribe, []string{drop}, false)
	if err != nil || op.Items[0].Result != "ok" || len(calls) != 1 {
		t.Fatalf("unsubscribe: %+v %v", op, err)
	}
	dir := calls[0][0]
	if strings.Join(calls[0][1:], " ") != "unsubscribe 7200" || !strings.HasPrefix(dir, e.App.LockBase) || strings.HasPrefix(dir, e.Game) {
		t.Fatalf("worker call %v", calls[0])
	}
	if b, err := os.ReadFile(filepath.Join(dir, "steam_appid.txt")); err != nil || string(b) != "294100" {
		t.Fatalf("steam_appid.txt %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "steam_api64.dll")); err != nil {
		t.Fatal("the DLL was not copied")
	}
	op, err = e.App.Workshop(steamworks.Subscribe, []string{left}, false)
	if err != nil || op.Items[0].Result != "ok" || strings.Join(calls[1][1:], " ") != "subscribe 7300" {
		t.Fatalf("subscribe: %+v %v", op, err)
	}
	op, err = e.App.WorkshopState([]string{keep})
	if err != nil || op.Items[0].State&steamworks.ItemSubscribed == 0 {
		t.Fatalf("state: %+v %v", op, err)
	}

	// Steam not running: the worker says so
	e.App.SteamWorker = func(string, []string) (string, error) {
		return steamworks.ErrorLine("Steam API did not start; is Steam running and signed in?"), errors.New("exit status 3")
	}
	if _, err := e.App.Workshop(steamworks.Subscribe, []string{left}, false); !errors.As(err, &u) || !strings.Contains(u.Why, "Steam running") {
		t.Fatalf("not running: %v", err)
	}
	// Steam says no for an item
	e.App.SteamWorker = func(_ string, args []string) (string, error) {
		return steamworks.ItemLine(7300, "failed:15", 0) + "\n" + steamworks.DoneLine(args[0], 1), nil
	}
	if _, err := e.App.Workshop(steamworks.Subscribe, []string{left}, false); err == nil || errors.Is(err, app.ErrSteamUnavailable) || !strings.Contains(err.Error(), "failed:15") {
		t.Fatalf("refused: %v", err)
	}
}

func TestSteamParse(t *testing.T) {
	r := steamworks.Parse("[S_API] SteamAPI_Init(): Loaded\nITEM 1 ok 5\nSTATE 2 1\nDONE state 1\n")
	if !r.Done || len(r.Items) != 2 || r.Items[0].Result != "ok" || r.Items[0].State != 5 || r.Items[1].State != 1 {
		t.Fatalf("%+v", r)
	}
	if steamworks.StateText(5) != "subscribed, installed" || steamworks.StateText(0) != "not subscribed" {
		t.Fatal(steamworks.StateText(5))
	}
}
