package app_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/store"
)

// 04 §4: a list save waits for another connection's write instead of failing with
// "database is locked". Another session (a background scan, an analysis, a second window)
// holds the write lock while the window saves; the save waits busy_timeout and succeeds,
// and its operation log row is written too.
func TestSaveWaitsForAnotherWriter(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("disable", "pkg:gone.mod")

	other, err := store.OpenSession(e.App.Store.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	tx, err := other.DB.Begin() // IMMEDIATE: takes the write lock now
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO meta(key, value) VALUES('test:busy', '1')`); err != nil {
		t.Fatal(err)
	}
	const hold = 1500 * time.Millisecond
	released := make(chan error, 1)
	go func() {
		time.Sleep(hold)
		released <- tx.Commit()
	}()

	start := time.Now()
	if _, err := e.App.Save(false); err != nil {
		t.Fatalf("save while another connection writes: %v", err)
	}
	if took := time.Since(start); took < hold/2 {
		t.Fatalf("the save did not wait for the other writer (%v): the lock was not held", took)
	}
	if err := <-released; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.ReadModsConfig(), "tester.a") || strings.Contains(e.ReadModsConfig(), "gone.mod") {
		t.Fatalf("not written:\n%s", e.ReadModsConfig())
	}
	var n int
	if err := e.App.Store.DB.QueryRow(`SELECT count(*) FROM op_log WHERE op='modsconfig.write'`).Scan(&n); err != nil || n == 0 {
		t.Fatalf("operation log row missing: %d %v", n, err)
	}
}
