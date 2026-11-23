package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/source"
)

// cjkUser is a Chinese Windows user name, the one the VM round used.
var cjkUser = string([]rune{0x65b0, 0x73a9, 0x5bb6})

// SteamCMD exits at once from a folder whose path has non-English characters (a Chinese
// Windows user name, VM 2026-11-23); RimArcho starts such a copy through a junction under
// %PUBLIC%\RimArcho that points at its folder, reuses it, and uninstall removes it (17 §2.1).
func TestSteamCMDLaunchPathNonASCII(t *testing.T) {
	if runtime.GOOS != "windows" {
		if p, err := steamCMDLaunchPath("/home/"+cjkUser+"/steamcmd/steamcmd.sh"); err != nil || p != "/home/"+cjkUser+"/steamcmd/steamcmd.sh" {
			t.Fatalf("off Windows the path is kept: %q %v", p, err)
		}
		return
	}
	tmp := t.TempDir()
	local := filepath.Join(tmp, cjkUser, "Local", "RimArcho")
	dir := filepath.Join(local, "steamcmd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "steamcmd.exe")
	if err := os.WriteFile(exe, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	pub := filepath.Join(tmp, "Public")
	t.Setenv("PUBLIC", pub)

	ascii := filepath.Join(tmp, "Player", "steamcmd.exe")
	if p, err := steamCMDLaunchPath(ascii); err != nil || p != ascii {
		t.Fatalf("an English path is kept: %q %v", p, err)
	}
	p, err := steamCMDLaunchPath(exe)
	if err != nil || !isASCII(p) || !strings.HasPrefix(p, filepath.Join(pub, "RimArcho", "steamcmd-")) || filepath.Base(p) != "steamcmd.exe" {
		t.Fatalf("launch path %q %v", p, err)
	}
	if b, err := os.ReadFile(p); err != nil || string(b) != "x" {
		t.Fatalf("the link does not reach the program: %q %v", b, err)
	}
	if again, err := steamCMDLaunchPath(exe); err != nil || again != p {
		t.Fatalf("second start: %q %v", again, err)
	}
	// a link left pointing elsewhere is replaced
	alias := filepath.Dir(p)
	if err := source.RemoveLink(alias); err != nil {
		t.Fatal(err)
	}
	if err := source.CreateLink(alias, tmp); err != nil {
		t.Fatal(err)
	}
	if again, err := steamCMDLaunchPath(exe); err != nil || again != p {
		t.Fatalf("stale link: %q %v", again, err)
	}
	if real, err := source.RealPath(alias); err != nil || !samePath(real, dir) {
		t.Fatalf("stale link not replaced: %q %v", real, err)
	}

	// another home's link stays; this home's goes with its folder, the program stays
	other := filepath.Join(pub, "RimArcho", "steamcmd-other")
	if err := source.CreateLink(other, tmp); err != nil {
		t.Fatal(err)
	}
	if gone := removeSteamCMDAliases(local); len(gone) != 1 || gone[0] != alias {
		t.Fatalf("removed %v", gone)
	}
	if _, err := os.Stat(exe); err != nil || !source.IsLink(other) {
		t.Fatalf("uninstall removed more than its link: %v", err)
	}

	// no usable %PUBLIC%: an error that says what to do
	t.Setenv("PUBLIC", filepath.Join(tmp, string([]rune{0x516c, 0x7528})))
	if _, err := steamCMDLaunchPath(exe); err == nil || !strings.Contains(err.Error(), "non-English characters") || !strings.Contains(err.Error(), "steamcmd.path") {
		t.Fatalf("no link: %v", err)
	}
}
