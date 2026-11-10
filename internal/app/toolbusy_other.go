//go:build !windows && !linux

package app

import "errors"

func toolIdle(string, string) error {
	return errors.New("tool replacement requires process detection on this platform")
}
