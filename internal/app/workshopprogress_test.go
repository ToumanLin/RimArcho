package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type workshopCancelableProc struct {
	wait      func() error
	killed    chan struct{}
	once      sync.Once
	treeKills int
}

func (p *workshopCancelableProc) Wait() error { return p.wait() }
func (p *workshopCancelableProc) KillTree() error {
	p.once.Do(func() { p.treeKills++; close(p.killed) })
	return nil
}
func (p *workshopCancelableProc) Kill() error { return errors.New("must kill owned tree") }

func TestWorkshopLogProgressBeforeExitAndCancellation(t *testing.T) {
	base := t.TempDir()
	exe := filepath.Join(base, "steamcmd.exe")
	log := filepath.Join(base, "logs", "console_log.txt")
	if err := os.MkdirAll(filepath.Dir(log), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(log, []byte("Success. Downloaded item 9999\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observed := make(chan struct{})
	var once sync.Once
	w := &workshopTransfer{ctx: ctx, items: []TaskItem{{Ref: "ws:8100", Name: "First", State: ItemWait, Size: 100}, {Ref: "ws:8200", Name: "Second", State: ItemWait, Size: 300}}}
	progress := &Task{}
	w.changed = func(items []TaskItem) {
		progress.SetWorkshopProgress(items)
		if items[0].State == ItemOK {
			once.Do(func() { close(observed) })
		}
	}
	p := &workshopCancelableProc{killed: make(chan struct{})}
	p.wait = func() error {
		f, err := os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer f.Close()
		fmt.Fprint(f, "Success. Downloaded item 81")
		fmt.Fprint(f, "00\n")
		select {
		case <-observed:
		case <-time.After(3 * time.Second):
			return errors.New("progress waited for process exit")
		}
		cancel()
		<-p.killed
		fmt.Fprint(f, "ERROR! Download item 8200 failed (File Not Found).")
		return errors.New("cancelled process")
	}
	a := &App{SteamCMDPoll: time.Millisecond, SteamCMDStart: func(string, []string, bool) (SteamCMDProc, error) { return p, nil }}
	out, err := a.runWorkshopSteamCMD(exe, nil, false, w)
	if !errors.Is(err, context.Canceled) || p.treeKills != 1 || strings.Contains(out, "9999") || w.items[1].FailureKind != "missing" {
		t.Fatalf("out=%s err=%v kills=%d items=%+v", out, err, p.treeKills, w.items)
	}
	if progress.Done != 2 || progress.Frac != 1 {
		t.Fatalf("progress %+v", progress)
	}
	// The same executable lock is released even when cancelled.
	a.SteamCMDRun = func(string, []string) (string, error) { return "Success. Downloaded item 8100", nil }
	if _, err := a.runWorkshopSteamCMD(exe, nil, false); err != nil {
		t.Fatal(err)
	}
}

func TestWorkshopRetryRoundsAndMonotonicProgress(t *testing.T) {
	base := t.TempDir()
	exe := filepath.Join(base, "steamcmd.exe")
	var ids []string
	w := &workshopTransfer{ctx: context.Background()}
	for i := 0; i < 28; i++ {
		id := fmt.Sprint(8100 + i)
		ids = append(ids, id)
		w.items = append(w.items, TaskItem{Ref: "ws:" + id, Name: "Mod " + id, Size: int64(i + 1), State: ItemWait})
	}
	progress := &Task{}
	var previous float64
	w.changed = func(items []TaskItem) {
		progress.SetWorkshopProgress(items)
		if progress.Frac < previous {
			t.Fatalf("progress reversed %f -> %f", previous, progress.Frac)
		}
		previous = progress.Frac
	}
	var calls [][]string
	a := &App{SteamCMDWait: func(time.Duration) {}}
	a.SteamCMDRun = func(_ string, args []string) (string, error) {
		part := scriptIDs(t, exe, args)
		calls = append(calls, part)
		var out []string
		for _, id := range part {
			switch {
			case id == "8100":
				out = append(out, "ERROR! Download item "+id+" failed (File Not Found).")
			case id == "8101":
				out = append(out, "ERROR! Download item "+id+" failed (Not enough disk space).")
			case id == "8102" && len(calls) <= 2:
				out = append(out, "ERROR! Timeout downloading item "+id)
			case id == "8125" && len(calls) <= 2: // unconfirmed crash in the second internal batch
			case id == "8126" && len(calls) <= 2:
				out = append(out, "ERROR! Download item "+id+" failed (Failure).")
			default:
				out = append(out, "Success. Downloaded item "+id)
			}
		}
		if len(calls) == 2 {
			return strings.Join(out, "\n"), errors.New("exit status 0xc0000374")
		}
		return strings.Join(out, "\n"), nil
	}
	results, err := a.runWorkshopBatches(exe, base, "anonymous", ids, w)
	if err != nil || len(calls) != 3 || !slices.Equal(calls[0], ids[:25]) || !slices.Equal(calls[1], ids[25:]) || !slices.Equal(calls[2], []string{"8102", "8125", "8126"}) {
		t.Fatalf("calls=%v err=%v", calls, err)
	}
	if !results["8102"].confirmed || !results["8125"].confirmed || !results["8126"].confirmed || !workshopPermanent(results["8100"].kind) || !workshopPermanent(results["8101"].kind) {
		t.Fatalf("outcomes %+v", results)
	}
	if progress.Total != 28 || progress.Done != 28 || progress.Frac != 1 {
		t.Fatalf("progress %+v", progress)
	}
}
