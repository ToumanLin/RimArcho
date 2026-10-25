package accounts

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Login starts a dedicated native host process, with no Wails runtime or services.
// Only the local session-directory path crosses the process boundary.
func Login(dir string) error { return runLoginHost(dir, false) }

func runLoginHost(dir string, check bool) error {
	if err := privateDir(dir); err != nil {
		return helperFailure("session directory unavailable", 0)
	}
	log := filepath.Join(dir, "login-helper.log")
	if safePath(log) != nil {
		return helperFailure("session directory unavailable", 0)
	}
	_ = os.Remove(log)
	exe, err := os.Executable()
	if err != nil {
		return ErrSession
	}
	args := []string{"account-login-helper", dir}
	if check {
		args = append(args, "--check")
	}
	cmd := exec.Command(exe, args...)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(e), "WEBVIEW2_") && !strings.HasPrefix(e, "RIMARCHO_DEVTOOLS_PORT=") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	// Empty values suppress registry overrides too; removing them alone does not.
	for _, key := range []string{"WEBVIEW2_PIPE_FOR_SCRIPT_DEBUGGER", "WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "WEBVIEW2_BROWSER_EXECUTABLE_FOLDER", "WEBVIEW2_USER_DATA_FOLDER"} {
		cmd.Env = append(cmd.Env, key+"=")
	}
	hideLoginHost(cmd)
	if cmd.Run() != nil {
		if reason, err := os.ReadFile(log); err == nil && len(reason) > 0 && len(reason) <= 256 {
			return errors.New(strings.TrimSpace(string(reason)))
		}
		return helperFailure("login helper did not complete", 0)
	}
	return nil
}

// Helper is handled before CLI/database initialization. It never opens player data.
func Helper(args []string) (handled bool, code int) {
	if len(args) == 0 || args[0] != "account-login-helper" {
		return false, 0
	}
	if len(args) != 2 && !(len(args) == 3 && args[2] == "--check") {
		return true, 1
	}
	for _, key := range []string{"WEBVIEW2_PIPE_FOR_SCRIPT_DEBUGGER", "WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "WEBVIEW2_BROWSER_EXECUTABLE_FOLDER", "WEBVIEW2_USER_DATA_FOLDER"} {
		_ = os.Setenv(key, "")
	}
	defer func() {
		if recover() != nil {
			writeHelperLog(args[1], helperFailure("login helper stopped unexpectedly", 0))
			handled, code = true, 1
		}
	}()
	s, err := Open(args[1])
	if err != nil {
		writeHelperLog(args[1], helperFailure("session directory unavailable", 0))
		return true, 1
	}
	err = s.login(len(args) == 3)
	writeHelperLog(args[1], err)
	if err != nil {
		return true, 1
	}
	return true, 0
}

// Only fixed stage names and HRESULTs enter this log; never OS error strings,
// URLs, account names, cookie values, or browser response bodies.
type loginHelperError string

func (e loginHelperError) Error() string { return string(e) }
func helperFailure(stage string, code uintptr) error {
	if code != 0 {
		return loginHelperError(fmt.Sprintf("%s 0x%08X", stage, uint32(code)))
	}
	return loginHelperError(stage)
}
func writeHelperLog(dir string, err error) {
	if privateDir(dir) != nil {
		return
	}
	log := filepath.Join(dir, "login-helper.log")
	if safePath(log) != nil {
		return
	}
	reason := "login window completed"
	if err != nil {
		reason = "login window closed without a usable session"
		var failure loginHelperError
		if errors.As(err, &failure) {
			reason = failure.Error()
		}
	}
	_ = os.WriteFile(log, []byte(reason+"\n"), 0o600)
}

// LoginCheck uses a fresh session directory and saves no account cookies. It
// runs before the command surfaces open the player's database.
func LoginCheck() (string, error) {
	dir, err := os.MkdirTemp("", "RimArcho-login-check-")
	if err != nil {
		return "", helperFailure("session directory unavailable", 0)
	}
	return filepath.Join(dir, "login-helper.log"), runLoginHost(dir, true)
}
