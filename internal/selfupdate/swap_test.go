package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func swapFixture(t *testing.T) (program, stage, old string, original, staged map[string]string) {
	t.Helper()
	program = t.TempDir()
	stage, old = filepath.Join(program, "stage"), filepath.Join(program, "old")
	original = map[string]string{Files[0]: "old command", "README.md": "old readme", "monitor/old.txt": "old monitor"}
	if len(Files) == 6 {
		original["RimArcho.exe"] = "old gui"
	}
	staged = make(map[string]string)
	for _, name := range Files {
		if name == "monitor" {
			staged["monitor/new.txt"] = "new monitor"
		} else {
			staged[name] = "new " + name
		}
	}
	writeTree(t, program, original)
	writeTree(t, stage, staged)
	return
}

func TestApplyRollback(t *testing.T) {
	program, stage, old, original, staged := swapFixture(t)
	s, err := Apply(program, stage, old)
	if err != nil {
		t.Fatal(err)
	}
	if s.Program != program || s.Stage != stage || s.Old != old {
		t.Fatalf("wrong swap paths: %+v", s)
	}
	wantInstalled := make(map[string]string)
	for name, data := range staged {
		wantInstalled[name] = data
	}
	for name, data := range original {
		wantInstalled["old/"+name] = data
	}
	if !equalTrees(tree(t, program), wantInstalled) || !equalTrees(tree(t, old), original) || len(tree(t, stage)) != 0 {
		t.Fatal("Apply did not install staged files and preserve originals")
	}
	if err := s.Rollback(); err != nil {
		t.Fatal(err)
	}
	wantRolledBack := make(map[string]string)
	for name, data := range original {
		wantRolledBack[name] = data
	}
	for name, data := range staged {
		wantRolledBack["stage/"+name] = data
	}
	if !equalTrees(tree(t, program), wantRolledBack) || !equalTrees(tree(t, stage), staged) {
		t.Fatal("Rollback did not restore original files and complete stage")
	}
	if err := s.Rollback(); err != nil {
		t.Fatalf("second Rollback: %v", err)
	}
}

func TestApplyRenameFailure(t *testing.T) {
	// Exercise every forward rename, including the specifically required fourth call.
	for failAt := 1; failAt <= 10; failAt++ {
		t.Run(string(rune('a'+failAt-1)), func(t *testing.T) {
			program, stage, old, _, staged := swapFixture(t)
			before := tree(t, program)
			originalRename := rename
			t.Cleanup(func() { rename = originalRename })
			calls := 0
			injected := errors.New("injected rename failure")
			rename = func(from, to string) error {
				calls++
				if calls == failAt {
					return injected
				}
				return os.Rename(from, to)
			}
			s, err := Apply(program, stage, old)
			if err == nil {
				// Non-Windows has fewer forward moves than Windows.
				if calls >= failAt {
					t.Fatalf("injected error lost on call %d", failAt)
				}
				rename = originalRename
				if err := s.Rollback(); err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, injected) || s != nil {
				t.Fatalf("Apply = %+v, %v", s, err)
			}
			if !equalTrees(tree(t, program), before) || !equalTrees(tree(t, stage), staged) {
				t.Fatalf("failure on call %d did not restore original trees", failAt)
			}
		})
	}
}

func TestApplyAbsentEntries(t *testing.T) {
	program := t.TempDir()
	stage, old := filepath.Join(program, "stage"), filepath.Join(program, "old")
	original := map[string]string{Files[0]: "old", "monitor/old.txt": "keep", "custom.txt": "keep custom"}
	writeTree(t, program, original)
	writeTree(t, stage, map[string]string{Files[0]: "new"})
	s, err := Apply(program, stage, old)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"monitor/old.txt": "keep", "custom.txt": "keep custom", Files[0]: "new"} {
		got, err := os.ReadFile(filepath.Join(program, filepath.FromSlash(name)))
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v", name, got, err)
		}
	}
	if err := s.Rollback(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyExistingOld(t *testing.T) {
	program, stage, old, _, _ := swapFixture(t)
	writeTree(t, old, map[string]string{"sentinel": "keep"})
	before := tree(t, program)
	if _, err := Apply(program, stage, old); err == nil {
		t.Fatal("accepted existing backup directory")
	}
	if !equalTrees(tree(t, program), before) {
		t.Fatal("existing backup or installation changed")
	}
}

func TestRollbackContinuesAndJoinsErrors(t *testing.T) {
	program, stage, old, _, _ := swapFixture(t)
	before := tree(t, program)
	s, err := Apply(program, stage, old)
	if err != nil {
		t.Fatal(err)
	}
	originalRename := rename
	t.Cleanup(func() { rename = originalRename })
	first, second := errors.New("first undo failure"), errors.New("second undo failure")
	calls := 0
	rename = func(from, to string) error {
		calls++
		if calls == 1 {
			return first
		}
		if calls == 2 {
			return second
		}
		return os.Rename(from, to)
	}
	err = s.Rollback()
	if !errors.Is(err, first) || !errors.Is(err, second) || calls <= 2 {
		t.Fatalf("Rollback = %v, calls = %d", err, calls)
	}
	rename = originalRename
	if err := s.Rollback(); err != nil {
		t.Fatalf("retry Rollback: %v", err)
	}
	if !equalTrees(tree(t, program), before) {
		t.Fatal("retry did not restore all bytes")
	}
}

func TestApplyJoinsUndoError(t *testing.T) {
	program, stage, old, _, _ := swapFixture(t)
	originalRename := rename
	t.Cleanup(func() { rename = originalRename })
	forward, undo := errors.New("forward rename failure"), errors.New("undo rename failure")
	calls := 0
	rename = func(from, to string) error {
		calls++
		if calls == 4 {
			return forward
		}
		if calls == 5 {
			return undo
		}
		return os.Rename(from, to)
	}
	_, err := Apply(program, stage, old)
	if !errors.Is(err, forward) || !errors.Is(err, undo) || calls <= 5 {
		t.Fatalf("Apply = %v, calls = %d", err, calls)
	}
}
