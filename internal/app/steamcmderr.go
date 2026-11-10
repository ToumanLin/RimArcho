package app

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type steamCMDLibraryError struct{ cause error }

func (e *steamCMDLibraryError) Error() string {
	return "SteamCMD could not start; it needs 32-bit libraries. Install lib32gcc-s1 on Debian/Ubuntu, glibc.i686 and libstdc++.i686 on Fedora, or lib32-gcc-libs on Arch, then retry"
}
func (e *steamCMDLibraryError) Unwrap() error { return e.cause }

// Shell launchers report a missing ELF loader or shared library as exit 126/127.
// Do not expose raw output: it can contain Steam authentication details.
func steamCMDStartError(exe, output string, err error) error {
	if err == nil || runtime.GOOS != "linux" {
		return err
	}
	var exit *exec.ExitError
	missing := errors.As(err, &exit) && (exit.ExitCode() == 126 || exit.ExitCode() == 127)
	missing = missing || strings.Contains(output, "error while loading shared libraries")
	if errors.Is(err, os.ErrNotExist) {
		if _, statErr := os.Stat(exe); statErr == nil {
			missing = true
		}
	}
	if !missing {
		return err
	}
	return &steamCMDLibraryError{cause: err}
}
