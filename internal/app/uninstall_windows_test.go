package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// The after-exit script itself (cmd.exe), on throwaway folders: it waits for the process,
// deletes the folders whole (a junction inside is removed as a link: its target stays),
// deletes only the listed program files, removes the program folder only when empty, copes
// with non-ASCII and % & in paths, and deletes itself.
func TestAfterExitScript(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	data := filepath.Join(root, "数据 100% & co")
	outside := filepath.Join(root, "outside")
	write := func(p, s string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(data, "archo.db"), "db")
	write(filepath.Join(data, "cache", "a", "b.json"), "{}")
	write(filepath.Join(outside, "keep.txt"), "the player's")
	if err := source.CreateLink(filepath.Join(data, "link"), outside); err != nil {
		t.Fatal(err)
	}
	prog := filepath.Join(root, "Program")
	write(filepath.Join(prog, "archo.exe"), "MZ")
	write(filepath.Join(prog, "fonts", "f.ttf"), "f")
	write(filepath.Join(prog, "notes.txt"), "the player's")
	log := filepath.Join(root, "uninstall.log")

	// A process that has already ended: the script does not wait.
	c := exec.Command("cmd", "/c", "exit", "0")
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	x := app.AfterExit{Dirs: []string{data, filepath.Join(prog, "fonts")}, Files: []string{filepath.Join(prog, "archo.exe")}, Empty: []string{prog}}
	script := filepath.Join(root, "run.cmd")
	write(script, app.AfterExitScript(x, c.Process.Pid, log))
	out, err := exec.Command("cmd", "/d", "/c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, gone := range []string{data, filepath.Join(prog, "archo.exe"), filepath.Join(prog, "fonts"), script} {
		if exists(gone) {
			t.Fatalf("%s is left (output %s)", gone, out)
		}
	}
	if b, err := os.ReadFile(filepath.Join(outside, "keep.txt")); err != nil || string(b) != "the player's" {
		t.Fatal("rd /s went through the junction")
	}
	if !exists(filepath.Join(prog, "notes.txt")) {
		t.Fatal("a file that is not RimArcho's was deleted")
	}
	if b, _ := os.ReadFile(log); strings.TrimSpace(string(b)) != "" {
		t.Fatalf("log: %s", b)
	}
}

// The script waits while the process runs, even with another find first on PATH (Git's
// usr/bin): a folder is deleted only after the process has ended.
func TestAfterExitScriptWaits(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	decoy := filepath.Join(root, "decoy")
	os.MkdirAll(decoy, 0o755)
	os.WriteFile(filepath.Join(decoy, "find.cmd"), []byte("@exit /b 1\r\n"), 0o644)
	running := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "ping.exe"), "-n", "4", "127.0.0.1")
	if err := running.Start(); err != nil {
		t.Fatal(err)
	}
	ended := make(chan time.Time, 1)
	go func() { running.Wait(); ended <- time.Now() }()
	script := filepath.Join(root, "run.cmd")
	os.WriteFile(script, []byte(app.AfterExitScript(app.AfterExit{Dirs: []string{data}}, running.Process.Pid, filepath.Join(root, "log"))), 0o644)
	c := exec.Command("cmd", "/d", "/c", script)
	c.Env = append(os.Environ(), "PATH="+decoy+";"+os.Getenv("PATH"))
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	done := time.Now()
	select {
	case e := <-ended:
		if done.Before(e) {
			t.Fatal("the script finished before the process ended")
		}
	default:
		t.Fatal("the script did not wait for the process")
	}
	if exists(data) {
		t.Fatal("not deleted after the process ended")
	}
}
