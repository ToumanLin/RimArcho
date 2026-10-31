package selfupdate

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	switch os.Getenv("SELFUPDATE_HELPER") {
	case "sleep":
		fmt.Println("ready")
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "version":
		home := os.Getenv("RIMARCHO_HOME")
		entries, err := os.ReadDir(home)
		if len(os.Args) != 2 || os.Args[1] != "version" || err != nil || len(entries) != 0 || os.Getenv("SELFUPDATE_INHERITED") != "inherited" {
			os.Exit(2)
		}
		if err := os.WriteFile(filepath.Join(home, "probe"), []byte("synthetic data"), 0o600); err != nil {
			os.Exit(3)
		}
		fmt.Println("archo 9.9.9 (abc1234, 2026-10-31)")
		fmt.Println("data folder:", home)
		os.Exit(0)
	case "garbage":
		fmt.Println("garbage")
		os.Exit(0)
	case "invalid version":
		fmt.Println("archo dev (abc1234)")
		os.Exit(0)
	case "exit failure":
		fmt.Println("archo 9.9.9")
		os.Exit(4)
	}
	os.Exit(m.Run())
}

func copyTestBinary(t *testing.T, target string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	r, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	w, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(w, r)
	if closeErr := w.Close(); err != nil || closeErr != nil {
		t.Fatalf("copy helper: %v, %v", err, closeErr)
	}
}

func TestVersionOf(t *testing.T) {
	root := t.TempDir()
	// Keep VersionOf's own transient directory under this test's TempDir too.
	probeTemp := filepath.Join(root, "probe-temp")
	if err := os.Mkdir(probeTemp, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TEMP", "TMP", "TMPDIR"} {
		t.Setenv(name, probeTemp)
	}
	t.Setenv("RIMARCHO_HOME", filepath.Join(root, "parent-home-must-not-be-opened"))
	t.Setenv("SELFUPDATE_INHERITED", "inherited")
	exe := filepath.Join(root, "archo.exe")
	copyTestBinary(t, exe)
	for _, mode := range []string{"version", "garbage", "invalid version", "exit failure"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SELFUPDATE_HELPER", mode)
			got, err := VersionOf(context.Background(), exe)
			if mode == "version" {
				if err != nil || got != "9.9.9" {
					t.Fatalf("VersionOf = %q, %v", got, err)
				}
			} else if err == nil {
				t.Fatalf("accepted helper mode %q: %s", mode, got)
			}
			entries, err := os.ReadDir(probeTemp)
			if err != nil || len(entries) != 0 {
				t.Fatalf("probe data directory not removed: %v, %v", entries, err)
			}
		})
	}
	if _, err := os.Stat(os.Getenv("RIMARCHO_HOME")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("probe touched parent home: %v", err)
	}
	t.Setenv("SELFUPDATE_HELPER", "sleep")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err := VersionOf(ctx, exe); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("probe did not honor deadline: %v", err)
	}
	if entries, err := os.ReadDir(probeTemp); err != nil || len(entries) != 0 {
		t.Fatalf("cancelled probe left data: %v, %v", entries, err)
	}
}

func TestRenameWhileRunning(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows executable rename behavior")
	}
	program := t.TempDir()
	stage, old := filepath.Join(program, "stage"), filepath.Join(program, "old")
	if err := os.Mkdir(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	copyTestBinary(t, filepath.Join(program, "archo.exe"))
	copyTestBinary(t, filepath.Join(stage, "archo.exe"))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(program, "archo.exe"))
	cmd.Env = append(os.Environ(), "SELFUPDATE_HELPER=sleep")
	hideWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "ready" {
		t.Fatalf("helper not ready: %q, %v", line, err)
	}
	if _, err := Apply(program, stage, old); err != nil {
		t.Fatalf("running executable rename failed: %v", err)
	}
	for _, name := range []string{filepath.Join(program, "archo.exe"), filepath.Join(old, "archo.exe")} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("missing executable after swap: %v", err)
		}
	}
}

func TestFirstLineWriter(t *testing.T) {
	var w firstLineWriter
	_, _ = w.Write([]byte("archo "))
	_, _ = w.Write([]byte("9.9.9\nother line"))
	_, _ = w.Write([]byte("more output"))
	if string(w.line) != "archo 9.9.9" || w.overflow {
		t.Fatalf("wrong first line: %+v", w)
	}
	w = firstLineWriter{}
	_, _ = w.Write([]byte(strings.Repeat("x", (64<<10)+1)))
	if !w.overflow || len(w.line) > 64<<10 {
		t.Fatal("unbounded first line")
	}
}
