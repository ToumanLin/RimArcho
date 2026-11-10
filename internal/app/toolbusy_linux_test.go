package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLinuxToolIdle(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bin := filepath.Join(dir, "linux32")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	program, err := os.ReadFile("/bin/sleep")
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(bin, "arbitrary-name")
	if err := os.WriteFile(exe, program, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	if err := toolIdle("steamcmd", dir); err == nil {
		t.Fatal("running executable under the tool directory was reported idle")
	}
	if err := toolIdle("steamcmd", dir+"-sibling"); err != nil {
		t.Fatal("sibling directory:", err)
	}
	// A deleted executable still executes until its process exits.
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	if err := toolIdle("steamcmd", dir); err == nil {
		t.Fatal("deleted running executable was reported idle")
	}
	if err := toolIdleAt(filepath.Join(dir, "missing-proc"), dir); err == nil {
		t.Fatal("unavailable process table was reported idle")
	}
}

func TestToolProcessName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bin := filepath.Join(dir, "runtime")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "long-tool-runtime-name"), nil, 0o700); err != nil {
		t.Fatal(err)
	}
	if match, err := toolProcessName(dir, "long-tool-runt"); err != nil || match {
		t.Fatalf("partial name must not match: %v %v", match, err)
	}
	if match, err := toolProcessName(dir, "long-tool-runti"); err != nil || !match {
		t.Fatalf("15-byte kernel name must protect nested runtime: %v %v", match, err)
	}
	if match, err := toolProcessName(dir, "(sd-pam)"); err != nil || match {
		t.Fatalf("unrelated protected process: %v %v", match, err)
	}
}
