package app_test

import (
	"context"
	"database/sql"
	"github.com/ToumanLin/RimArcho/internal/app"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// P2, P3 (16 §2.14): caches are measured and cleared; a backup is a database that opens; launch
// arguments reach the game's command line.
func TestSettingsCommands(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.WriteFile(filepath.Join(e.Home, "cache", "previews", "a.png"), strings.Repeat("x", 4096))
	out := e.MustRun("cache", "status")
	if !strings.Contains(out, "previews: 4.0 KB, 1 items") {
		t.Fatalf("cache status:\n%s", out)
	}
	e.MustRun("cache", "clear", "previews")
	if _, err := os.Stat(filepath.Join(e.Home, "cache", "previews")); !os.IsNotExist(err) {
		t.Fatal("previews not cleared")
	}
	if _, code := e.Run("cache", "clear", "everything"); code == 0 {
		t.Fatal("unknown cache accepted")
	}

	out = e.MustRun("db", "backup")
	p := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(out, "\n", 2)[0], "backup written to "))
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM instance`).Scan(&n); err != nil || n == 0 {
		t.Fatalf("backup: %d instances, %v", n, err)
	}
	db.Close()
	if out := e.MustRun("db", "status"); !strings.Contains(out, "backups: 1") {
		t.Fatalf("db status:\n%s", out)
	}
	e.MustRun("db", "vacuum")

	e.MustRun("env", "set", "main", "--launch", "direct", "--launch-args", "-popupwindow -quicktest")
	e.MustRun("save")
	out = e.MustRun("launch", "--dry-run")
	if !strings.Contains(out, "-popupwindow -quicktest") {
		t.Fatalf("launch args:\n%s", out)
	}
	e.MustRun("env", "set", "main", "--launch-args", "")
	if out := e.MustRun("launch", "--dry-run"); strings.Contains(out, "-popupwindow") {
		t.Fatalf("launch args not removed:\n%s", out)
	}
}

func TestLaunchArgsRejectDataOverride(t *testing.T) {
	for _, args := range []string{"-savedatafolder=elsewhere", "-SaveDataFolder=elsewhere", `"-savedatafolder=folder with spaces"`, "-savedatafolder elsewhere", "-popupwindow -SAVEDATAFOLDER=elsewhere"} {
		t.Run(args, func(t *testing.T) {
			e := loadoutEnv(t)
			scanNow(t, e)
			before, err := e.App.Store.EnvByName("main")
			if err != nil {
				t.Fatal(err)
			}
			out, code := e.Run("env", "set", "main", "--saves", "own", "--launch-args", args)
			if code == 0 || !strings.Contains(out, "archo env set main --user-data <dir>") {
				t.Fatalf("refusal: %d %s", code, out)
			}
			after, _ := e.App.Store.EnvByName("main")
			if after.UserDataRoot != before.UserDataRoot {
				t.Fatal("refusal changed data root")
			}
			out, code = e.Run("env", "add", "rejected", "--game", e.Game, "--launch-args", args)
			if code == 0 || !strings.Contains(out, "archo env set rejected --user-data <dir>") {
				t.Fatalf("add refusal: %d %s", code, out)
			}
			if err := e.App.Store.SetMeta("env:"+before.ID+":launch_args", args); err != nil {
				t.Fatal(err)
			}
			config := e.ReadModsConfig()
			for _, launch := range []func() error{
				func() error { _, err := e.App.Launch(false); return err },
				func() error { _, err := e.App.LaunchPack("missing", false); return err },
				func() error { _, err := e.App.TestRun(context.Background(), app.TestRunOptions{}); return err },
			} {
				if err := launch(); err == nil || !strings.Contains(err.Error(), "archo env set main --user-data <dir>") {
					t.Fatalf("stored refusal: %v", err)
				}
			}
			if config != e.ReadModsConfig() || len(e.Launched) != 0 || e.App.LaunchArgs(before.ID) != args {
				t.Fatal("stored refusal changed state")
			}
		})
	}
}

func TestLaunchArgumentsDisplayRoundTrip(t *testing.T) {
	for _, stored := range []string{`["-name","a b","","C:\\My Games\\","a\"b","a'b","[literal]"]`, `["\u65e5\u672c\u8a9e \u306e\u540d\u524d", "-popupwindow"]`} {
		want, err := app.ParseLaunchArgs(stored)
		if err != nil {
			t.Fatal(err)
		}
		shown := app.DisplayLaunchArgs(stored)
		got, err := app.ParseLaunchArgs(shown)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%q: %v != %v (%v)", shown, got, want, err)
		}
	}
	got, err := app.ParseLaunchArgs(`-name "a b" -path 'C:\My Games'`)
	if err != nil || !reflect.DeepEqual(got, []string{"-name", "a b", "-path", `C:\My Games`}) {
		t.Fatalf("quoted arguments: %v %v", got, err)
	}
}
