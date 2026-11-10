//go:build linux

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SteamCMD must not see the player's home, where the desktop Steam client keeps
// its registry and library list.
func TestSteamCMDOwnHome(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	script := "#!/bin/sh\necho \"HOME=$HOME\"\necho \"CONFIG=$XDG_CONFIG_HOME\"\necho \"DATA=$XDG_DATA_HOME\"\n"
	write := func(dir string) string {
		exe := filepath.Join(dir, "steamcmd.sh")
		if err := os.WriteFile(exe, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return exe
	}
	check := func(t *testing.T, out, home string) {
		t.Helper()
		for _, want := range []string{"HOME=" + home, "CONFIG=" + filepath.Join(home, ".config"), "DATA=" + filepath.Join(home, ".local", "share")} {
			if !strings.Contains(out, want+"\n") {
				t.Fatalf("missing %q in:\n%s", want, out)
			}
		}
	}

	own := t.TempDir()
	exe := write(own)
	home := filepath.Join(own, "home")
	out, err := runSteamCMD(exe, nil)
	if err != nil {
		t.Fatal(err)
	}
	check(t, out, home)
	out, err = runBranchSteamCMD(context.Background(), exe, nil)
	if err != nil {
		t.Fatal(err)
	}
	check(t, out, home)
	logPath := filepath.Join(t.TempDir(), "out")
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = f // the console run writes to archo's terminal
	p, err := startWorkshopCMD(exe, nil, true)
	os.Stdout = stdout
	if err == nil {
		err = p.Wait()
	}
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(logPath)
	check(t, string(b), home)

	// A program in a folder that cannot be written (a distribution's /usr/games/steamcmd)
	// gets its home under the cache folder.
	if os.Geteuid() == 0 {
		t.Skip("root can write a read-only folder")
	}
	ro := t.TempDir()
	exe = write(ro)
	if err := os.Chmod(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(ro, 0o755) })
	out, err = runSteamCMD(exe, nil)
	if err != nil {
		t.Fatal(err)
	}
	check(t, out, filepath.Join(cache, "rimarcho", "steamcmd-home"))
}
