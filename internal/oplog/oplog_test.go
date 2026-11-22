package oplog_test

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/oplog"
	"github.com/ToumanLin/RimArcho/internal/store"
)

func open(t *testing.T) (*store.Store, *oplog.Log, *time.Time) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	l := oplog.New(s, oplog.GUI)
	l.Now = func() time.Time { return now }
	return s, l, &now
}

// A nil log records and reads nothing, without panicking (bare Apps in tests).
func TestNilLog(t *testing.T) {
	var l *oplog.Log
	if l.Add(oplog.Row{Op: oplog.Trash}) != 0 || l.Origin() != "" || l.Count() != 0 {
		t.Fatal("nil log did something")
	}
	h := l.Begin(oplog.Row{Op: oplog.Launch})
	h.Child(oplog.Row{Op: oplog.LinkCreate})
	h.End(oplog.OK, "")
	if rows, err := l.Recent(oplog.Query{}); rows != nil || err != nil {
		t.Fatal(rows, err)
	}
	if _, err := l.Prune(); err != nil {
		t.Fatal(err)
	}
}

func TestAddBeginRecent(t *testing.T) {
	_, l, now := open(t)
	id := l.Add(oplog.Row{Op: oplog.Trash, Subject: "Mod A", Target: `C:\Mods\A`})
	h := l.Begin(oplog.Row{Op: oplog.Launch, Subject: "default"})
	h.Child(oplog.Row{Op: oplog.ModsConfigWrite, Subject: "default", Detail: "v3, 12 mods, launch"})
	h.Child(oplog.Row{Op: oplog.LinkCreate, Result: oplog.Failed, Detail: "access denied\nline two"})
	h.End(oplog.Failed, "access denied")
	h.End(oplog.OK, "") // the first End wins
	*now = now.Add(time.Hour)
	l.SetOrigin(oplog.Agent)
	l.Add(oplog.Row{Op: oplog.SaveRename, Subject: "Colony", Result: oplog.Refused, Detail: "the game is running"})

	rows, err := l.Recent(oplog.Query{})
	if err != nil || len(rows) != 5 || id != rows[4].ID {
		t.Fatalf("%d rows, %v", len(rows), err)
	}
	if r := rows[0]; r.Origin != oplog.Agent || r.Result != oplog.Refused || r.Op != oplog.SaveRename {
		t.Fatalf("newest %+v", r)
	}
	if p := rows[3]; p.Op != oplog.Launch || p.Result != oplog.Failed || p.Origin != oplog.GUI {
		t.Fatalf("parent %+v", p)
	}
	if c := rows[1]; c.Batch != rows[3].ID || c.Detail != "access denied; line two" {
		t.Fatalf("child %+v", c)
	}
	for q, want := range map[oplog.Query]int{
		{Failed: true}:                 3,
		{Op: "save"}:                   1,
		{Op: "link"}:                   1,
		{Op: oplog.ModsConfigWrite}:    1,
		{Since: now.Add(-time.Minute)}: 1,
		{Limit: 2}:                     2,
		{Op: "modsconfig", Limit: 1}:   1,
	} {
		if rows, _ := l.Recent(q); len(rows) != want {
			t.Errorf("%+v: %d rows, want %d", q, len(rows), want)
		}
	}
	head, more := oplog.Text(rows[2], time.UTC)
	if !strings.Contains(head, "modsconfig.write") || !strings.Contains(head, "(part of #") || len(more) != 2 || !strings.Contains(more[1], "checkout v3") {
		t.Fatalf("%s %q", head, more)
	}
}

// A failed write is swallowed: Add returns 0 and nothing is stored.
func TestFailedWrite(t *testing.T) {
	_, l, _ := open(t)
	l.Fail = true
	if l.Add(oplog.Row{Op: oplog.Trash}) != 0 || l.Count() != 0 {
		t.Fatal("a failed write stored a row")
	}
	l.Begin(oplog.Row{Op: oplog.Launch}).End(oplog.OK, "")
}

// Retention: the newer of 90 days and 500 rows.
func TestPrune(t *testing.T) {
	s, l, now := open(t)
	start := *now
	*now = start.Add(-100 * 24 * time.Hour)
	l.Add(oplog.Row{Op: oplog.Trash, Subject: "old"})
	*now = start
	tx, _ := s.DB.Begin()
	for i := 0; i < oplog.MaxRows+20; i++ {
		tx.Exec(`INSERT INTO op_log(at, origin, op, result) VALUES(?, 'cli', 'trash', 'ok')`, start.UnixMilli())
	}
	tx.Commit()
	n, err := l.Prune()
	if err != nil || n != 21 || l.Count() != oplog.MaxRows {
		t.Fatalf("pruned %d (%v), %d left", n, err, l.Count())
	}
	if rows, _ := l.Recent(oplog.Query{Limit: 1}); rows[0].ID != int64(oplog.MaxRows+21) {
		t.Fatalf("newest row %d gone", rows[0].ID)
	}
	if n, _ := l.Prune(); n != 0 {
		t.Fatalf("second prune removed %d", n)
	}
}

// Processes write at once (GUI, a CLI child, MCP): every row lands.
func TestConcurrentWriters(t *testing.T) {
	s, _, _ := open(t)
	path := s.Path
	var wg sync.WaitGroup
	for _, origin := range []string{oplog.GUI, oplog.CLI, oplog.MCP} {
		other, err := store.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer other.Close()
		l := oplog.New(other, origin)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				l.Add(oplog.Row{Op: oplog.Trash})
			}
		}()
	}
	wg.Wait()
	if n := oplog.New(s, "").Count(); n != 90 {
		t.Fatalf("%d rows, want 90", n)
	}
}

func TestOriginFromEnv(t *testing.T) {
	t.Setenv(oplog.EnvVar, "agent")
	if oplog.OriginFromEnv(oplog.MCP) != oplog.Agent {
		t.Fatal("inherited origin not taken")
	}
	t.Setenv(oplog.EnvVar, "something")
	if oplog.OriginFromEnv(oplog.MCP) != oplog.MCP {
		t.Fatal("unknown origin taken")
	}
}
