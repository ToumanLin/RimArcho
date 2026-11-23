//go:build !windows && !linux

package app

import (
	"os"
	"time"
)

func logCreationTime(string, os.FileInfo) time.Time { return time.Time{} }
