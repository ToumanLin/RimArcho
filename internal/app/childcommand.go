package app

import (
	"context"
	"os"
	"os/exec"
)

// ChildEnvVar marks a process RimArcho started as a helper (a scan, an analysis, a
// conversion, a probe). Such a process never opens a window (17 §3): a host whose
// executable ignored the helper's arguments would otherwise open a window that starts the
// same helper again, without end.
const ChildEnvVar = "RIMARCHO_CHILD"

// ChildEnv is the environment of a helper process on data directory home.
func ChildEnv(home string) []string {
	return append(os.Environ(), "RIMARCHO_HOME="+home, ChildEnvVar+"=1")
}

// IsChild reports whether this process was started by RimArcho as a helper.
func IsChild() bool { return os.Getenv(ChildEnvVar) != "" }

// ChildCommandContext keeps scan/analysis memory in a child and returns its raw CLI
// output and exit status. Arguments go directly to the executable, never a shell.
func ChildCommandContext(ctx context.Context, exe, home, env string, argv []string) (string, int) {
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			return "error: " + err.Error() + "\n", 1
		}
	}
	args := append([]string(nil), argv...)
	if env != "" {
		args = append(args, "--env", env)
	}
	c := exec.CommandContext(ctx, exe, args...)
	c.Env = ChildEnv(home)
	hideWindow(c)
	raw, err := c.CombinedOutput()
	out := stripScanProgress(string(raw))
	if err == nil {
		return out, 0
	}
	if e, ok := err.(*exec.ExitError); ok {
		return out, e.ExitCode()
	}
	return out + "error: " + err.Error() + "\n", 1
}
