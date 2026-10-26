package app_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
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
// folder outside the game; unsubscribing is refused while the game runs, and only names the
// plans that use the mod; without Steam the error carries the Workshop pages.
func TestWorkshopSubscribe(t *testing.T) {
	t.Parallel()
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
	// enabled: the player's word goes; the plan using it is named, nothing is refused
	if op, err := e.App.Workshop(steamworks.Unsubscribe, []string{keep}, true); err != nil || len(op.Items[0].UsedBy) != 1 || op.Items[0].UsedBy[0] != "plan:default" {
		t.Fatalf("enabled: %+v %v", op, err)
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
	t.Parallel()
	r := steamworks.Parse("[S_API] SteamAPI_Init(): Loaded\nITEM 1 ok 5\nSTATE 2 1\nDONE state 1\n")
	if !r.Done || len(r.Items) != 2 || r.Items[0].Result != "ok" || r.Items[0].State != 5 || r.Items[1].State != 1 {
		t.Fatalf("%+v", r)
	}
	if steamworks.StateText(5) != "subscribed, installed" || steamworks.StateText(0) != "not subscribed" {
		t.Fatal(steamworks.StateText(5))
	}
}

func TestWorkshopResubscribe(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	folder := e.AddWorkshop("8100", testkit.Sample("tester.repair", "Repair"))
	e.AddWorkshop("8200", testkit.Sample("tester.residue", "Residue"))
	incoming := filepath.Join(e.Game, "Mods", "_Link_8100")
	e.Junction(incoming, folder)
	e.Unsubscribe("8200")
	e.WriteFile(filepath.Join(folder, "junk.txt"), "untracked junk")
	e.WriteFile(filepath.Join(folder, ".git", "config"), "author repository")
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.repair")
	scanNow(t, e)
	steamDLL(e)
	id := refID(t, e, "Repair", source.Workshop)
	left := refID(t, e, "Residue", source.Workshop)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	folder, err = source.RealPath(folder)
	if err != nil {
		t.Fatal(err)
	}
	original := *st.Ix.ByID[id]
	before := slices.Clone(st.List)
	var calls [][]string
	fake := fakeSteamWorker(&calls)
	e.App.SteamWorker = fake
	op, err := e.App.Workshop(steamworks.Resubscribe, []string{id, left}, true)
	if err != nil || len(calls) != 0 || op.Items[0].Folder != filepath.Join(e.Workshop, "8100") || op.Items[1].Folder != "" {
		t.Fatalf("dry run: %+v %v %v", op, err, calls)
	}
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42} }
	if _, err := e.App.Workshop(steamworks.Resubscribe, []string{id}, false); err == nil || len(calls) != 0 {
		t.Fatalf("running game: %v %v", err, calls)
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	// Steam leaves the old files; RimArcho recycles them before a changed download.
	recycled := filepath.Join(e.Root, "Recycled8100")
	e.App.TrashFunc = func(path string) error {
		if len(calls) != 1 || calls[0][1] != steamworks.Unsubscribe || path != filepath.Join(e.Workshop, "8100") {
			t.Fatalf("cleanup before confirmed unsubscribe: %s %v", path, calls)
		}
		if err := os.Rename(path, recycled); err != nil {
			return err
		}
		scanNow(t, e) // identity and incoming links must survive a scan during the gap
		return nil
	}
	e.App.SteamWorker = func(dir string, args []string) (string, error) {
		out, err := fake(dir, args)
		if args[0] == steamworks.Unsubscribe {
			e.Unsubscribe("8100")
		} else {
			if _, err := os.Lstat(folder); !os.IsNotExist(err) {
				t.Fatalf("Subscribe called before folder was gone: %v", err)
			}
			for _, rel := range []string{"junk.txt", filepath.Join(".git", "config")} {
				if _, err := os.Stat(filepath.Join(recycled, rel)); err != nil {
					t.Fatalf("old files did not reach recycle bin: %s: %v", rel, err)
				}
			}
			e.AddWorkshop("8100", testkit.Sample("tester.repair", "Repair downloaded"))
			e.Subscribe("8200")
		}
		return out, err
	}
	op, err = e.App.Workshop(steamworks.Resubscribe, []string{id, left}, false)
	if err != nil || len(calls) != 2 || strings.Join(calls[0][1:], " ") != "unsubscribe 8100" || strings.Join(calls[1][1:], " ") != "subscribe 8100 8200" {
		t.Fatalf("resubscribe: %+v %v %v", op, err, calls)
	}
	scanNow(t, e)
	st, err = e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	m := st.Ix.ByID[id]
	if m == nil || m.LineageID != original.LineageID || m.WorkshopID() != "8100" || st.Ix.ModDir(m) != folder || m.Name != "Repair downloaded" || m.Status != "live" {
		t.Fatalf("identity after redownload: %+v", m)
	}
	if !source.IsLink(incoming) {
		t.Fatal("incoming link was removed")
	}
	if target, err := source.RealPath(incoming); err != nil || target != folder {
		t.Fatalf("incoming link: %s %v", target, err)
	}
	if !slices.Equal(st.List, before) {
		t.Fatalf("plan changed: %v -> %v", before, st.List)
	}
	// A plain subscription remains supported for leftovers.
	e.Unsubscribe("8200")
	scanNow(t, e)
	e.App.SteamWorker = fake
	if _, err := e.App.Workshop(steamworks.Subscribe, []string{left}, false); err != nil {
		t.Fatal(err)
	}
	// Local/locked copies cannot be passed through the direct API.
	e.WriteMod(filepath.Join(e.Game, "Mods", "Local"), testkit.Sample("tester.local", "Local"))
	scanNow(t, e)
	if _, err := e.App.Workshop(steamworks.Resubscribe, []string{"pkg:tester.local"}, true); err == nil {
		t.Fatal("accepted a local copy")
	}
}

