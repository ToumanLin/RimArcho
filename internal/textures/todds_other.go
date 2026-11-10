//go:build !windows

package textures

// The Linux 0.4.1 release requires GLIBCXX_3.4.32, absent on older common
// distributions. Use a locally built or otherwise compatible native copy.
const (
	ToddsExecutable = "todds"
	ToddsZipURL     = ""
	ToddsZipSize    = 0
	ToddsZipSHA256  = ""
	ToddsMissing    = "Texture optimisation needs a Linux todds 0.4.1. Build it from https://github.com/todds-encoder/todds (tag 0.4.1), then put todds on PATH or set textures.todds to its absolute path."
)
