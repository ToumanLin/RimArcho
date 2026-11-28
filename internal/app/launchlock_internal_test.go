package app

import (
	testhide "github.com/ToumanLin/RimArcho/internal/testkit/hide"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/store"
)

func TestCreditLaunchLockHelper(t *testing.T) {
	home := os.Getenv("RIMARCHO_TEST_LAUNCH_LOCK")
	if home == "" {
		return
	}
	a := &App{Home: home, Now: time.Now, launchWaitLimit: 100 * time.Millisecond}
	// Launch and list writes must refuse before they can read or mutate saved state.
	if _, err := a.LaunchWith(false, false); err == nil || !strings.Contains(err.Error(), "another launch") {
		t.Fatalf("launch bypassed process lock: %v", err)
	}
	if _, err := a.lockLaunch(); err == nil {
		t.Fatal("handoff bypassed process lock")
	}
	if err := a.sync(&LoadoutState{}); err == nil {
		t.Fatal("list sync bypassed process lock")
	}
	if _, err := a.SaveWith(false, false); err == nil {
		t.Fatal("list save bypassed process lock")
	}
	if _, _, err := a.UndoRedo(false); err == nil {
		t.Fatal("undo bypassed process lock")
	}
	if _, err := a.Stage(nil); err == nil {
		t.Fatal("staging bypassed process lock")
	}
	if err := a.editPlan(&store.Plan{}, &store.Workspace{}, nil, nil, "edit"); err == nil {
		t.Fatal("plan edit bypassed process lock")
	}

	if _, err := a.LaunchPackWith("fixture", false, false); err == nil || !strings.Contains(err.Error(), "another launch") {
		t.Fatalf("pack launch bypassed process lock: %v", err)
	}
}

func TestCreditLaunchLockAcrossProcesses(t *testing.T) {
	t.Parallel()
	a := &App{Home: t.TempDir()}
	unlock, err := a.lockLaunch()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	cmd := testhide.Cmd(exec.Command(os.Args[0], "-test.run=^TestCreditLaunchLockHelper$", "-test.count=1"))
	cmd.Env = append(os.Environ(), "RIMARCHO_TEST_LAUNCH_LOCK="+a.Home)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cross-process exclusion: %v\n%s", err, out)
	}
}

func TestLaunchWaitAcquiresReleasedLock(t *testing.T) {
	home := t.TempDir()
	held, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		a := &App{Home: home}
		unlock, err := a.lockLaunch()
		if err != nil {
			done <- err
			close(held)
			return
		}
		close(held)
		<-release
		unlock()
		done <- nil
	}()
	<-held
	a := &App{Home: home, Now: time.Now, launchWaitLimit: time.Second}
	notices := 0
	a.LaunchWait = func(text string) {
		if text != "waiting for background replacement to finish" {
			t.Errorf("notice %q", text)
		}
		notices++
		close(release)
	}
	unlock, err := a.waitLaunch()
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if notices != 1 {
		t.Fatalf("waiting notices %d", notices)
	}
}

func TestBusyMessageHelper(t *testing.T) {
	home := os.Getenv("RIMARCHO_TEST_BUSY")
	if home == "" {
		return
	}
	a := &App{Home: home, Now: time.Now, launchWaitLimit: 100 * time.Millisecond}
	_, err := a.lockLaunch()
	want := "busy: a test run of plan:Colony (Game Config main) (process " + os.Getenv("RIMARCHO_TEST_BUSY_PID") + ") has been running since "
	if err == nil || !strings.HasPrefix(err.Error(), want) || !strings.Contains(err.Error(), " and ends by ") {
		t.Fatalf("busy message: %v", err)
	}
	if err := a.sync(&LoadoutState{}); err == nil || !strings.Contains(err.Error(), "a test run of plan:Colony") {
		t.Fatalf("list sync: %v", err)
	}
}

// TestBusyMessage: a call the launch lock refuses names the long task that holds it, when it
// started and when it ends at the latest; after the task the record is gone (14 §1 "Busy").
func TestBusyMessage(t *testing.T) {
	t.Parallel()
	a := &App{Home: t.TempDir()}
	unlock, err := a.lockLaunch()
	if err != nil {
		t.Fatal(err)
	}
	clear := a.markBusy("a test run of plan:Colony (Game Config main)", time.Now().Add(10*time.Minute))
	cmd := testhide.Cmd(exec.Command(os.Args[0], "-test.run=^TestBusyMessageHelper$", "-test.count=1"))
	cmd.Env = append(os.Environ(), "RIMARCHO_TEST_BUSY="+a.Home, "RIMARCHO_TEST_BUSY_PID="+strconv.Itoa(os.Getpid()))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("busy message across processes: %v\n%s", err, out)
	}
	clear()
	unlock()
	if _, ok := readBusy(a.Home); ok {
		t.Fatal("busy.json left after the task")
	}
	// A record of a killed holder, past its limit, is not shown.
	stale := &App{Home: a.Home}
	stale.markBusy("a test run of plan:Old", time.Now().Add(-time.Hour))
	if err := busyError(a.Home, os.ErrExist); !strings.HasPrefix(err.Error(), "busy: another launch") {
		t.Fatalf("stale record: %v", err)
	}
}
