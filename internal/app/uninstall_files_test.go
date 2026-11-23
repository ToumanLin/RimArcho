package app

import (
	"slices"
	"testing"
)

// The Linux tarball's two programs are removed by uninstall; Windows keeps its exact list.
func TestProgramFilesByPlatform(t *testing.T) {
	linux := programFiles("linux")
	if !slices.Contains(linux, "archo") || !slices.Contains(linux, "rimarcho") {
		t.Fatalf("linux: %v", linux)
	}
	win := programFiles("windows")
	if slices.Contains(win, "archo") || slices.Contains(win, "rimarcho") || !slices.Contains(win, "RimArcho.exe") {
		t.Fatalf("windows: %v", win)
	}
}
