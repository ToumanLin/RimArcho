package app

import (
	"errors"
	"fmt"
	"io/fs"
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
			// matching process must block replacement, as on Windows. Ubuntu's
			// same-user sd-pam is also protected, so unrelated names cannot block
			// every tool forever. Compare against names throughout the tool tree.
			info, statErr := os.Stat(p)
			if os.IsPermission(err) && statErr == nil && info.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
				continue
			}
			if os.IsPermission(err) {
				comm, commErr := os.ReadFile(filepath.Join(p, "comm"))
				if commErr == nil {
					match, nameErr := toolProcessName(dir, strings.TrimSuffix(string(comm), "\n"))
					if nameErr == nil && !match {
						continue
					}
				}
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

// Linux comm holds at most 15 bytes. Inspect the tool tree only when a same-user
// executable link is protected; readable processes always use their full path.
func toolProcessName(dir, comm string) (bool, error) {
	match := false
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		name := entry.Name()
		if len(name) > 15 {
			name = name[:15]
		}
		if !entry.IsDir() && name == comm {
			match = true
			return fs.SkipAll
		}
		return nil
	})
	return match, err
}
