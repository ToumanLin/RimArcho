package gametest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRefuseGameRootWrites(t *testing.T) {
	for _, which := range []string{"data", "run", "equal", "config-link", "log-link"} {
		t.Run(which, func(t *testing.T) {
			o := testOptions(t, "idle", false)
			switch which {
			case "data":
				o.DataDir = filepath.Join(o.GameRoot, "new", "data")
			case "run":
				o.RunDir = filepath.Join(o.GameRoot, "new", "run")
			case "equal":
				o.RunDir = o.GameRoot
			case "config-link":
				o.DataDir = filepath.Join(filepath.Dir(o.GameRoot), "linked-data")
				if err := os.Mkdir(o.DataDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(o.GameRoot, filepath.Join(o.DataDir, "Config")); err != nil {
					t.Skipf("directory symlink unavailable: %v", err)
				}
			case "log-link":
				if err := os.Mkdir(o.RunDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(o.GameRoot, "Version.txt"), filepath.Join(o.RunDir, "Player.log")); err != nil {
					t.Skipf("file symlink unavailable: %v", err)
				}
			}
			called := false
			o.Start = func(*exec.Cmd) error { called = true; return nil }
			if r, err := Run(context.Background(), o); err == nil || r != nil || called {
				t.Fatalf("guard: result %+v, error %v, called %v", r, err, called)
			}
			b, err := os.ReadFile(filepath.Join(o.GameRoot, "Version.txt"))
			if err != nil || string(b) != "1.6.4871 rev1261" {
				t.Fatalf("game root changed: %q, %v", b, err)
			}
		})
	}
}

func TestManagedExtraArgs(t *testing.T) {
	for _, arg := range []string{"-savedatafolder=elsewhere", "-logfile", "-NoFileLog", "quicktest"} {
		t.Run(arg, func(t *testing.T) {
			o := testOptions(t, "idle", false)
			o.ExtraArgs = []string{arg}
			if _, err := Run(context.Background(), o); err == nil || !strings.Contains(err.Error(), "managed option") {
				t.Fatalf("argument %q: %v", arg, err)
			}
		})
	}
}

func TestProtonRefusesIsolatedRun(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux Proton launch guard")
	}
	o := testOptions(t, "idle", false)
	base := t.TempDir()
	o.GameRoot = filepath.Join(base, "steamapps", "common", "RimWorld")
	o.DataDir, o.RunDir = filepath.Join(base, "data"), filepath.Join(base, "run")
	writeTestFile(t, filepath.Join(o.GameRoot, "RimWorldWin64.exe"), "fake")
	if err := os.MkdirAll(filepath.Join(base, "steamapps", "compatdata", "294100", "pfx"), 0o755); err != nil {
		t.Fatal(err)
	}
	called := false
	o.Start = func(*exec.Cmd) error { called = true; return nil }
	if _, err := Run(context.Background(), o); err == nil || !strings.Contains(err.Error(), "Proton") || called {
		t.Fatalf("Proton run: %v, started %v", err, called)
	}
	for _, dir := range []string{o.DataDir, o.RunDir} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("refused run created %s", dir)
		}
	}
}

func TestExecutable(t *testing.T) {
	for _, tc := range []struct {
		name, version, remove, want string
		dll, waylith, native        bool
	}{
		{name: "unity", version: "1.6.4871 rev1261", want: "RimWorldWin64.exe"},
		{name: "version", version: "1.7.4985 rev1269", want: "RimWorld.exe", waylith: true},
		{name: "dll", want: "RimWorld.exe", dll: true, waylith: true},
		{name: "unity fallback", remove: "RimWorldWin64.exe", want: "RimWorld.exe"},
		{name: "waylith fallback", dll: true, remove: "RimWorld.exe", want: "RimWorldWin64.exe", waylith: true},
		{name: "native unity", version: "1.6.4871 rev1261", want: "RimWorldLinux", native: true},
		{name: "native waylith", version: "1.7.4985 rev1269", want: "RimWorldLinux", native: true, waylith: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := testOptions(t, "idle", tc.dll)
			o.GameVersion = tc.version
			if tc.native {
				if runtime.GOOS != "linux" {
					t.Skip("Linux executable selection")
				}
				writeTestFile(t, filepath.Join(o.GameRoot, "RimWorldLinux"), "fake")
			}
			if tc.remove != "" {
				if err := os.Remove(filepath.Join(o.GameRoot, tc.remove)); err != nil {
					t.Fatal(err)
				}
			}
			exe, waylith := executable(o)
			if filepath.Base(exe) != tc.want || waylith != tc.waylith {
				t.Fatalf("%s, waylith=%v", exe, waylith)
			}
		})
	}
}
