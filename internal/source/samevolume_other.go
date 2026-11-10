//go:build !windows

package source

import (
	"os"
	"path/filepath"
	"syscall"
)

// SameVolume compares the devices of the nearest existing ancestors. Destinations
// need not exist yet; an unreadable ancestor selects the checked-copy path.
func SameVolume(from, to string) bool {
	a, err := deviceID(from)
	if err != nil {
		return false
	}
	b, err := deviceID(to)
	return err == nil && a == b
}

func deviceID(path string) (uint64, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}
	for {
		info, err := os.Stat(abs)
		if err == nil {
			return uint64(info.Sys().(*syscall.Stat_t).Dev), nil
		}
		if !os.IsNotExist(err) || filepath.Dir(abs) == abs {
			return 0, err
		}
		abs = filepath.Dir(abs)
	}
}
