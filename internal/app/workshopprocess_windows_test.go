package app

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWorkshopProcessHelper(t *testing.T) {
	args := flag.Args()
	if len(args) != 2 || (args[0] != "workshop-owner" && args[0] != "workshop-child" && args[0] != "workshop-launcher") {
		return
	}
	if args[0] == "workshop-child" {
		time.Sleep(time.Minute)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		os.Exit(2)
	}
	cmd := exec.Command(exe, "-test.run=^TestWorkshopProcessHelper$", "--", "workshop-child", args[1])
	hideWindow(cmd)
	if cmd.Start() != nil {
		os.Exit(3)
	}
	if os.WriteFile(args[1], []byte(strconv.Itoa(cmd.Process.Pid)), 0600) != nil {
		os.Exit(4)
	}
	if args[0] == "workshop-launcher" {
		return
	}
	cmd.Wait()
}

func TestWorkshopWaitTracksUpdaterChild(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(t.TempDir(), "updater.pid")
	proc, err := startWorkshopCMD(exe, []string{"-test.run=^TestWorkshopProcessHelper$", "--", "workshop-launcher", pidFile}, false)
	if err != nil {
		t.Fatal(err)
	}
	p := proc.(*consoleProcess)
	defer func() { p.KillTree(); p.Wait() }()
	if state, err := windows.WaitForSingleObject(p.handle, 5000); err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatalf("launcher did not exit: state=%d err=%v", state, err)
	}
	done := make(chan error, 1)
	go func() { done <- p.Wait() }()
	select {
	case err := <-done:
		t.Fatalf("returned before updater finished: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := p.KillTree(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("job did not finish after cancelling updater")
	}
}

func TestWorkshopCancellationKillsOnlyOwnedProcessTree(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// An unrelated SteamCMD-shaped invocation must survive cancellation of the owner.
	unrelated, err := startWorkshopCMD(exe, []string{"-test.run=^TestWorkshopProcessHelper$", "--", "workshop-child", "unused"}, false)
	if err != nil {
		t.Fatal(err)
	}
	u := unrelated.(*consoleProcess)
	defer func() { u.KillTree(); u.Wait() }()
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	owned, err := startWorkshopCMD(exe, []string{"-test.run=^TestWorkshopProcessHelper$", "--", "workshop-owner", pidFile}, false)
	if err != nil {
		t.Fatal(err)
	}
	p := owned.(*consoleProcess)
	defer func() { p.KillTree(); p.Wait() }()
	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(pidFile)
		pid, _ = strconv.Atoi(string(b))
		if pid > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("owned child never started")
	}
	child, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(child)
	if err := p.KillTree(); err != nil {
		t.Fatal(err)
	}
	if state, err := windows.WaitForSingleObject(child, 5000); err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatalf("descendant survived: state=%d err=%v", state, err)
	}
	if err := p.Wait(); err == nil {
		t.Fatal("cancelled owner returned success")
	}
	if state, err := windows.WaitForSingleObject(u.handle, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("unrelated invocation was killed: state=%d err=%v", state, err)
	}
}
