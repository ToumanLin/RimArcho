package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ChildScan runs `<this executable> scan --env <name>` as a separate process with the same
// data directory, so the scan's memory never stays in an interface process (17 §3). It
// returns the command's output.
func ChildScan(home, envName string, flags ...string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return ChildScanExe(exe, home, envName, flags...)
}

// ChildMonitor runs `<this executable> monitor install` in a separate process (19 §6): the
// install may scan, and scans never run in an interface process.
func ChildMonitor(home, envName string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return ChildMonitorExe(exe, home, envName)
}

// ChildMonitorExe is ChildMonitor with an explicit archo executable.
func ChildMonitorExe(exe, home, envName string) (string, error) {
	args := []string{"monitor", "install"}
	if envName != "" {
		args = append(args, "--env", envName)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "RIMARCHO_HOME="+home)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ChildScanExe is ChildScan with an explicit archo executable; flags go to scan
// (--game-only).
func ChildScanExe(exe, home, envName string, flags ...string) (string, error) {
	return ChildScanProgress(exe, home, envName, nil, flags...)
}

// ChildScanProgress is ChildScanExe that hands the scan's progress lines to progress as
// they come (nil ignores them). They are left out of the returned output.
func ChildScanProgress(exe, home, envName string, progress func(ScanProgress), flags ...string) (string, error) {
	args := append([]string{"scan"}, flags...)
	if progress != nil {
		args = append(args, "--progress")
	}
	if envName != "" {
		args = append(args, "--env", envName)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "RIMARCHO_HOME="+home)
	hideWindow(cmd)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		return "", err
	}
	var out strings.Builder
	read := make(chan struct{})
	go func() {
		defer close(read)
		br := bufio.NewReader(pr)
		for {
			line, err := br.ReadString('\n')
			if p, ok := ParseScanProgress(line); ok {
				if progress != nil {
					progress(p)
				}
			} else {
				out.WriteString(line)
			}
			if err != nil {
				io.Copy(io.Discard, pr) // never block the child on a full pipe
				return
			}
		}
	}()
	err := cmd.Wait()
	pw.Close()
	<-read
	return out.String(), err
}

// ScanProgressLine is how `archo scan` reports its progress on stderr.
func ScanProgressLine(p ScanProgress) string {
	if p.Saving {
		return fmt.Sprintf("progress: scan saving %d", p.Total)
	}
	return fmt.Sprintf("progress: scan mods %d/%d", p.Done, p.Total)
}

var scanProgressRe = regexp.MustCompile(`^progress: scan (?:mods (\d+)/(\d+)|saving (\d+))$`)

// ParseScanProgress reads a ScanProgressLine back.
func ParseScanProgress(line string) (ScanProgress, bool) {
	m := scanProgressRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return ScanProgress{}, false
	}
	if m[3] != "" {
		n, _ := strconv.Atoi(m[3])
		return ScanProgress{Saving: true, Done: n, Total: n}, true
	}
	d, _ := strconv.Atoi(m[1])
	t, _ := strconv.Atoi(m[2])
	return ScanProgress{Done: d, Total: t}, true
}

// stripScanProgress drops a scan's progress lines from a command's output.
func stripScanProgress(out string) string {
	if !strings.Contains(out, "progress: scan ") {
		return out
	}
	var b strings.Builder
	for _, l := range strings.SplitAfter(out, "\n") {
		if _, ok := ParseScanProgress(l); !ok {
			b.WriteString(l)
		}
	}
	return b.String()
}
