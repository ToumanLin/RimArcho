package source

import (
	"runtime"
	"testing"
)

// VolumeFS names the temp folder's file system on Windows (08 §9 checks Mods/ with it).
func TestVolumeFS(t *testing.T) {
	fs := VolumeFS(t.TempDir())
	if runtime.GOOS == "windows" && fs == "" {
		t.Fatal("no file system name for the temp folder")
	}
	t.Logf("temp folder is on %q", fs)
}
