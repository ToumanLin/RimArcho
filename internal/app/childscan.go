package app

import (
	"os"
	"os/exec"
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
	args := append([]string{"scan"}, flags...)
	if envName != "" {
		args = append(args, "--env", envName)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "RIMARCHO_HOME="+home)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
