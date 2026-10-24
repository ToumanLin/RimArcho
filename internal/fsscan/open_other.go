//go:build !windows

package fsscan

import "os"

func dirPath(path string) string { return path }

func openDir(path string) (*os.File, error) { return os.Open(path) }