func TestWorkshopResubscribePartialFailure(t *testing.T) {
	t.Parallel()
	for _, result := range []string{"failed:15", "timeout"} {
		t.Run(result, func(t *testing.T) {
			e := setup(t, false)
			for _, id := range []string{"8100", "8200", "8300"} {
				e.AddWorkshop(id, testkit.Sample("tester.m"+id, "Mod "+id))
			}
			scanNow(t, e)
			steamDLL(e)
			e.App.TrashFunc = os.RemoveAll
			var calls [][]string
			e.App.SteamWorker = func(_ string, args []string) (string, error) {
				calls = append(calls, slices.Clone(args))
				if args[0] == steamworks.Unsubscribe {
					return steamworks.ItemLine(8100, result, 1) + "\n" + steamworks.ItemLine(8200, "ok", 0) + "\n" + steamworks.ItemLine(8300, "ok", 0) + "\n" + steamworks.DoneLine(args[0], 3), nil
				}
				return steamworks.ItemLine(8200, "ok", 33) + "\n" + steamworks.ItemLine(8300, "failed:9", 0) + "\n" + steamworks.DoneLine(args[0], 2), nil
			}
			op, err := e.App.Workshop(steamworks.Resubscribe, []string{"ws:8100", "ws:8200", "ws:8300"}, false)
			if err == nil || op == nil || op.Items[0].Result != result || op.Items[1].Result != "ok" || op.Items[2].Result != "failed:9" || len(calls) != 2 || strings.Join(calls[1], " ") != "subscribe 8200 8300" {
				t.Fatalf("partial failure: %+v %v %v", op, err, calls)
			}
		})
	}
}

