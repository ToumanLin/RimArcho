package deps

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/textures"
)

func file(t *testing.T, p string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func fixture(t *testing.T) (*Finder, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	settings := map[string]string{}
	f := &Finder{LocalDir: filepath.Join(dir, "local"),
		Setting: func(k string) string { return settings[k] }, Version: func(name, path string) (string, error) { return textures.ToddsVersion, nil }}
	return f, settings
}

func TestToddsOrderAndCompatibility(t *testing.T) {
	f, settings := fixture(t)
	configured := file(t, filepath.Join(t.TempDir(), textures.ToddsExecutable))
	own := file(t, filepath.Join(f.LocalDir, Todds, textures.ToddsExecutable))
	fromPath := file(t, filepath.Join(t.TempDir(), textures.ToddsExecutable))
	f.LookPath = func(n string) (string, error) {
		if n != "todds" {
			t.Fatalf("PATH lookup %q", n)
		}
		return fromPath, nil
	}
	settings["textures.todds"] = filepath.Dir(configured)
	for _, tc := range []struct {
		path, source string
		own          bool
	}{{configured, "config", false}, {own, "own", true}, {fromPath, "path", false}} {
		i := f.Status(Todds)
		if i.Path != tc.path || i.Source != tc.source || i.Own != tc.own || !i.OK || i.Installable != (textures.ToddsZipURL != "") || i.ConfigKey != "textures.todds" {
			t.Fatalf("status %+v, want %+v", i, tc)
		}
		if err := os.Remove(tc.path); err != nil {
			t.Fatal(err)
		}
	}
	f.LookPath = nil
	if i := f.Status(Todds); i.OK || i.Path != "" || i.Err == "" {
		t.Fatalf("missing %+v", i)
	}
	file(t, own)
	f.Version = func(string, string) (string, error) { return "0.1.0", nil }
	f.probes = probeCache{}
	path, version, ok := f.Locate(Todds)
	if path != own || version != "0.1.0" || ok || !strings.Contains(f.Status(Todds).Err, textures.ToddsVersion) {
		t.Fatalf("incompatible %q %q %v", path, version, ok)
	}
	if err := os.WriteFile(own, []byte("different size"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.Version = func(string, string) (string, error) { return "", errors.New("cannot execute") }
	if i := f.Status(Todds); i.OK || i.Err != "cannot execute" {
		t.Fatalf("probe failure %+v", i)
	}
}

func TestSteamCMDOrderWithoutRunningIt(t *testing.T) {
	f, settings := fixture(t)
	f.Version = func(string, string) (string, error) { t.Fatal("SteamCMD must not be run"); return "", nil }
	configured := file(t, filepath.Join(t.TempDir(), SteamCMDExecutable()))
	own := file(t, filepath.Join(f.LocalDir, SteamCMD, SteamCMDExecutable()))
	fromPath := file(t, filepath.Join(t.TempDir(), "steamcmd"))
	f.LookPath = func(n string) (string, error) {
		if n != "steamcmd" {
			t.Fatalf("PATH %q", n)
		}
		return fromPath, nil
	}
	settings["steamcmd.path"] = configured
	if i := f.Status(SteamCMD); i.Path != configured || i.Source != "config" || i.Own || i.Version != "" {
		t.Fatalf("configured %+v", i)
	}
	// Preserve the legacy fallback to siblings of a configured missing executable.
	settings["steamcmd.path"] = filepath.Join(filepath.Dir(configured), "missing.exe")
	if i := f.Status(SteamCMD); i.Path != configured {
		t.Fatalf("sibling fallback %+v", i)
	}
	settings["steamcmd.path"] = ""
	if runtime.GOOS == "linux" {
		if i := f.Status(SteamCMD); i.Path != fromPath || i.Own || i.Source != "path" {
			t.Fatalf("external preference %+v", i)
		}
		f.LookPath = nil
	}
	if i := f.Status(SteamCMD); i.Path != own || !i.Own || !i.OK {
		t.Fatalf("own %+v", i)
	}
	if err := os.Remove(own); err != nil {
		t.Fatal(err)
	}
	f.LookPath = func(string) (string, error) { return fromPath, nil }
	if i := f.Status(SteamCMD); i.Path != fromPath || i.Source != "path" || i.Own {
		t.Fatalf("PATH %+v", i)
	}
	f.LookPath = nil
	if i := f.Status(SteamCMD); i.OK {
		t.Fatalf("nil LookPath %+v", i)
	}
}

func TestAllAndUnknown(t *testing.T) {
	f, _ := fixture(t)
	var names []string
	for _, i := range f.All() {
		names = append(names, i.Name)
	}
	if !reflect.DeepEqual(names, Names) {
		t.Fatalf("names %v", names)
	}
	if i := f.Status("unknown"); i.OK || i.Err == "" || i.Installable {
		t.Fatalf("unknown %+v", i)
	}
}

func TestVersionCacheInvalidatesByPathSizeAndMtime(t *testing.T) {
	f, settings := fixture(t)
	calls := 0
	f.Version = func(string, string) (string, error) { calls++; return textures.ToddsVersion, nil }
	p := file(t, filepath.Join(f.LocalDir, Todds, textures.ToddsExecutable))
	for range 3 {
		f.Status(Todds)
	}
	if calls != 1 {
		t.Fatalf("cached probe calls %d", calls)
	}
	if err := os.WriteFile(p, []byte("different size"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.Status(Todds)
	if calls != 2 {
		t.Fatalf("size change calls %d", calls)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	stamp := st.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(p, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	f.Status(Todds)
	if calls != 3 {
		t.Fatalf("mtime change calls %d", calls)
	}
	settings["textures.todds"] = file(t, filepath.Join(t.TempDir(), textures.ToddsExecutable))
	f.Status(Todds)
	if calls != 4 {
		t.Fatalf("path change calls %d", calls)
	}
}

// A failed probe may be passing (a virus scanner holding the file): it is not cached, so the
// next status sees the tool once it runs.
func TestVersionCacheRetriesFailures(t *testing.T) {
	f, _ := fixture(t)
	calls := 0
	f.Version = func(string, string) (string, error) { calls++; return "", errors.New("access denied") }
	file(t, filepath.Join(f.LocalDir, Todds, textures.ToddsExecutable))
	for range 2 {
		if f.Status(Todds).OK {
			t.Fatal("failing tool usable")
		}
	}
	if calls != 2 {
		t.Fatalf("failed probe ran %d times, want 2", calls)
	}
	f.Version = func(string, string) (string, error) { calls++; return textures.ToddsVersion, nil }
	if !f.Status(Todds).OK {
		t.Fatal("tool still unusable after the probe succeeds")
	}
}

func TestRedirectedOwnDirectoryIsNotOwned(t *testing.T) {
	f, _ := fixture(t)
	external := t.TempDir()
	file(t, filepath.Join(external, SteamCMDExecutable()))
	if err := os.MkdirAll(f.LocalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(f.LocalDir, SteamCMD)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if i := f.Status(SteamCMD); !i.OK || i.Own {
		t.Fatalf("redirected directory %+v", i)
	}
}
