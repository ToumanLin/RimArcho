//go:build !windows

package app

import (
	"os"
	"time"
)

func logCreationTime(os.FileInfo) time.Time { return time.Time{} }
