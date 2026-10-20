package app_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// P2, P3 (10 §7): caches are measured and cleared; a backup is a database that opens; launch
// arguments reach the game's command line.
func TestSettingsCommands(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	e.WriteFile(filepath.Join(e.Home, "cache", "previews", "a.png"), strings.Repeat("x", 4096))
	out := e.MustRun("cache", "status")
	if !strings.Contains(out, "previews: 0.0 MB, 1 items") {
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
