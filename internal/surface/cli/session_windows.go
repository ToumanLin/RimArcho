package cli

import "os/exec"

// ownSession: a Windows GUI program has no terminal to be hung up by.
func ownSession(*exec.Cmd) {}
