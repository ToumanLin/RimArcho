package app_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/oplog"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// launchCopyEnv is a direct-launch environment with two saves, B the newer.
func launchCopyEnv(t *testing.T) (*testkit.Env, string) {
	t.Helper()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("env", "set", "main", "--launch", "direct")
	saves := filepath.Join(e.UserData, "Saves")
	e.WriteFile(filepath.Join(saves, "A.rws"), "save a")
	e.WriteFile(filepath.Join(saves, "B.rws"), "save b, newer")
	touch(t, filepath.Join(saves, "A.rws"), e.Now().Add(-2*time.Hour))
	touch(t, filepath.Join(saves, "B.rws"), e.Now().Add(-time.Hour))
	return e, saves
}

func touch(t *testing.T, p string, at time.Time) {
	t.Helper()
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
}

// launchOnce launches and waits for the automatic copy.
func launchOnce(t *testing.T, e *testkit.Env) {
	t.Helper()
	n := len(e.Launched)
	e.MustRun("launch")
	e.App.WaitLaunchCopies()
	if len(e.Launched) != n+1 {
		t.Fatalf("the game did not start (%d launches)", len(e.Launched))
	}
	e.Advance(time.Minute) // the next copy gets a folder of its own
}

// backupDirs lists backups/saves' folders and what each holds.
func backupDirs(t *testing.T, e *testkit.Env) map[string][]string {
	t.Helper()
	root := filepath.Join(e.Home, "backups", "saves")
	ents, _ := os.ReadDir(root)
	out := map[string][]string{}
	for _, de := range ents {
		inner, _ := os.ReadDir(filepath.Join(root, de.Name()))
		var names []string
		for _, f := range inner {
			names = append(names, f.Name())
		}
		sort.Strings(names)
		out[de.Name()] = names
	}
	return out
}

func autoCopies(dirs map[string][]string) int {
	n := 0
	for _, files := range dirs {
		if strings.Contains(strings.Join(files, " "), "rimarcho-auto.json") {
			n++
		}
	}
	return n
}

// 12 §4.4: a launch copies the newest save; an unchanged save is not copied again; only
// the newest automatic copies stay, and a backup the player made is never deleted.
func TestLaunchCopiesNewestSave(t *testing.T) {
	t.Parallel()
	e, saves := launchCopyEnv(t)
	// A backup the player made, older than every automatic copy.
	manual := filepath.Join(e.Home, "backups", "saves", "20000101-000000")
	e.WriteFile(filepath.Join(manual, "A.rws"), "manual")

	launchOnce(t, e)
	dirs := backupDirs(t, e)
	stamp := e.Now().Add(-time.Minute).UTC().Format("20060102-150405")
	if got := strings.Join(dirs[stamp], ","); got != "B.rws,rimarcho-auto.json" {
		t.Fatalf("first launch: %s holds %q; all: %v", stamp, got, dirs)
	}
	b, err := os.ReadFile(filepath.Join(e.Home, "backups", "saves", stamp, "B.rws"))
	if err != nil || string(b) != "save b, newer" {
		t.Fatalf("copy: %q %v", b, err)
	}
	rows, _ := e.App.Operations(oplog.Query{})
	found := false
	for _, r := range rows {
		if r.Op == oplog.SaveBackup && r.Subject == "B" && r.Result == oplog.OK && strings.Contains(r.Detail, "automatic") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no save.backup row: %+v", rows)
	}

	// Unchanged: no second copy.
	launchOnce(t, e)
	if n := autoCopies(backupDirs(t, e)); n != 1 {
		t.Fatalf("an unchanged save was copied again: %d copies", n)
	}

	// Seven changes of the save: the last five copies stay, the manual backup too.
	for i := 0; i < 7; i++ {
		touch(t, filepath.Join(saves, "B.rws"), e.Now())
		launchOnce(t, e)
	}
	dirs = backupDirs(t, e)
	if n := autoCopies(dirs); n != app.LaunchCopyKeep {
		t.Fatalf("%d automatic copies kept, want %d: %v", n, app.LaunchCopyKeep, dirs)
	}
	if got := strings.Join(dirs["20000101-000000"], ","); got != "A.rws" {
		t.Fatalf("the player's backup changed: %q", got)
	}
	if _, ok := dirs[stamp]; ok {
		t.Fatal("the oldest automatic copy was not deleted")
	}
	// A folder the player added a file to is theirs: never deleted.
	var newest string
	for d := range dirs {
		if d > newest {
			newest = d
		}
	}
	e.WriteFile(filepath.Join(e.Home, "backups", "saves", newest, "note.txt"), "mine")
	for i := 0; i < app.LaunchCopyKeep+1; i++ {
		touch(t, filepath.Join(saves, "B.rws"), e.Now())
		launchOnce(t, e)
	}
	if _, ok := backupDirs(t, e)[newest]; !ok {
		t.Fatal("a copy the player changed was deleted")
	}
}

// A copy that cannot be made never stops the launch; it is recorded as failed.
func TestLaunchCopyFailureStillLaunches(t *testing.T) {
	t.Parallel()
	e, _ := launchCopyEnv(t)
	// backups/saves is a file: no folder can be made in it.
	e.WriteFile(filepath.Join(e.Home, "backups", "saves"), "not a folder")
	launchOnce(t, e)
	rows, _ := e.App.Operations(oplog.Query{})
	failed := false
	for _, r := range rows {
		if r.Op == oplog.SaveBackup && r.Result == oplog.Failed {
			failed = true
		}
		if r.Op == oplog.Launch && r.Result != oplog.OK {
			t.Fatalf("launch row: %+v", r)
		}
	}
	if !failed {
		t.Fatalf("no failed save.backup row: %+v", rows)
	}
}

// The setting off: no copy at launch; a folder without saves copies nothing.
func TestLaunchCopyOffAndEmpty(t *testing.T) {
	t.Parallel()
	e, saves := launchCopyEnv(t)
	e.MustRun("config", "set", "saves.launch_copy", "off")
	if !strings.Contains(e.MustRun("config", "get", "saves.launch_copy"), "off") {
		t.Fatal("config get")
	}
	launchOnce(t, e)
	if n := autoCopies(backupDirs(t, e)); n != 0 {
		t.Fatalf("copied with the setting off: %d", n)
	}
	e.MustRun("config", "set", "saves.launch_copy", "on")
	os.RemoveAll(saves)
	if r := e.App.LaunchCopy(saves); r.Skipped != "no saves" || r.Err != nil {
		t.Fatalf("no saves: %+v", r)
	}
	launchOnce(t, e)
	if n := autoCopies(backupDirs(t, e)); n != 0 {
		t.Fatalf("copied from an empty folder: %d", n)
	}
}
