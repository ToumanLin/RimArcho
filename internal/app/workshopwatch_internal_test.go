package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

// The helper is a real child process with no Steam DLL or API calls.
func TestSteamStreamHelper(t *testing.T) {
	if os.Getenv("RIMARCHO_FAKE_WATCH") != "1" {
		return
	}
	fmt.Println("PROGRESS 1 45 100 21")
	if os.Getenv("RIMARCHO_FAKE_WATCH_EOF") == "1" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestSteamStreamingIsLiveAndCancellationReapsWorker(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSteamStreamHelper$")
	cmd.Env = append(os.Environ(), "RIMARCHO_FAKE_WATCH=1")
	seen := false
	err := streamSteamCommand(cmd, func(line string) {
		if line == "PROGRESS 1 45 100 21" {
			seen = true
			cancel()
		}
	})
	if !seen || err == nil || cmd.ProcessState == nil || ctx.Err() == nil {
		t.Fatalf("seen=%v err=%v process=%v", seen, err, cmd.ProcessState)
	}
}

func TestTaskDownloadProgressAndRetry(t *testing.T) {
	t.Parallel()
	ts := OpenTasks("")
	hold := make(chan struct{})
	reported := make(chan struct{})
	ts.Batch = func(kind string, refs []string, b BatchCalls) bool {
		b.Start(0)
		b.Start(1)
		b.Progress(0, 45, 100)
		b.Done(1, nil)
		close(reported)
		<-hold
		b.Done(0, fmt.Errorf("check Steam"))
		return true
	}
	sub := ts.Submit("resubscribe", LaneSteam, "2 mods", items("a", "b"))
	<-reported
	running := ts.List()[0]
	if running.Items[0].Got != 45 || running.Items[0].Size != 100 || running.Done != 1 || running.Frac != 0.725 || running.Phase != "Steam is downloading 2 of 2" {
		t.Fatalf("%+v", running)
	}
	close(hold)
	ended := waitEnded(t, ts, sub.ID)
	if ended.State != TaskPartial || ended.Items[0].Got != 45 {
		t.Fatalf("%+v", ended)
	}
	ts.Batch = func(_ string, _ []string, b BatchCalls) bool { b.Start(0); b.Done(0, nil); return true }
	again, err := ts.Again(sub.ID, ItemFail, "", "retry")
	if err != nil || again.Items[0].Got != 0 || again.Items[0].Size != 0 {
		t.Fatalf("%+v %v", again, err)
	}
	waitEnded(t, ts, again.ID)
}

func TestSteamWatchEOFShutsDownCleanly(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSteamStreamHelper$")
	cmd.Env = append(os.Environ(), "RIMARCHO_FAKE_WATCH=1", "RIMARCHO_FAKE_WATCH_EOF=1")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	cmd.Cancel = input.Close
	seen := false
	err = streamSteamCommand(cmd, func(line string) {
		if line == "PROGRESS 1 45 100 21" {
			seen = true
			cancel()
		}
	})
	if !seen || err == nil || cmd.ProcessState == nil || !cmd.ProcessState.Success() {
		t.Fatalf("seen=%v err=%v process=%v", seen, err, cmd.ProcessState)
	}
}

func TestTaskProgressNotificationsAreThrottled(t *testing.T) {
	t.Parallel()
	ts := OpenTasks("")
	var changes atomic.Int64
	ts.Changed = func() { changes.Add(1) }
	ts.Batch = func(_ string, _ []string, b BatchCalls) bool {
		b.Start(0)
		before := changes.Load()
		for i := int64(0); i < 100; i++ {
			b.Progress(0, i, 100)
		}
		if n := changes.Load() - before; n != 1 {
			t.Errorf("burst sent %d changes", n)
		}
		b.Done(0, nil)
		return true
	}
	sub := ts.Submit("resubscribe", LaneSteam, "mod", items("a"))
	waitEnded(t, ts, sub.ID)
}
