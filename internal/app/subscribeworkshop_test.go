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
	t.Parallel()
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
			e.MkdirAll(filepath.Dir(exe))
			e.App.FindTool = func(name string) (string, bool) { return exe, name == "steamcmd" }
			var calls [][]string
			e.App.SteamCMDRun = func(gotExe string, args []string) (string, error) {
				if gotExe != exe {
					t.Fatal(gotExe)
				}
				calls = append(calls, args)
				var out []string
				for _, id := range workshopScriptIDs(t, exe, args) {
					p := filepath.Join(args[1], "steamapps", "workshop", "content", "294100", id)
					e.WriteMod(p, testkit.Sample("example.workshop"+id, "Example Workshop "+id))
					out = append(out, "Success. Downloaded item "+id)
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
			if len(calls) != 1 || !slices.Equal(calls[0][:5], []string{"+force_install_dir", e.App.SteamCMDModsDir(), "+login", "anonymous", "+runscript"}) {
				t.Fatalf("args: %v", calls)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(exe), calls[0][5])); !os.IsNotExist(err) {
				t.Fatalf("script remains: %v", err)
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
		e.App.SteamCMDRun = func(exe string, args []string) (string, error) {
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
		e.MkdirAll(filepath.Dir(exe))
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
		if _, err := e.App.SubscribeWorkshop([]string{"8100"}, true, false); err == nil || !strings.Contains(err.Error(), "does not own RimWorld") || strings.Contains(err.Error(), "No subscription") {
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

// Read while the runner is active; after it returns, the unique script must be gone.
func workshopScriptIDs(t *testing.T, exe string, args []string) []string {
	t.Helper()
	if len(args) != 6 || args[4] != "+runscript" {
		t.Fatalf("expected runscript: %v", args)
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(exe), args[5]))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "@ShutdownOnFailedCommand 0\n@NoPromptForPassword 0\n") || !strings.HasSuffix(string(b), "quit\n") {
		t.Fatalf("script policy: %s", b)
	}
	var ids []string
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "workshop_download_item" {
			if len(fields) != 3 || fields[1] != "294100" {
				t.Fatalf("invalid download command: %s", line)
			}
			ids = append(ids, fields[2])
		}
	}
	return ids
}

func TestWorkshopBatchesPartialSuccess(t *testing.T) {
	e := setup(t, false)
	e.App.FindTool = func(string) (string, bool) { return filepath.Join(e.Root, "steamcmd.exe"), true }
	var ids, scripts []string
	for i := 1; i <= 52; i++ {
		ids = append(ids, fmt.Sprint(9000+i))
	}
	var sizes []int
	e.App.SteamCMDRun = func(exe string, args []string) (string, error) {
		batch := workshopScriptIDs(t, exe, args)
		sizes = append(sizes, len(batch))
		if slices.Contains(scripts, filepath.Join(filepath.Dir(exe), args[5])) {
			t.Fatal("script name reused")
		}
		for _, previous := range scripts {
			if _, err := os.Stat(previous); !os.IsNotExist(err) {
				t.Fatalf("previous script remains: %v", err)
			}
		}
		scripts = append(scripts, filepath.Join(filepath.Dir(exe), args[5]))
		// No provenance or scan until all batches have finished.
		for _, id := range ids {
			if _, err := os.Stat(filepath.Join(args[1], "steamapps", "workshop", "content", "294100", id, subscribe.File)); !os.IsNotExist(err) {
				t.Fatal("provenance written between batches")
			}
		}
		var out []string
		for _, id := range batch {
			if id == "9002" {
				out = append(out, "Success. Downloaded item "+id, "ERROR! Download item "+id+" failed (AccessDenied).")
				continue
			}
			if id != "9027" { // A success line alone is insufficient without About.xml.
				e.WriteMod(filepath.Join(args[1], "steamapps", "workshop", "content", "294100", id), testkit.Sample("batch.mod"+id, id))
			}
			out = append(out, "Success. Downloaded item "+id)
		}
		return strings.Join(out, "\n"), nil // Exit zero, even on item failure.
	}
	op, err := e.App.SubscribeWorkshop(ids, true, false)
	if err == nil || !strings.Contains(err.Error(), "9002") || !strings.Contains(err.Error(), "9027") {
		t.Fatalf("partial error: %v", err)
	}
	if !slices.Equal(sizes, []int{25, 25, 2}) || len(op.Items) != 52 {
		t.Fatalf("batches: %v, items: %d", sizes, len(op.Items))
	}
	for i, item := range op.Items {
		if i == 1 || i == 26 {
			if !strings.HasPrefix(item.Result, "failed:") || item.To != "" {
				t.Fatalf("failed item: %+v", item)
			}
		} else if item.Ref == "" || item.To == "" || item.Result != "downloaded with SteamCMD" {
			t.Fatalf("lost success: %+v", item)
		}
	}
	ss, err := e.App.Subscriptions()
	if err != nil || len(ss) != 50 {
		t.Fatalf("successful subscriptions: %d %v", len(ss), err)
	}
	for _, script := range scripts {
		if _, err := os.Stat(script); !os.IsNotExist(err) {
			t.Fatalf("script remains: %v", err)
		}
	}
}

func TestWorkshopRetryClassification(t *testing.T) {
	for _, tc := range []struct {
		name, output, reason string
		calls                int
	}{
		{"timeout", "ERROR! Timeout downloading item 8100", "could not be reached", 3},
		{"connection", "ERROR! Download item 8100 failed (NoConnection).", "could not be reached", 3},
		{"global connection", "ERROR! Failed to connect to Steam servers", "could not be reached", 3},
		{"split connection", "Connecting anonymously to Steam Public...\nFAILED (No Connection)", "could not be reached", 3},
		{"password", "Logging in user 'player'...FAILED (Invalid Password)", "password", 1},
		{"guard", "ERROR! InvalidAuthCode", "Steam Guard", 1},
		{"permission", "ERROR! Download item 8100 failed (AccessDenied).", "access was denied", 1},
		{"invisible", "ERROR! Download item 8100 failed (FileNotFound).", "no longer on the Workshop", 1},
		{"disk", "ERROR! Download item 8100 failed (Not enough disk space).", "disk is full", 1},
		{"unknown", "ERROR! Download item 8100 failed (Failure).", "did not finish", 1},
		{"unconfirmed", "", "not confirmed", 3},
		{"rate limit", "ERROR! Rate limit exceeded", "did not finish", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setup(t, false)
			e.App.FindTool = func(string) (string, bool) { return filepath.Join(e.Root, "steamcmd.exe"), true }
			var waits []time.Duration
			e.App.SteamCMDWait = func(d time.Duration) { waits = append(waits, d) }
			var scripts []string
			e.App.SteamCMDRun = func(exe string, args []string) (string, error) {
				workshopScriptIDs(t, exe, args)
				scripts = append(scripts, filepath.Join(filepath.Dir(exe), args[5]))
				return tc.output, nil
			}
			op, err := e.App.SubscribeWorkshop([]string{"8100"}, true, false)
			if err == nil || !strings.Contains(err.Error(), tc.reason) || !strings.HasPrefix(op.Items[0].Result, "failed:") || len(scripts) != tc.calls {
				t.Fatalf("error %v, result %+v, calls %d", err, op, len(scripts))
			}
			if tc.calls == 3 && !slices.Equal(waits, []time.Duration{5 * time.Second, 15 * time.Second}) || tc.calls == 1 && len(waits) != 0 {
				t.Fatalf("backoff %v", waits)
			}
			for _, script := range scripts {
				if _, err := os.Stat(script); !os.IsNotExist(err) {
					t.Fatalf("script remains: %v", err)
				}
			}
		})
	}
}

