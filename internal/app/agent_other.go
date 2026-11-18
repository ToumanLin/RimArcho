//go:build !windows

package app

import "errors"

// startAgentConsole: Archo Agent is only built for Windows.
func startAgentConsole(exe string, args []string, dir string, env []string) error {
	return errors.New("Archo Agent is only built for Windows")
}
