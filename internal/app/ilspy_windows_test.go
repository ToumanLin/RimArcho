//go:build windows

package app

import (
	"os"
	"path/filepath"
	"testing"
)

// ilspycmd 11 installs as a .cmd launcher; Archo runs the exe it names, not cmd.exe.
func TestILSpyExeFromLauncher(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, ".store", "ilspycmd", "11.2.0.10050", "tools", "any", "win-x64", "ilspycmd.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("MZ"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := filepath.Join(dir, "ilspycmd.cmd")
	shim := "@echo off\r\n\"%~dp0.store\\ilspycmd\\11.2.0.10050\\tools\\any\\win-x64\\ilspycmd.exe\" %*\r\n"
	if err := os.WriteFile(cmd, []byte(shim), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ilspyExe(cmd); filepath.Clean(got) != filepath.Clean(exe) {
		t.Fatalf("launcher resolved to %s, want %s", got, exe)
	}
	other := filepath.Join(dir, "ilspycmd.exe")
	if got := ilspyExe(other); got != other {
		t.Fatalf("exe path changed: %s", got)
	}
	t.Setenv("ARCHO_ILSPYCMD", cmd)
	if got := ILSpyPath(); filepath.Clean(got) != filepath.Clean(exe) {
		t.Fatalf("ARCHO_ILSPYCMD launcher resolved to %s", got)
	}
}
