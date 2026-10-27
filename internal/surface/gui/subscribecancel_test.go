package gui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

type subscriptionProcess struct {
	wait   func() error
	killed chan struct{}
	once   sync.Once
	kills  int
}

func (p *subscriptionProcess) Wait() error { return p.wait() }
func (p *subscriptionProcess) KillTree() error {
	p.once.Do(func() { p.kills++; close(p.killed) })
	return nil
}
func (p *subscriptionProcess) Kill() error { return errors.New("must kill invocation tree") }

func TestSubscribeCancelPreservesSuccessAndResumesRemaining(t *testing.T) {
	e, api, _ := fixture(t)
	exe := filepath.Join(e.Root, "steamcmd.exe")
	e.App.FindTool = func(string) (string, bool) { return exe, true }
	e.App.SteamCMDPoll = time.Millisecond
	e.App.SteamCMDWait = func(time.Duration) {}
	p := &subscriptionProcess{killed: make(chan struct{})}
	p.wait = func() error {
		e.WriteMod(filepath.Join(e.App.SteamCMDModsDir(), "steamapps", "workshop", "content", "294100", "8100"), testkit.Sample("cancel.first", "First Mod"))
		log := filepath.Join(e.Root, "logs", "console_log.txt")
		if err := os.MkdirAll(filepath.Dir(log), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(log, []byte("Success. Downloaded item 8100\n"), 0600); err != nil {
			return err
		}
		<-p.killed
		return errors.New("cancelled")
	}
	e.App.SteamCMDStart = func(_ string, args []string, console bool) (app.SteamCMDProc, error) {
		if console {
			t.Error("anonymous download opened a console")
		}
		return p, nil
	}
	if _, err := api.Subscribe(SubscriptionRequest{Kind: "workshop", URL: "8100 8200", SteamCMD: true, Batch: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.KillTree() })
	var running view.TaskRow
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		page := api.Tasks()
		if len(page.Rows) == 1 && len(page.Rows[0].Items) == 2 && page.Rows[0].Items[0].State == app.ItemOK {
			running = page.Rows[0]
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if running.ID == "" || running.State != app.TaskRunning || len(running.Steps) != 0 {
		t.Fatalf("running %+v", running)
	}
	if _, err := api.TaskAct("stop", []string{running.ID}, ""); err != nil {
		t.Fatal(err)
	}
	ended := endedTask(t, api, view.TaskSubscribe)
	if ended.State != app.TaskStopped || len(ended.Items) != 2 || ended.Items[0].State != app.ItemOK || ended.Items[1].State != app.ItemSkip || p.kills != 1 {
		t.Fatalf("ended %+v kills=%d", ended, p.kills)
	}
	if !hasAct(ended, "resume") || hasAct(ended, "retry") {
		t.Fatalf("cancel actions %+v", ended.Acts)
	}
	subs, err := e.App.Subscriptions()
	if err != nil || len(subs) != 1 {
		t.Fatalf("successful download not scanned: %v %v", subs, err)
	}
	batches, err := e.App.Store.CreditBatches("")
	if err != nil || len(batches) != 1 {
		t.Fatalf("successful download not credited: %v %v", batches, err)
	}
	files, err := filepath.Glob(filepath.Join(e.Root, "*.txt"))
	if err != nil || len(files) != 0 {
		t.Fatalf("runscript not cleaned: %v %v", files, err)
	}
	var resumed []string
	e.App.SteamCMDRun = func(exe string, args []string) (string, error) {
		data, err := os.ReadFile(filepath.Join(filepath.Dir(exe), args[5]))
		if err != nil {
			return "", err
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 3 && fields[0] == "workshop_download_item" {
				resumed = append(resumed, fields[2])
			}
		}
		e.WriteMod(filepath.Join(args[1], "steamapps", "workshop", "content", "294100", "8200"), testkit.Sample("cancel.second", "Second Mod"))
		return "Success. Downloaded item 8200", nil
	}
	if _, err := api.TaskAct("resume", []string{ended.ID}, ""); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	var next view.TaskRow
	for time.Now().Before(deadline) {
		for _, r := range api.Tasks().Rows {
			if r.ID != ended.ID && r.State == app.TaskDone {
				next = r
			}
		}
		if next.ID != "" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if next.ID == "" || fmt.Sprint(resumed) != "[8200]" || len(api.Tasks().Rows) != 2 {
		t.Fatalf("resumed=%v next=%+v", resumed, next)
	}
	if _, err := api.TaskAct("retry", []string{ended.ID}, ended.Items[0].Ref); err == nil {
		t.Fatal("completed item was resubmitted")
	}
}

func TestSubscriptionRetrySkipsPermanentFailures(t *testing.T) {
	e, api, _ := fixture(t)
	exe := filepath.Join(e.Root, "steamcmd.exe")
	e.App.FindTool = func(string) (string, bool) { return exe, true }
	e.App.SteamCMDWait = func(time.Duration) {}
	e.App.SteamCMDRun = func(string, []string) (string, error) {
		return "ERROR! Download item 8100 failed (File Not Found).\nERROR! Timeout downloading item 8200", nil
	}
	if _, err := api.Subscribe(SubscriptionRequest{Kind: "workshop", URL: "8100 8200", SteamCMD: true, Batch: true}); err != nil {
		t.Fatal(err)
	}
	failed := endedTask(t, api, view.TaskSubscribe)
	if failed.State != app.TaskFailed || !failed.Items[0].Permanent || failed.Items[1].Permanent {
		t.Fatalf("failed %+v", failed)
	}
	if _, err := api.TaskAct("retry", []string{failed.ID}, failed.Items[0].Ref); err == nil {
		t.Fatal("permanent item could be retried")
	}
	var requested []string
	e.App.SteamCMDRun = func(exe string, args []string) (string, error) {
		b, err := os.ReadFile(filepath.Join(filepath.Dir(exe), args[5]))
		if err != nil {
			return "", err
		}
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) == 3 && f[0] == "workshop_download_item" {
				requested = append(requested, f[2])
			}
		}
		e.WriteMod(filepath.Join(args[1], "steamapps", "workshop", "content", "294100", "8200"), testkit.Sample("retry.only", "Retry Mod"))
		return "Success. Downloaded item 8200", nil
	}
	if _, err := api.TaskAct("retry", []string{failed.ID}, ""); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, r := range api.Tasks().Rows {
			if r.ID != failed.ID && r.State == app.TaskDone {
				if fmt.Sprint(requested) != "[8200]" || len(r.Items) != 1 {
					t.Fatalf("requested %v result %+v", requested, r)
				}
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("retry did not finish")
}

func TestBulkSubscriptionStaysOneTaskAcrossProcesses(t *testing.T) {
	e, api, _ := fixture(t)
	exe := filepath.Join(e.Root, "steamcmd.exe")
	e.App.FindTool = func(string) (string, bool) { return exe, true }
	var ids []string
	for i := 0; i < 26; i++ {
		ids = append(ids, fmt.Sprint(8100+i))
	}
	processes := 0
	e.App.SteamCMDRun = func(exe string, args []string) (string, error) {
		processes++
		b, err := os.ReadFile(filepath.Join(filepath.Dir(exe), args[5]))
		if err != nil {
			return "", err
		}
		var out []string
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) != 3 || f[0] != "workshop_download_item" {
				continue
			}
			id := f[2]
			e.WriteMod(filepath.Join(args[1], "steamapps", "workshop", "content", "294100", id), testkit.Sample("bulk."+id, "Mod "+id))
			out = append(out, "Success. Downloaded item "+id)
		}
		return strings.Join(out, "\n"), nil
	}
	if _, err := api.Subscribe(SubscriptionRequest{Kind: "workshop", URL: strings.Join(ids, " "), SteamCMD: true, Batch: true}); err != nil {
		t.Fatal(err)
	}
	r := endedTask(t, api, view.TaskSubscribe)
	if r.State != app.TaskDone || processes != 2 || len(api.Tasks().Rows) != 1 || len(r.Steps) != 0 || len(r.Items) != 26 || r.Count != "26 / 26" {
		t.Fatalf("processes=%d task=%+v", processes, r)
	}
}
