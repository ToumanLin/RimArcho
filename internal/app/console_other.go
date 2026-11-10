//go:build !windows

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
)

// A hidden run gets its own process group, so cancellation reaches its updater
// children without signalling the terminal that launched archo. An interactive
// run stays in archo's group: a background group that reads the terminal is
// stopped by SIGTTIN, so a login prompt would hang.
func startWorkshopCMD(exe string, args []string, console bool) (SteamCMDProc, error) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	env, err := steamCMDEnv(exe)
	if err != nil {
		return nil, err
	}
	cmd.Env = env
	if console {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	} else {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if err := cmd.Start(); err != nil {
		return nil, steamCMDStartError(exe, "", err)
	}
	return &workshopProcess{Cmd: cmd, group: !console}, nil
}

type workshopProcess struct {
	*exec.Cmd
	group bool // the process leads its own group
	mu    sync.Mutex
	done  bool
}

func (p *workshopProcess) Wait() error {
	err := p.Cmd.Wait()
	p.mu.Lock()
	p.done = true
	p.mu.Unlock()
	return steamCMDStartError(p.Path, "", err)
}

func (p *workshopProcess) KillTree() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		return os.ErrProcessDone
	}
	if !p.group {
		return p.Process.Kill()
	}
	return syscall.Kill(-p.Process.Pid, syscall.SIGKILL)
}
