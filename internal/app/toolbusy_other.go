//go:build !windows

package app

import "errors"

func toolIdle(string) error { return errors.New("tool replacement requires Windows process detection") }
