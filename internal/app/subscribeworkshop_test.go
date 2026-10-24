package app_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

type workshopProc func() error

func (p workshopProc) Wait() error { return p() }

func TestSubscribeWorkshopFallback(t *testing.T) {
	t.Run("Steam available", func(t *testing.T) {
		e := setup(t, false)
		steamDLL(e)
		var calls [][]string
		e.App.SteamWorker = fakeSteamWorker(&calls)
		e.App.FindTool = func(string) (string, bool) { t.Fatal("SteamCMD lookup on Steam success"); return "", false }
		e.App.SteamCMDRun = func(string, []string) (string, error) { t.Fatal("SteamCMD called"); return "", nil }
		e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42} }
		out := e.MustRun("subscribe", "workshop", "https://steamcommunity.com/sharedfiles/filedetails/?id=8100")
		if len(calls) != 1 || strings.Join(calls[0][1:], " ") != "subscribe 8100" || !strings.Contains(out, "next scan") {
			t.Fatalf("Steam: %v %s", calls, out)
		}
		if _, err := os.Stat(e.App.SteamCMDModsDir()); !os.IsNotExist(err) {
			t.Fatal("SteamCMD directory created")
		}
	})
	t.Run("Steam refused", func(t *testing.T) {
		e := setup(t, false)
		steamDLL(e)
		e.App.SteamWorker = func(string, []string) (string, error) {
			return steamworks.ItemLine(8100, "failed:15", 0) + "\n" + steamworks.DoneLine("subscribe", 1), nil
		}
		e.App.FindTool = func(string) (string, bool) { t.Fatal("fallback for item refusal"); return "", false }
		if _, err := e.App.SubscribeWorkshop([]string{"8100"}, false, false); err == nil {
			t.Fatal("Steam refusal ignored")
		}
	})
	t.Run("not found", func(t *testing.T) {
		e := setup(t, false)
		e.App.FindTool = func(name string) (string, bool) {
			if name != "steamcmd" {
				t.Fatal(name)
			}
			return "", false
		}
		if _, err := e.App.SubscribeWorkshop([]string{"8100"}, false, false); err == nil || !strings.Contains(err.Error(), "Settings") || !strings.Contains(err.Error(), "https://steamcommunity.com/sharedfiles/filedetails/?id=8100") {
			t.Fatalf("missing: %v", err)
		}
	})
	for _, forced := range []bool{false, true} {
		t.Run(fmt.Sprintf("download forced=%v", forced), func(t *testing.T) {
			e := setup(t, false)
			if forced {
				e.App.SteamWorker = func(string, []string) (string, error) { t.Fatal("forced SteamCMD called Steam"); return "", nil }
			} else {
				steamDLL(e)
				e.App.SteamWorker = func(string, []string) (string, error) {
					return steamworks.ErrorLine("Steam unavailable"), errors.New("worker")
				}
			}
			exe := filepath.Join(e.Root, "tools", "steamcmd.exe")
			e.App.FindTool = func(name string) (string, bool) { return exe, name == "steamcmd" }
			var calls [][]string
			e.App.SteamCMDRun = func(gotExe string, args []string) (string, error) {
				if gotExe != exe {
					t.Fatal(gotExe)
				}
				calls = append(calls, args)
				var out []string
				for i, arg := range args {
					if arg == "+workshop_download_item" {
						id := args[i+2]
						p := filepath.Join(args[1], "steamapps", "workshop", "content", "294100", id)
						e.WriteMod(p, testkit.Sample("example.workshop"+id, "Example Workshop "+id))
						out = append(out, "Success. Downloaded item "+id)
					}
				}
				return strings.Join(out, "\n"), nil
			}
			if _, err := e.App.SubscribeWorkshop([]string{"8100", "8200"}, true, true); err != nil || len(calls) != 0 {
				t.Fatalf("dry run: %v %v", err, calls)
			}
			e.MustRun("subscribe", "workshop", "8100", "--steamcmd", "--dry-run")
			if _, err := os.Stat(e.App.SteamCMDModsDir()); !os.IsNotExist(err) {
				t.Fatal("dry run wrote files")
			}
			op, err := e.App.SubscribeWorkshop([]string{"8100", "8200"}, forced, false)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"+force_install_dir", e.App.SteamCMDModsDir(), "+login", "anonymous", "+workshop_download_item", "294100", "8100", "+workshop_download_item", "294100", "8200", "+quit"}
			if len(calls) != 1 || !slices.Equal(calls[0], want) {
				t.Fatalf("args: %v", calls)
			}
			ss, err := e.App.Subscriptions()
			if err != nil || len(ss) != 2 {
				t.Fatalf("subscriptions: %v %v", ss, err)
			}
			for _, s := range ss {
				r, err := subscribe.Read(s.Folder)
				if err != nil || r.Kind != "steamcmd" || r.WorkshopID != filepath.Base(s.Folder) || r.Installed != e.Now().Format(time.RFC3339) || s.Mod.Kind != source.SteamCMD || s.Ref == "" {
					t.Fatalf("record: %+v %v", r, err)
				}
			}
			check, err := e.App.CheckSubscriptions(nil)
			if err != nil || !strings.Contains(check.Items[0].Result, "unavailable offline") {
				t.Fatalf("check: %+v %v", check, err)
			}
			if _, err := e.App.CheckSubscriptions([]string{ss[0].Ref}); err == nil {
				t.Fatal("explicit unsupported check accepted")
			}
			e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42} }
			if _, err := e.App.SubscribeWorkshop([]string{"8300"}, true, false); err == nil || !strings.Contains(err.Error(), "game is running") {
				t.Fatalf("game running: %v", err)
			}
			e.App.GameProbe = func(string) *app.GameRun { return nil }
			e.Advance(time.Hour)
			if _, err := e.App.UpdateSubscriptions([]string{ss[0].Ref}, true); err != nil || len(calls) != 1 {
				t.Fatalf("update dry run: %v %v", err, calls)
			}
			if _, err := e.App.UpdateSubscriptions([]string{ss[0].Ref}, false); err != nil {
				t.Fatal(err)
			}
			updated, _ := e.App.Subscriptions()
			if updated[0].Mod.ID != ss[0].Mod.ID || updated[0].Record.Installed == ss[0].Record.Installed || len(calls) != 2 || op.Items[0].Ref == "" {
				t.Fatalf("update identity: %+v", updated)
			}
		})
	}
	t.Run("existing arbitrary content root", func(t *testing.T) {
		e := setup(t, false)
		root := filepath.Join(e.Root, "ExistingWorkshopMods")
		target := filepath.Join(root, "8100")
		e.WriteMod(target, testkit.Sample("example.existing", "Existing Workshop Mod"))
		if _, err := e.App.FolderAdd(source.SteamCMD, root); err != nil {
			t.Fatal(err)
		}
		scanNow(t, e)
		before, err := e.App.Subscriptions()
		if err != nil || len(before) != 1 {
			t.Fatalf("existing subscriptions: %+v %v", before, err)
		}
		e.App.FindTool = func(string) (string, bool) { return "fake-steamcmd", true }
		e.App.SteamCMDRun = func(_ string, args []string) (string, error) {
			p := filepath.Join(args[1], "steamapps", "workshop", "content", "294100", "8100")
			e.WriteMod(p, testkit.Sample("example.existing", "Existing Workshop Mod"))
			e.WriteFile(filepath.Join(p, "updated.xml"), "<Defs/>")
			return "Success. Downloaded item 8100", nil
		}
		var trashed []string
		e.App.TrashFunc = func(path string) error {
			trashed = append(trashed, path)
			return os.Rename(path, filepath.Join(e.Root, "old-workshop-item"))
		}
		if _, err := e.App.UpdateSubscriptions([]string{before[0].Ref}, false); err != nil {
			t.Fatal(err)
		}
		after, err := e.App.Subscriptions()
		if err != nil || len(after) != 1 || after[0].Mod.ID != before[0].Mod.ID || len(trashed) != 1 {
			t.Fatalf("replacement: %+v %v trash %v", after, err, trashed)
		}
		if _, err := os.Stat(filepath.Join(target, "updated.xml")); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("account console and failure", func(t *testing.T) {
		e := setup(t, false)
		exe := filepath.Join(e.Root, "tools", "steamcmd.exe")
		e.App.FindTool = func(string) (string, bool) { return exe, true }
		e.App.Store.SetMeta("cfg:steamcmd.account", "example-player")
		e.App.SteamCMDRun = func(string, []string) (string, error) { t.Fatal("named account ran hidden"); return "", nil }
		e.App.SteamCMDStart = func(_ string, args []string, console bool) (app.SteamCMDProc, error) {
			if !console || args[3] != "example-player" {
				t.Fatalf("account args %v console %v", args, console)
			}
			return workshopProc(func() error {
				e.WriteFile(filepath.Join(filepath.Dir(exe), "logs", "console_log.txt"), "ERROR! Download item 8100 failed (No subscription)\n")
				return nil
			}), nil
		}
		if _, err := e.App.SubscribeWorkshop([]string{"8100"}, true, false); err == nil || !strings.Contains(err.Error(), "No subscription") {
			t.Fatalf("failure: %v", err)
		}
		ss, _ := e.App.Folders()
		for _, s := range ss {
			if s.Kind == source.SteamCMD {
				t.Fatal("failed download registered source")
			}
		}
	})
}
