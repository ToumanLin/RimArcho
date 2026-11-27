package app_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// fakeWorkshopSteamCMD is a SteamCMD that answers each item by its login: anonymous runs are
// hidden (SteamCMDRun), signed-in runs have a console (SteamCMDStart) and write the log.
type fakeWorkshopSteamCMD struct {
	mu      sync.Mutex
	e       *testkit.Env
	exe     string
	answer  func(account, id string) string // "" downloads the item; else the ERROR reason
	runs    []fakeWorkshopRun
	signIns int
}

type fakeWorkshopRun struct {
	account string
	console bool
	ids     []string
}

func newFakeWorkshopSteamCMD(t *testing.T, e *testkit.Env, answer func(account, id string) string) *fakeWorkshopSteamCMD {
	f := &fakeWorkshopSteamCMD{e: e, exe: filepath.Join(e.Root, "tools", deps.SteamCMDExecutable()), answer: answer}
	e.MkdirAll(filepath.Join(filepath.Dir(f.exe), "logs"))
	e.App.FindTool = func(string) (string, bool) { return f.exe, true }
	e.App.SteamSignedIn = func() (string, bool) { return "", false }
	e.App.SteamCMDSignIn = func(open bool) {
		if open {
			f.mu.Lock()
			f.signIns++
			f.mu.Unlock()
		}
	}
	e.App.SteamCMDRun = func(exe string, args []string) (string, error) {
		return f.download(t, args, false), nil
	}
	e.App.SteamCMDStart = func(exe string, args []string, console bool) (app.SteamCMDProc, error) {
		return workshopProc(func() error {
			out := f.download(t, args, console)
			log, err := os.OpenFile(filepath.Join(filepath.Dir(f.exe), "logs", "console_log.txt"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			defer log.Close()
			_, err = fmt.Fprintln(log, out)
			return err
		}), nil
	}
	return f
}

func (f *fakeWorkshopSteamCMD) download(t *testing.T, args []string, console bool) string {
	ids := workshopScriptIDs(t, f.exe, args)
	account := args[3]
	f.mu.Lock()
	f.runs = append(f.runs, fakeWorkshopRun{account, console, ids})
	f.mu.Unlock()
	out := []string{"Connecting anonymously to Steam Public...OK"}
	if account != "anonymous" {
		out = []string{"Logging in user '" + account + "' to Steam Public...OK"}
	}
	out = append(out, "Waiting for user info...OK")
	for _, id := range ids {
		out = append(out, "Downloading item "+id+" ...")
		if why := f.answer(account, id); why != "" {
			out = append(out, "ERROR! Download item "+id+" failed ("+why+").")
			continue
		}
		f.e.WriteMod(filepath.Join(args[1], "steamapps", "workshop", "content", "294100", id), testkit.Sample("anon"+id+".mod", "Mod "+id))
		out = append(out, "Success. Downloaded item "+id+" to \""+args[1]+"\" (100 bytes)")
	}
	return strings.Join(out, "\n")
}

// 11 §3: a Workshop download runs anonymously even with a saved account, and signs in only
// for the items anonymous download is denied.
func TestWorkshopAnonymousFirst(t *testing.T) {
	t.Parallel()
	ids := []string{"8100", "8200", "8300"}
	downloaded := func(t *testing.T, op *app.SubscribeOp, want ...string) {
		t.Helper()
		for _, it := range op.Items {
			ok := it.Result == "downloaded with SteamCMD"
			if ok != slices.Contains(want, it.Record.WorkshopID) {
				t.Fatalf("item %s: %s (want downloaded: %v)", it.Record.WorkshopID, it.Result, want)
			}
		}
	}

	t.Run("all anonymous: no sign-in", func(t *testing.T) {
		e := setup(t, false)
		e.App.Store.SetMeta("cfg:steamcmd.account", "player")
		f := newFakeWorkshopSteamCMD(t, e, func(string, string) string { return "" })
		op, err := e.App.SubscribeWorkshop(ids, true, false)
		if err != nil {
			t.Fatal(err)
		}
		downloaded(t, op, ids...)
		if len(f.runs) != 1 || f.runs[0].account != "anonymous" || f.runs[0].console || f.signIns != 0 {
			t.Fatalf("runs %+v, sign-ins %d", f.runs, f.signIns)
		}
		for _, note := range op.Notes {
			if strings.Contains(note, "need the Steam account") {
				t.Fatalf("note %q", note)
			}
		}
	})

	t.Run("denied items retried signed in, only those", func(t *testing.T) {
		e := setup(t, false)
		e.App.Store.SetMeta("cfg:steamcmd.account", "player")
		f := newFakeWorkshopSteamCMD(t, e, func(account, id string) string {
			if account == "anonymous" && id == "8200" {
				return "AccessDenied"
			}
			if account == "anonymous" && id == "8300" {
				return "No subscription"
			}
			return ""
		})
		var last []app.TaskItem
		op, err := e.App.SubscribeWorkshop(ids, true, false, app.WorkshopCalls{Changed: func(items []app.TaskItem) { last = items }})
		if err != nil {
			t.Fatal(err)
		}
		downloaded(t, op, ids...)
		if len(f.runs) != 2 || f.runs[0].account != "anonymous" || f.runs[0].console ||
			f.runs[1].account != "player" || !f.runs[1].console || !slices.Equal(f.runs[1].ids, []string{"8200", "8300"}) || f.signIns != 1 {
			t.Fatalf("runs %+v, sign-ins %d", f.runs, f.signIns)
		}
		if note := fmt.Sprintf(app.WorkshopAccountNote, 2, "player", "8200, 8300"); !slices.Contains(op.Notes, note) {
			t.Fatalf("notes %q", op.Notes)
		}
		for _, it := range last {
			if it.State != app.ItemOK {
				t.Fatalf("task item %+v", it)
			}
		}
	})

	t.Run("denied and no account saved", func(t *testing.T) {
		e := setup(t, false)
		f := newFakeWorkshopSteamCMD(t, e, func(account, id string) string {
			if id == "8200" {
				return "AccessDenied"
			}
			return ""
		})
		var last []app.TaskItem
		op, err := e.App.SubscribeWorkshop(ids, true, false, app.WorkshopCalls{Changed: func(items []app.TaskItem) { last = items }})
		if err == nil || !strings.Contains(err.Error(), app.WorkshopNeedsAccount) {
			t.Fatalf("error %v", err)
		}
		downloaded(t, op, "8100", "8300")
		if it := op.Items[1]; it.FailureKind != "account" || !it.Permanent || it.Result != "failed: "+app.WorkshopNeedsAccount {
			t.Fatalf("item %+v", it)
		}
		if len(f.runs) != 1 || f.runs[0].account != "anonymous" || f.signIns != 0 {
			t.Fatalf("runs %+v, sign-ins %d", f.runs, f.signIns)
		}
		if i := slices.IndexFunc(last, func(it app.TaskItem) bool { return it.Ref == "ws:8200" }); i < 0 || last[i].FailureKind != "account" || last[i].State != app.ItemFail {
			t.Fatalf("task items %+v", last)
		}
	})

	t.Run("File Not Found never signs in", func(t *testing.T) {
		e := setup(t, false)
		e.App.Store.SetMeta("cfg:steamcmd.account", "player")
		f := newFakeWorkshopSteamCMD(t, e, func(account, id string) string {
			switch id {
			case "8100":
				return "File Not Found"
			case "8200":
				return "Not enough disk space"
			}
			return ""
		})
		op, err := e.App.SubscribeWorkshop(ids, true, false)
		if err == nil || !strings.Contains(err.Error(), "removed from the Workshop") {
			t.Fatalf("error %v", err)
		}
		downloaded(t, op, "8300")
		if op.Items[0].FailureKind != "missing" || op.Items[1].FailureKind != "disk" {
			t.Fatalf("items %+v", op.Items)
		}
		if len(f.runs) != 1 || f.runs[0].account != "anonymous" || f.signIns != 0 {
			t.Fatalf("runs %+v, sign-ins %d", f.runs, f.signIns)
		}
	})

	t.Run("explicit account skips the anonymous pass", func(t *testing.T) {
		e := setup(t, false)
		e.App.Store.SetMeta("cfg:steamcmd.account", "player")
		f := newFakeWorkshopSteamCMD(t, e, func(string, string) string { return "" })
		op, err := app.DownloadWorkshopAsForTest(e.App, ids, "other-player")
		if err != nil {
			t.Fatal(err)
		}
		downloaded(t, op, ids...)
		if len(f.runs) != 1 || f.runs[0].account != "other-player" || !f.runs[0].console || f.signIns != 1 {
			t.Fatalf("runs %+v, sign-ins %d", f.runs, f.signIns)
		}
	})

	t.Run("explicit anonymous stays anonymous", func(t *testing.T) {
		e := setup(t, false)
		e.App.Store.SetMeta("cfg:steamcmd.account", "player")
		f := newFakeWorkshopSteamCMD(t, e, func(account, id string) string {
			if account == "anonymous" && id == "8200" {
				return "AccessDenied"
			}
			return ""
		})
		op, err := app.DownloadWorkshopAsForTest(e.App, ids, "anonymous")
		if err == nil || op.Items[1].FailureKind != "account" {
			t.Fatalf("error %v, items %+v", err, op.Items)
		}
		if len(f.runs) != 1 || f.runs[0].account != "anonymous" || f.signIns != 0 {
			t.Fatalf("runs %+v, sign-ins %d", f.runs, f.signIns)
		}
	})
}

// Subscription updates share the batches and the rule: anonymous first, one signed-in run
// for the denied items.
func TestWorkshopUpdateAnonymousFirst(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	f := newFakeWorkshopSteamCMD(t, e, func(string, string) string { return "" })
	if _, err := e.App.SubscribeWorkshop([]string{"8100", "8200"}, true, false); err != nil {
		t.Fatal(err)
	}
	e.App.Store.SetMeta("cfg:steamcmd.account", "player")
	f.runs = nil
	f.answer = func(account, id string) string {
		if account == "anonymous" && id == "8100" {
			return "AccessDenied"
		}
		return ""
	}
	subs, err := e.App.Subscriptions()
	if err != nil || len(subs) != 2 {
		t.Fatalf("subscriptions %+v %v", subs, err)
	}
	refs := []string{subs[0].Ref, subs[1].Ref}
	if _, err := e.App.UpdateSubscriptions(refs, false); err != nil {
		t.Fatal(err)
	}
	if len(f.runs) != 2 || f.runs[0].account != "anonymous" || len(f.runs[0].ids) != 2 ||
		f.runs[1].account != "player" || !slices.Equal(f.runs[1].ids, []string{"8100"}) {
		t.Fatalf("runs %+v", f.runs)
	}
}
