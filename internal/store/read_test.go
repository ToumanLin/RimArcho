package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestReadSnapshotStaysConsistentWithoutBlockingWriter(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "archo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.SetMeta("read-test", "before"); err != nil {
		t.Fatal(err)
	}
	r, err := OpenSession(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	release, err := r.ReadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	change, index := r.ChangeKey(), r.IndexKey()
	done := make(chan error, 1)
	go func() {
		if err := s.SetMeta("read-test", "after"); err != nil {
			done <- err
			return
		}
		done <- s.SetAlias("", "L1", "renamed")
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("read snapshot blocked writer")
	}
	if r.Meta("read-test") != "before" || r.ChangeKey() != change || r.IndexKey() != index {
		t.Fatal("one page observed more than one committed state")
	}
	release()
	if r.Meta("read-test") != "after" || r.ChangeKey() == change || r.IndexKey() == index {
		t.Fatal("released reader did not observe later commits")
	}
	// Release is safe on an early return as well as the normal publication path.
	release()
}
