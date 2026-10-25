package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// Re-exec is a console-subsystem application. It reports console modes as well
// as FILE_TYPE_CHAR because NUL is also a character device.
func TestSteamConsoleHandles(t *testing.T) {
	if result := os.Getenv("RIMARCHO_CONSOLE_TEST_RESULT"); result != "" {
		type stream struct {
			Type    uint32
			Console bool
		}
		streams := []stream{}
		for _, id := range []uint32{windows.STD_INPUT_HANDLE, windows.STD_OUTPUT_HANDLE, windows.STD_ERROR_HANDLE} {
			handle, err := windows.GetStdHandle(id)
			kind, _ := windows.GetFileType(handle)
			var mode uint32
			streams = append(streams, stream{kind, err == nil && windows.GetConsoleMode(handle, &mode) == nil})
		}
		b, _ := json.Marshal(struct {
			Streams []stream
			Args    []string
		}{streams, os.Args[2:]})
		if os.WriteFile(result, b, 0600) != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	result := filepath.Join(t.TempDir(), "console handles.json")
	t.Setenv("RIMARCHO_CONSOLE_TEST_RESULT", result)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"-test.run=^TestSteamConsoleHandles$", "argument with spaces", `embedded"quote`, `C:\trailing space\`}
	process, err := startSteamCMD(exe, args, true)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- process.Wait() }()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		_ = process.(interface{ Kill() error }).Kill()
		<-done
		t.Fatal("console helper timed out")
	}
	var report struct {
		Streams []struct {
			Type    uint32
			Console bool
		}
		Args []string
	}
	b, err := os.ReadFile(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Streams) != 3 {
		t.Fatal(string(b))
	}
	for _, stream := range report.Streams {
		if stream.Type != windows.FILE_TYPE_CHAR || !stream.Console {
			t.Fatalf("child got NUL or non-console handles: %s", b)
		}
	}
	if !reflect.DeepEqual(report.Args, args[1:]) {
		t.Fatalf("argument quoting: %s", b)
	}
}

func TestConsoleKillAndWait(t *testing.T) {
	exe := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	process, err := startConsole(exe, []string{"/d", "/q", "/k"})
	if err != nil {
		t.Fatal(err)
	}
	if err := process.(interface{ Kill() error }).Kill(); err != nil {
		t.Fatal(err)
	}
	if err := process.Wait(); err == nil {
		t.Fatal("killed child reported success")
	}
	if err := process.(interface{ Kill() error }).Kill(); err != os.ErrProcessDone {
		t.Fatal(err)
	}
}
