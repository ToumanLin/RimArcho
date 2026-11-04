package mcp_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestPackCheckTool(t *testing.T) {
	t.Parallel()
	for _, in := range []mcp.PackIn{{Action: "check", Name: "pack"}, {Action: "check", File: "file.archopack"}} {
		ref := in.Name
		if in.File != "" {
			ref = in.File
		}
		if got := mcp.PackArgv(in); !reflect.DeepEqual(got, []string{"pack", "check", ref}) {
			t.Fatalf("check argv: %v", got)
		}
	}
	e := testkit.New(t)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("pack", "build", "checked")
	file := filepath.Join(e.Root, "checked.archopack")
	e.MustRun("pack", "export", "checked", file)
	cs := connect(t, e)
	before := e.App.Store.ChangeKey()
	for _, args := range []map[string]any{{"action": "check", "name": "checked"}, {"action": "check", "file": file}} {
		text, isError := call(t, cs, "manage_packs", args)
		if isError || !strings.Contains(text, "content: verified") || !strings.Contains(text, "DLC:") {
			t.Fatalf("check tool: %v %s", isError, text)
		}
	}
	if got := e.App.Store.ChangeKey(); got != before {
		t.Fatal("check wrote the store")
	}
}
