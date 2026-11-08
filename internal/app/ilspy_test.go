package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ilspycmd 9-11 print an update notice on stdout, and "Could not find a type named" with exit
// code 0 for a type that is not in the assembly; neither is source.
func TestCleanILSpy(t *testing.T) {
	src := "namespace Verse;\n\npublic class TickList\n{\n}\nYou are not using the latest version of the tool, please update.\r\nLatest version is '11.2.0.10050' (yours is '9.1.0.7988')\r\n"
	out, ok := cleanILSpy([]byte(src))
	if !ok || strings.Contains(string(out), "latest version") || !strings.HasSuffix(string(out), "}\n") {
		t.Fatalf("cleaned %q %v", out, ok)
	}
	for _, s := range []string{
		"You are not using the latest version of the tool, please update.\nLatest version is '11.2.0.10050' (yours is '9.1.0.7988')\n",
		"Could not find a type named 'RimWorld.PlayLogEntry_Interaction'.\n",
		"",
	} {
		if out, ok := cleanILSpy([]byte(s)); ok || len(out) != 0 {
			t.Errorf("%q: kept %q, type %v", s, out, ok)
		}
	}
}

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
