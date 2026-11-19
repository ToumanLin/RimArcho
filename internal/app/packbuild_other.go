//go:build !windows

package app

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

func diskFree(dir string) int64 {
	var st unix.Statfs_t
	if unix.Statfs(dir, &st) != nil {
		return -1
	}
	return int64(st.Bavail) * int64(st.Bsize)
}

// diskSpace: what the caller may still write, and the volume's size; -1, -1 when unknown.
func diskSpace(dir string) (int64, int64) {
	var st unix.Statfs_t
	if unix.Statfs(dir, &st) != nil {
		return -1, -1
	}
	return int64(st.Bavail) * int64(st.Bsize), int64(st.Blocks) * int64(st.Bsize)
}

// startDetached starts a child in its own session, so it outlives this process.
func startDetached(mk func() *exec.Cmd) (*exec.Cmd, error) {
	cmd := mk()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd, cmd.Start()
}
