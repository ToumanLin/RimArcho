//go:build !windows

package accounts

import "os/exec"

func hideLoginHost(*exec.Cmd)             {}
func (s *Session) login(check bool) error { return ErrSession }
