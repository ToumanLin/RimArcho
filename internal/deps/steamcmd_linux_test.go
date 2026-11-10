//go:build linux

package deps

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSteamCMDOnPATHIsExternal(t *testing.T) {
	dir := t.TempDir()
	external := file(t, filepath.Join(dir, "steamcmd"))
	t.Setenv("PATH", dir)
	f, _ := fixture(t)
	file(t, filepath.Join(f.LocalDir, SteamCMD, SteamCMDExecutable()))
	f.LookPath = exec.LookPath
	i := f.Status(SteamCMD)
	if i.Path != external || i.Source != "path" || i.Own || !i.OK {
		t.Fatal(i)
	}
}
