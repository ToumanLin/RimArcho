// Package procfs reads memory counters from Linux proc text. All readers accept
// synthetic input; no parser depends on the host's process tree.
package procfs

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ReadKB returns only requested fields, in bytes. Missing fields stay absent;
// zero is a valid reading. Proc labels kB are kibibytes, not decimal kilobytes.
func ReadKB(r io.Reader, keys ...string) (map[string]int64, error) {
	wanted := make(map[string]bool, len(keys))
	for _, key := range keys {
		wanted[key] = true
	}
	values := make(map[string]int64, len(keys))
	scan := bufio.NewScanner(r)
	for scan.Scan() {
		key, text, ok := strings.Cut(scan.Text(), ":")
		if !ok || !wanted[key] {
			continue
		}
		f := strings.Fields(text)
		if len(f) != 2 || f[1] != "kB" {
			return nil, fmt.Errorf("invalid proc memory field %s", key)
		}
		n, err := strconv.ParseUint(f[0], 10, 53)
		if err != nil {
			return nil, fmt.Errorf("invalid proc memory field %s: %w", key, err)
		}
		values[key] = int64(n) * 1024
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

type Memory struct{ RSS, Private, Peak int64 }

// Status reads resident and peak bytes. RssAnon is anonymous resident memory,
// the closest available private-memory counter here; it excludes file mappings
// and swapped-out pages and is not Windows committed PrivateUsage.
func Status(r io.Reader) (Memory, error) {
	v, err := ReadKB(r, "VmRSS", "VmHWM", "RssAnon")
	if err != nil {
		return Memory{}, err
	}
	return Memory{RSS: v["VmRSS"], Private: v["RssAnon"], Peak: v["VmHWM"]}, nil
}

// StatusAt reads self or a PID in a real or synthetic proc root. A process that
// exited, is inaccessible or has no memory fields yields no invented counters.
func StatusAt(root, pid string) (Memory, error) {
	f, err := os.Open(filepath.Join(root, pid, "status"))
	if err != nil {
		return Memory{}, err
	}
	defer f.Close()
	return Status(f)
}