func TestWorkshopRetryOnlyFailedIDs(t *testing.T) {
	e := setup(t, false)
	e.App.FindTool = func(string) (string, bool) { return filepath.Join(e.Root, "steamcmd.exe"), true }
	e.App.SteamCMDWait = func(time.Duration) {}
	var calls [][]string
	e.App.SteamCMDRun = func(exe string, args []string) (string, error) {
		ids := workshopScriptIDs(t, exe, args)
		calls = append(calls, ids)
		var out []string
		for _, id := range ids {
			if id == "8200" && len(calls) < 3 {
				out = append(out, "ERROR! Timeout downloading item "+id)
				continue
			}
			if id == "8300" {
				out = append(out, "ERROR! Download item "+id+" failed (No subscription).")
				continue
			}
			e.WriteMod(filepath.Join(args[1], "steamapps", "workshop", "content", "294100", id), testkit.Sample("retry.mod"+id, id))
			out = append(out, "Success! Downloaded item "+id)
		}
		return strings.Join(out, "\n"), nil
	}
	op, err := e.App.SubscribeWorkshop([]string{"8100", "8200", "8300"}, true, false)
	if err == nil || strings.Contains(err.Error(), "8200") || len(calls) != 3 || !slices.Equal(calls[1], []string{"8200"}) || !slices.Equal(calls[2], []string{"8200"}) {
		t.Fatalf("retry calls %v, error %v", calls, err)
	}
	if op.Items[0].Ref == "" || op.Items[1].Ref == "" || !strings.HasPrefix(op.Items[2].Result, "failed:") {
		t.Fatalf("partial outcomes: %+v", op.Items)
	}
}

