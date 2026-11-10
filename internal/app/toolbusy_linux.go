package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func toolIdle(_ string, dir string) error { return toolIdleAt("/proc", dir) }

func toolIdleAt(proc, dir string) error {
	root := realOrAbs(dir)
	entries, err := os.ReadDir(proc)
	if err != nil {
		return fmt.Errorf("cannot check whether the tool in %s is in use: %w", dir, err)
	}
	for _, entry := range entries {
		if pid, err := strconv.Atoi(entry.Name()); err != nil || pid <= 0 || !entry.IsDir() {
			continue
		}
		p := filepath.Join(proc, entry.Name())
		exe, err := os.Readlink(filepath.Join(p, "exe"))
		if os.IsNotExist(err) {
			continue // Exited, zombie, or kernel thread.
		}
		if err != nil {
			// Other users' process links are normally inaccessible. An unreadable
			// process of our user must block replacement rather than guess idle.
			info, statErr := os.Stat(p)
			if os.IsPermission(err) && statErr == nil && info.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
				continue
			}
			return fmt.Errorf("cannot check whether the tool in %s is in use: %w", dir, err)
		}
		exe = strings.TrimSuffix(exe, " (deleted)")
		if underDir(filepath.Clean(exe), root) {
			return errors.New("tool is in use; close it before updating or removing it")
		}
	}
	return nil
}
