//go:build !windows

package textures

// The Linux 0.4.1 release requires GLIBCXX_3.4.32, absent on older common
// distributions. Use a locally built or otherwise compatible native copy.
const (
	ToddsExecutable = "todds"
	ToddsZipURL     = ""
	ToddsZipSize    = 0
	ToddsZipSHA256  = ""
	ToddsMissing    = "Texture optimisation needs a Linux todds. The Linux build of todds 0.4.1 (https://github.com/todds-encoder/todds/releases/tag/0.4.1) runs on Ubuntu 24.04, Fedora 39, Arch and newer; on older systems, build it from source. Then put todds on PATH or set textures.todds to its full path."
)
