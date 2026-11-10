//go:build !windows

package accounts

import "os/exec"

func hideLoginHost(*exec.Cmd)             {}
func (s *Session) login(check bool) error { return ErrSession }

// WebView2Installed is true outside Windows, where the window does not use WebView2.
func WebView2Installed() bool { return true }
