package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// steamCMDEnv is the environment for a SteamCMD process; nil inherits archo's.
// On Linux SteamCMD otherwise shares the desktop Steam client's ~/.steam and
// ~/.local/share/Steam, and can rewrite its registry and library list. It gets a
// home of its own (17 §2.1). Windows SteamCMD keeps its state in its own folder.
func steamCMDEnv(exe string) ([]string, error) {
	if runtime.GOOS != "linux" {
		return nil, nil
	}
	home, err := steamCMDHome(exe)
	if err != nil {
		return nil, err
	}
	config, data := filepath.Join(home, ".config"), filepath.Join(home, ".local", "share")
	for _, d := range []string{config, data} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, fmt.Errorf("SteamCMD needs a home folder of its own: %w", err)
		}
	}
	// exec uses the last value of a duplicated key, so these override archo's.
	return append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+config, "XDG_DATA_HOME="+data), nil
}

// steamCMDHome is "home" next to the program when that folder is writable (RimArcho's
// own copy, or one the player unpacked), else under the user cache folder (a
// distribution's steamcmd in /usr/games). A sign-in is kept there between runs.
func steamCMDHome(exe string) (string, error) {
	abs, err := filepath.Abs(exe)
	if err != nil {
		return "", err
	}
	home := filepath.Join(filepath.Dir(abs), "home")
	if os.MkdirAll(home, 0o700) == nil {
		return home, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil || !filepath.IsAbs(cache) {
		return "", fmt.Errorf("SteamCMD needs a home folder of its own, and %s is not writable: set an absolute HOME or XDG_CACHE_HOME", filepath.Dir(abs))
	}
	home = filepath.Join(cache, "rimarcho", "steamcmd-home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return "", fmt.Errorf("SteamCMD needs a home folder of its own: %w", err)
	}
	return home, nil
}
