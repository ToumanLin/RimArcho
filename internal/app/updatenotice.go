package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// InstallKindFile is installed beside the resolved executable by Linux packages.
// Its UTF-8 content is one of deb, rpm or aur, followed by an optional newline.
// Missing, unreadable or unknown markers identify a tarball install (17 §5.1).
const InstallKindFile = "archo-install-kind"

// SelfUpdateSupported reports whether RimArcho can replace its own program files.
func SelfUpdateSupported() bool { return runtime.GOOS == "windows" }

// SelfUpdateInstructions describes the update method for the effective installation.
func (a *App) SelfUpdateInstructions() string {
	if dir, err := a.SelfProgramDir(); err == nil {
		b, _ := os.ReadFile(filepath.Join(dir, InstallKindFile))
		switch strings.TrimSpace(string(b)) {
		case "deb":
			return "update RimArcho through your Debian/Ubuntu package manager"
		case "rpm":
			return "update RimArcho through your RPM package manager"
		case "aur":
			return "update RimArcho through your AUR package manager"
		}
	}
	return "download the new Linux tarball from the release page and extract it"
}
