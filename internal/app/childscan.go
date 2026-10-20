package app

import (
	"os"
	"os/exec"
)

// ChildScan runs `<this executable> scan --env <name>` as a separate process with the same
// data directory, so the scan's memory never stays in an interface process (07 §3). It
// returns the command's output.
func ChildScan(home, envName string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return ChildScanExe(exe, home, envName)
}

// ChildScanExe is ChildScan with an explicit archo executable.
func ChildScanExe(exe, home, envName string) (string, error) {
	args := []string{"scan"}
	if envName != "" {
		args = append(args, "--env", envName)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "RIMARCHO_HOME="+home)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
