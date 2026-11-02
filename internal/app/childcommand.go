package app

import (
	"context"
	"os"
	"os/exec"
)

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
	c.Env = append(os.Environ(), "RIMARCHO_HOME="+home)
	hideWindow(c)
	out, err := c.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	if e, ok := err.(*exec.ExitError); ok {
		return string(out), e.ExitCode()
	}
	return string(out) + "error: " + err.Error() + "\n", 1
}
