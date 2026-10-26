package app_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// Unity's 1.6 log has no start header. Its creation time admits companion entries from
// earlier in a long run, while rejecting files older than that run.
func TestCompanionWindowsCreationTime(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	player, crow := filepath.Join(root, "Player.log"), filepath.Join(root, "RimCrow_Realtime.log")
	before := time.Now()
	if err := os.WriteFile(player, []byte(unityHead), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(crow, []byte(`{"message":"ready","level":"INFO"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	end := before.Add(time.Hour)
	if err := os.Chtimes(player, end, end); err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{before.Add(-time.Minute), before.Add(time.Minute)} {
		if err := os.Chtimes(crow, at, at); err != nil {
			t.Fatal(err)
		}
		want := ""
		if at.After(before) {
			want = crow
		}
		if got := app.CompanionFor(player); got != want {
			t.Fatalf("mtime %v: got %q, want %q", at, got, want)
		}
	}
}
