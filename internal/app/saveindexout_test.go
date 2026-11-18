package app

import (
	"strings"
	"testing"
)

// The child's output when one save could not be read (an NTFS permission, VM 2026-11-10):
// the save is an indented error under the summary, not the run failing.
func TestReadSavesIndexOutput(t *testing.T) {
	out := "progress: saves 1/2 sv:aa C-8/S2A copy\n" +
		"progress: saves 2/2 sv:bb S2A locked/S2A locked\n" +
		"progress: saves-failed sv:bb open C:\\Saves\\S2A locked.rws: Access is denied.\n" +
		"saves index: read 1 save (13.1 MB) in 0.9 s; 204 already read\n" +
		"  error: S2A locked: open C:\\Saves\\S2A locked.rws: Access is denied.\n" +
		"next:\n  archo saves list\n"
	var lines []string
	_, errLine := readSavesIndexOutput(strings.NewReader(out), func(l string) { lines = append(lines, l) })
	if errLine != "" {
		t.Fatalf("a save that could not be read made the run fatal: %q", errLine)
	}
	if len(lines) != 7 {
		t.Fatalf("progress saw %d lines", len(lines))
	}
	if _, errLine = readSavesIndexOutput(strings.NewReader("error: database is locked\n"), nil); errLine != "database is locked" {
		t.Fatalf("the run's own error: %q", errLine)
	}
}
