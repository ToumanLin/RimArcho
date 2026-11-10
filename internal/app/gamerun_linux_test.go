package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLinuxGameProcesses(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	program, err := os.ReadFile("/bin/sleep")
	if err != nil {
		t.Fatal(err)
	}
	for _, loader := range []string{"RimWorldLinux", "RimWorldWin64.exe", "wine64-preloader", "wine"} {
		t.Run(loader, func(t *testing.T) {
			game := filepath.Join(dir, loader, "Game With Spaces")
			if err := os.MkdirAll(game, 0o700); err != nil {
				t.Fatal(err)
			}
			exe := filepath.Join(game, loader)
			if err := os.WriteFile(exe, program, 0o700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(exe, "30")
			want := exe
			if strings.HasPrefix(loader, "wine") {
				want = filepath.Join(game, "RimWorldWin64.exe")
				cmd.Args[0] = `Z:` + strings.ReplaceAll(want, "/", `\`)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
			run := processIn(gameProcesses(), game)
			if run == nil || run.PID != cmd.Process.Pid || run.Exe != want || !run.ArgsKnown || run.Started.IsZero() || time.Since(run.Started) > 5*time.Second {
				t.Fatalf("live process: %+v", run)
			}
			if processIn(gameProcesses(), game+"-sibling") != nil {
				t.Fatal("matched a sibling game folder")
			}
			alias := game + "-alias"
			if err := os.Symlink(game, alias); err != nil {
				t.Fatal(err)
			}
			if processIn(gameProcesses(), alias) == nil {
				t.Fatal("game folder alias did not match its running process")
			}
		})
	}
	// A fixture supplies arguments a harmless sleep process cannot accept.
	proc := filepath.Join(dir, "proc")
	p := filepath.Join(proc, "123")
	if err := os.MkdirAll(p, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/synthetic/RimWorldLinux", filepath.Join(p, "exe")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "cmdline"), []byte("RimWorldLinux\x00-savedatafolder=/synthetic/My Saves\x00"), 0o600); err != nil {
		t.Fatal(err)
	}
	runs := gameProcessesAt(proc)
	if len(runs) != 1 || runs[0].SaveData != "/synthetic/My Saves" || !runs[0].ArgsKnown {
		t.Fatalf("NUL-separated arguments: %+v", runs)
	}
	if err := os.Remove(filepath.Join(p, "cmdline")); err != nil {
		t.Fatal(err)
	}
	if runs := gameProcessesAt(proc); len(runs) != 1 || runs[0].ArgsKnown {
		t.Fatalf("unreadable arguments must still detect the native game: %+v", runs)
	}
}

func TestWinePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prefix := filepath.Join(dir, "prefix")
	devices := filepath.Join(prefix, "dosdevices")
	if err := os.MkdirAll(devices, 0o700); err != nil {
		t.Fatal(err)
	}
	drive := t.TempDir()
	if err := os.Symlink(drive, filepath.Join(devices, "c:")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "environ"), []byte("OTHER=ignored\x00WINEPREFIX="+prefix+"\x00"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(drive, filepath.Join(dir, "cwd")); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{
		"C:/Game/RimWorldWin64.exe": filepath.Join(drive, "Game/RimWorldWin64.exe"),
		"Z:/home/player/My Saves":   "/home/player/My Saves",
		"RimWorldWin64.exe":         filepath.Join(drive, "RimWorldWin64.exe"),
		"Q:/unmapped":               "",
	} {
		if got := winePath(p, dir); got != want {
			t.Errorf("winePath(%q)=%q, want %q", p, got, want)
		}
	}
}