func TestWorkshopScriptCleanupOnStartFailure(t *testing.T) {
	for _, account := range []string{"anonymous", "player"} {
		t.Run(account, func(t *testing.T) {
			e := setup(t, false)
			e.App.Store.SetMeta("cfg:steamcmd.account", account)
			e.App.FindTool = func(string) (string, bool) { return filepath.Join(e.Root, "steamcmd.exe"), true }
			var script string
			exe := filepath.Join(e.Root, "steamcmd.exe")
			fail := func(args []string) (string, error) {
				workshopScriptIDs(t, exe, args)
				script = filepath.Join(filepath.Dir(exe), args[5])
				return "", errors.New("process start failed")
			}
			e.App.SteamCMDRun = func(exe string, args []string) (string, error) { return fail(args) }
			e.App.SteamCMDStart = func(_ string, args []string, console bool) (app.SteamCMDProc, error) {
				if !console {
					t.Fatal("named account lost native console")
				}
				_, err := fail(args)
				return nil, err
			}
			if _, err := e.App.SubscribeWorkshop([]string{"8100"}, true, false); err == nil {
				t.Fatal("process failure ignored")
			}
			if script == "" {
				t.Fatal("runner not called")
			}
			if _, err := os.Stat(script); !os.IsNotExist(err) {
				t.Fatalf("script remains: %v", err)
			}
		})
	}
}

func TestWorkshopSuccessWithProcessError(t *testing.T) {
	e := setup(t, false)
	e.App.FindTool = func(string) (string, bool) { return filepath.Join(e.Root, "steamcmd.exe"), true }
	e.App.SteamCMDRun = func(exe string, args []string) (string, error) {
		workshopScriptIDs(t, exe, args)
		e.WriteMod(filepath.Join(args[1], "steamapps", "workshop", "content", "294100", "8100"), testkit.Sample("exit.mod", "Exit Mod"))
		return "Success. Downloaded item 8100", errors.New("process exit failed")
	}
	op, err := e.App.SubscribeWorkshop([]string{"8100"}, true, false)
	if err == nil || !strings.Contains(err.Error(), "did not exit successfully") || op.Items[0].Ref == "" || op.Items[0].Result != "downloaded with SteamCMD" {
		t.Fatalf("confirmed result lost: %+v %v", op, err)
	}
}
