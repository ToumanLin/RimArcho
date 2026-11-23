package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// fakeProc writes one process of a synthetic proc tree: its exe link and stat line.
func fakeProc(t *testing.T, proc string, pid int, exe, stat string) {
	t.Helper()
	dir := filepath.Join(proc, strconv.Itoa(pid))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if exe != "" {
		if err := os.Symlink(exe, filepath.Join(dir, "exe")); err != nil {
			t.Fatal(err)
		}
	}
	if stat != "" {
		if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// statLine is a /proc/<pid>/stat line with comm, state and starttime (field 22).
func statLine(pid int, comm string, state byte, start int64) string {
	s := strconv.Itoa(pid) + " (" + comm + ") " + string(state)
	for i := 4; i <= 21; i++ {
		s += " 0"
	}
	return s + " " + strconv.FormatInt(start, 10) + " 0 0\n"
}

func TestLinuxArchoProcessesSynthetic(t *testing.T) {
	t.Parallel()
	proc := filepath.Join(t.TempDir(), "proc")
	if err := os.MkdirAll(proc, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proc, "stat"), []byte("cpu 1 2 3\nbtime 1700000000\nprocesses 9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fakeProc(t, proc, 10, "/opt/RimArcho/archo", statLine(10, "archo", 'S', 250))
	fakeProc(t, proc, 11, "/opt/RimArcho/rimarcho (deleted)", statLine(11, "rimarcho) x (y", 'R', 300))
	fakeProc(t, proc, 12, "/opt/RimArcho/archo", statLine(12, "archo", 'Z', 400)) // exited, not reaped
	fakeProc(t, proc, 13, "/usr/bin/archo-helper", statLine(13, "archo-helper", 'S', 1))
	fakeProc(t, proc, 14, "", statLine(14, "archo", 'S', 1)) // another user's: link unreadable
	fakeProc(t, proc, 15, "/opt/RimArcho/archo", "")         // stat gone: still listed, start unknown
	if err := os.MkdirAll(filepath.Join(proc, "self"), 0o700); err != nil {
		t.Fatal(err)
	}
	runs := archoProcessesAt(proc)
	got := map[int]*GameRun{}
	for _, r := range runs {
		got[r.PID] = r
	}
	if len(runs) != 3 || got[10] == nil || got[11] == nil || got[15] == nil {
		t.Fatalf("processes: %+v", runs)
	}
	if got[10].Exe != "/opt/RimArcho/archo" || !got[10].Started.Equal(time.Unix(1700000002, int64(500*time.Millisecond))) {
		t.Fatalf("archo: %+v", got[10])
	}
	if got[11].Exe != "/opt/RimArcho/rimarcho" || !got[11].Started.Equal(time.Unix(1700000003, 0)) {
		t.Fatalf("deleted rimarcho with ')' in comm: %+v", got[11])
	}
	if !got[15].Started.IsZero() {
		t.Fatalf("unknown start: %+v", got[15])
	}
	if archoProcessesAt(filepath.Join(proc, "missing")) != nil {
		t.Fatal("an unreadable proc tree lists processes")
	}
}

func TestLinuxProcessStartSynthetic(t *testing.T) {
	t.Parallel()
	proc := filepath.Join(t.TempDir(), "proc")
	fakeProc(t, proc, 20, "", statLine(20, "game (1)", 'S', 123456))
	fakeProc(t, proc, 21, "", statLine(21, "dead", 'Z', 99))
	fakeProc(t, proc, 22, "", "22 (broken")
	if s, alive := processStartAt(proc, 20); s != 123456 || !alive {
		t.Fatalf("running: %d %v", s, alive)
	}
	if s, alive := processStartAt(proc, 21); s != 99 || alive {
		t.Fatalf("zombie: %d %v", s, alive)
	}
	if _, alive := processStartAt(proc, 22); alive {
		t.Fatal("an unreadable stat in a synthetic tree counts as running")
	}
	if _, alive := processStartAt(proc, 23); alive {
		t.Fatal("a missing process counts as running")
	}
	if _, alive := processStartAt(proc, 0); alive {
		t.Fatal("pid 0 counts as running")
	}
}

// TestLinuxArchoProcessesLive runs a harmless copy named archo and finds it with its
// folder and start time; processStart gives the same value on every call and ends with it.
func TestLinuxArchoProcessesLive(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "Program Folder")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "archo")
	if err := os.WriteFile(exe, sleeperProgram(t), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), sleeperEnvVar)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	var found *GameRun
	for _, r := range archoProcesses() {
		if r.PID == cmd.Process.Pid {
			found = r
		}
	}
	if found == nil || found.Exe != exe || found.Started.IsZero() || time.Since(found.Started) > 10*time.Second {
		t.Fatalf("live archo: %+v", found)
	}
	first, alive := processStart(uint32(cmd.Process.Pid))
	again, _ := processStart(uint32(cmd.Process.Pid))
	if first == 0 || !alive || again != first {
		t.Fatalf("processStart: %d %v, then %d", first, alive, again)
	}
	if self, _ := processStart(uint32(os.Getpid())); self == 0 || self > first {
		t.Fatalf("this test started at %d, after its child at %d", self, first)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if _, alive := processStart(uint32(cmd.Process.Pid)); alive {
		t.Fatal("an ended process still runs")
	}
}
