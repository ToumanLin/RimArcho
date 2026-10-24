//go:build !windows

package accounts

import "os/exec"

func hideLoginHost(*exec.Cmd)   {}
func (s *Session) login() error { return ErrSession }
