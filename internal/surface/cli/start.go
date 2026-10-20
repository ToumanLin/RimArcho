package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// startAlive is how a bare `archo` starts the WezTerm host (06 §1.1). The host must not be
// tied to archo's console: on a double click that console closes as soon as archo exits,
// and everything attached to it is closed with it. A host that dies within the grace period
// (a bad argument, a broken install) is an error, so the caller falls back to running here
// instead of exiting silently. WezTerm's own output goes to <data>/logs/wezterm-start.log.
func startAlive(home, name string, args ...string) error {
	logDir := filepath.Join(home, "logs")
	_ = os.MkdirAll(logDir, 0o755)
	logPath := filepath.Join(logDir, "wezterm-start.log")
	f, err := os.Create(logPath)
	if err != nil {
		return err
	}
	c := exec.Command(name, args...)
	c.Stdout, c.Stderr = f, f
	c.SysProcAttr = detachedAttr()
	if err := c.Start(); err != nil {
		f.Close()
		return err
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case err := <-done:
		f.Close()
		if err == nil {
			return nil // handed over to an already running WezTerm
		}
		b, _ := os.ReadFile(logPath)
		msg := strings.TrimSpace(string(b))
		if len(msg) > 600 {
			msg = msg[:600] + "…"
		}
		return fmt.Errorf("WezTerm exited at once (%v): %s", err, msg)
	case <-time.After(1500 * time.Millisecond):
		f.Close() // the child keeps its own handle
		return nil
	}
}
