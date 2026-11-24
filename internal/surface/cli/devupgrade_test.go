package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/store"
)

// `archo version` never opens, creates or upgrades the database (17 §5): release.ps1 runs it
// on a fresh build, which once upgraded the maintainer's real database to an unreleased schema.
func TestVersionLeavesTheDatabaseAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIMARCHO_HOME", home)
	var out, errb bytes.Buffer
	if code := Run([]string{"version"}, &out, &errb); code != 0 {
		t.Fatal(code, errb.String())
	}
	if !strings.HasPrefix(out.String(), "archo ") || !strings.Contains(out.String(), home) {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(filepath.Join(home, "archo.db")); !os.IsNotExist(err) {
		t.Fatal("version created or opened the database", err)
	}
}

// A development build refuses to upgrade an older database (17 §4); a missing one or one at
// the current schema is not its business.
func TestDevUpgradeCheck(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "archo.db")
	if err := devUpgradeCheck(db); err != nil {
		t.Fatal("no database:", err)
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := devUpgradeCheck(db); err != nil {
		t.Fatal("current schema:", err)
	}
	if _, err := s.DB.Exec(`UPDATE meta SET value=? WHERE key='schema_version'`, strconv.Itoa(store.SchemaVersion-1)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	err = devUpgradeCheck(db)
	if err == nil || !strings.Contains(err.Error(), AllowDevUpgradeEnv) || !strings.Contains(err.Error(), "RIMARCHO_HOME") {
		t.Fatal("older schema not refused:", err)
	}
}
