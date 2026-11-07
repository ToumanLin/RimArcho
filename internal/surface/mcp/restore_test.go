package mcp_test

import (
	"slices"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
)

// edit_loadout restore with no refs still restores every staged mod: the CLI needs --all
// for that (13 §3.3), the tool's arguments stay as they were.
func TestRestoreArgv(t *testing.T) {
	t.Parallel()
	if got := mcp.EditArgv(mcp.EditIn{Action: "restore"}); !slices.Equal(got, []string{"stage", "restore", "--all"}) {
		t.Errorf("no refs: %q", got)
	}
	if got := mcp.EditArgv(mcp.EditIn{Action: "restore", Refs: []string{"#nyh3"}, At: 2}); !slices.Equal(got, []string{"stage", "restore", "#nyh3", "--at", "2"}) {
		t.Errorf("one ref: %q", got)
	}
}
