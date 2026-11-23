package fsscan

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

var deviceKinds sync.Map

// storageKind reads queue/rotational of the disk under the file system holding path:
// /sys/dev/block/<major>:<minor> for its device, or the mount's source device when the
// file system has no block device number of its own (btrfs subvolumes). Partitions go to
// their disk; device-mapper, LVM and md volumes to the disks below them. Network, tmpfs
// and overlay file systems are unknown.
func storageKind(path string) diskKind {
	path, err := filepath.Abs(path)
	if err != nil {
		return diskUnknown
	}
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	var st unix.Stat_t
	if unix.Stat(path, &st) != nil {
		return diskUnknown
	}
	key := st.Dev
	if cached, ok := deviceKinds.Load(key); ok {
		return cached.(diskKind)
	}
	kind := deviceKind("/sys", "/dev", unix.Major(st.Dev), unix.Minor(st.Dev))
	if kind == diskUnknown {
		kind = sourceKind("/sys", "/dev", mountSource("/proc/self/mountinfo", path))
	}
	cached, _ := deviceKinds.LoadOrStore(key, kind)
	return cached.(diskKind)
}

// deviceKind is the kind of block device major:minor under the sys tree.
func deviceKind(sys, dev string, major, minor uint32) diskKind {
	if major == 0 {
		return diskUnknown // anonymous device: no block device behind the number
	}
	link := filepath.Join(sys, "dev", "block", strconv.FormatUint(uint64(major), 10)+":"+strconv.FormatUint(uint64(minor), 10))
	dir, err := filepath.EvalSymlinks(link)
	if err != nil {
		return diskUnknown
	}
	return blockKind(dir, 0)
}

// sourceKind is the kind of a mount source such as /dev/nvme0n1p2 or /dev/mapper/root.
func sourceKind(sys, dev, source string) diskKind {
	if !strings.HasPrefix(source, "/dev/") {
		return diskUnknown
	}
	source = filepath.Join(dev, strings.TrimPrefix(source, "/dev/"))
	if target, err := filepath.EvalSymlinks(source); err == nil {
		source = target
	}
	dir, err := filepath.EvalSymlinks(filepath.Join(sys, "class", "block", filepath.Base(source)))
	if err != nil {
		return diskUnknown
	}
	return blockKind(dir, 0)
}

// blockKind reads the disk kind of the block device whose sysfs folder is dir. A partition
// (it has a partition file) is its parent disk; a stacked device (slaves/ not empty) is
// rotating when any device below it is, and an SSD when all are.
func blockKind(dir string, depth int) diskKind {
	if depth > 8 {
		return diskUnknown
	}
	if _, err := os.Stat(filepath.Join(dir, "partition")); err == nil {
		dir = filepath.Dir(dir)
	}
	if slaves, err := os.ReadDir(filepath.Join(dir, "slaves")); err == nil && len(slaves) > 0 {
		kind := diskSSD
		for _, s := range slaves {
			below, err := filepath.EvalSymlinks(filepath.Join(dir, "slaves", s.Name()))
			if err != nil {
				return diskUnknown
			}
			switch blockKind(below, depth+1) {
			case diskRotating:
				return diskRotating
			case diskUnknown:
				kind = diskUnknown
			}
		}
		return kind
	}
	b, err := os.ReadFile(filepath.Join(dir, "queue", "rotational"))
	if err != nil {
		return diskUnknown
	}
	switch strings.TrimSpace(string(b)) {
	case "1":
		return diskRotating
	case "0":
		return diskSSD
	}
	return diskUnknown
}

// mountSource is the source device of the deepest mount holding path, from a
// mountinfo file ("" when none is found).
func mountSource(mountinfo, path string) string {
	f, err := os.Open(mountinfo)
	if err != nil {
		return ""
	}
	defer f.Close()
	best, source := -1, ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		// id parent major:minor root mountpoint options [optional...] - fstype source super
		left, right, ok := strings.Cut(sc.Text(), " - ")
		fields, tail := strings.Fields(left), strings.Fields(right)
		if !ok || len(fields) < 5 || len(tail) < 2 {
			continue
		}
		mount := unescapeMount(fields[4])
		if !within(path, mount) || len(mount) <= best {
			continue
		}
		best, source = len(mount), unescapeMount(tail[1])
	}
	return source
}

// unescapeMount decodes mountinfo's octal escapes (\040 for a space).
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func within(path, mount string) bool {
	if mount == "/" {
		return strings.HasPrefix(path, "/")
	}
	return path == mount || strings.HasPrefix(path, mount+"/")
}
