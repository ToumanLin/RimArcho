package accounts

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// Login starts a dedicated native host process, with no Wails runtime or services.
// Only the local session-directory path crosses the process boundary.
func Login(dir string) error {
	exe, err := os.Executable()
	if err != nil {
		return ErrSession
	}
	cmd := exec.Command(exe, "account-login-helper", dir)
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
		return errors.New("登录窗口未完成或会话清理失败，请重试；也可在浏览器手动下载")
	}
	return nil
}

// Helper is handled before CLI/database initialization. It never opens player data.
func Helper(args []string) (bool, int) {
	if len(args) == 0 || args[0] != "account-login-helper" {
		return false, 0
	}
	if len(args) != 2 {
		return true, 1
	}
	for _, key := range []string{"WEBVIEW2_PIPE_FOR_SCRIPT_DEBUGGER", "WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "WEBVIEW2_BROWSER_EXECUTABLE_FOLDER", "WEBVIEW2_USER_DATA_FOLDER"} {
		_ = os.Setenv(key, "")
	}
	s, err := Open(args[1])
	if err != nil {
		return true, 1
	}
	if s.login() != nil {
		return true, 1
	}
	return true, 0
}
