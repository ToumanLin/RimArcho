package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/source"
)

// steamCMDAliasDir is the folder under %PUBLIC% that holds the junctions SteamCMD starts
// through (steamCMDLaunchPath); uninstall removes the ones pointing into its home (02 §2).
func steamCMDAliasDir() string {
	pub := os.Getenv("PUBLIC")
	if pub == "" {
		return ""
	}
	return filepath.Join(pub, "RimArcho")
}

// steamCMDLaunchPath is the path a SteamCMD program is started from (17 §2.1). SteamCMD
// refuses to run from a folder whose path has non-English characters ("cannot run from a
// folder path that includes non-English characters"), which a Windows user name
// in Chinese puts in %LOCALAPPDATA%\RimArcho\steamcmd: it then exits at once and every
// download said only "SteamCMD exited before confirming the download" (VM 2026-11-23).
// Such a copy starts through a junction under %PUBLIC%\RimArcho (C:\Users\Public: ASCII,
// open to every user) that points at its own folder, so its files, sign-in and downloads
// stay where they are, behind the profile's permissions. Elsewhere, and for ASCII paths, the
// path is exe itself.
func steamCMDLaunchPath(exe string) (string, error) {
	if runtime.GOOS != "windows" || isASCII(exe) {
		return exe, nil
	}
	dir := filepath.Dir(exe)
	root := steamCMDAliasDir()
	if root == "" || !isASCII(root) {
		return "", steamCMDPathError(dir, nil)
	}
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(dir))))
	alias := filepath.Join(root, "steamcmd-"+hex.EncodeToString(sum[:])[:10])
	if real, err := source.RealPath(alias); err == nil && source.IsLink(alias) && samePath(real, dir) {
		return filepath.Join(alias, filepath.Base(exe)), nil
	}
	if source.IsLink(alias) {
		if err := source.RemoveLink(alias); err != nil {
			return "", steamCMDPathError(dir, err)
		}
	} else if _, err := os.Lstat(alias); err == nil {
		return "", steamCMDPathError(dir, fmt.Errorf("%s is in the way", alias))
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", steamCMDPathError(dir, err)
	}
	if err := source.CreateLink(alias, dir); err != nil {
		return "", steamCMDPathError(dir, err)
	}
	return filepath.Join(alias, filepath.Base(exe)), nil
}

func steamCMDPathError(dir string, err error) error {
	why := ""
	if err != nil {
		why = " (" + err.Error() + ")"
	}
	return fmt.Errorf("SteamCMD cannot run from a folder whose path has non-English characters (%s), and no link to it could be made under %%PUBLIC%%\\RimArcho%s; put steamcmd.exe in a folder with English letters only and choose it: archo config set steamcmd.path <folder>\\steamcmd.exe", dir, why)
}

// removeSteamCMDAliases deletes the junctions steamCMDLaunchPath made that point into
// under (uninstall, 02 §2), and the folder when nothing else is left in it. Only links are
// removed, never what they point at.
func removeSteamCMDAliases(under string) []string {
	root := steamCMDAliasDir()
	ents, err := os.ReadDir(root)
	if root == "" || err != nil {
		return nil
	}
	if r, err := source.RealPath(under); err == nil {
		under = r
	}
	var gone []string
	for _, e := range ents {
		p := filepath.Join(root, e.Name())
		if !strings.HasPrefix(e.Name(), "steamcmd-") || !source.IsLink(p) {
			continue
		}
		if t, err := source.RealPath(p); err == nil && !source.Within(t, under) {
			continue
		}
		if source.RemoveLink(p) == nil {
			gone = append(gone, p)
		}
	}
	if left, err := os.ReadDir(root); err == nil && len(left) == 0 {
		_ = os.Remove(root)
	}
	return gone
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
