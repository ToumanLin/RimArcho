package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/oplog"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func opRows(t *testing.T, e *testkit.Env) []oplog.Row {
	t.Helper()
	rows, err := e.App.Operations(oplog.Query{})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func findOp(rows []oplog.Row, op, result string) *oplog.Row {
	for i := range rows {
		if rows[i].Op == op && (result == "" || rows[i].Result == result) {
			return &rows[i]
		}
	}
	return nil
}

// A launch is a parent row; the ModsConfig.xml write hangs under it with the version it
// made. A refused launch is one refused row; a dry run writes none (13 §1.3).
func TestOpLogLaunch(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.App.Ops.SetOrigin(oplog.TUI)
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("enable", refOf(t, e, "B", ""))
	e.MustRun("launch", "--dry-run")
	if rows := opRows(t, e); len(rows) != 0 {
		t.Fatalf("a dry run wrote %d rows", len(rows))
	}
	e.MustRun("launch")
	rows := opRows(t, e)
	l := findOp(rows, oplog.Launch, oplog.OK)
	w := findOp(rows, oplog.ModsConfigWrite, oplog.OK)
	if l == nil || w == nil || w.Batch != l.ID || l.Origin != oplog.TUI || l.Subject != "default" {
		t.Fatalf("rows: %+v", rows)
	}
	if !strings.HasPrefix(w.Detail, "v2, ") || !strings.HasSuffix(w.Target, "ModsConfig.xml") || l.Detail != "method direct" {
		t.Fatalf("write %+v, launch %+v", w, l)
	}

	os.Remove(filepath.Join(e.Game, "RimWorldWin64.exe"))
	if _, code := e.Run("launch"); code == 0 {
		t.Fatal("launch without an executable")
	}
	r := opRows(t, e)[0]
	if r.Op != oplog.Launch || r.Result != oplog.Refused || !strings.Contains(r.Detail, "no game executable") {
		t.Fatalf("refusal row %+v", r)
	}
}

// Saving writes one modsconfig.write row of its own.
func TestOpLogSave(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("enable", refOf(t, e, "B", ""))
	e.MustRun("save")
	rows := opRows(t, e)
	if len(rows) == 0 || rows[0].Op != oplog.ModsConfigWrite || rows[0].Batch != 0 || !strings.Contains(rows[0].Detail, ", user") {
		t.Fatalf("rows %+v", rows)
	}
	if !strings.Contains(e.MustRun("ops"), "back: the plan's history (archo history; archo loadout checkout v2") {
		t.Fatal(e.MustRun("ops"))
	}
}

// The recycle bin: a path off the allow-list is a refused row, a recycled folder an ok row
// that says where it went.
func TestOpLogTrash(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	var gone []string
	e.App.TrashFunc = func(p string) error { gone = append(gone, p); return nil }
	outside := filepath.Join(e.Root, "elsewhere")
	e.MkdirAll(outside)
	if err := e.App.ToTrash(outside); !errors.Is(err, app.ErrNotInTrashList) {
		t.Fatal(err)
	}
	mod := filepath.Join(e.Game, "Mods", "B")
	if err := e.App.ToTrash(mod); err != nil || len(gone) != 1 {
		t.Fatal(err, gone)
	}
	rows := opRows(t, e)
	if len(rows) != 2 || rows[1].Result != oplog.Refused || rows[1].Target != outside || rows[0].Result != oplog.OK || rows[0].Subject != "B" {
		t.Fatalf("rows %+v", rows)
	}
	out := e.MustRun("ops", "--failed")
	if !strings.Contains(out, "1 row") || !strings.Contains(out, "refused") || strings.Contains(out, "recycle bin\n") {
		t.Fatal(out)
	}
	if !strings.Contains(e.MustRun("ops", "--op", "trash"), "back: the recycle bin") {
		t.Fatal(e.MustRun("ops"))
	}
	if _, code := e.Run("ops", "--op", "nonsense"); code == 0 {
		t.Fatal("unknown --op accepted")
	}
}

// A log that cannot be written never fails the action.
func TestOpLogWriteFailureKeepsTheAction(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.App.Ops.Fail = true
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("enable", refOf(t, e, "B", ""))
	e.MustRun("launch")
	if len(e.Launched) != 1 || !strings.Contains(e.ReadModsConfig(), "tester.b") {
		t.Fatal("the launch did not happen")
	}
	e.App.Ops.Fail = false
	if rows := opRows(t, e); len(rows) != 0 {
		t.Fatalf("%d rows", len(rows))
	}
}

// Save rename and copy: one row each, with the new name.
func TestOpLogSaves(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	dir := filepath.Join(e.Root, "saves")
	e.WriteFile(filepath.Join(dir, "Colony.rws"), "<savegame/>")
	s := &app.Save{Path: filepath.Join(dir, "Colony.rws"), File: "Colony.rws", Name: "Colony"}
	if _, err := e.App.RenameSave(s, "bad/name"); err == nil {
		t.Fatal("bad name accepted")
	}
	to, err := e.App.CopySave(s)
	if err != nil {
		t.Fatal(err)
	}
	rows := opRows(t, e)
	if len(rows) != 2 || rows[1].Op != oplog.SaveRename || rows[1].Result != oplog.Refused ||
		rows[0].Op != oplog.SaveCopy || rows[0].Result != oplog.OK || rows[0].Detail != "to "+filepath.Base(to) {
		t.Fatalf("rows %+v", rows)
	}
}

// The process origin: an inherited ARCHO_ORIGIN wins over the surface's own.
func TestOpLogOrigin(t *testing.T) {
	e := testkit.New(t)
	t.Setenv(oplog.EnvVar, oplog.Agent)
	e.App.SetOrigin(oplog.MCP)
	if e.App.Ops.Origin() != oplog.Agent {
		t.Fatalf("origin %s", e.App.Ops.Origin())
	}
	os.Unsetenv(oplog.EnvVar)
	e.App.SetOrigin(oplog.GUI)
	if e.App.Ops.Origin() != oplog.GUI || os.Getenv(oplog.EnvVar) != oplog.GUI {
		t.Fatal("origin not passed on to children")
	}
}
