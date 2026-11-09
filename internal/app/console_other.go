//go:build !windows

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
)

// A separate process group keeps cancellation scoped to this invocation and
// its updater children, without signalling the terminal that launched archo.
func startWorkshopCMD(exe string, args []string, console bool) (SteamCMDProc, error) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if console {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &workshopProcess{Cmd: cmd}, nil
}

type workshopProcess struct {
	*exec.Cmd
	mu   sync.Mutex
	done bool
}

func (p *workshopProcess) Wait() error {
	err := p.Cmd.Wait()
	p.mu.Lock()
	p.done = true
	p.mu.Unlock()
	return err
}

func (p *workshopProcess) KillTree() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		return os.ErrProcessDone
	}
	return syscall.Kill(-p.Process.Pid, syscall.SIGKILL)
}
