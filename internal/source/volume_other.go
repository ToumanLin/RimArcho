//go:build !windows

package source

// VolumeFS is unknown outside Windows: symbolic links work on the usual file systems.
func VolumeFS(dir string) string { return "" }