func TestWorkshopResubscribeCleanup(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"already gone", "gone during recycle", "sharing violation", "still present", "recycle error", "replacement link"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			e := setup(t, false)
			folder := e.AddWorkshop("8100", testkit.Sample("tester.repair", "Repair"))
			residue := e.AddWorkshop("8200", testkit.Sample("tester.residue", "Residue"))
			e.Unsubscribe("8200")
			e.WriteFile(filepath.Join(folder, "junk.txt"), "keep until recycled")
			scanNow(t, e)
			steamDLL(e)
			attempts := 0
			e.App.TrashFunc = func(path string) error {
				attempts++
				if filepath.Clean(path) != filepath.Clean(folder) {
					t.Fatalf("cleanup touched residue or link target: %s", path)
				}
				switch mode {
				case "still present":
					return nil // a successful shell reply alone is insufficient
				case "recycle error":
					return errors.New("recycle bin unavailable")
				case "sharing violation":
					if attempts == 1 {
						return &os.PathError{Op: "recycle", Path: path, Err: syscall.Errno(32)}
					}
				}
				if err := os.RemoveAll(path); err != nil {
					return err
				}
				if mode == "gone during recycle" {
					return os.ErrNotExist // Steam won the race with the shell
				}
				return nil
			}
			fail := mode == "still present" || mode == "recycle error" || mode == "replacement link"
			var calls [][]string
			fake := fakeSteamWorker(&calls)
			external := filepath.Join(e.Root, "External")
			e.App.SteamWorker = func(dir string, args []string) (string, error) {
				if args[0] == steamworks.Unsubscribe {
					if mode == "already gone" {
						if err := os.RemoveAll(folder); err != nil {
							t.Fatal(err)
						}
					}
					if mode == "replacement link" {
						e.Move(folder, external)
						e.Junction(folder, external)
					}
				} else {
					want := []string{steamworks.Subscribe, "8100", "8200"}
					if fail {
						want = []string{steamworks.Subscribe, "8200"}
					} else if _, err := os.Lstat(folder); !os.IsNotExist(err) {
						t.Fatalf("Subscribe before cleanup: %v", err)
					}
					if !slices.Equal(args, want) {
						t.Fatalf("Subscribe: got %v, want %v", args, want)
					}
				}
				return fake(dir, args)
			}
			op, err := e.App.Workshop(steamworks.Resubscribe, []string{"ws:8100", "ws:8200"}, false)
			if op == nil || (err != nil) != fail || len(calls) != 2 || op.Items[1].Result != "ok" {
				t.Fatalf("cleanup %s: %+v %v %v", mode, op, err, calls)
			}
			if fail {
				if !strings.HasPrefix(op.Items[0].Result, "failed:folder cleanup:") {
					t.Fatalf("missing cleanup failure: %+v", op.Items[0])
				}
				if mode != "replacement link" && (!strings.Contains(op.Items[0].Result, "still exists") || attempts < 2) {
					t.Fatalf("did not retry and verify removal: %+v attempts=%d", op.Items[0], attempts)
				}
			} else if op.Items[0].Result != "ok" || mode == "sharing violation" && attempts != 2 {
				t.Fatalf("cleanup: %+v attempts=%d", op.Items[0], attempts)
			}
			if mode == "already gone" && attempts != 0 || mode == "replacement link" && attempts != 0 {
				t.Fatalf("unexpected recycle call: %d", attempts)
			}
			if mode == "replacement link" {
				if _, err := os.Stat(filepath.Join(external, "junk.txt")); err != nil {
					t.Fatalf("link target was touched: %v", err)
				}
			}
			if _, err := os.Stat(residue); err != nil {
				t.Fatalf("residue path was changed: %v", err)
			}
		})
	}
}

func TestWorkshopResubscribeBatches(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	var refs []string
	for n := 0; n < 51; n++ {
		id := fmt.Sprint(9000 + n)
		e.AddWorkshop(id, testkit.Sample("tester.m"+id, "Mod "+id))
		refs = append(refs, "ws:"+id)
	}
	scanNow(t, e)
	steamDLL(e)
	var calls [][]string
	e.App.SteamWorker = fakeSteamWorker(&calls)
	e.App.TrashFunc = os.RemoveAll
	op, err := e.App.Workshop(steamworks.Resubscribe, refs, false)
	if err != nil || len(op.Items) != 51 || len(calls) != 4 {
		t.Fatalf("batches: %+v %v %v", op, err, calls)
	}
	for n, action := range []string{"unsubscribe", "unsubscribe", "subscribe", "subscribe"} {
		want := 52
		if n%2 == 1 {
			want = 3
		}
		if calls[n][1] != action || len(calls[n]) != want {
			t.Fatalf("call %d: %v", n, calls[n])
		}
	}
}

func TestWorkshopResubscribeRejectsLinkAndLock(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	folder := e.AddWorkshop("8100", testkit.Sample("tester.repair", "Repair"))
	scanNow(t, e)
	id := refID(t, e, "Repair", source.Workshop)
	moved := filepath.Join(e.Root, "External")
	e.Move(folder, moved)
	e.Junction(folder, moved)
	scanNow(t, e)
	if _, err := e.App.Workshop(steamworks.Resubscribe, []string{id}, true); err == nil {
		t.Fatal("accepted a linked Workshop copy")
	}
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	if lib.St.Ix.CanResubscribe(lib.St.Ix.ByID[id]) {
		t.Fatal("eligibility resolved away the link")
	}
	// A locked copy keeps its Workshop ID but must never drive Steam subscriptions.
	e.MustRun("pin", id)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	locks := st.Locked()
	if len(locks) != 1 {
		t.Fatalf("locks: %v", locks)
	}
	if _, err := e.App.Workshop(steamworks.Resubscribe, []string{locks[0].Copy.ID}, true); err == nil {
		t.Fatal("accepted a locked copy")
	}
}
