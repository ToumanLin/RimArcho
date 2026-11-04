package mcp

import (
	"slices"
	"testing"
)

func TestManageLibraryResubscribe(t *testing.T) {
	t.Parallel()
	for _, dry := range []bool{false, true} {
		want := []string{"workshop", "resubscribe", "ws:8100", "ws:8200"}
		if dry {
			want = append(want, "--dry-run")
		}
		got := ManageLibraryArgv(ManageLibraryIn{Action: "resubscribe", Refs: []string{"ws:8100", "ws:8200"}, DryRun: dry})
		if !slices.Equal(got, want) {
			t.Fatalf("%v != %v", got, want)
		}
	}
}
